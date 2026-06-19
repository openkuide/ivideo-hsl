package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/chamrong/ivideo-hls/internal/app"
	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/testutil/fakes"
)

func TestRecoveryService_FindIncomplete(t *testing.T) {
	want := []job.IncompleteWorkspace{
		{Name: "vid1", Workspace: "/ws/vid1", Stage: job.StageConvert},
	}
	finder := &fakes.WorkspaceFinder{
		FindIncompleteFn: func(_ context.Context, scriptDir string) ([]job.IncompleteWorkspace, error) {
			if scriptDir != "/script" {
				t.Errorf("want scriptDir=/script, got %q", scriptDir)
			}
			return want, nil
		},
	}
	svc := app.NewRecoveryService(finder)

	got, err := svc.FindIncomplete(context.Background(), "/script")
	if err != nil {
		t.Fatalf("FindIncomplete: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 incomplete workspace, got %d", len(got))
	}
	if got[0].Name != "vid1" {
		t.Errorf("want Name=vid1, got %q", got[0].Name)
	}
}

func TestRecoveryService_FindIncomplete_Error(t *testing.T) {
	finder := &fakes.WorkspaceFinder{
		FindIncompleteFn: func(_ context.Context, _ string) ([]job.IncompleteWorkspace, error) {
			return nil, errors.New("scan failed")
		},
	}
	svc := app.NewRecoveryService(finder)

	_, err := svc.FindIncomplete(context.Background(), "/script")
	if err == nil {
		t.Fatal("want error from FindIncomplete, got nil")
	}
}

func TestRecoveryService_FindRetryReady(t *testing.T) {
	want := []job.RetryWorkspace{
		{Name: "vid2", Workspace: "/ws/vid2", Branch: "vid2"},
	}
	finder := &fakes.WorkspaceFinder{
		FindRetryReadyFn: func(_ context.Context, scriptDir string) ([]job.RetryWorkspace, error) {
			if scriptDir != "/script" {
				t.Errorf("want scriptDir=/script, got %q", scriptDir)
			}
			return want, nil
		},
	}
	svc := app.NewRecoveryService(finder)

	got, err := svc.FindRetryReady(context.Background(), "/script")
	if err != nil {
		t.Fatalf("FindRetryReady: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 retry workspace, got %d", len(got))
	}
	if got[0].Name != "vid2" {
		t.Errorf("want Name=vid2, got %q", got[0].Name)
	}
}

func TestRecoveryService_FindRetryReady_Error(t *testing.T) {
	finder := &fakes.WorkspaceFinder{
		FindRetryReadyFn: func(_ context.Context, _ string) ([]job.RetryWorkspace, error) {
			return nil, errors.New("scan failed")
		},
	}
	svc := app.NewRecoveryService(finder)

	_, err := svc.FindRetryReady(context.Background(), "/script")
	if err == nil {
		t.Fatal("want error from FindRetryReady, got nil")
	}
}
