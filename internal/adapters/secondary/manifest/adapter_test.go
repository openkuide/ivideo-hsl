package manifest_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/manifest"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// TestAdapter_SatisfiesPorts is a compile-time proof that *Adapter satisfies
// ports.ManifestWriter.
func TestAdapter_SatisfiesPorts(t *testing.T) {
	var _ ports.ManifestWriter = manifest.New("")
}

// TestAdapter_Record_EmptyDirs_NoOp verifies that Record with no HLS dirs
// does not write any file and returns nil.
func TestAdapter_Record_EmptyDirs_NoOp(t *testing.T) {
	src := t.TempDir()
	sourceFile := filepath.Join(src, "v.mp4")
	os.WriteFile(sourceFile, []byte("x"), 0o644)

	a := manifest.New("")
	err := a.Record(context.Background(), sourceFile, "main", nil, "test-job", nil)
	if err != nil {
		t.Fatalf("Record(empty): %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(src, "urls.json")); !os.IsNotExist(statErr) {
		t.Error("urls.json must not be written when hlsDirs is empty")
	}
}

// TestAdapter_Record_WritesJSON verifies that Record creates urls.json next
// to the source file and that it contains exactly one entry.
func TestAdapter_Record_WritesJSON(t *testing.T) {
	src := t.TempDir()
	sourceFile := filepath.Join(src, "v.mp4")
	os.WriteFile(sourceFile, []byte("x"), 0o644)

	a := manifest.New("https://raw.githubusercontent.com/org/repo/{branch}/{subdir}/{filename}")
	err := a.Record(context.Background(), sourceFile, "mybranch", []string{"/ws/x"}, "v", nil)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(src, "urls.json"))
	if err != nil {
		t.Fatalf("urls.json not written: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(m) != 1 {
		t.Errorf("want 1 key, got %d: %v", len(m), m)
	}
}

// TestAdapter_Record_MergesExisting verifies that a second call to Record
// appends to the existing urls.json rather than overwriting it.
func TestAdapter_Record_MergesExisting(t *testing.T) {
	src := t.TempDir()
	sourceFile := filepath.Join(src, "v.mp4")
	os.WriteFile(sourceFile, []byte("x"), 0o644)

	a := manifest.New("https://raw.githubusercontent.com/org/repo/{branch}/{subdir}/{filename}")
	a.Record(context.Background(), sourceFile, "branch1", []string{"/ws/x"}, "v", nil)
	a.Record(context.Background(), sourceFile, "branch2", []string{"/ws/x"}, "v", nil)

	data, _ := os.ReadFile(filepath.Join(src, "urls.json"))
	var m map[string]string
	json.Unmarshal(data, &m)
	if len(m) != 2 {
		t.Errorf("want 2 keys after merge, got %d: %v", len(m), m)
	}
}
