package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// PublishingService orchestrates writing manifests, committing to git,
// optionally pushing, and recording the public URL.
type PublishingService struct {
	git      ports.GitRepository
	manifest ports.ManifestWriter
}

// NewPublishingService constructs a PublishingService wired with port adapters.
func NewPublishingService(git ports.GitRepository, mw ports.ManifestWriter) *PublishingService {
	return &PublishingService{git: git, manifest: mw}
}

// Publish orchestrates the full publishing pipeline for a single video:
//  1. WriteWorkspace — write urls.json into each HLS dir (warn+continue on error)
//  2. git.Init — initialise git repo in workspaceDir
//  3. git.CheckoutBranch — switch to the video's branch
//  4. git.StageAndCommit — stage everything and commit
//  5. git.ForcePush — force-push (skipped when pushURL is empty)
//  6. manifest.Record — record to source-dir manifest (warn+continue on error)
//
// videoPath is the original source file path; its directory is used as sourceDir
// for the Record call.  An empty pushURL suppresses the push step.
func (s *PublishingService) Publish(
	ctx context.Context,
	videoPath, workspaceDir, branch, pushURL string,
	hlsDirs []string,
	jobName string,
	e job.Emitter,
) error {
	// Step 1: write workspace manifest (non-fatal)
	if err := s.manifest.WriteWorkspace(ctx, branch, hlsDirs, jobName, e); err != nil {
		job.Emit(e, job.LevelWarn, jobName, job.StageGitPush,
			"write workspace manifest: "+err.Error())
	}

	// Step 2: git init — pass pushURL so the remote is set correctly.
	if err := s.git.Init(ctx, workspaceDir, pushURL); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	// Step 3: checkout branch
	if err := s.git.CheckoutBranch(ctx, workspaceDir, branch); err != nil {
		return fmt.Errorf("git checkout branch: %w", err)
	}

	// Step 4: stage and commit
	if err := s.git.StageAndCommit(ctx, workspaceDir, "add HLS for "+jobName); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	// Step 5: force-push (optional)
	if pushURL != "" {
		if err := s.git.ForcePush(ctx, workspaceDir, pushURL, branch); err != nil {
			return fmt.Errorf("git push: %w", err)
		}
	}

	// Step 6: record to source manifest (non-fatal)
	sourceDir := filepath.Dir(videoPath)
	if err := s.manifest.Record(ctx, sourceDir, branch, hlsDirs, jobName, e); err != nil {
		job.Emit(e, job.LevelWarn, jobName, job.StageGitPush,
			"manifest record failed: "+err.Error())
	}

	return nil
}

// PushWorkspace force-pushes an already-committed workspace to the remote.
// Used by the retry-failed / recover recovery paths to push without re-encoding.
func (s *PublishingService) PushWorkspace(ctx context.Context, dir, branch, pushURL string) error {
	if err := s.git.ForcePush(ctx, dir, pushURL, branch); err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	return nil
}
