package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/chamrong/ivideo-hls/internal/app"
	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
	"github.com/chamrong/ivideo-hls/internal/ports/portstest"
)

func newRunnerFakes() (*portstest.Encoder, *portstest.Prober, *portstest.Splitter, *portstest.Workspace, *portstest.GitRepository, *portstest.ManifestWriter) {
	return &portstest.Encoder{}, &portstest.Prober{}, &portstest.Splitter{}, &portstest.Workspace{}, &portstest.GitRepository{}, &portstest.ManifestWriter{}
}

func TestRunner_Run_SingleVideo(t *testing.T) {
	enc, prober, splitter, ws, git, mw := newRunnerFakes()

	cfg := settings.Default("/script")
	cfg.Push = false

	encSvc := app.NewEncodingService(enc, prober, splitter, ws)
	pubSvc := app.NewPublishingService(git, mw)
	runner := app.NewRunner(encSvc, pubSvc, 1)

	videos := []video.Video{video.NewVideo("/src/a.mp4")}
	results := runner.Run(context.Background(), videos, cfg, nil)

	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Errorf("want success, got err: %v", results[0].Err)
	}
	if len(enc.ConvertCalls) != 1 {
		t.Errorf("want 1 ConvertToHLS call, got %d", len(enc.ConvertCalls))
	}
	if len(git.InitCalls) != 1 {
		t.Errorf("want 1 git.Init call (publish ran), got %d", len(git.InitCalls))
	}
}

func TestRunner_Run_ErrorPropagates(t *testing.T) {
	enc, prober, splitter, ws, git, mw := newRunnerFakes()

	enc.ConvertToHLSFn = func(_ context.Context, _, _ string, _ settings.Settings, _ string, _ job.Emitter) error {
		return errors.New("ffmpeg error")
	}

	cfg := settings.Default("/script")
	encSvc := app.NewEncodingService(enc, prober, splitter, ws)
	pubSvc := app.NewPublishingService(git, mw)
	runner := app.NewRunner(encSvc, pubSvc, 1)

	videos := []video.Video{video.NewVideo("/src/b.mp4")}
	results := runner.Run(context.Background(), videos, cfg, nil)

	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("want failure result, got success")
	}
	if results[0].Err == nil {
		t.Error("want non-nil Err in result")
	}
	// Publish must NOT have been called
	if len(git.InitCalls) != 0 {
		t.Errorf("want 0 git.Init calls (publish should not run on error), got %d", len(git.InitCalls))
	}
}

func TestRunner_Run_MultipleVideos(t *testing.T) {
	enc, prober, splitter, ws, git, mw := newRunnerFakes()

	cfg := settings.Default("/script")
	cfg.Push = false

	encSvc := app.NewEncodingService(enc, prober, splitter, ws)
	pubSvc := app.NewPublishingService(git, mw)
	runner := app.NewRunner(encSvc, pubSvc, 2)

	videos := []video.Video{
		video.NewVideo("/src/a.mp4"),
		video.NewVideo("/src/b.mp4"),
	}
	results := runner.Run(context.Background(), videos, cfg, nil)

	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("video %s failed: %v", r.VideoPath, r.Err)
		}
	}
}
