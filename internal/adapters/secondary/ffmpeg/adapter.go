// Package ffmpeg provides a secondary adapter that satisfies ports.Encoder and
// ports.Splitter by shelling out to the ffmpeg/ffprobe binaries resolved
// through the deps package.
package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chamrong/ivideo-hls/internal/infrastructure/deps"
	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// compile-time interface checks
var _ ports.Encoder = (*Adapter)(nil)
var _ ports.Splitter = (*Adapter)(nil)

const (
	splitThresholdBytes = 2 * 1024 * 1024 * 1024 // 2 GB

	hlsOutputName = "index"
	tsSuffix      = ".ts"
	marriedSuffix = ".married"
	m3u8Suffix    = ".m3u8"
	singleSuffix  = ".single"
	marriedSingle = hlsOutputName + singleSuffix
)

// Adapter wraps ffmpeg (and ffprobe for duration probing) to satisfy
// ports.Encoder and ports.Splitter. Binaries are resolved at call time via
// deps.FFmpegPath() / deps.FFprobePath().
type Adapter struct{}

// New returns a ready-to-use Adapter.
func New() *Adapter { return &Adapter{} }

// Compress runs a single-pass libx264 compress on v.Path and returns the path
// to the compressed output. The source file is not deleted. An atomic rename
// pattern is used: ffmpeg writes to a .partial sibling that is renamed to the
// final destination only on success.
func (a *Adapter) Compress(ctx context.Context, v video.Video, jobName string, e job.Emitter) (string, error) {
	finalPath := compressedOutputPath(v.Path)
	partialPath := partialCompressedPath(v.Path)

	args := []string{
		"-i", v.Path,
		"-nostats",
		"-loglevel", "error",
		"-progress", "pipe:1",
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "28",
		"-vf", "scale='min(1920,iw)':'min(1080,ih)':force_original_aspect_ratio=decrease",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
		"-y", partialPath,
	}

	job.Emit(e, job.LevelInfo, jobName, job.StageCompress, "compressing "+filepath.Base(v.Path))
	cmd := exec.CommandContext(ctx, deps.FFmpegPath(), args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg compress: %w\n%s", err, out)
	}
	if err := os.Rename(partialPath, finalPath); err != nil {
		return "", fmt.Errorf("finalize compressed output: %w", err)
	}
	job.Emit(e, job.LevelSuccess, jobName, job.StageCompress, "compression complete")
	return finalPath, nil
}

// ConvertToHLS encodes inputPath as an HLS stream into outputDir. The segment
// and playlist filenames use the downstream-required suffixes (.married /
// .single) via a post-encode rename (see RenameHLSOutputs).
func (a *Adapter) ConvertToHLS(ctx context.Context, inputPath, outputDir string, cfg settings.Settings, jobName string, e job.Emitter) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create hls output dir: %w", err)
	}

	s := hlsSettingsFor(cfg)
	job.Emit(e, job.LevelInfo, jobName, job.StageConvert,
		fmt.Sprintf("HLS convert @ %s / %s", cfg.Quality, cfg.Compression))

	args := []string{
		"-i", inputPath,
		"-nostats",
		"-loglevel", "error",
		"-progress", "pipe:1",
		"-c:v", "libx264", "-c:a", "aac",
		"-b:v", s.videoBitrate, "-maxrate", s.videoBitrate, "-bufsize", s.bufsize,
		"-b:a", s.audioBitrate,
		"-vf", "scale=" + s.resolution,
		"-preset", s.preset, "-crf", s.crf,
		"-g", "48", "-sc_threshold", "0",
		"-profile:v", "high", "-level", "4.0",
		"-movflags", "+faststart",
		"-hls_time", "6", "-hls_playlist_type", "vod",
		"-hls_segment_filename", filepath.Join(outputDir, hlsOutputName+"_%03d"+tsSuffix),
		filepath.Join(outputDir, hlsOutputName+m3u8Suffix),
	}

	cmd := exec.CommandContext(ctx, deps.FFmpegPath(), args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg hls: %w\n%s", err, out)
	}
	return nil
}

// RenameHLSOutputs applies the downstream-required rename inside outDir:
//   - <name>.ts  → <name>.married
//   - <name>.m3u8 → <name>.single  (with in-file .ts refs rewritten to .married)
func (a *Adapter) RenameHLSOutputs(outDir, jobName string, e job.Emitter) error {
	if err := renameSegments(outDir); err != nil {
		return fmt.Errorf("rename segments: %w", err)
	}
	if err := rewritePlaylist(outDir); err != nil {
		return fmt.Errorf("rewrite playlist: %w", err)
	}
	job.Emit(e, job.LevelDim, jobName, job.StageRename,
		"renamed "+tsSuffix+" → "+marriedSuffix+", "+m3u8Suffix+" → "+singleSuffix)
	return nil
}

