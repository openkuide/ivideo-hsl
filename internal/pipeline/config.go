package pipeline

import (
	"os"
	"path/filepath"
)

// InputDirName is the conventional subdirectory where users drop source
// videos before running. Gitignored in this project. DefaultSourceDir
// prefers it when it exists.
const InputDirName = "input"

// DefaultSourceDir returns <scriptDir>/input when that directory exists,
// otherwise scriptDir itself. This lets the zero-config workflow
// (`mkdir input && mv *.mp4 input/ && ivideo-hls`) work without the user
// setting anything, while preserving the legacy "run from a folder of
// .mp4s" flow when no input/ subdirectory exists.
func DefaultSourceDir(scriptDir string) string {
	candidate := filepath.Join(scriptDir, InputDirName)
	info, err := os.Stat(candidate)
	if err == nil && info.IsDir() {
		return candidate
	}
	return scriptDir
}

type Quality string

const (
	QualityLow    Quality = "low"
	QualityMedium Quality = "medium"
	QualityHigh   Quality = "high"
)

type Compression string

const (
	CompressionFast     Compression = "fast"
	CompressionBalanced Compression = "balanced"
	CompressionBest     Compression = "best"
)

type Config struct {
	Videos       []string
	ParallelMode bool
	MaxParallel  int
	Quality      Quality
	Compression  Compression
	PreCompress  bool
	AutoMode     bool
	KeepSource   bool // when true, skip deleting the original .mp4 on success
	Push         bool // when false, commit locally but skip git push
	Cleanup      bool // when false, keep hero_*/ workspace after success
	SourceDir    string
	Recursive    bool // when true, ScanVideos walks subdirectories
	// ResumeReuseCompressed lets resume-failed skip the compress stage when
	// a clean _compressed.mp4 sibling is present (see CompressedReusable).
	// Default false — fully redo every stage, always correct.
	ResumeReuseCompressed bool

	ScriptDir   string
	BaseHeroDir string

	// RemoteURL is the user-facing URL safe to log. It's what gets written to
	// `git remote` and shown in the TUI.
	RemoteURL string
	// PushURL is the credential-bearing URL used when actually pushing. When
	// empty, RemoteURL is used directly. Never log this.
	PushURL string
	// PublicURLPattern is the template used to render urls.txt entries.
	// Placeholders: {branch}, {subdir}, {filename}. Empty = write local path instead.
	PublicURLPattern string
}

// EffectivePushURL returns PushURL when set, else RemoteURL. Always use this
// at the push boundary; never log its return value.
func (c *Config) EffectivePushURL() string {
	if c.PushURL != "" {
		return c.PushURL
	}
	return c.RemoteURL
}

func NewConfig(scriptDir string) *Config {
	return &Config{
		MaxParallel: 1,
		Quality:     QualityMedium,
		Compression: CompressionBalanced,
		Push:        true,
		Cleanup:     true,
		SourceDir:   DefaultSourceDir(scriptDir),
		ScriptDir:   scriptDir,
		BaseHeroDir: filepath.Join(scriptDir, "hero"),
		RemoteURL:   "git@github.com:username/repo.git",
	}
}

func ValidQuality(q string) bool {
	switch Quality(q) {
	case QualityLow, QualityMedium, QualityHigh:
		return true
	}
	return false
}

func ValidCompression(c string) bool {
	switch Compression(c) {
	case CompressionFast, CompressionBalanced, CompressionBest:
		return true
	}
	return false
}
