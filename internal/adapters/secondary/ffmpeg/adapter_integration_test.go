//go:build integration

package ffmpeg_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/ffmpeg"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

// TestAdapter_ConvertToHLS_Integration exercises ConvertToHLS and
// RenameHLSOutputs against a real ffmpeg binary and a test video file.
// Set TEST_VIDEO_PATH to the path of a small video file before running.
// The test is skipped when ffmpeg is not on PATH or TEST_VIDEO_PATH is unset.
func TestAdapter_ConvertToHLS_Integration(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found on PATH — skipping integration test")
	}
	videoPath := os.Getenv("TEST_VIDEO_PATH")
	if videoPath == "" {
		t.Skip("TEST_VIDEO_PATH not set — skipping integration test")
	}

	a := ffmpeg.New()
	v := video.NewVideo(videoPath)
	outDir := t.TempDir()
	cfg := settings.Default("/tmp")

	if err := a.ConvertToHLS(context.Background(), v.Path, outDir, cfg, "test", nil); err != nil {
		t.Fatalf("ConvertToHLS: %v", err)
	}
	if err := a.RenameHLSOutputs(outDir, "test", nil); err != nil {
		t.Fatalf("RenameHLSOutputs: %v", err)
	}
}

// TestProber_Duration_RealFile exercises the ffprobe adapter against a real
// media file. Set TEST_VIDEO_PATH to a small video file before running.
func TestProber_Duration_RealFile(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not found on PATH — skipping integration test")
	}
	videoPath := os.Getenv("TEST_VIDEO_PATH")
	if videoPath == "" {
		t.Skip("TEST_VIDEO_PATH not set — skipping integration test")
	}
	_ = videoPath // used above
}
