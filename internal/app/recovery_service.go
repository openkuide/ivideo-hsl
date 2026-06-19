package app

import (
	"context"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// RecoveryService wraps ports.WorkspaceFinder to surface incomplete and
// retry-ready workspaces.
type RecoveryService struct {
	finder ports.WorkspaceFinder
}

// NewRecoveryService constructs a RecoveryService backed by the given finder.
func NewRecoveryService(finder ports.WorkspaceFinder) *RecoveryService {
	return &RecoveryService{finder: finder}
}

// FindIncomplete delegates to the underlying WorkspaceFinder.
func (s *RecoveryService) FindIncomplete(ctx context.Context, scriptDir string) ([]job.IncompleteWorkspace, error) {
	return s.finder.FindIncomplete(ctx, scriptDir)
}

// FindRetryReady delegates to the underlying WorkspaceFinder.
func (s *RecoveryService) FindRetryReady(ctx context.Context, scriptDir string) ([]job.RetryWorkspace, error) {
	return s.finder.FindRetryReady(ctx, scriptDir)
}
