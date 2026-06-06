package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasPartialSibling(t *testing.T) {
	dir := t.TempDir()
	compressed := filepath.Join(dir, "lesson_compressed.mp4")
	partial := filepath.Join(dir, "lesson_compressed.partial.mp4")

	// No partial → false.
	mustWriteBytes(t, compressed, 2048)
	if hasPartialSibling(compressed) {
		t.Fatal("no partial exists, but hasPartialSibling returned true")
	}

	// Partial present → true.
	mustWriteBytes(t, partial, 1024)
	if !hasPartialSibling(compressed) {
		t.Fatal("partial exists, but hasPartialSibling returned false")
	}

	// Different base name → false (partial belongs to a different video).
	other := filepath.Join(dir, "other_compressed.mp4")
	mustWriteBytes(t, other, 2048)
	if hasPartialSibling(other) {
		t.Fatal("partial belongs to a different base, should not match")
	}

	// Path that doesn't end in _compressed.mp4 → false.
	weird := filepath.Join(dir, "not-a-compressed-output.mp4")
	mustWriteBytes(t, weird, 2048)
	if hasPartialSibling(weird) {
		t.Fatal("non-compressed path should not be considered")
	}
}

func TestCompressedReusable(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name  string
		setup func(t *testing.T) string
		want  bool
	}{
		{
			name: "empty path",
			setup: func(t *testing.T) string {
				return ""
			},
			want: false,
		},
		{
			name: "missing file",
			setup: func(t *testing.T) string {
				return filepath.Join(dir, "missing_compressed.mp4")
			},
			want: false,
		},
		{
			name: "tiny file below 1KiB",
			setup: func(t *testing.T) string {
				p := filepath.Join(dir, "tiny_compressed.mp4")
				mustWriteBytes(t, p, 128)
				return p
			},
			want: false,
		},
		{
			name: "sibling partial present — must not reuse",
			setup: func(t *testing.T) string {
				p := filepath.Join(dir, "interrupted_compressed.mp4")
				mustWriteBytes(t, p, 4096)
				mustWriteBytes(t, filepath.Join(dir, "interrupted_compressed.partial.mp4"), 1024)
				return p
			},
			want: false,
		},
		// Duration check can't be exercised without a real ffprobe + real
		// video, so any "ok" path stops at the preconditions. The end-to-end
		// duration gate is covered in resume-failed smoke tests.
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := c.setup(t)
			if got := CompressedReusable(t.Context(), path); got != c.want {
				t.Errorf("CompressedReusable(%q) = %v, want %v", path, got, c.want)
			}
		})
	}
}

func mustWriteBytes(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}
