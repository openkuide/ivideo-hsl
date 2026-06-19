package ports

import (
	"context"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
)

type GitRepository interface {
	Init(ctx context.Context, dir, remoteURL string) error
	CheckoutBranch(ctx context.Context, dir, branch string) error
	StageAndCommit(ctx context.Context, dir, message string) error
	ForcePush(ctx context.Context, dir, pushURL, branch string) error
}

type ManifestWriter interface {
	Record(ctx context.Context, sourceDir, branch string, hlsDirs []string, jobName string, e job.Emitter) error
	WriteWorkspace(ctx context.Context, branch string, hlsDirs []string, jobName string, e job.Emitter) error
}
