package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// manifestFilename is the name of the per-directory URL log ivideo-hls appends
// to after each successful push. One line per video.
const manifestFilename = "urls.txt"

// manifestWriter serializes appends to urls.txt files so parallel jobs that
// share a source directory don't interleave writes.
type manifestWriter struct {
	mu sync.Mutex
}

var defaultManifestWriter = &manifestWriter{}

// recordSuccess appends the public URL (or a local-path fallback) for a
// completed video to urls.txt in the video's own source directory. Failures
// to write the manifest do not fail the job — they are surfaced as warnings.
func (w *manifestWriter) recordSuccess(cfg *Config, videoPath, branch, workspace, job string, e Emitter) {
	entry := renderManifestEntry(cfg.PublicURLPattern, branch, workspace)
	manifestPath := filepath.Join(filepath.Dir(videoPath), manifestFilename)

	if err := w.appendLine(manifestPath, entry); err != nil {
		warn(e, job, StageDone, "urls.txt not updated: "+err.Error())
		return
	}
	dim(e, job, StageDone, "appended to "+manifestFilename+": "+entry)
}

// writeWorkspaceManifest drops a single-line urls.txt inside the workspace's
// output directory before commit, so the pushed branch contains its own URL
// and is self-describing for downstream consumers browsing the repo.
func (w *manifestWriter) writeWorkspaceManifest(cfg *Config, branch, workspace, job string, e Emitter) {
	entry := renderManifestEntry(cfg.PublicURLPattern, branch, workspace)
	outDir := filepath.Join(workspace, "x")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		warn(e, job, StageGitPush, "workspace urls.txt skipped: "+err.Error())
		return
	}
	path := filepath.Join(outDir, manifestFilename)
	if err := os.WriteFile(path, []byte(entry+"\n"), 0o644); err != nil {
		warn(e, job, StageGitPush, "workspace urls.txt not written: "+err.Error())
	}
}

// renderManifestEntry produces the line to write for one video. When the
// pattern is empty (user hasn't configured a public URL shape), falls back to
// the local filesystem path so the file is still useful.
func renderManifestEntry(pattern, branch, workspace string) string {
	if pattern == "" {
		return filepath.Join(workspace, "x", marriedSingle)
	}
	out := pattern
	out = strings.ReplaceAll(out, "{branch}", branch)
	out = strings.ReplaceAll(out, "{filename}", marriedSingle)
	return out
}

func (w *manifestWriter) appendLine(path, line string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
