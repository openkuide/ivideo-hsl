// Package doctor runs read-only diagnostics against the local environment
// and the user's persisted configuration. It answers the question: "why
// won't my next run work?" without making any changes.
//
// Inspired by `brew doctor` — every check returns a single Finding; the
// caller renders them. Ordering is stable so output is easy to compare
// across runs.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/jsonconfig"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/workspacefinder"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/infrastructure/deps"
)

// Level expresses how badly a check failed.
type Level int

const (
	LevelOK Level = iota
	LevelWarn
	LevelFail
)

// Finding is the outcome of one diagnostic check.
type Finding struct {
	Level  Level
	Title  string // left-column label, e.g. "remote URL"
	Detail string // the discovered value or short status
	Hint   string // actionable remediation when Level != LevelOK
}

// Result is the full set of findings in display order.
type Result struct {
	Findings []Finding
}

// OK reports whether every finding is LevelOK. Warnings do not fail the
// overall result — the caller decides whether to treat them as blocking.
func (r Result) OK() bool {
	for _, f := range r.Findings {
		if f.Level == LevelFail {
			return false
		}
	}
	return true
}

// Check runs every diagnostic and returns the collected findings.
// Network-touching checks share the given context and its deadline.
func Check(ctx context.Context) Result {
	settingPath := settingFilePath()
	store := jsonconfig.New(settingPath)
	loaded, loadErr := store.Load()
	token := resolveToken(loaded)
	authMethod := resolveAuthMethod(loaded)
	effectiveURL := effectiveRemoteURL(loaded.RemoteURL, token, authMethod)

	checks := []func() Finding{
		checkBinary("ffmpeg", deps.FFmpegPath()),
		checkBinary("ffprobe", deps.FFprobePath()),
		checkGit,
		func() Finding { return checkConfigFile(settingPath, loadErr) },
		func() Finding { return checkRemoteURL(loaded.RemoteURL) },
		func() Finding { return checkAuthMethod(loaded.RemoteURL, authMethod) },
		func() Finding { return checkToken(loaded, token, authMethod) },
		func() Finding { return checkSSHKeysIfNeeded(authMethod) },
		func() Finding { return checkSourceDir(loaded.SourceDir) },
		func() Finding { return checkRemoteReachable(ctx, effectiveURL, loaded.RemoteURL) },
		func() Finding { return checkPlaybackURL(loaded.PublicURLPattern) },
		func() Finding { return checkPendingRetries(ctx) },
		func() Finding { return checkIncompleteWorkspaces(ctx) },
	}

	var findings []Finding
	for _, c := range checks {
		findings = append(findings, c())
	}
	return Result{Findings: findings}
}

// ---------- checks ----------

func checkBinary(name, path string) func() Finding {
	return func() Finding {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return Finding{
				Level: LevelFail,
				Title: name,
				Hint:  "run `ivideo-hls install-deps` or `brew install ffmpeg`",
			}
		}
		version := firstLine(runOrEmpty(resolved, "-version"))
		return Finding{Level: LevelOK, Title: name, Detail: resolved + "  " + summarizeVersion(version)}
	}
}

func checkGit() Finding {
	path, err := exec.LookPath("git")
	if err != nil {
		return Finding{Level: LevelFail, Title: "git", Hint: "install git (brew install git, apt install git)"}
	}
	return Finding{Level: LevelOK, Title: "git", Detail: path}
}

func checkConfigFile(path string, loadErr error) Finding {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return Finding{
			Level:  LevelWarn,
			Title:  "config file",
			Detail: "not present — defaults in use",
			Hint:   "run `ivideo-hls run` to create one",
		}
	}
	if loadErr != nil {
		return Finding{Level: LevelFail, Title: "config file", Detail: loadErr.Error(), Hint: "inspect " + path}
	}
	info, err := os.Stat(path)
	if err != nil {
		return Finding{Level: LevelWarn, Title: "config file", Detail: path, Hint: err.Error()}
	}
	mode := info.Mode().Perm()
	detail := path
	if mode != 0o600 {
		return Finding{
			Level:  LevelWarn,
			Title:  "config file",
			Detail: fmt.Sprintf("%s  (mode %o — expected 600 since it may contain a token)", path, mode),
			Hint:   "chmod 600 " + path,
		}
	}
	return Finding{Level: LevelOK, Title: "config file", Detail: detail}
}

func checkRemoteURL(url string) Finding {
	if url == "" {
		return Finding{
			Level: LevelWarn,
			Title: "remote URL",
			Hint:  "built-in default will be used — set one via settings to avoid surprises",
		}
	}
	if err := validateRemoteURL(url); err != nil {
		return Finding{Level: LevelFail, Title: "remote URL", Detail: url, Hint: err.Error()}
	}
	return Finding{Level: LevelOK, Title: "remote URL", Detail: url}
}

