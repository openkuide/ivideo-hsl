package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/chamrong/ivideo-hls/internal/app"
	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/ports/portstest"
)

func TestPublishingService_Publish_CallsCommitAndPush(t *testing.T) {
	git := &portstest.GitRepository{}
	mw := &portstest.ManifestWriter{}

	svc := app.NewPublishingService(git, mw)
	err := svc.Publish(context.Background(), "/src/v.mp4", "/ws/hero_v", "mybranch", "https://token@github.com/org/repo.git", []string{"/ws/hero_v/x"}, "v", nil)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(git.CommitCalls) != 1 {
		t.Fatalf("want 1 commit, got %d", len(git.CommitCalls))
	}
	if len(git.PushCalls) != 1 {
		t.Fatalf("want 1 push, got %d", len(git.PushCalls))
	}
	if git.PushCalls[0].Branch != "mybranch" {
		t.Errorf("push branch = %q, want %q", git.PushCalls[0].Branch, "mybranch")
	}
}

func TestPublishingService_Publish_NoPush_SkipsPush(t *testing.T) {
	git := &portstest.GitRepository{}
	mw := &portstest.ManifestWriter{}

	svc := app.NewPublishingService(git, mw)
	// empty pushURL signals skip
	err := svc.Publish(context.Background(), "/src/v.mp4", "/ws/hero_v", "mybranch", "", []string{"/ws/hero_v/x"}, "v", nil)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(git.PushCalls) != 0 {
		t.Fatalf("empty pushURL: want 0 push calls, got %d", len(git.PushCalls))
	}
}

func TestPublishingService_Publish_WritesManifests(t *testing.T) {
	git := &portstest.GitRepository{}
	mw := &portstest.ManifestWriter{}

	svc := app.NewPublishingService(git, mw)
	_ = svc.Publish(context.Background(), "/src/v.mp4", "/ws/hero_v", "mybranch", "https://url", []string{"/ws/hero_v/x"}, "v", nil)

	if len(mw.WriteWorkspaceCalls) != 1 {
		t.Fatalf("want 1 WriteWorkspace call, got %d", len(mw.WriteWorkspaceCalls))
	}
	if len(mw.RecordCalls) != 1 {
		t.Fatalf("want 1 Record call, got %d", len(mw.RecordCalls))
	}
}

func TestPublishingService_HappyPath_AllCallsMade(t *testing.T) {
	git := &portstest.GitRepository{}
	mw := &portstest.ManifestWriter{}

	svc := app.NewPublishingService(git, mw)
	err := svc.Publish(context.Background(), "/src/v.mp4", "/ws/hero_v", "mybranch", "https://token@github.com/org/repo.git", []string{"/ws/hero_v/x"}, "v", nil)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	// All git calls should be made: Init, CheckoutBranch, StageAndCommit, ForcePush
	if len(git.InitCalls) != 1 {
		t.Errorf("want 1 Init call, got %d", len(git.InitCalls))
	}
	if len(git.CheckoutCalls) != 1 {
		t.Errorf("want 1 CheckoutBranch call, got %d", len(git.CheckoutCalls))
	}
	if len(git.CommitCalls) != 1 {
		t.Errorf("want 1 StageAndCommit call, got %d", len(git.CommitCalls))
	}
	if len(git.PushCalls) != 1 {
		t.Errorf("want 1 ForcePush call, got %d", len(git.PushCalls))
	}
	// Both manifest calls should be made
	if len(mw.WriteWorkspaceCalls) != 1 {
		t.Errorf("want 1 WriteWorkspace call, got %d", len(mw.WriteWorkspaceCalls))
	}
	if len(mw.RecordCalls) != 1 {
		t.Errorf("want 1 Record call, got %d", len(mw.RecordCalls))
	}
}

func TestPublishingService_PushDisabled_NoPushCall(t *testing.T) {
	git := &portstest.GitRepository{}
	mw := &portstest.ManifestWriter{}

	svc := app.NewPublishingService(git, mw)
	// empty pushURL → ForcePush must NOT be called
	err := svc.Publish(context.Background(), "/src/v.mp4", "/ws/hero_v", "mybranch", "", []string{"/ws/hero_v/x"}, "v", nil)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(git.PushCalls) != 0 {
		t.Errorf("want 0 ForcePush calls when pushURL empty, got %d", len(git.PushCalls))
	}
	// other git calls should still happen
	if len(git.InitCalls) != 1 {
		t.Errorf("want 1 Init call, got %d", len(git.InitCalls))
	}
	if len(git.CommitCalls) != 1 {
		t.Errorf("want 1 Commit call, got %d", len(git.CommitCalls))
	}
}

func TestPublishingService_GitInitError_ReturnsError(t *testing.T) {
	git := &portstest.GitRepository{
		InitFn: func(_ context.Context, _, _ string) error {
			return errors.New("init failed")
		},
	}
	mw := &portstest.ManifestWriter{}

	svc := app.NewPublishingService(git, mw)
	err := svc.Publish(context.Background(), "/src/v.mp4", "/ws/hero_v", "mybranch", "https://url", []string{"/ws/hero_v/x"}, "v", nil)
	if err == nil {
		t.Fatal("expected error from git.Init, got nil")
	}
}

func TestPublishingService_ManifestWriteError_Continues(t *testing.T) {
	git := &portstest.GitRepository{}
	mw := &portstest.ManifestWriter{
		WriteWorkspaceFn: func(_ context.Context, _ string, _ []string, _ string, _ job.Emitter) error {
			return errors.New("write workspace failed")
		},
	}

	svc := app.NewPublishingService(git, mw)
	// WriteWorkspace error should NOT abort; Publish should return nil
	err := svc.Publish(context.Background(), "/src/v.mp4", "/ws/hero_v", "mybranch", "https://url", []string{"/ws/hero_v/x"}, "v", nil)
	if err != nil {
		t.Fatalf("expected no error when WriteWorkspace fails (warn+continue), got: %v", err)
	}
	// git calls should still proceed
	if len(git.CommitCalls) != 1 {
		t.Errorf("want 1 commit after manifest warn, got %d", len(git.CommitCalls))
	}
}
