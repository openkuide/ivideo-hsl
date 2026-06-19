package fakes

import (
	"context"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
)

type WorkspaceFinder struct {
	FindIncompleteFn func(ctx context.Context, scriptDir string) ([]job.IncompleteWorkspace, error)
	FindRetryReadyFn func(ctx context.Context, scriptDir string) ([]job.RetryWorkspace, error)
}

func (f *WorkspaceFinder) FindIncomplete(ctx context.Context, scriptDir string) ([]job.IncompleteWorkspace, error) {
	if f.FindIncompleteFn != nil { return f.FindIncompleteFn(ctx, scriptDir) }
	return nil, nil
}

func (f *WorkspaceFinder) FindRetryReady(ctx context.Context, scriptDir string) ([]job.RetryWorkspace, error) {
	if f.FindRetryReadyFn != nil { return f.FindRetryReadyFn(ctx, scriptDir) }
	return nil, nil
}
