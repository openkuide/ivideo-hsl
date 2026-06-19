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

func TestEncodingService_SingleEpisode(t *testing.T) {
	enc := &portstest.Encoder{}
	prober := &portstest.Prober{} // default: 10 min duration (< 30 min threshold)
	splitter := &portstest.Splitter{} // default: returns single episode, no split
	ws := &portstest.Workspace{}

	svc := app.NewEncodingService(enc, prober, splitter, ws)
	v := video.NewVideo("/src/myvideo.mp4")
	cfg := settings.Default("/script")

	hlsDirs, err := svc.Process(context.Background(), v, cfg, "myvideo", nil)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(hlsDirs) != 1 {
		t.Fatalf("want 1 hlsDir, got %d", len(hlsDirs))
	}
	if len(enc.ConvertCalls) != 1 {
		t.Fatalf("want 1 ConvertToHLS call, got %d", len(enc.ConvertCalls))
	}
	if len(enc.CompressCalls) != 0 {
		t.Fatalf("PreCompress=false: want 0 compress calls, got %d", len(enc.CompressCalls))
	}
}

func TestEncodingService_PreCompress(t *testing.T) {
	enc := &portstest.Encoder{}
	prober := &portstest.Prober{}
	splitter := &portstest.Splitter{}
	ws := &portstest.Workspace{}

	svc := app.NewEncodingService(enc, prober, splitter, ws)
	v := video.NewVideo("/src/myvideo.mp4")
	cfg := settings.Default("/script")
	cfg.PreCompress = true

	_, err := svc.Process(context.Background(), v, cfg, "myvideo", nil)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(enc.CompressCalls) != 1 {
		t.Fatalf("PreCompress=true: want 1 compress call, got %d", len(enc.CompressCalls))
	}
	// Verify the compress was called with the original video
	if enc.CompressCalls[0].V.Path != v.Path {
		t.Fatalf("want Compress called with path %q, got %q", v.Path, enc.CompressCalls[0].V.Path)
	}
	// Verify ConvertToHLS uses the compressed path (default fake returns path+"_compressed.mp4")
	if len(enc.ConvertCalls) != 1 {
		t.Fatalf("want 1 ConvertToHLS call, got %d", len(enc.ConvertCalls))
	}
	if enc.ConvertCalls[0].InputPath != v.Path+"_compressed.mp4" {
		t.Fatalf("want ConvertToHLS input %q, got %q", v.Path+"_compressed.mp4", enc.ConvertCalls[0].InputPath)
	}
}

func TestEncodingService_SplitEpisodes(t *testing.T) {
	enc := &portstest.Encoder{}
	prober := &portstest.Prober{}
	splitter := &portstest.Splitter{
		SplitFn: func(_ context.Context, videoPath, _ string, _ job.Emitter) ([]video.Episode, error) {
			return []video.Episode{
				{Path: videoPath + "_a.mp4", Suffix: "a"},
				{Path: videoPath + "_b.mp4", Suffix: "b"},
			}, nil
		},
	}
	ws := &portstest.Workspace{}

	svc := app.NewEncodingService(enc, prober, splitter, ws)
	v := video.NewVideo("/src/big.mp4")
	cfg := settings.Default("/script")

	hlsDirs, err := svc.Process(context.Background(), v, cfg, "big", nil)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(hlsDirs) != 2 {
		t.Fatalf("want 2 hlsDirs for split, got %d", len(hlsDirs))
	}
	if len(enc.ConvertCalls) != 2 {
		t.Fatalf("want 2 ConvertToHLS calls for 2 episodes, got %d", len(enc.ConvertCalls))
	}
}

func TestEncodingService_WorkspaceError(t *testing.T) {
	enc := &portstest.Encoder{}
	prober := &portstest.Prober{}
	splitter := &portstest.Splitter{}
	ws := &portstest.Workspace{
		SetupFn: func(_ context.Context, _ video.Video, _ settings.Settings, _ string, _ job.Emitter) (string, error) {
			return "", errors.New("workspace setup failed")
		},
	}

	svc := app.NewEncodingService(enc, prober, splitter, ws)
	v := video.NewVideo("/src/myvideo.mp4")
	cfg := settings.Default("/script")

	_, err := svc.Process(context.Background(), v, cfg, "myvideo", nil)
	if err == nil {
		t.Fatal("want error from workspace setup failure, got nil")
	}
	if len(enc.ConvertCalls) != 0 {
		t.Fatalf("want 0 ConvertToHLS calls after workspace error, got %d", len(enc.ConvertCalls))
	}
}

func TestEncodingService_CleanupAlwaysCalled(t *testing.T) {
	enc := &portstest.Encoder{
		ConvertToHLSFn: func(_ context.Context, _, _ string, _ settings.Settings, _ string, _ job.Emitter) error {
			return errors.New("convert failed")
		},
	}
	prober := &portstest.Prober{}
	splitter := &portstest.Splitter{}
	ws := &portstest.Workspace{}

	svc := app.NewEncodingService(enc, prober, splitter, ws)
	v := video.NewVideo("/src/myvideo.mp4")
	cfg := settings.Default("/script")

	_, _ = svc.Process(context.Background(), v, cfg, "myvideo", nil)

	if len(ws.CleanupCalls) == 0 {
		t.Fatal("want Cleanup called even when ConvertToHLS errors, got 0 calls")
	}
}
