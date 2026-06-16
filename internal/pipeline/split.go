package pipeline

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chamrong/ivideo-hls/internal/deps"
)

const splitThresholdBytes = 2 * 1024 * 1024 * 1024 // 2 GB

func probeSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// episode holds one segment of a (possibly split) video.
// suffix is empty for an unsplit file, or "a", "b", "c", … for split parts.
type episode struct {
	path   string
	suffix string // "" = no split; "a","b",… = part letter
}

// partSuffix converts a 0-based index to a letter suffix: 0→"a", 1→"b", …
// Supports up to 26 parts (z). Beyond that falls back to "aa", "ab", etc.
func partSuffix(i int) string {
	if i < 26 {
		return string(rune('a' + i))
	}
	return string(rune('a'+i/26-1)) + string(rune('a'+i%26))
}

// splitIntoEpisodes splits videoPath into N equal-duration parts using ffmpeg
// stream-copy (no re-encode). Returns a single-element slice (suffix="") when
// the file is under the 2 GB threshold.
func splitIntoEpisodes(ctx context.Context, videoPath, job string, e Emitter) ([]episode, error) {
	if probeSize(videoPath) <= splitThresholdBytes {
		return []episode{{path: videoPath, suffix: ""}}, nil
	}

	total := probeDuration(ctx, videoPath)
	if total <= 0 {
		return nil, fmt.Errorf("could not probe duration of %s", filepath.Base(videoPath))
	}

	size := probeSize(videoPath)
	numParts := int(math.Ceil(float64(size) / float64(splitThresholdBytes)))
	partDur := total / time.Duration(numParts)

	info(e, job, StageConvert, fmt.Sprintf(
		"file %.2f GB > 2 GB — splitting into %d part(s) of ~%.0fs each",
		float64(size)/1024/1024/1024, numParts, partDur.Seconds(),
	))

	dir := filepath.Dir(videoPath)
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))

	var episodes []episode
	for i := range numParts {
		suf := partSuffix(i)
		start := time.Duration(i) * partDur
		outPath := filepath.Join(dir, fmt.Sprintf("%s%s.mp4", base, suf))

		args := splitArgs(videoPath, outPath, start, partDur, i == numParts-1)
		opts := runOpts{
			job:     job,
			stage:   StageConvert,
			emitter: e,
			silent:  true,
		}
		info(e, job, StageConvert, fmt.Sprintf("cutting part %s → %s", suf, filepath.Base(outPath)))
		if err := run(ctx, opts, deps.FFmpegPath(), args...); err != nil {
			_ = os.Remove(outPath)
			return nil, fmt.Errorf("split part %s: %w", suf, err)
		}
		episodes = append(episodes, episode{path: outPath, suffix: suf})
	}
	return episodes, nil
}

// splitArgs returns the ffmpeg arguments to extract one segment. The last
// segment omits -t so it captures any remaining frames (avoids 1-frame gaps
// from rounding).
func splitArgs(input, output string, start, dur time.Duration, isLast bool) []string {
	args := []string{
		"-ss", fmt.Sprintf("%.3f", start.Seconds()),
		"-i", input,
		"-c", "copy", // stream-copy — fast, no quality loss
		"-avoid_negative_ts", "make_zero",
	}
	if !isLast {
		args = append(args, "-t", fmt.Sprintf("%.3f", dur.Seconds()))
	}
	args = append(args, "-y", output)
	return args
}
