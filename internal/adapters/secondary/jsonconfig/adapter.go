package jsonconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

var _ ports.ConfigStore = (*Adapter)(nil)

type Adapter struct {
	path string
}

func New(path string) *Adapter {
	return &Adapter{path: path}
}

type jsonFile struct {
	RemoteURL             string `json:"remote_url"`
	AuthMethod            string `json:"auth_method"`
	Token                 string `json:"token"`
	DefaultQuality        string `json:"default_quality"`
	DefaultCompression    string `json:"default_compression"`
	DefaultPreCompress    bool   `json:"default_pre_compress"`
	DefaultKeepSource     bool   `json:"default_keep_source"`
	DefaultSourceDir      string `json:"default_source_dir"`
	DefaultRecursive      bool   `json:"default_recursive"`
	DefaultPushDisabled   bool   `json:"default_push_disabled"`
	DefaultCleanupDisabled bool  `json:"default_cleanup_disabled"`
	DefaultParallel       int    `json:"default_parallel"`
	ResumeReuseCompressed bool   `json:"resume_reuse_compressed"`
	PublicURLPattern      string `json:"public_url_pattern"`
}

func (a *Adapter) Load() (settings.Settings, error) {
	data, err := os.ReadFile(a.path)
	if errors.Is(err, os.ErrNotExist) {
		return settings.Settings{}, nil
	}
	if err != nil {
		return settings.Settings{}, fmt.Errorf("read %s: %w", a.path, err)
	}
	var f jsonFile
	if err := json.Unmarshal(data, &f); err != nil {
		return settings.Settings{}, fmt.Errorf("parse %s: %w", a.path, err)
	}
	return settings.Settings{
		RemoteURL:             f.RemoteURL,
		AuthMethod:            settings.AuthMethod(f.AuthMethod),
		Token:                 f.Token,
		Quality:               video.Quality(f.DefaultQuality),
		Compression:           video.Compression(f.DefaultCompression),
		PreCompress:           f.DefaultPreCompress,
		KeepSource:            f.DefaultKeepSource,
		SourceDir:             f.DefaultSourceDir,
		Recursive:             f.DefaultRecursive,
		Push:                  !f.DefaultPushDisabled,
		Cleanup:               !f.DefaultCleanupDisabled,
		MaxParallel:           f.DefaultParallel,
		ParallelMode:          f.DefaultParallel > 1,
		ResumeReuseCompressed: f.ResumeReuseCompressed,
		PublicURLPattern:      f.PublicURLPattern,
	}, nil
}

func (a *Adapter) Save(s settings.Settings) error {
	if err := os.MkdirAll(filepath.Dir(a.path), 0o700); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	f := jsonFile{
		RemoteURL:             s.RemoteURL,
		AuthMethod:            string(s.AuthMethod),
		Token:                 s.Token,
		DefaultQuality:        string(s.Quality),
		DefaultCompression:    string(s.Compression),
		DefaultPreCompress:    s.PreCompress,
		DefaultKeepSource:     s.KeepSource,
		DefaultSourceDir:      s.SourceDir,
		DefaultRecursive:      s.Recursive,
		DefaultPushDisabled:   !s.Push,
		DefaultCleanupDisabled: !s.Cleanup,
		DefaultParallel:       s.MaxParallel,
		ResumeReuseCompressed: s.ResumeReuseCompressed,
		PublicURLPattern:      s.PublicURLPattern,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')
	tmp := a.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, a.path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
