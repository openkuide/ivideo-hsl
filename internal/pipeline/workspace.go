package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

var baseHeroMu sync.Mutex

func prepareBaseHero(ctx context.Context, cfg *Config, e Emitter) error {
	baseHeroMu.Lock()
	defer baseHeroMu.Unlock()

	if _, err := os.Stat(filepath.Join(cfg.BaseHeroDir, ".git")); err != nil {
		return nil
	}
	info(e, BaseJob, StageWorkspace, "preparing base hero folder…")
	if err := cleanBaseHeroBranches(ctx, cfg.BaseHeroDir, e); err != nil {
		return err
	}
	return configureRemoteOrigin(ctx, cfg.BaseHeroDir, cfg.RemoteURL, e, BaseJob)
}

func setupWorkspace(ctx context.Context, videoPath string, cfg *Config, job string, e Emitter) (string, error) {
	videoBase := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	sanitized := sanitizeRe.ReplaceAllString(videoBase, "_")
	workspaceDir := filepath.Join(cfg.ScriptDir, "hero_"+sanitized)

	if workspaceDir == cfg.BaseHeroDir {
		return "", fmt.Errorf("refusing to overwrite base hero folder")
	}

	if _, err := os.Stat(workspaceDir); err == nil {
		info(e, job, StageWorkspace, "using existing workspace hero_"+sanitized)
		if err := configureRemoteOrigin(ctx, workspaceDir, cfg.RemoteURL, e, job); err != nil {
			return "", err
		}
		return workspaceDir, nil
	}

	if _, err := os.Stat(filepath.Join(cfg.BaseHeroDir, ".git")); err == nil {
		info(e, job, StageWorkspace, "cloning base → hero_"+sanitized)
		start := time.Now()
		if err := runQuiet(ctx, "", "cp", "-r", cfg.BaseHeroDir, workspaceDir); err != nil {
			return "", fmt.Errorf("copy base hero: %w", err)
		}
		dim(e, job, StageWorkspace, fmt.Sprintf("copy finished in %.1fs", time.Since(start).Seconds()))

		_ = runQuiet(ctx, workspaceDir, "git", "clean", "-fd")
		_ = runQuiet(ctx, workspaceDir, "git", "reset", "--hard", "HEAD")
		_ = runQuiet(ctx, workspaceDir, "git", "checkout", "main")
		if err := configureRemoteOrigin(ctx, workspaceDir, cfg.RemoteURL, e, job); err != nil {
			return "", err
		}
		return workspaceDir, nil
	}

	info(e, job, StageWorkspace, "creating new workspace hero_"+sanitized)
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, ".git")); os.IsNotExist(err) {
		if err := runQuiet(ctx, workspaceDir, "git", "init", "-b", "main"); err != nil {
			return "", err
		}
		if err := configureRemoteOrigin(ctx, workspaceDir, cfg.RemoteURL, e, job); err != nil {
			return "", err
		}
		if _, err := os.Stat(cfg.BaseHeroDir); os.IsNotExist(err) {
			_ = os.WriteFile(filepath.Join(workspaceDir, ".gitkeep"), nil, 0o644)
			_ = runQuiet(ctx, workspaceDir, "git", "add", ".gitkeep")
			_ = runQuiet(ctx, workspaceDir, "git", "commit", "-m", "Initial commit")
		}
	}
	return workspaceDir, nil
}

func cleanupWorkspace(workspaceDir string, e Emitter, job string) {
	if filepath.Base(workspaceDir) == "hero" {
		warn(e, job, StageWorkspace, "refusing to cleanup base 'hero' folder")
		return
	}
	if err := os.RemoveAll(workspaceDir); err != nil {
		errorf(e, job, StageWorkspace, "cleanup failed: "+err.Error())
		return
	}
	dim(e, job, StageWorkspace, "workspace removed")
}
