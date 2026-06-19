// Package workspacefinder provides a secondary adapter that satisfies
// ports.WorkspaceFinder by scanning hero_* directories on disk.
package workspacefinder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// compile-time interface check
var _ ports.WorkspaceFinder = (*Adapter)(nil)

const marriedSingle = "index.single"
const tsSuffix = ".ts"
const m3u8Suffix = ".m3u8"

// Adapter scans the scriptDir for hero_* workspaces and reports their state.
type Adapter struct{ gitBinary string }

// New returns an Adapter using the provided git binary path.
// Pass "git" to use the system git on PATH.
func New(gitBinary string) *Adapter { return &Adapter{gitBinary: gitBinary} }

// FindIncomplete returns hero_* directories that have a .git but do NOT have
// a finished index.single — these need a full pipeline re-run.
func (a *Adapter) FindIncomplete(ctx context.Context, scriptDir string) ([]job.IncompleteWorkspace, error) {
	entries, err := os.ReadDir(scriptDir)
	if err != nil {
		return nil, err
	}
	var out []job.IncompleteWorkspace
	for _, e := range entries {
		if !e.IsDir() || !isHeroWorkspace(e.Name()) {
			continue
		}
		dir := filepath.Join(scriptDir, e.Name())
		if !hasGit(dir) {
			continue
		}
		if hasFinishedOutput(dir) {
			continue // retry-ready territory
		}
		out = append(out, inspectIncomplete(scriptDir, dir))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// FindRetryReady returns hero_* directories that have a committed, unpushed
// index.single — these can be retried with a push-only operation.
func (a *Adapter) FindRetryReady(ctx context.Context, scriptDir string) ([]job.RetryWorkspace, error) {
	entries, err := os.ReadDir(scriptDir)
	if err != nil {
		return nil, err
	}
	var out []job.RetryWorkspace
	for _, e := range entries {
		if !e.IsDir() || !isHeroWorkspace(e.Name()) {
			continue
		}
		dir := filepath.Join(scriptDir, e.Name())
		if !hasGit(dir) || !hasFinishedOutput(dir) {
			continue
		}
		branch, ok := a.currentBranch(ctx, dir)
		if !ok {
			continue
		}
		if !a.hasUnpushedCommits(ctx, dir) {
			continue
		}
		name := strings.TrimPrefix(filepath.Base(dir), "hero_")
		out = append(out, job.RetryWorkspace{
			Name:      name,
			Workspace: dir,
			Branch:    branch,
			Size:      outputSize(filepath.Join(dir, "x")),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// --- helpers ------------------------------------------------------------------

func isHeroWorkspace(name string) bool {
	return strings.HasPrefix(name, "hero_") && name != "hero"
}

func hasGit(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func hasFinishedOutput(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "x", marriedSingle))
	return err == nil
}

func (a *Adapter) currentBranch(ctx context.Context, workspace string) (string, bool) {
	out, err := exec.CommandContext(ctx, a.gitBinary,
		"-C", workspace, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", false
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" || branch == "" {
		return "", false
	}
	return branch, true
}

// hasUnpushedCommits returns true when the workspace has commits not yet seen
// by the upstream. When no upstream exists (first push) the branch qualifies.
func (a *Adapter) hasUnpushedCommits(ctx context.Context, workspace string) bool {
	if err := runQuiet(a.gitBinary, workspace, "rev-parse", "HEAD"); err != nil {
		return false
	}
	out, err := exec.CommandContext(ctx, a.gitBinary,
		"-C", workspace, "rev-list", "@{upstream}..HEAD", "--count").Output()
	if err != nil {
		return true // no upstream → treat as unpushed
	}
	return strings.TrimSpace(string(out)) != "0"
}

func inspectIncomplete(scriptDir, workspace string) job.IncompleteWorkspace {
	name := strings.TrimPrefix(filepath.Base(workspace), "hero_")
	src := guessSourcePath(scriptDir, name)
	_, srcErr := os.Stat(src)

	w := job.IncompleteWorkspace{
		Name:         name,
		Workspace:    workspace,
		SourcePath:   src,
		SourceExists: srcErr == nil,
	}

	if compressed := findCompressedSibling(src); compressed != "" {
		w.CompressedPath = compressed
	}

	w.Stage, w.Hint = describeIncomplete(workspace, w.CompressedPath)
	return w
}

func describeIncomplete(workspace, compressedPath string) (job.Stage, string) {
	segDir := filepath.Join(workspace, "x")
	entries, err := os.ReadDir(segDir)
	if err != nil || len(entries) == 0 {
		if compressedPath != "" {
			if size := fileSize(compressedPath); size > 0 {
				return job.StageCompress, fmt.Sprintf("partial _compressed.mp4 (%s)", humanBytes(size))
			}
		}
		return job.StageFailed, "no segments yet"
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
		return job.StageRename, fmt.Sprintf("%d .ts segments, playlist pre-rename", tsCount)
	}
	return job.StageConvert, fmt.Sprintf("%d .ts segments written", tsCount)
}

func guessSourcePath(sourceDir, name string) string {
	for _, ext := range []string{".mp4", ".mov", ".m4v", ".mkv", ".webm", ".avi"} {
		candidate := filepath.Join(sourceDir, name+ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join(sourceDir, name+".mp4")
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

func runQuiet(binary, dir string, args ...string) error {
	all := append([]string{"-C", dir}, args...)
	return exec.Command(binary, all...).Run()
}
