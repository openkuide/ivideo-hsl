package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func removeGitLocks(dir string) {
	lock := filepath.Join(dir, ".git", "index.lock")
	info, err := os.Stat(lock)
	if err != nil {
		return
	}
	if time.Since(info.ModTime()) > 2*time.Minute {
		_ = os.Remove(lock)
	}
}

func configureRemoteOrigin(ctx context.Context, repoDir, remoteURL string, e Emitter, job string) error {
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		return nil
	}
	current, err := runCapture(ctx, repoDir, "git", "remote", "get-url", "origin")
	if err != nil {
		if addErr := runQuiet(ctx, repoDir, "git", "remote", "add", "origin", remoteURL); addErr != nil {
			return fmt.Errorf("add remote: %w", addErr)
		}
		if e != nil {
			dim(e, job, StageWorkspace, "remote added: "+remoteURL)
		}
		return nil
	}
	if current != remoteURL {
		if err := runQuiet(ctx, repoDir, "git", "remote", "set-url", "origin", remoteURL); err != nil {
			return fmt.Errorf("set remote: %w", err)
		}
		if e != nil {
			dim(e, job, StageWorkspace, "remote updated → "+remoteURL)
		}
	}
	return nil
}

func cleanBaseHeroBranches(ctx context.Context, baseHeroDir string, e Emitter) error {
	if _, err := os.Stat(filepath.Join(baseHeroDir, ".git")); err != nil {
		return nil
	}
	removeGitLocks(baseHeroDir)

	// ensure main branch exists
	if err := runQuiet(ctx, baseHeroDir, "git", "show-ref", "--verify", "--quiet", "refs/heads/main"); err != nil {
		if err2 := runQuiet(ctx, baseHeroDir, "git", "show-ref", "--verify", "--quiet", "refs/heads/master"); err2 == nil {
			_ = runQuiet(ctx, baseHeroDir, "git", "branch", "-m", "master", "main")
		} else {
			if err := runQuiet(ctx, baseHeroDir, "git", "checkout", "-b", "main"); err != nil {
				_ = runQuiet(ctx, baseHeroDir, "git", "checkout", "--orphan", "main")
			}
			if err := runQuiet(ctx, baseHeroDir, "git", "log", "-1"); err != nil {
				keep := filepath.Join(baseHeroDir, ".gitkeep")
				if _, err := os.Stat(keep); errors.Is(err, os.ErrNotExist) {
					_ = os.WriteFile(keep, nil, 0o644)
				}
				_ = runQuiet(ctx, baseHeroDir, "git", "add", ".gitkeep")
				_ = runQuiet(ctx, baseHeroDir, "git", "commit", "-m", "Initial commit")
			}
		}
	}

	_ = runQuiet(ctx, baseHeroDir, "git", "checkout", "main")

	out, err := runCapture(ctx, baseHeroDir, "git", "branch")
	if err != nil {
		return nil
	}

	removed := 0
	for line := range strings.SplitSeq(out, "\n") {
		b := strings.TrimSpace(line)
		if b == "" || b == "main" || strings.HasPrefix(b, "*") {
			continue
		}
		if err := runQuiet(ctx, baseHeroDir, "git", "branch", "-D", b); err == nil {
			removed++
		}
	}
	if e != nil {
		if removed > 0 {
			success(e, BaseJob, StageWorkspace, fmt.Sprintf("cleaned %d local branches (kept main)", removed))
		} else {
			dim(e, BaseJob, StageWorkspace, "no branches to clean")
		}
	}
	return nil
}
