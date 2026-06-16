package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RetryCandidate describes a per-video workspace left behind by a failed
// run that looks ready to push — convert and rename finished, commit is
// in place, just the push didn't land.
type RetryCandidate struct {
	Name      string // sanitized name (the hero_ suffix)
	Workspace string // absolute path to hero_<name>/
	Branch    string // current branch in the workspace
	// Size of the output (sum of files under x/) for display purposes.
	Size int64
}

// FindRetryCandidates scans scriptDir for hero_<name>/ directories that
// carry a committed, unpushed playlist ready for retry. Workspaces that
// are partially-encoded (missing index.single) are ignored — retrying
// them would skip the conversion that never finished.
func FindRetryCandidates(ctx context.Context, scriptDir string) ([]RetryCandidate, error) {
	entries, err := os.ReadDir(scriptDir)
	if err != nil {
		return nil, err
	}
	var out []RetryCandidate
	for _, e := range entries {
		if !e.IsDir() || !isHeroWorkspace(e.Name()) {
			continue
		}
		candidate, ok := inspectWorkspace(ctx, filepath.Join(scriptDir, e.Name()))
		if !ok {
			continue
		}
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func isHeroWorkspace(name string) bool {
	return strings.HasPrefix(name, "hero_") && name != "hero"
}

// inspectWorkspace decides whether a single hero_* directory qualifies as
// a retry candidate. It's a pure inspection — no remote calls — so that
// FindRetryCandidates is cheap enough to run from `doctor`.
func inspectWorkspace(ctx context.Context, dir string) (RetryCandidate, bool) {
	if !hasGit(dir) {
		return RetryCandidate{}, false
	}
	if !hasFinishedOutput(dir) {
		return RetryCandidate{}, false
	}
	branch, ok := currentBranch(ctx, dir)
	if !ok {
		return RetryCandidate{}, false
	}
	if !hasUnpushedCommits(ctx, dir) {
		return RetryCandidate{}, false
	}
	name := strings.TrimPrefix(filepath.Base(dir), "hero_")
	return RetryCandidate{
		Name:      name,
		Workspace: dir,
		Branch:    branch,
		Size:      outputSize(filepath.Join(dir, "x")),
	}, true
}

func hasGit(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func hasFinishedOutput(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "x", marriedSingle))
	return err == nil
}

func currentBranch(ctx context.Context, workspace string) (string, bool) {
	out, err := runCapture(ctx, workspace, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || out == "HEAD" || out == "" {
		return "", false
	}
	return out, true
}

// hasUnpushedCommits returns true when the workspace has commits that the
// upstream hasn't seen. When no upstream exists (first push), we treat any
// commit as unpushed so a never-before-pushed branch qualifies for retry.
func hasUnpushedCommits(ctx context.Context, workspace string) bool {
	if err := runQuiet(ctx, workspace, "git", "rev-parse", "HEAD"); err != nil {
		return false
	}
	// If rev-list against @{upstream} errors, there is no upstream —
	// treat that as "has unpushed commits" so brand-new branches qualify.
	out, err := runCapture(ctx, workspace, "git", "rev-list", "@{upstream}..HEAD", "--count")
	if err != nil {
		return true
	}
	return strings.TrimSpace(out) != "0"
}

func outputSize(dir string) int64 {
	var total int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

// RetryOne runs the push-only recovery for a single candidate. It reuses
// the runner's existing force-push path, so behavior (mutexes, semaphores,
// logging, credential redaction) stays identical to a normal successful
// run. On success, the workspace is cleaned up and the matching source
// .mp4 is deleted when Cleanup + Push are both enabled in cfg.
func (r *Runner) RetryOne(ctx context.Context, c RetryCandidate) error {
	jc := &jobContext{
		job:       c.Name,
		branch:    c.Branch,
		workspace: c.Workspace,
		videoPath: guessSourcePath(r.cfg.SourceDir, c.Name),
	}
	info(r.emitter, jc.job, StageGitPush, "retry: force-pushing existing commit")
	if err := r.forcePush(ctx, jc); err != nil {
		return err
	}
	defaultManifestWriter.recordSuccess(r.cfg, jc.videoPath, jc.branch, discoverHLSDirs(jc.workspace), jc.job, r.emitter)
	r.stepFinalize(jc)
	success(r.emitter, jc.job, StageDone, "retry complete")
	return nil
}

// IncompleteStage names the pipeline stage a workspace got stuck at.
// Used for display only — a single enum so the UI can show the operator
// what they're about to redo.
type IncompleteStage string

const (
	IncompleteCompress IncompleteStage = "compress"
	IncompleteConvert  IncompleteStage = "convert"
	IncompleteRename   IncompleteStage = "rename"
	IncompleteUnknown  IncompleteStage = "unknown"
)

// IncompleteWorkspace describes a hero_<name>/ directory from a failed run
// that is NOT retry-ready — the pipeline stopped before the playlist was
// finalized. These need a full re-run from the source .mp4.
type IncompleteWorkspace struct {
	Name         string
	Workspace    string
	SourcePath   string // best-guess from the name; may be ""
	SourceExists bool
	Stage        IncompleteStage
	// Hint is a short human-readable detail ("7 .ts files written",
	// "2.1 GB partial _compressed.mp4") — never structured data, so the UI
	// can just render it. Empty when nothing interesting to say.
	Hint string
	// CompressedPath is the sibling _compressed.mp4 next to SourcePath when
	// one exists. Empty when there isn't one.
	CompressedPath string
}

// FindIncompleteWorkspaces returns hero_<name>/ directories that are NOT
// retry-ready: convert/rename didn't finish, so there's no x/index.single
// to push. These need a full pipeline re-run from the source.
//
// Workspaces without a .git subdirectory are skipped — those aren't
// ivideo-hls workspaces. Workspaces that *are* retry-ready are also
// skipped (retry-failed owns them).
func FindIncompleteWorkspaces(ctx context.Context, scriptDir string) ([]IncompleteWorkspace, error) {
	entries, err := os.ReadDir(scriptDir)
	if err != nil {
		return nil, err
	}
	var out []IncompleteWorkspace
	for _, e := range entries {
		if !e.IsDir() || !isHeroWorkspace(e.Name()) {
			continue
		}
		dir := filepath.Join(scriptDir, e.Name())
		if !hasGit(dir) {
			continue
		}
		if hasFinishedOutput(dir) {
			continue // retry-failed's territory
		}
		out = append(out, inspectIncomplete(scriptDir, dir))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func inspectIncomplete(scriptDir, workspace string) IncompleteWorkspace {
	name := strings.TrimPrefix(filepath.Base(workspace), "hero_")
	src := guessSourcePath(scriptDir, name)
	_, srcErr := os.Stat(src)
	w := IncompleteWorkspace{
		Name:         name,
		Workspace:    workspace,
		SourcePath:   src,
		SourceExists: srcErr == nil,
	}
	compressedPath := findCompressedSibling(src)
	if compressedPath != "" {
		w.CompressedPath = compressedPath
	}
	w.Stage, w.Hint = describeIncomplete(workspace, compressedPath)
	return w
}

// describeIncomplete decides which stage a workspace got stuck at. The
// signals are deliberately coarse — we only need enough detail to show
// the operator "this one's roughly here."
func describeIncomplete(workspace, compressedPath string) (IncompleteStage, string) {
	segDir := filepath.Join(workspace, "x")
	entries, err := os.ReadDir(segDir)
	if err != nil || len(entries) == 0 {
		if compressedPath != "" {
			if size := fileSize(compressedPath); size > 0 {
				return IncompleteCompress,
					fmt.Sprintf("partial _compressed.mp4 (%s)", humanBytes(size))
			}
		}
		return IncompleteUnknown, "no segments yet"
	}
	var tsCount, m3u8Count int
	for _, e := range entries {
		switch filepath.Ext(e.Name()) {
		case tsSuffix:
			tsCount++
		case m3u8Suffix:
			m3u8Count++
		}
	}
	if m3u8Count > 0 {
		// playlist present but not renamed yet
		return IncompleteRename, fmt.Sprintf("%d .ts segments, playlist pre-rename", tsCount)
	}
	return IncompleteConvert, fmt.Sprintf("%d .ts segments written", tsCount)
}

func findCompressedSibling(sourcePath string) string {
	if sourcePath == "" {
		return ""
	}
	dir := filepath.Dir(sourcePath)
	base := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	candidate := filepath.Join(dir, base+"_compressed.mp4")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// CompressedReusable reports whether `path` looks like a clean,
// finished _compressed.mp4 that resume-failed can trust. All four conditions
// must hold:
//
//  1. path is non-empty and exists
//  2. size > 1 KiB (guard against truncated writes)
//  3. ffprobe-readable duration > 1s
//  4. no `_compressed.partial.mp4` sibling exists
//
// (4) is load-bearing: the pipeline writes to .partial and renames on clean
// exit, so a sibling .partial means the last compress was killed mid-write
// and the final file (if any) is stale at best, wrong at worst.
func CompressedReusable(ctx context.Context, path string) bool {
	if path == "" {
		return false
	}
	if fileSize(path) < 1024 {
		return false
	}
	if probeDuration(ctx, path) <= 1*time.Second {
		return false
	}
	return !hasPartialSibling(path)
}

// hasPartialSibling returns true when a <base>_compressed.partial.mp4 lives
// next to <base>_compressed.mp4 — a signal that compress was interrupted.
func hasPartialSibling(compressedPath string) bool {
	dir := filepath.Dir(compressedPath)
	base := filepath.Base(compressedPath)
	// compressedPath is .../<name>_compressed.mp4 — derive the partial form.
	if !strings.HasSuffix(base, "_compressed.mp4") {
		return false
	}
	stem := strings.TrimSuffix(base, "_compressed.mp4")
	partial := filepath.Join(dir, stem+"_compressed.partial.mp4")
	_, err := os.Stat(partial)
	return err == nil
}

// guessSourcePath picks the most likely original .mp4 for a retry: we
// check common video extensions under sourceDir with the candidate name.
// Falls back to <sourceDir>/<name>.mp4 (which may not exist; cleanup
// logic already tolerates that).
func guessSourcePath(sourceDir, name string) string {
	for _, ext := range VideoExtensions {
		candidate := filepath.Join(sourceDir, name+ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join(sourceDir, name+".mp4")
}
