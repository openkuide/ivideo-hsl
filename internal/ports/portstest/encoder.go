package portstest

import (
	"context"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

type CompressCall struct{ V video.Video; JobName string }
type ConvertCall  struct{ InputPath, OutputDir, JobName string }
type RenameCall   struct{ OutDir, JobName string }

type Encoder struct {
	CompressFn         func(ctx context.Context, v video.Video, jobName string, e job.Emitter) (string, error)
	ConvertToHLSFn     func(ctx context.Context, inputPath, outputDir string, cfg settings.Settings, jobName string, e job.Emitter) error
	RenameHLSOutputsFn func(outDir, jobName string, e job.Emitter) error
	CompressCalls      []CompressCall
	ConvertCalls       []ConvertCall
	RenameCalls        []RenameCall
}

func (f *Encoder) Compress(ctx context.Context, v video.Video, jobName string, e job.Emitter) (string, error) {
	f.CompressCalls = append(f.CompressCalls, CompressCall{V: v, JobName: jobName})
	if f.CompressFn != nil {
		return f.CompressFn(ctx, v, jobName, e)
	}
	return v.Path + "_compressed.mp4", nil
}

func (f *Encoder) ConvertToHLS(ctx context.Context, inputPath, outputDir string, cfg settings.Settings, jobName string, e job.Emitter) error {
	f.ConvertCalls = append(f.ConvertCalls, ConvertCall{InputPath: inputPath, OutputDir: outputDir, JobName: jobName})
	if f.ConvertToHLSFn != nil {
		return f.ConvertToHLSFn(ctx, inputPath, outputDir, cfg, jobName, e)
	}
	return nil
}

func (f *Encoder) RenameHLSOutputs(outDir, jobName string, e job.Emitter) error {
	f.RenameCalls = append(f.RenameCalls, RenameCall{OutDir: outDir, JobName: jobName})
	if f.RenameHLSOutputsFn != nil {
		return f.RenameHLSOutputsFn(outDir, jobName, e)
	}
	return nil
}
