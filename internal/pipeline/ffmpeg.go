package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chamrong/ivideo-hls/internal/deps"
)

// compressVideo runs the optional pre-compression pass and returns the path to
// the finished compressed file. Integrity discipline: ffmpeg writes to a
// <name>_compressed.partial.mp4 sibling; the file is renamed to its final
// name only after ffmpeg exits 0. A crash anywhere in the middle leaves a
// .partial file on disk — cheap for the operator or `doctor` to spot.
//
// The caller is responsible for cleaning the final file up when the job as a
// whole succeeds.
func compressVideo(ctx context.Context, inputVideo, job string, e Emitter) (string, error) {
	info(e, job, StageCompress, "compressing file size…")

	finalPath := compressedOutputPath(inputVideo)
	partialPath := partialCompressedPath(inputVideo)
	total := probeDuration(ctx, inputVideo)
	reporter := newProgressReporter(total, func(pct, speed float64, bitrate string) {
		emitProgress(e, job, StageCompress, pct, "compressing", speed, bitrate)
	})

	args := compressArgs(inputVideo, partialPath)
	opts := runOpts{
		job:      job,
		stage:    StageCompress,
		emitter:  e,
		silent:   true,
		onStdout: reporter.consume,
	}
	if err := run(ctx, opts, deps.FFmpegPath(), args...); err != nil {
		// leave the .partial in place so diagnostics (doctor, resume-failed)
		// can see the stage was killed rather than finished unclean
		return "", fmt.Errorf("ffmpeg compress: %w", err)
	}
	if err := os.Rename(partialPath, finalPath); err != nil {
		return "", fmt.Errorf("finalize compressed output: %w", err)
	}

	reportCompressionRatio(e, job, inputVideo, finalPath)
	return finalPath, nil
}

func compressedOutputPath(inputVideo string) string {
	dir := filepath.Dir(inputVideo)
	base := strings.TrimSuffix(filepath.Base(inputVideo), filepath.Ext(inputVideo))
	return filepath.Join(dir, base+"_compressed.mp4")
}

// partialCompressedPath is the sibling ffmpeg writes to before the atomic
// rename. Its presence means "last compress was killed mid-flight."
func partialCompressedPath(inputVideo string) string {
	dir := filepath.Dir(inputVideo)
	base := strings.TrimSuffix(filepath.Base(inputVideo), filepath.Ext(inputVideo))
	return filepath.Join(dir, base+"_compressed.partial.mp4")
}

func compressArgs(input, out string) []string {
	return []string{
		"-i", input,
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
		"-y", out,
	}
}

func reportCompressionRatio(e Emitter, job, orig, compressed string) {
	origStat, err1 := os.Stat(orig)
	newStat, err2 := os.Stat(compressed)
	if err1 != nil || err2 != nil {
		return
	}
	origMB := float64(origStat.Size()) / 1024 / 1024
	newMB := float64(newStat.Size()) / 1024 / 1024
	reduction := (1.0 - float64(newStat.Size())/float64(origStat.Size())) * 100
	success(e, job, StageCompress, fmt.Sprintf(
		"compressed %.2fMB → %.2fMB (-%.1f%%)", origMB, newMB, reduction))
}

// hlsSettings is the set of encoder knobs derived from the user's quality and
// compression choices.
type hlsSettings struct {
	preset       string
	crf          string
	videoBitrate string
	audioBitrate string
	resolution   string
	bufsize      string
}

func settingsFor(cfg *Config) hlsSettings {
	s := hlsSettings{
		preset:       "medium",
		crf:          "23",
		videoBitrate: "2800k",
		audioBitrate: "128k",
		resolution:   "-2:720",
		bufsize:      "5600k",
	}
	switch cfg.Compression {
	case CompressionBest:
		s.preset, s.crf = "slow", "26"
	case CompressionFast:
		s.preset, s.crf = "fast", "23"
	}
	switch cfg.Quality {
	case QualityLow:
		s.videoBitrate, s.audioBitrate, s.resolution, s.bufsize = "800k", "96k", "-2:480", "1600k"
	case QualityHigh:
		s.videoBitrate, s.audioBitrate, s.resolution, s.bufsize = "5000k", "192k", "-2:1080", "10000k"
	}
	return s
}

const (
	hlsOutputName = "index"

	// marriedSuffix and singleSuffix are the downstream-required file
	// extensions. .ts → .married and .m3u8 → .single is an invariant the
	// downstream player stack depends on (see ARCHITECTURE.md). Changing them
	// requires a paired downstream update.
	tsSuffix       = ".ts"
	marriedSuffix  = ".married"
	m3u8Suffix     = ".m3u8"
	singleSuffix   = ".single"
	marriedSingle  = hlsOutputName + singleSuffix
)

func convertToHLS(ctx context.Context, inputVideo, outputDir string, cfg *Config, job string, e Emitter) error {
	videoOutputDir := filepath.Join(outputDir, "x")
	if err := os.MkdirAll(videoOutputDir, 0o755); err != nil {
		return fmt.Errorf("create hls output dir: %w", err)
	}

	settings := settingsFor(cfg)
	info(e, job, StageConvert, fmt.Sprintf("HLS convert @ %s / %s", cfg.Quality, cfg.Compression))

	total := probeDuration(ctx, inputVideo)
	reporter := newProgressReporter(total, func(pct, speed float64, bitrate string) {
		emitProgress(e, job, StageConvert, pct, "encoding", speed, bitrate)
	})

	args := hlsArgs(inputVideo, videoOutputDir, settings)
	opts := runOpts{
		job:      job,
		stage:    StageConvert,
		emitter:  e,
		silent:   true,
		onStdout: reporter.consume,
	}
	if err := run(ctx, opts, deps.FFmpegPath(), args...); err != nil {
		return fmt.Errorf("ffmpeg convert: %w", err)
	}

	if err := renameHLSOutputs(videoOutputDir, job, e); err != nil {
		return err
	}
	success(e, job, StageConvert, "conversion complete")
	return nil
}

func hlsArgs(input, outDir string, s hlsSettings) []string {
	return []string{
		"-i", input,
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
		"-hls_segment_filename", filepath.Join(outDir, hlsOutputName+"_%03d"+tsSuffix),
		filepath.Join(outDir, hlsOutputName+m3u8Suffix),
	}
}

// renameHLSOutputs applies the downstream-required rename:
// `.ts → .married`, `.m3u8 → .single` (with in-file refs rewritten).
// This step is an invariant — see ARCHITECTURE.md.
func renameHLSOutputs(outDir, job string, e Emitter) error {
	info(e, job, StageRename, "renaming "+tsSuffix+" → "+marriedSuffix+", "+m3u8Suffix+" → "+singleSuffix)

	if err := renameSegments(outDir); err != nil {
		return fmt.Errorf("rename segments: %w", err)
	}
	if err := rewritePlaylist(outDir); err != nil {
		return fmt.Errorf("rewrite playlist: %w", err)
	}
	return nil
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
