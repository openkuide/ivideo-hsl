// Package gitrepo provides a secondary adapter that satisfies ports.GitRepository
// by shelling out to the git binary.
package gitrepo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chamrong/ivideo-hls/internal/ports"
)

// compile-time interface check
var _ ports.GitRepository = (*Adapter)(nil)

// Adapter wraps git binary calls to implement ports.GitRepository.
type Adapter struct{ binaryPath string }

// New returns an Adapter using the provided git binary path.
// Pass "git" to use the system git on PATH.
func New(binaryPath string) *Adapter { return &Adapter{binaryPath: binaryPath} }

// Init initialises dir as a git repository (if it isn't already) and
// ensures the remote "origin" points to remoteURL.
func (a *Adapter) Init(ctx context.Context, dir, remoteURL string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		if err := a.run(ctx, dir, "init", "-b", "main"); err != nil {
			return err
		}
	}
	return a.setRemote(ctx, dir, remoteURL)
}

// CheckoutBranch creates or resets the named branch, best-effort syncing
// with main first.
func (a *Adapter) CheckoutBranch(ctx context.Context, dir, branch string) error {
	_ = a.run(ctx, dir, "checkout", "main")
	_ = a.run(ctx, dir, "pull", "origin", "main")
	return a.run(ctx, dir, "checkout", "-B", branch)
}

// StageAndCommit stages all changes and commits with message. It is a no-op
// when there is nothing to commit.
func (a *Adapter) StageAndCommit(ctx context.Context, dir, message string) error {
	a.removeStaleLock(dir)
	_ = a.run(ctx, dir, "add", ".")
	// diff --cached --quiet exits 0 when nothing is staged → nothing to commit
	if a.run(ctx, dir, "diff", "--cached", "--quiet") == nil {
		return nil
	}
	return a.run(ctx, dir, "commit", "-m", message)
}

// ForcePush force-pushes branch to pushURL.
func (a *Adapter) ForcePush(ctx context.Context, dir, pushURL, branch string) error {
	return a.run(ctx, dir, "push", "-u", "-f", pushURL, branch)
}

// --- helpers ------------------------------------------------------------------

func (a *Adapter) setRemote(ctx context.Context, dir, remoteURL string) error {
	if remoteURL == "" {
		return nil
	}
	out, err := exec.CommandContext(ctx, a.binaryPath, "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return a.run(ctx, dir, "remote", "add", "origin", remoteURL)
	}
	current := strings.TrimSpace(string(out))
	if current != remoteURL {
		return a.run(ctx, dir, "remote", "set-url", "origin", remoteURL)
	}
	return nil
}

func (a *Adapter) removeStaleLock(dir string) {
	lock := filepath.Join(dir, ".git", "index.lock")
	if fi, err := os.Stat(lock); err == nil && time.Since(fi.ModTime()) > 2*time.Minute {
		_ = os.Remove(lock)
	}
}

func (a *Adapter) run(ctx context.Context, dir string, args ...string) error {
	all := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, a.binaryPath, all...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %w\n%s", args, err, out)
	}
	return nil
}