func checkAuthMethod(url string, method settings.AuthMethod) Finding {
	inferred := inferAuthMethod(url, method)
	if inferred != method {
		return Finding{
			Level:  LevelWarn,
			Title:  "auth method",
			Detail: string(method) + " (URL suggests " + string(inferred) + ")",
			Hint:   "switch auth method in settings, or change the URL",
		}
	}
	return Finding{Level: LevelOK, Title: "auth method", Detail: string(method)}
}

func checkToken(loaded settings.Settings, token string, method settings.AuthMethod) Finding {
	if method != settings.AuthHTTPS {
		return Finding{Level: LevelOK, Title: "token", Detail: "(not needed for " + string(method) + ")"}
	}
	if token == "" {
		return Finding{
			Level: LevelFail,
			Title: "token",
			Hint:  "set $IVIDEO_HLS_TOKEN or configure one in settings — HTTPS push will fail without it",
		}
	}
	source := "from config"
	if loaded.Token == "" {
		source = "from $IVIDEO_HLS_TOKEN"
	}
	return Finding{Level: LevelOK, Title: "token", Detail: maskToken(token) + "  (" + source + ")"}
}

func checkSSHKeysIfNeeded(method settings.AuthMethod) Finding {
	if method != settings.AuthSSH {
		return Finding{Level: LevelOK, Title: "ssh keys", Detail: "(n/a)"}
	}
	if _, err := exec.LookPath("ssh-add"); err != nil {
		return Finding{Level: LevelWarn, Title: "ssh keys", Detail: "ssh-add not on PATH", Hint: "can't verify agent"}
	}
	out, err := exec.Command("ssh-add", "-l").CombinedOutput()
	if err != nil || strings.Contains(string(out), "no identities") {
		return Finding{
			Level:  LevelWarn,
			Title:  "ssh keys",
			Detail: "agent has no keys loaded",
			Hint:   "ssh-add ~/.ssh/id_ed25519 (or your key) before running",
		}
	}
	return Finding{Level: LevelOK, Title: "ssh keys", Detail: firstLine(string(out))}
}

// checkPendingRetries surfaces hero_<name>/ workspaces left behind by a
// previous failed run. These are ready to push but waiting for explicit
// action via `ivideo-hls retry-failed`.
func checkPendingRetries(ctx context.Context) Finding {
	wd, err := os.Getwd()
	if err != nil {
		return Finding{Level: LevelOK, Title: "pending retries", Detail: "(skipped — cwd error)"}
	}
	finder := workspacefinder.New("git")
	candidates, err := finder.FindRetryReady(ctx, wd)
	if err != nil {
		return Finding{Level: LevelOK, Title: "pending retries", Detail: "(scan failed: " + err.Error() + ")"}
	}
	if len(candidates) == 0 {
		return Finding{Level: LevelOK, Title: "pending retries", Detail: "none"}
	}
	names := make([]string, 0, len(candidates))
	for _, c := range candidates {
		names = append(names, c.Name)
	}
	return Finding{
		Level:  LevelWarn,
		Title:  "pending retries",
		Detail: fmt.Sprintf("%d workspace(s) waiting: %s", len(candidates), strings.Join(names, ", ")),
		Hint:   "run `ivideo-hls retry-failed` to finish them",
	}
}

// checkIncompleteWorkspaces surfaces hero_<name>/ directories that stopped
// mid-pipeline (before the playlist was finalized). Paired with pending
// retries so the operator sees both classes of leftover work in one place.
func checkIncompleteWorkspaces(ctx context.Context) Finding {
	wd, err := os.Getwd()
	if err != nil {
		return Finding{Level: LevelOK, Title: "incomplete workspaces", Detail: "(skipped — cwd error)"}
	}
	finder := workspacefinder.New("git")
	candidates, err := finder.FindIncomplete(ctx, wd)
	if err != nil {
		return Finding{Level: LevelOK, Title: "incomplete workspaces", Detail: "(scan failed: " + err.Error() + ")"}
	}
	if len(candidates) == 0 {
		return Finding{Level: LevelOK, Title: "incomplete workspaces", Detail: "none"}
	}
	names := make([]string, 0, len(candidates))
	for _, c := range candidates {
		names = append(names, c.Name+" ("+string(c.Stage)+")")
	}
	return Finding{
		Level:  LevelWarn,
		Title:  "incomplete workspaces",
		Detail: fmt.Sprintf("%d stopped mid-pipeline: %s", len(candidates), strings.Join(names, ", ")),
		Hint:   "run `ivideo-hls resume-failed` to delete and re-run from source",
	}
}

