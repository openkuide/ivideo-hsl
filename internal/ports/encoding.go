package ports

import (
	"context"
	"time"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

type Encoder interface {
	Compress(ctx context.Context, v video.Video, jobName string, e job.Emitter) (compressedPath string, err error)
	ConvertToHLS(ctx context.Context, inputPath, outputDir string, cfg settings.Settings, jobName string, e job.Emitter) error
	RenameHLSOutputs(outDir, jobName string, e job.Emitter) error
}

type Prober interface {
	Duration(ctx context.Context, path string) (time.Duration, error)
	FileSize(path string) int64
}

type Splitter interface {
	Split(ctx context.Context, videoPath, jobName string, e job.Emitter) ([]video.Episode, error)
}
