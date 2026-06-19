package ffmpeg_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/ffmpeg"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// TestAdapter_SatisfiesPorts is a compile-time proof that *Adapter satisfies
// both ports.Encoder and ports.Splitter.
func TestAdapter_SatisfiesPorts(t *testing.T) {
	var _ ports.Encoder = ffmpeg.New()
	var _ ports.Splitter = ffmpeg.New()
}

// TestAdapter_RenameHLSOutputs_EmptyDir verifies that RenameHLSOutputs does
// not return an error when called on a directory that contains no HLS output
// files (.ts / .m3u8). This is a pure-filesystem test — no ffmpeg binary is
// invoked.
func TestAdapter_RenameHLSOutputs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	a := ffmpeg.New()
	if err := a.RenameHLSOutputs(dir, "test-job", nil); err != nil {
		t.Errorf("RenameHLSOutputs on empty dir: %v", err)
	}
}

// TestAdapter_RenameHLSOutputs_RenamesSegments verifies that .ts files are
// renamed to .married and that an .m3u8 playlist is rewritten and renamed to
// .single with its internal .ts references updated.
func TestAdapter_RenameHLSOutputs_RenamesSegments(t *testing.T) {
	dir := t.TempDir()

	// Create fake .ts segments
	for _, name := range []string{"index_000.ts", "index_001.ts", "index_002.ts"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("ts-data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Create a fake .m3u8 playlist referencing .ts files
	playlist := "#EXTM3U\n#EXT-X-VERSION:3\n" +
		"index_000.ts\nindex_001.ts\nindex_002.ts\n" +
		"#EXT-X-ENDLIST\n"
	if err := os.WriteFile(filepath.Join(dir, "index.m3u8"), []byte(playlist), 0o644); err != nil {
		t.Fatal(err)
	}

	a := ffmpeg.New()
	if err := a.RenameHLSOutputs(dir, "test-job", nil); err != nil {
		t.Fatalf("RenameHLSOutputs: %v", err)
	}

	// .ts segments should be gone, .married files should exist
	for _, name := range []string{"index_000.married", "index_001.married", "index_002.married"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}
	// original .ts files must be absent
	for _, name := range []string{"index_000.ts", "index_001.ts", "index_002.ts"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", name)
		}
	}
	// .m3u8 → .single
	singlePath := filepath.Join(dir, "index.single")
	data, err := os.ReadFile(singlePath)
	if err != nil {
		t.Fatalf("index.single not found: %v", err)
	}
	content := string(data)
	if !contains(content, "index_000.married") {
		t.Errorf("index.single does not reference .married files:\n%s", content)
	}
	if contains(content, ".ts") {
		t.Errorf("index.single still references .ts files:\n%s", content)
	}
	// original .m3u8 must be absent
	if _, err := os.Stat(filepath.Join(dir, "index.m3u8")); !os.IsNotExist(err) {
		t.Error("expected index.m3u8 to be removed")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
