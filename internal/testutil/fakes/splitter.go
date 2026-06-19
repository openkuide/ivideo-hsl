package fakes

import (
	"context"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

type SplitCall struct{ VideoPath, JobName string }

type Splitter struct {
	SplitFn    func(ctx context.Context, videoPath, jobName string, e job.Emitter) ([]video.Episode, error)
	SplitCalls []SplitCall
}

func (f *Splitter) Split(ctx context.Context, videoPath, jobName string, e job.Emitter) ([]video.Episode, error) {
	f.SplitCalls = append(f.SplitCalls, SplitCall{VideoPath: videoPath, JobName: jobName})
	if f.SplitFn != nil {
		return f.SplitFn(ctx, videoPath, jobName, e)
	}
	return []video.Episode{{Path: videoPath, Suffix: ""}}, nil
}
