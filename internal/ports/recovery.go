package ports

import (
	"context"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
)

type WorkspaceFinder interface {
	FindIncomplete(ctx context.Context, scriptDir string) ([]job.IncompleteWorkspace, error)
	FindRetryReady(ctx context.Context, scriptDir string) ([]job.RetryWorkspace, error)
}
