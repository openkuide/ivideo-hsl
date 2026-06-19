package app

import (
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// ConfigService wraps ports.ConfigStore and provides settings merge logic.
type ConfigService struct {
	store ports.ConfigStore
}

// NewConfigService constructs a ConfigService backed by the given store.
func NewConfigService(store ports.ConfigStore) *ConfigService {
	return &ConfigService{store: store}
}

// Load delegates to the underlying ConfigStore.
func (s *ConfigService) Load() (settings.Settings, error) {
	return s.store.Load()
}

// Save delegates to the underlying ConfigStore.
func (s *ConfigService) Save(cfg settings.Settings) error {
	return s.store.Save(cfg)
}

// Merge copies base and overlays non-zero fields from overrides onto it.
// "Non-zero" means the field is not the Go zero value for its type.
func (s *ConfigService) Merge(base settings.Settings, overrides settings.Settings) settings.Settings {
	out := base

	if overrides.RemoteURL != "" {
		out.RemoteURL = overrides.RemoteURL
	}
	if overrides.PushURL != "" {
		out.PushURL = overrides.PushURL
	}
	if overrides.AuthMethod != "" {
		out.AuthMethod = overrides.AuthMethod
	}
	if overrides.Token != "" {
		out.Token = overrides.Token
	}
	if overrides.PublicURLPattern != "" {
		out.PublicURLPattern = overrides.PublicURLPattern
	}
	if overrides.Quality != "" {
		out.Quality = overrides.Quality
	}
	if overrides.Compression != "" {
		out.Compression = overrides.Compression
	}
	if overrides.PreCompress {
		out.PreCompress = overrides.PreCompress
	}
	if overrides.MaxParallel != 0 {
		out.MaxParallel = overrides.MaxParallel
	}
	if overrides.ParallelMode {
		out.ParallelMode = overrides.ParallelMode
	}
	if overrides.Push {
		out.Push = overrides.Push
	}
	if overrides.Cleanup {
		out.Cleanup = overrides.Cleanup
	}
	if overrides.KeepSource {
		out.KeepSource = overrides.KeepSource
	}
	if overrides.SourceDir != "" {
		out.SourceDir = overrides.SourceDir
	}
	if overrides.Recursive {
		out.Recursive = overrides.Recursive
	}
	if overrides.ScriptDir != "" {
		out.ScriptDir = overrides.ScriptDir
	}
	if overrides.ResumeReuseCompressed {
		out.ResumeReuseCompressed = overrides.ResumeReuseCompressed
	}

	return out
}

// SaveRunConfig is a convenience method to partially update persisted settings.
func (s *ConfigService) SaveRunConfig(quality video.Quality, compression video.Compression, parallel int, preCompress, keepSource bool) error {
	current, err := s.store.Load()
	if err != nil {
		return err
	}
	if quality != "" {
		current.Quality = quality
	}
	if compression != "" {
		current.Compression = compression
	}
	if parallel >= 1 {
		current.MaxParallel = parallel
		current.ParallelMode = parallel > 1
	}
	current.PreCompress = preCompress
	current.KeepSource = keepSource
	return s.store.Save(current)
}
