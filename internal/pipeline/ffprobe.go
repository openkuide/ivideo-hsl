package pipeline

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/chamrong/ivideo-hls/internal/deps"
)

// probeDuration returns the duration of a media file in seconds using ffprobe.
// Zero is returned when the duration is unavailable; callers should treat that
// as "unknown" and skip progress math rather than failing.
func probeDuration(ctx context.Context, path string) time.Duration {
	out, err := runCapture(ctx, "",
		deps.FFprobePath(),
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=nokey=1:noprint_wrappers=1",
		path,
	)
	if err != nil {
		return 0
	}
	secs, err := strconv.ParseFloat(strings.TrimSpace(out), 64)
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs * float64(time.Second))
}

// progressReporter turns ffmpeg's `-progress pipe:1` key=value stream into a
// callback. ffmpeg emits a group of lines (frame=, fps=, bitrate=, speed=,
// out_time_ms=, …) terminated by `progress=continue` or `progress=end`.
// We report once per group so the UI sees a coherent snapshot instead of
// partial updates.
type progressReporter struct {
	total   time.Duration
	report  func(pct float64, speed float64, bitrate string)
	speed   float64
	bitrate string
	pct     float64
}

func newProgressReporter(total time.Duration, report func(pct float64, speed float64, bitrate string)) *progressReporter {
	return &progressReporter{total: total, report: report}
}

// consume parses a single line from ffmpeg's progress stream. It returns true
// when the line belongs to the progress protocol (and should not be logged
// elsewhere). The reporter accumulates fields until a `progress=` sentinel
// arrives, then flushes one snapshot.
func (p *progressReporter) consume(line string) bool {
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return false
	}
	switch key {
	case "out_time_ms", "out_time_us":
		p.pct = fractionOf(parseMicros(value), p.total)
		return true
	case "speed":
		p.speed = parseSpeed(value)
		return true
	case "bitrate":
		p.bitrate = strings.TrimSpace(value)
		return true
	case "progress":
		p.flush()
		return true
	case "frame", "fps", "total_size", "stream_0_0_q",
		"dup_frames", "drop_frames", "out_time":
		return true
	}
	return false
}

func (p *progressReporter) flush() {
	if p.report == nil || p.pct <= 0 {
		return
	}
	p.report(p.pct, p.speed, p.bitrate)
}

func fractionOf(elapsed, total time.Duration) float64 {
	if total <= 0 || elapsed <= 0 {
		return 0
	}
	pct := float64(elapsed) / float64(total)
	if pct < 0 {
		return 0
	}
	if pct > 1 {
		return 1
	}
	return pct
}

// parseSpeed reads ffmpeg's speed field, which looks like "2.1x" or "N/A".
// Returns 0 when unparseable.
func parseSpeed(v string) float64 {
	v = strings.TrimSuffix(strings.TrimSpace(v), "x")
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

func parseMicros(v string) time.Duration {
	micros, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || micros < 0 {
		return 0
	}
	return time.Duration(micros) * time.Microsecond
}
