package jsonconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/jsonconfig"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

func TestAdapter_SaveLoad_Roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	a := jsonconfig.New(path)

	original := settings.Settings{
		RemoteURL:   "https://github.com/org/repo.git",
		AuthMethod:  settings.AuthHTTPS,
		Token:       "ghp_test",
		Quality:     video.QualityHigh,
		Compression: video.CompressionBest,
		MaxParallel: 3,
		ParallelMode: true,  // derived from MaxParallel > 1
		Push:        true,
		Cleanup:     true,
	}
	if err := a.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := a.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != original {
		t.Errorf("round-trip mismatch:\n got  %+v\nwant %+v", got, original)
	}
}

func TestAdapter_Load_MissingFile_ReturnsZero(t *testing.T) {
	a := jsonconfig.New(filepath.Join(t.TempDir(), "setting.json"))
	got, err := a.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if got != (settings.Settings{}) {
		t.Errorf("expected zero Settings, got %+v", got)
	}
}

func TestAdapter_SatisfiesPort(t *testing.T) {
	var _ interface {
		Load() (settings.Settings, error)
		Save(settings.Settings) error
	} = jsonconfig.New("")
}

func TestAdapter_Save_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "setting.json")
	a := jsonconfig.New(path)

	original := settings.Settings{
		RemoteURL:  "https://github.com/org/repo.git",
		AuthMethod: settings.AuthHTTPS,
		Token:      "ghp_test",
	}
	if err := a.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// After Save, there should be no .tmp file left behind
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); err == nil {
		t.Errorf("expected .tmp file to be cleaned up, but it exists")
	}

	// Check that the final file exists and is readable
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected setting.json to exist: %v", err)
	}

	// Check file permissions are 0600
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode()&0o777 != 0o600 {
		t.Errorf("expected file mode 0600, got %o", info.Mode()&0o777)
	}
}

func TestAdapter_Save_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "setting.json")
	a := jsonconfig.New(path)

	cfg := settings.Settings{
		RemoteURL:  "https://github.com/org/repo.git",
		AuthMethod: settings.AuthHTTPS,
	}
	if err := a.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load it back to verify it was written
	got, err := a.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.RemoteURL != cfg.RemoteURL {
		t.Errorf("RemoteURL mismatch: got %q, want %q", got.RemoteURL, cfg.RemoteURL)
	}
}

