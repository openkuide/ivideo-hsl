package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// EncodingService orchestrates the full encode pipeline:
// workspace setup → optional pre-compress → split decision → HLS conversion → rename outputs.
type EncodingService struct {
	encoder  ports.Encoder
	prober   ports.Prober
	splitter ports.Splitter
	ws       ports.Workspace
}

// NewEncodingService constructs an EncodingService wired with the given port adapters.
func NewEncodingService(enc ports.Encoder, prober ports.Prober, split ports.Splitter, ws ports.Workspace) *EncodingService {
	return &EncodingService{encoder: enc, prober: prober, splitter: split, ws: ws}
}

// Process runs the full pipeline for one video:
//  1. Workspace setup
//  2. Optional pre-compress (cfg.PreCompress)
//  3. Split decision (duration > 30 min → split; else single episode)
//  4. ConvertToHLS + RenameHLSOutputs for each episode
//
// Returns the workspace directory and one HLS output directory per episode.
// The caller is responsible for calling CleanupWorkspace after publishing.
func (s *EncodingService) Process(ctx context.Context, v video.Video, cfg settings.Settings, jobID string, e job.Emitter) (wsDir string, hlsDirs []string, err error) {
	// Step 1: workspace setup
	workspaceDir, err := s.ws.Setup(ctx, v, cfg, jobID, e)
	if err != nil {
		return "", nil, fmt.Errorf("workspace setup: %w", err)
	}

	// From here on, always return workspaceDir so the caller can clean up even on error.

	// Step 2: optional pre-compress
	inputPath := v.Path
	if cfg.PreCompress {
		// Reuse existing compressed file when the setting is on and the file is present.
		if cfg.ResumeReuseCompressed {
			if cp := precompressedPath(v.Path); fileExists(cp) {
				job.Emit(e, job.LevelInfo, jobID, job.StageCompress, "reusing "+filepath.Base(cp))
				inputPath = cp
			}
		}
		if inputPath == v.Path {
			compressed, err2 := s.encoder.Compress(ctx, v, jobID, e)
			if err2 != nil {
				return workspaceDir, nil, fmt.Errorf("pre-compress: %w", err2)
			}
			inputPath = compressed
		}
	}

	// Step 3: split decision
	episodes, err2 := s.splitter.Split(ctx, inputPath, jobID, e)
	if err2 != nil {
		return workspaceDir, nil, fmt.Errorf("split: %w", err2)
	}

	// Step 4: convert each episode to HLS
	hlsDirs = make([]string, 0, len(episodes))
	for _, ep := range episodes {
		var outDir string
		if ep.Suffix == "" {
			outDir = filepath.Join(workspaceDir, "x")
		} else {
			outDir = filepath.Join(workspaceDir, "ep"+ep.Suffix, "x")
		}

		epJobID := jobID + ep.Suffix

		if err2 := s.encoder.ConvertToHLS(ctx, ep.Path, outDir, cfg, epJobID, e); err2 != nil {
			return workspaceDir, nil, fmt.Errorf("convert episode %q: %w", ep.Suffix, err2)
		}
		if err2 := s.encoder.RenameHLSOutputs(outDir, epJobID, e); err2 != nil {
			return workspaceDir, nil, fmt.Errorf("rename episode %q: %w", ep.Suffix, err2)
		}
		hlsDirs = append(hlsDirs, outDir)
	}

	return workspaceDir, hlsDirs, nil
}

// CleanupWorkspace removes the workspace directory when cfg.Cleanup is true.
func (s *EncodingService) CleanupWorkspace(wsDir string, cfg settings.Settings, e job.Emitter, jobID string) {
	if cfg.Cleanup {
		s.ws.Cleanup(wsDir, e, jobID)
	}
}

// precompressedPath returns the path where Compress() writes its output for videoPath.
func precompressedPath(videoPath string) string {
	dir := filepath.Dir(videoPath)
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	return filepath.Join(dir, base+"_compressed.mp4")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