// checkPlaybackURL flags common shape mistakes in the HTTP(S) template used
// by urls.json. Push URLs (SSH) cannot serve raw files to a player, so using
// one as a playback URL is almost certainly a paste-by-mistake.
func checkPlaybackURL(pattern string) Finding {
	if pattern == "" {
		return Finding{Level: LevelOK, Title: "playback URL", Detail: "(not set — urls.json will log local paths)"}
	}
	switch {
	case strings.HasPrefix(pattern, "git@"), strings.HasPrefix(pattern, "ssh://"):
		return Finding{
			Level:  LevelFail,
			Title:  "playback URL",
			Detail: pattern,
			Hint:   "playback URLs need HTTP(S); SSH schemes don't serve raw files to players",
		}
	case !strings.HasPrefix(pattern, "http://") && !strings.HasPrefix(pattern, "https://"):
		return Finding{
			Level:  LevelWarn,
			Title:  "playback URL",
			Detail: pattern,
			Hint:   "expected an https:// URL",
		}
	case !strings.Contains(pattern, "{branch}") && !strings.Contains(pattern, "{filename}"):
		return Finding{
			Level:  LevelWarn,
			Title:  "playback URL",
			Detail: pattern,
			Hint:   "no {branch} or {filename} placeholder — every video will produce the same URL",
		}
	}
	return Finding{Level: LevelOK, Title: "playback URL", Detail: pattern}
}

func checkSourceDir(dir string) Finding {
	if dir == "" {
		return Finding{Level: LevelOK, Title: "source dir", Detail: "(current working directory at launch)"}
	}
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return Finding{Level: LevelFail, Title: "source dir", Detail: dir, Hint: "directory does not exist"}
	}
	if err != nil {
		return Finding{Level: LevelFail, Title: "source dir", Detail: dir, Hint: err.Error()}
	}
	if !info.IsDir() {
		return Finding{Level: LevelFail, Title: "source dir", Detail: dir, Hint: "path is not a directory"}
	}
	return Finding{Level: LevelOK, Title: "source dir", Detail: dir}
}

func checkRemoteReachable(ctx context.Context, effectiveURL, displayURL string) Finding {
	if displayURL == "" {
		return Finding{Level: LevelWarn, Title: "remote reachable", Detail: "skipped — no URL configured"}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(ctx, "git", "ls-remote", effectiveURL)
	out, err := cmd.CombinedOutput()
	dur := time.Since(start).Round(time.Millisecond)
	if err != nil {
		return Finding{
			Level:  LevelFail,
			Title:  "remote reachable",
			Detail: fmt.Sprintf("failed in %s", dur),
			Hint:   firstLine(string(out)),
		}
	}
	return Finding{Level: LevelOK, Title: "remote reachable", Detail: fmt.Sprintf("git ls-remote OK (%s)", dur)}
}

// ---------- helpers ----------

func settingFilePath() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "setting.json")
	}
	wd, _ := os.Getwd()
	return filepath.Join(wd, "setting.json")
}

func resolveToken(loaded settings.Settings) string {
	if v := os.Getenv("IVIDEO_HLS_TOKEN"); v != "" {
		return v
	}
	return loaded.Token
}

func resolveAuthMethod(loaded settings.Settings) settings.AuthMethod {
	if loaded.AuthMethod != "" {
		return loaded.AuthMethod
	}
	return inferAuthMethod(loaded.RemoteURL, settings.AuthSSH)
}

func inferAuthMethod(url string, current settings.AuthMethod) settings.AuthMethod {
	switch {
	case strings.HasPrefix(url, "https://"):
		return settings.AuthHTTPS
	case strings.HasPrefix(url, "git@"), strings.HasPrefix(url, "ssh://"):
		return settings.AuthSSH
	}
	return current
}

func effectiveRemoteURL(displayURL, token string, method settings.AuthMethod) string {
	if method != settings.AuthHTTPS || token == "" {
		return displayURL
	}
	if !strings.HasPrefix(displayURL, "https://") {
		return displayURL
	}
	// Avoid double-injection if the URL already carries userinfo.
	if strings.Contains(displayURL[len("https://"):], "@") {
		return displayURL
	}
	return "https://" + token + "@" + strings.TrimPrefix(displayURL, "https://")
}

func validateRemoteURL(url string) error {
	switch {
	case url == "":
		return errors.New("remote URL is required")
	case strings.HasPrefix(url, "git@"),
		strings.HasPrefix(url, "ssh://"),
		strings.HasPrefix(url, "https://"):
		return nil
	}
	return errors.New("URL must start with git@, ssh://, or https://")
}

func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return strings.Repeat("•", len(token))
	}
	return strings.Repeat("•", 8) + token[len(token)-4:]
}

func runOrEmpty(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx > 0 {
		return s[:idx]
	}
	return s
}

func summarizeVersion(line string) string {
	// "ffmpeg version 7.1-tessus https://..." → "7.1-tessus"
	parts := strings.Fields(line)
	if len(parts) >= 3 && parts[1] == "version" {
		return "v" + parts[2]
	}
	return ""
}
