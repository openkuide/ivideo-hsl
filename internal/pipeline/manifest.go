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

// recordSuccess appends one public URL per HLS dir to urls.txt in the video's
// source directory. For a single-episode video hlsDirs has one entry; for a
// split video it has one per episode. Failures are surfaced as warnings.
func (w *manifestWriter) recordSuccess(cfg *Config, videoPath, branch string, hlsDirs []string, job string, e Emitter) {
	manifestPath := filepath.Join(filepath.Dir(videoPath), manifestFilename)
	for _, hlsDir := range hlsDirs {
		entry := renderManifestEntry(cfg.PublicURLPattern, branch, hlsDir)
		if err := w.appendLine(manifestPath, entry); err != nil {
			warn(e, job, StageDone, "urls.txt not updated: "+err.Error())
			return
		}
		dim(e, job, StageDone, "appended to "+manifestFilename+": "+entry)
	}
}

// writeWorkspaceManifest drops a urls.txt inside each HLS output dir before
// commit, so every pushed branch directory is self-describing for downstream
// consumers. One file per episode for split videos.
func (w *manifestWriter) writeWorkspaceManifest(cfg *Config, branch string, hlsDirs []string, job string, e Emitter) {
	for _, hlsDir := range hlsDirs {
		entry := renderManifestEntry(cfg.PublicURLPattern, branch, hlsDir)
		if err := os.MkdirAll(hlsDir, 0o755); err != nil {
			warn(e, job, StageGitPush, "workspace urls.txt skipped: "+err.Error())
			continue
		}
		path := filepath.Join(hlsDir, manifestFilename)
		if err := os.WriteFile(path, []byte(entry+"\n"), 0o644); err != nil {
			warn(e, job, StageGitPush, "workspace urls.txt not written: "+err.Error())
		}
	}
}

// renderManifestEntry produces the URL line for one HLS output dir. When the
// pattern is empty, falls back to the local filesystem path.
// Placeholders: {branch}, {subdir}, {filename}. {subdir} is the last path
// component of hlsDir ("x" for single-episode, "ep1"/"ep2"/… for split).
func renderManifestEntry(pattern, branch, hlsDir string) string {
	if pattern == "" {
		return filepath.Join(hlsDir, marriedSingle)
	}
	subdir := filepath.Base(hlsDir) // "x", "ep1", "ep2", …
	out := pattern
	out = strings.ReplaceAll(out, "{branch}", branch)
	out = strings.ReplaceAll(out, "{subdir}", subdir)
	out = strings.ReplaceAll(out, "{filename}", marriedSingle)
	return out
}

// discoverHLSDirs returns the HLS output dirs present in workspace. For a
// single-episode workspace the only dir is workspace/x. For a split workspace
// the dirs are workspace/ep1/x, workspace/ep2/x, … in numeric order.
// Falls back to [workspace/x] when nothing else is found.
func discoverHLSDirs(workspace string) []string {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return []string{filepath.Join(workspace, "x")}
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "x" {
			// single-episode layout — return immediately
			return []string{filepath.Join(workspace, "x")}
		}
		if strings.HasPrefix(name, "ep") {
			dirs = append(dirs, filepath.Join(workspace, name, "x"))
		}
	}
	if len(dirs) == 0 {
		return []string{filepath.Join(workspace, "x")}
	}
	return dirs
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
