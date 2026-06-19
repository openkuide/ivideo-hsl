// Package manifest provides a secondary adapter that satisfies ports.ManifestWriter
// by writing and merging urls.json files alongside source videos and inside HLS
// output directories.
package manifest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// compile-time interface check
var _ ports.ManifestWriter = (*Adapter)(nil)

const manifestFilename = "urls.json"
const marriedSingle = "index.single"

// Adapter merges per-video HLS URLs into urls.json files. It is safe for
// concurrent use — an internal mutex serialises reads+writes to the same
// destination file.
type Adapter struct {
	pattern string
	mu      sync.Mutex
}

// New returns an Adapter using pattern as the public URL template.
// Placeholders: {branch}, {subdir}, {filename}.
// Pass an empty string to use local filesystem paths instead.
func New(pattern string) *Adapter { return &Adapter{pattern: pattern} }

// Record merges one public URL per HLS dir into a urls.json file that lives
// next to sourceFile (i.e. in filepath.Dir(sourceFile)). It is a no-op when
// hlsDirs is empty.
func (a *Adapter) Record(ctx context.Context, sourceFile, branch string, hlsDirs []string, jobName string, e job.Emitter) error {
	if len(hlsDirs) == 0 {
		return nil
	}
	entries := make(map[string]string, len(hlsDirs))
	for _, d := range hlsDirs {
		k, v := a.entry(branch, d)
		entries[k] = v
	}
	for k, v := range entries {
		job.Emit(e, job.LevelDim, jobName, job.StageDone, "urls.json: "+k+" → "+v)
	}
	manifestPath := filepath.Join(filepath.Dir(sourceFile), manifestFilename)
	return a.mergeJSON(manifestPath, entries)
}

// WriteWorkspace drops a single-key urls.json inside each HLS output dir.
// Downstream consumers can read the URL from the workspace without knowing
// the branch or overall directory layout.
func (a *Adapter) WriteWorkspace(ctx context.Context, branch string, hlsDirs []string, jobName string, e job.Emitter) error {
	for _, d := range hlsDirs {
		k, v := a.entry(branch, d)
		data, err := marshalJSON(map[string]string{k: v})
		if err != nil {
			job.Emit(e, job.LevelWarn, jobName, job.StageGitPush, "workspace "+manifestFilename+" skipped: "+err.Error())
			continue
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			job.Emit(e, job.LevelWarn, jobName, job.StageGitPush, "workspace "+manifestFilename+" skipped: "+err.Error())
			continue
		}
		path := filepath.Join(d, manifestFilename)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			job.Emit(e, job.LevelWarn, jobName, job.StageGitPush, "workspace "+manifestFilename+" not written: "+err.Error())
		}
	}
	return nil
}

// --- helpers ------------------------------------------------------------------

// entry returns the (key, value) pair for a single HLS output dir.
//
// key   = "branch/subdir/index.single"
// value = public raw URL (or local path when pattern is empty)
//
// hlsDir is either <workspace>/x (single-episode) or <workspace>/ep1/x (split).
// For split videos the meaningful identifier is the episode dir, so we use the
// parent when the leaf is "x" and the parent starts with "ep".
func (a *Adapter) entry(branch, hlsDir string) (key, value string) {
	leaf := filepath.Base(hlsDir)
	parent := filepath.Base(filepath.Dir(hlsDir))
	var subdir string
	if leaf == "x" && strings.HasPrefix(parent, "ep") {
		subdir = parent
	} else {
		subdir = leaf
	}
	key = branch + "/" + subdir + "/" + marriedSingle
	if a.pattern == "" {
		value = filepath.Join(hlsDir, marriedSingle)
		return
	}
	// {subdir} in the URL is always the leaf ("x") regardless of episode numbering.
	out := a.pattern
	out = strings.ReplaceAll(out, "{branch}", branch)
	out = strings.ReplaceAll(out, "{subdir}", leaf)
	out = strings.ReplaceAll(out, "{filename}", marriedSingle)
	value = out
	return
}

// mergeJSON reads the existing JSON map at path (if any), merges in entries,
// then writes the result atomically via a temp-file rename.
func (a *Adapter) mergeJSON(path string, entries map[string]string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	existing := make(map[string]string)
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &existing)
	}
	for k, v := range entries {
		existing[k] = v
	}
	data, err := marshalJSON(existing)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	return os.Rename(tmp, path)
}

func marshalJSON(m map[string]string) ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal urls.json: %w", err)
	}
	return append(data, '\n'), nil
}
