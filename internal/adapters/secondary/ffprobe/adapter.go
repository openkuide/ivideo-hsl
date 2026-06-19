// Package ffprobe provides a secondary adapter that satisfies ports.Prober
// by shelling out to the ffprobe binary resolved through the deps package.
package ffprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"os/exec"

	"github.com/chamrong/ivideo-hls/internal/deps"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// compile-time interface check
var _ ports.Prober = (*Adapter)(nil)

// Adapter wraps ffprobe to satisfy ports.Prober.
type Adapter struct{}

// New returns a ready-to-use Adapter. The ffprobe binary is resolved at call
// time via deps.FFprobePath(), which honours the local ./bin/, XDG cache,
// and $PATH lookup chain.
func New() *Adapter { return &Adapter{} }

// Duration returns the media duration reported by ffprobe. An error is
// returned if ffprobe is not found, exits non-zero, or reports no duration.
func (a *Adapter) Duration(ctx context.Context, path string) (time.Duration, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_entries", "format=duration",
		path,
	}
	out, err := exec.CommandContext(ctx, deps.FFprobePath(), args...).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}

	var result struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, fmt.Errorf("parse ffprobe output: %w", err)
	}

	s := strings.TrimSpace(result.Format.Duration)
	if s == "" || s == "N/A" {
		return 0, fmt.Errorf("ffprobe: no duration for %s", path)
	}
	sec, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", s, err)
	}
	return time.Duration(sec * float64(time.Second)), nil
}

// FileSize returns the byte size of the file at path, or 0 if the file does
// not exist or cannot be stat-ed.
func (a *Adapter) FileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}
