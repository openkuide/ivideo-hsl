// Package appconfig loads and saves the user's persistent ivideo-hls
// configuration: preferred remote URL, authentication method, GitHub token,
// and default encoding choices.
//
// The file lives at $XDG_CONFIG_HOME/ivideo-hls/config.toml (falling back to
// ~/.config/ivideo-hls/config.toml) with mode 0600 because it may contain
// a token.
//
// The format is a minimal hand-rolled TOML-ish key=value grammar. Using the
// stdlib keeps the binary dependency-free, and the surface is small enough
// (~7 fields) that the parser is trivial.
package appconfig

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AuthMethod selects how git push authenticates with the remote.
type AuthMethod string

const (
	AuthSSH   AuthMethod = "ssh"
	AuthHTTPS AuthMethod = "https"
)

// File is the persisted shape. Zero values are treated as "not set" so the
// precedence chain (flag > env > file > built-in) can fall through cleanly.
type File struct {
	RemoteURL          string
	AuthMethod         AuthMethod
	Token              string
	DefaultQuality     string
	DefaultCompression string
	DefaultPreCompress bool
	DefaultKeepSource  bool
	// DefaultSourceDir is the folder scanned when no -i/-a/--source is given.
	// Empty = use the current working directory.
	DefaultSourceDir string
	// DefaultRecursive enables subdirectory scanning by default.
	DefaultRecursive bool
	// DefaultPush controls whether `git push` runs. Stored as a tri-state via
	// the *bool idiom would be ideal, but for simplicity the zero value means
	// "push enabled" and DefaultPushDisabled flips it off.
	DefaultPushDisabled    bool
	DefaultCleanupDisabled bool
	// DefaultParallel is the preferred number of concurrent jobs when no
	// -j / -p flag is given. Zero = serial (today's default).
	DefaultParallel int
	// ResumeReuseCompressed enables the opt-in resume optimization: when
	// resume-failed sees a _compressed.mp4 that passes CompressedReusable
	// (clean finish, no .partial sibling, valid duration), skip the compress
	// stage on retry. Default false — always redo.
	ResumeReuseCompressed bool
	// PublicURLPattern is the template used to render each published video's
	// public URL into urls.txt. Placeholders: {branch}, {subdir}, {filename}.
	// {subdir} is "x" for single-episode videos and "ep1", "ep2", … for splits.
	// Empty = fall back to writing the local workspace path.
	// Example: "https://raw.githubusercontent.com/org/repo/{branch}/{subdir}/{filename}"
	PublicURLPattern string
}

// Path returns the absolute path where the config file lives (creating no
// directories). Honors $XDG_CONFIG_HOME if set.
func Path() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ivideo-hls", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "ivideo-hls", "config.toml"), nil
}

// Exists reports whether a config file is already present.
func Exists() bool {
	p, err := Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Load reads the config file. Returns a zero-value File and no error when the
// file does not exist — "no config yet" is a normal state, not a failure.
func Load() (File, error) {
	p, err := Path()
	if err != nil {
		return File{}, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return File{}, nil
	}
	if err != nil {
		return File{}, fmt.Errorf("open %s: %w", p, err)
	}
	defer f.Close()
	return parse(f)
}

// Save writes the config file atomically with mode 0600.
func Save(cfg File) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	tmp := p + ".tmp"
	data := render(cfg)
	if err := os.WriteFile(tmp, []byte(data), 0o600); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func render(cfg File) string {
	var b strings.Builder
	b.WriteString("# ivideo-hls config — edit via the settings TUI ('s' on the picker)\n")
	b.WriteString("# Token is stored plaintext at 0600. Prefer $IVIDEO_HLS_TOKEN for sensitive use.\n\n")
	writeString(&b, "remote_url", cfg.RemoteURL)
	writeString(&b, "auth_method", string(cfg.AuthMethod))
	writeString(&b, "token", cfg.Token)
	writeString(&b, "default_quality", cfg.DefaultQuality)
	writeString(&b, "default_compression", cfg.DefaultCompression)
	writeBool(&b, "default_pre_compress", cfg.DefaultPreCompress)
	writeBool(&b, "default_keep_source", cfg.DefaultKeepSource)
	writeString(&b, "default_source_dir", cfg.DefaultSourceDir)
	writeBool(&b, "default_recursive", cfg.DefaultRecursive)
	writeBool(&b, "default_push_disabled", cfg.DefaultPushDisabled)
	writeBool(&b, "default_cleanup_disabled", cfg.DefaultCleanupDisabled)
	writeInt(&b, "default_parallel", cfg.DefaultParallel)
	writeBool(&b, "resume_reuse_compressed", cfg.ResumeReuseCompressed)
	writeString(&b, "public_url_pattern", cfg.PublicURLPattern)
	return b.String()
}

func writeString(b *strings.Builder, key, value string) {
	if value == "" {
		fmt.Fprintf(b, "%s = \"\"\n", key)
		return
	}
	fmt.Fprintf(b, "%s = %q\n", key, value)
}

func writeBool(b *strings.Builder, key string, value bool) {
	fmt.Fprintf(b, "%s = %t\n", key, value)
}

func writeInt(b *strings.Builder, key string, value int) {
	fmt.Fprintf(b, "%s = %d\n", key, value)
}

func parse(r *os.File) (File, error) {
	var cfg File
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, value, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		assign(&cfg, key, value)
	}
	if err := sc.Err(); err != nil {
		return File{}, fmt.Errorf("read config: %w", err)
	}
	return cfg, nil
}

func parseLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	k, v, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	k = strings.TrimSpace(k)
	v = strings.TrimSpace(v)
	if unquoted, err := strconv.Unquote(v); err == nil {
		v = unquoted
	}
	return k, v, true
}

func assign(cfg *File, key, value string) {
	switch key {
	case "remote_url":
		cfg.RemoteURL = value
	case "auth_method":
		cfg.AuthMethod = AuthMethod(value)
	case "token":
		cfg.Token = value
	case "default_quality":
		cfg.DefaultQuality = value
	case "default_compression":
		cfg.DefaultCompression = value
	case "default_pre_compress":
		cfg.DefaultPreCompress = value == "true"
	case "default_keep_source":
		cfg.DefaultKeepSource = value == "true"
	case "default_source_dir":
		cfg.DefaultSourceDir = value
	case "default_recursive":
		cfg.DefaultRecursive = value == "true"
	case "default_push_disabled":
		cfg.DefaultPushDisabled = value == "true"
	case "default_cleanup_disabled":
		cfg.DefaultCleanupDisabled = value == "true"
	case "default_parallel":
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			cfg.DefaultParallel = n
		}
	case "resume_reuse_compressed":
		cfg.ResumeReuseCompressed = value == "true"
	case "public_url_pattern":
		cfg.PublicURLPattern = value
	}
}

// SaveRunConfig persists only the run-config fields that the picker screen can
// change (quality, compression, parallel, pre-compress, keep-source). All
// other fields — remote, token, source dir, etc. — are left as-is. This lets
// the session choices survive to the next startup without touching credentials.
//
// If no config file exists yet the full file is written with zero values for
// the fields the picker doesn't control; effectively the same as a first Save.
func SaveRunConfig(quality, compression string, parallel int, preCompress, keepSource bool) error {
	current, err := Load()
	if err != nil {
		return err
	}
	if quality != "" {
		current.DefaultQuality = quality
	}
	if compression != "" {
		current.DefaultCompression = compression
	}
	if parallel >= 1 {
		current.DefaultParallel = parallel
	}
	current.DefaultPreCompress = preCompress
	current.DefaultKeepSource = keepSource
	return Save(current)
}

// EffectiveRemoteURL returns the URL to hand to `git push`, with a token
// injected into the URL's userinfo when appropriate. The original remote URL
// is returned unchanged for SSH or when no token is available — never log
// the return value of this function; it can contain credentials.
func EffectiveRemoteURL(displayURL, token string, method AuthMethod) string {
	if method != AuthHTTPS || token == "" {
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

// MaskToken returns a short, log-safe preview of a secret.
func MaskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return strings.Repeat("•", len(token))
	}
	return strings.Repeat("•", 8) + token[len(token)-4:]
}

// ValidateRemoteURL returns nil for any URL shape ivideo-hls knows how to
// push to, and an error describing what's wrong otherwise.
func ValidateRemoteURL(url string) error {
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

// InferAuthMethod picks the likely auth method from the URL scheme. Returned
// unchanged when the URL shape doesn't imply an answer.
func InferAuthMethod(url string, current AuthMethod) AuthMethod {
	switch {
	case strings.HasPrefix(url, "https://"):
		return AuthHTTPS
	case strings.HasPrefix(url, "git@"), strings.HasPrefix(url, "ssh://"):
		return AuthSSH
	}
	return current
}
