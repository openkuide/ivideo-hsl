package app

import (
	"context"
	"fmt"
	"path/filepath"

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
//  5. Workspace cleanup (deferred)
//
// Returns one HLS output directory per episode.
func (s *EncodingService) Process(ctx context.Context, v video.Video, cfg settings.Settings, jobID string, e job.Emitter) ([]string, error) {
	// Step 1: workspace setup
	workspaceDir, err := s.ws.Setup(ctx, v, cfg, jobID, e)
	if err != nil {
		return nil, fmt.Errorf("workspace setup: %w", err)
	}
	defer s.ws.Cleanup(workspaceDir, e, jobID)

	// Step 2: optional pre-compress
	inputPath := v.Path
	if cfg.PreCompress {
		compressed, err := s.encoder.Compress(ctx, v, jobID, e)
		if err != nil {
			return nil, fmt.Errorf("pre-compress: %w", err)
		}
		inputPath = compressed
	}

	// Step 3: split decision
	episodes, err := s.splitter.Split(ctx, inputPath, jobID, e)
	if err != nil {
		return nil, fmt.Errorf("split: %w", err)
	}

	// Step 4: convert each episode to HLS
	hlsDirs := make([]string, 0, len(episodes))
	for _, ep := range episodes {
		var outDir string
		if ep.Suffix == "" {
			outDir = filepath.Join(workspaceDir, "x")
		} else {
			outDir = filepath.Join(workspaceDir, "ep"+ep.Suffix, "x")
		}

		epJobID := jobID + ep.Suffix

		if err := s.encoder.ConvertToHLS(ctx, ep.Path, outDir, cfg, epJobID, e); err != nil {
			return nil, fmt.Errorf("convert episode %q: %w", ep.Suffix, err)
		}
		if err := s.encoder.RenameHLSOutputs(outDir, epJobID, e); err != nil {
			return nil, fmt.Errorf("rename episode %q: %w", ep.Suffix, err)
		}
		hlsDirs = append(hlsDirs, outDir)
	}

	return hlsDirs, nil
}
