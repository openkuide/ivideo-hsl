package ports

import (
	"context"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

type Workspace interface {
	Setup(ctx context.Context, v video.Video, cfg settings.Settings, jobName string, e job.Emitter) (workspaceDir string, err error)
	Cleanup(workspaceDir string, e job.Emitter, jobName string)
	PrepareBase(ctx context.Context, cfg settings.Settings, e job.Emitter) error
}
