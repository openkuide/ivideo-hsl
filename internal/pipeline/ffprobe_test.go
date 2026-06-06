package pipeline

import (
	"testing"
	"time"
)

func TestFractionOf(t *testing.T) {
	total := 10 * time.Second
	cases := []struct {
		elapsed time.Duration
		want    float64
	}{
		{0, 0},
		{-1 * time.Second, 0}, // negative floors to 0
		{5 * time.Second, 0.5},
		{10 * time.Second, 1.0},
		{20 * time.Second, 1.0}, // capped at 1
	}
	for _, c := range cases {
		got := fractionOf(c.elapsed, total)
		if got != c.want {
			t.Errorf("fractionOf(%v, %v) = %v, want %v", c.elapsed, total, got, c.want)
		}
	}
	// total <= 0 always yields 0
	if got := fractionOf(time.Second, 0); got != 0 {
		t.Errorf("fractionOf with zero total: got %v", got)
	}
}

func TestParseSpeed(t *testing.T) {
	cases := map[string]float64{
		"2.1x":  2.1,
		"1x":    1,
		"0.5x":  0.5,
		" 2x ":  2,
		"N/A":   0,
		"":      0,
		"fast":  0,
	}
	for in, want := range cases {
		if got := parseSpeed(in); got != want {
			t.Errorf("parseSpeed(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestProgressReporter_EmitsOnceAtProgressSentinel(t *testing.T) {
	var calls []struct {
		pct     float64
		speed   float64
		bitrate string
	}
	r := newProgressReporter(10*time.Second, func(pct, speed float64, bitrate string) {
		calls = append(calls, struct {
			pct     float64
			speed   float64
			bitrate string
		}{pct, speed, bitrate})
	})

	// Simulate ffmpeg emitting one group of progress lines.
	lines := []string{
		"frame=120",
		"fps=24",
		"bitrate=2800.0kbits/s",
		"total_size=1234",
		"out_time_ms=2500000", // 2.5s of 10s total
		"speed=2.1x",
		"progress=continue",
	}
	for _, line := range lines {
		if !r.consume(line) {
			t.Errorf("consume(%q) returned false; expected true for known key", line)
		}
	}

	if len(calls) != 1 {
		t.Fatalf("expected exactly one flush, got %d", len(calls))
	}
	got := calls[0]
	if got.pct != 0.25 {
		t.Errorf("pct: got %v, want 0.25", got.pct)
	}
	if got.speed != 2.1 {
		t.Errorf("speed: got %v, want 2.1", got.speed)
	}
	if got.bitrate != "2800.0kbits/s" {
		t.Errorf("bitrate: got %q, want %q", got.bitrate, "2800.0kbits/s")
	}
}

func TestProgressReporter_IgnoresUnknownLines(t *testing.T) {
	r := newProgressReporter(time.Second, func(float64, float64, string) {})
	if r.consume("some-unstructured-stderr-line") {
		t.Error("consume returned true for unknown line; should return false")
	}
}

func TestProgressReporter_NoEmitWithoutElapsed(t *testing.T) {
	called := false
	r := newProgressReporter(10*time.Second, func(float64, float64, string) {
		called = true
	})
	// progress sentinel arrives but no out_time_ms was ever seen
	r.consume("speed=1.0x")
	r.consume("progress=continue")
	if called {
		t.Error("flushed without any elapsed time — would render 0% forever")
	}
}

func TestParseMicros(t *testing.T) {
	cases := map[string]time.Duration{
		"1000000":  time.Second,
		"2500000":  2500 * time.Millisecond,
		"0":        0,
		"-1":       0, // negative yields 0
		"garbage":  0,
		"  1500  ": 1500 * time.Microsecond,
	}
	for in, want := range cases {
		if got := parseMicros(in); got != want {
			t.Errorf("parseMicros(%q) = %v, want %v", in, got, want)
		}
	}
}
