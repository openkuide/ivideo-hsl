// Package workspace provides a secondary adapter that satisfies ports.Workspace
// by managing per-video hero_* git working trees on disk.
package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// compile-time interface check
var _ ports.Workspace = (*Adapter)(nil)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// baseMu serialises PrepareBase so concurrent jobs don't race on the
// shared hero/ base directory.
var baseMu sync.Mutex

// Adapter manages the per-video workspace directories (hero_*) used to
// stage HLS output before it is committed and pushed.
type Adapter struct{ gitBinary string }

// New returns an Adapter using the provided git binary path.
// Pass "git" to use the system git on PATH.
func New(gitBinary string) *Adapter { return &Adapter{gitBinary: gitBinary} }

// PrepareBase ensures the base hero/ directory is on the main branch and its
// remote is up to date. It is safe to call concurrently.
func (a *Adapter) PrepareBase(ctx context.Context, cfg settings.Settings, e job.Emitter) error {
	baseMu.Lock()
	defer baseMu.Unlock()

	baseDir := filepath.Join(cfg.ScriptDir, "hero")
	if _, err := os.Stat(filepath.Join(baseDir, ".git")); err != nil {
		return nil // base dir doesn't exist yet; nothing to prepare
	}
	job.Emit(e, job.LevelInfo, job.BaseJob, job.StageWorkspace, "preparing base hero folder…")
	return a.run(ctx, baseDir, "checkout", "main")
}

// Setup ensures a workspace directory exists for video v and returns its
// absolute path. It prefers to clone from the hero/ base when available,
// or creates a fresh git repository otherwise.
func (a *Adapter) Setup(ctx context.Context, v video.Video, cfg settings.Settings, jobName string, e job.Emitter) (string, error) {
	sanitized := sanitizeRe.ReplaceAllString(v.Name, "_")
	wsDir := filepath.Join(cfg.ScriptDir, "hero_"+sanitized)
	baseDir := filepath.Join(cfg.ScriptDir, "hero")

	// Guard: refuse to overwrite the base directory itself
	if wsDir == baseDir {
		return "", fmt.Errorf("refusing to overwrite base hero folder")
	}

	// Use existing workspace when one is present
	if _, err := os.Stat(wsDir); err == nil {
		job.Emit(e, job.LevelInfo, jobName, job.StageWorkspace, "using existing workspace hero_"+sanitized)
		return wsDir, nil
	}

	// Clone from base if it has a .git
	if _, err := os.Stat(filepath.Join(baseDir, ".git")); err == nil {
		job.Emit(e, job.LevelInfo, jobName, job.StageWorkspace, "cloning base → hero_"+sanitized)
		start := time.Now()
		if err := exec.CommandContext(ctx, "cp", "-r", baseDir, wsDir).Run(); err != nil {
			return "", fmt.Errorf("copy base hero: %w", err)
		}
		job.Emit(e, job.LevelDim, jobName, job.StageWorkspace,
			fmt.Sprintf("copy finished in %.1fs", time.Since(start).Seconds()))
		_ = a.run(ctx, wsDir, "clean", "-fd")
		_ = a.run(ctx, wsDir, "reset", "--hard", "HEAD")
		_ = a.run(ctx, wsDir, "checkout", "main")
		return wsDir, nil
	}

	// Create a fresh workspace
	job.Emit(e, job.LevelInfo, jobName, job.StageWorkspace, "creating new workspace hero_"+sanitized)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(wsDir, ".git")); os.IsNotExist(err) {
		if err := a.run(ctx, wsDir, "init", "-b", "main"); err != nil {
			return "", err
		}
		keep := filepath.Join(wsDir, ".gitkeep")
		_ = os.WriteFile(keep, nil, 0o644)
		_ = a.run(ctx, wsDir, "add", ".gitkeep")
		_ = a.run(ctx, wsDir, "commit", "-m", "Initial commit")
	}
	return wsDir, nil
}

// Cleanup removes the workspace directory. It refuses to remove the base
// "hero" directory to prevent accidental data loss.
func (a *Adapter) Cleanup(workspaceDir string, e job.Emitter, jobName string) {
	if filepath.Base(workspaceDir) == "hero" {
		job.Emit(e, job.LevelWarn, jobName, job.StageWorkspace, "refusing to cleanup base 'hero' folder")
		return
	}
	if err := os.RemoveAll(workspaceDir); err != nil {
		job.Emit(e, job.LevelError, jobName, job.StageWorkspace, "cleanup failed: "+err.Error())
		return
	}
	job.Emit(e, job.LevelDim, jobName, job.StageWorkspace, "workspace removed")
}

// --- helpers ------------------------------------------------------------------

func (a *Adapter) run(ctx context.Context, dir string, args ...string) error {
	all := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, a.gitBinary, all...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %w\n%s", args, err, out)
	}
	return nil
}
