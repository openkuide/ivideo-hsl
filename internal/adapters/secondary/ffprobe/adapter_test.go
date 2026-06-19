package ffprobe_test

import (
	"testing"

	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/ffprobe"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// TestAdapter_SatisfiesPorts is a compile-time proof that *Adapter satisfies
// ports.Prober. The var _ declaration in adapter.go catches this at compile
// time; this test makes the intent explicit and visible in test output.
func TestAdapter_SatisfiesPorts(t *testing.T) {
	var _ ports.Prober = ffprobe.New()
}

// TestAdapter_FileSize_NonExistent verifies that FileSize returns 0 for a
// path that does not exist, rather than panicking or returning a negative
// value.
func TestAdapter_FileSize_NonExistent(t *testing.T) {
	a := ffprobe.New()
	got := a.FileSize("/nonexistent/path/that/should/not/exist.mp4")
	if got != 0 {
		t.Errorf("FileSize(nonexistent) = %d, want 0", got)
	}
}