// Split splits videoPath into equal-duration parts using ffmpeg stream-copy
// when the file exceeds the 2 GB threshold. Files under that threshold are
// returned as-is in a single-element slice.
func (a *Adapter) Split(ctx context.Context, videoPath, jobName string, e job.Emitter) ([]video.Episode, error) {
	fi, err := os.Stat(videoPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", videoPath, err)
	}
	if fi.Size() <= splitThresholdBytes {
		return []video.Episode{{Path: videoPath, Suffix: ""}}, nil
	}

	total, err := probeDuration(ctx, videoPath)
	if err != nil || total <= 0 {
		return nil, fmt.Errorf("probe duration of %s: %w", filepath.Base(videoPath), err)
	}

	numParts := int(math.Ceil(float64(fi.Size()) / float64(splitThresholdBytes)))
	partDur := total / time.Duration(numParts)

	job.Emit(e, job.LevelInfo, jobName, job.StageConvert, fmt.Sprintf(
		"file %.2f GB > 2 GB — splitting into %d part(s) of ~%.0fs each",
		float64(fi.Size())/1024/1024/1024, numParts, partDur.Seconds(),
	))

	dir := filepath.Dir(videoPath)
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))

	var episodes []video.Episode
	for i := range numParts {
		suf := partSuffix(i)
		start := time.Duration(i) * partDur
		outPath := filepath.Join(dir, fmt.Sprintf("%s%s.mp4", base, suf))

		args := splitArgs(videoPath, outPath, start, partDur, i == numParts-1)
		job.Emit(e, job.LevelInfo, jobName, job.StageConvert,
			fmt.Sprintf("cutting part %s → %s", suf, filepath.Base(outPath)))
		cmd := exec.CommandContext(ctx, deps.FFmpegPath(), args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			_ = os.Remove(outPath)
			return nil, fmt.Errorf("split part %s: %w\n%s", suf, err, out)
		}
		episodes = append(episodes, video.Episode{Path: outPath, Suffix: suf})
	}
	return episodes, nil
}

// --- helpers -----------------------------------------------------------------

type hlsEncodeSettings struct {
	preset       string
	crf          string
	videoBitrate string
	audioBitrate string
	resolution   string
	bufsize      string
}

func hlsSettingsFor(cfg settings.Settings) hlsEncodeSettings {
	s := hlsEncodeSettings{
		preset:       "medium",
		crf:          "23",
		videoBitrate: "2800k",
		audioBitrate: "128k",
		resolution:   "-2:720",
		bufsize:      "5600k",
	}
	switch cfg.Compression {
	case video.CompressionBest:
		s.preset, s.crf = "slow", "26"
	case video.CompressionFast:
		s.preset, s.crf = "fast", "23"
	}
	switch cfg.Quality {
	case video.QualityLow:
		s.videoBitrate, s.audioBitrate, s.resolution, s.bufsize = "800k", "96k", "-2:480", "1600k"
	case video.QualityHigh:
		s.videoBitrate, s.audioBitrate, s.resolution, s.bufsize = "5000k", "192k", "-2:1080", "10000k"
	}
	return s
}

func compressedOutputPath(inputVideo string) string {
	dir := filepath.Dir(inputVideo)
	base := strings.TrimSuffix(filepath.Base(inputVideo), filepath.Ext(inputVideo))
	return filepath.Join(dir, base+"_compressed.mp4")
}

func partialCompressedPath(inputVideo string) string {
	dir := filepath.Dir(inputVideo)
	base := strings.TrimSuffix(filepath.Base(inputVideo), filepath.Ext(inputVideo))
	return filepath.Join(dir, base+"_compressed.partial.mp4")
}

func renameSegments(outDir string) error {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), tsSuffix) {
			continue
		}
		oldPath := filepath.Join(outDir, entry.Name())
		newPath := filepath.Join(outDir, strings.TrimSuffix(entry.Name(), tsSuffix)+marriedSuffix)
		if err := os.Rename(oldPath, newPath); err != nil {
			return err
		}
	}
	return nil
}

func rewritePlaylist(outDir string) error {
	src := filepath.Join(outDir, hlsOutputName+m3u8Suffix)
	dst := filepath.Join(outDir, marriedSingle)

	data, err := os.ReadFile(src)
	if err != nil {
		// No playlist file is not an error if the dir is empty / no HLS output
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read playlist %s: %w", src, err)
	}
	patched := strings.ReplaceAll(string(data), tsSuffix, marriedSuffix)
	if err := os.WriteFile(dst, []byte(patched), 0o644); err != nil {
		return fmt.Errorf("write playlist %s: %w", dst, err)
	}
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("remove source playlist %s: %w", src, err)
	}
	return nil
}

// partSuffix converts a 0-based index to a letter suffix: 0→"a", 1→"b", …
func partSuffix(i int) string {
	if i < 26 {
		return string(rune('a' + i))
	}
	return string(rune('a'+i/26-1)) + string(rune('a'+i%26))
}

func splitArgs(input, output string, start, dur time.Duration, isLast bool) []string {
	args := []string{
		"-ss", fmt.Sprintf("%.3f", start.Seconds()),
		"-i", input,
		"-c", "copy",
		"-avoid_negative_ts", "make_zero",
	}
	if !isLast {
		args = append(args, "-t", fmt.Sprintf("%.3f", dur.Seconds()))
	}
	return append(args, "-y", output)
}

// probeDuration queries ffprobe for the duration of videoPath. Used internally
// by Split to determine how many parts to cut.
func probeDuration(ctx context.Context, videoPath string) (time.Duration, error) {
	type probeResult struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	out, err := exec.CommandContext(ctx, deps.FFprobePath(),
		"-v", "quiet",
		"-print_format", "json",
		"-show_entries", "format=duration",
		videoPath,
	).Output()
	if err != nil {
		return 0, err
	}
	var r probeResult
	if err := json.Unmarshal(out, &r); err != nil {
		return 0, err
	}
	sec, err := strconv.ParseFloat(strings.TrimSpace(r.Format.Duration), 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(sec * float64(time.Second)), nil
}
