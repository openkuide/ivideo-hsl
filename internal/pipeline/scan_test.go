package pipeline

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestIsVideoFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"lesson.mp4", true},
		{"LESSON.MP4", true},
		{"movie.MOV", true},
		{"clip.m4v", true},
		{"recording.3gp", true},
		{"screencast.webm", true},
		{"index.m3u8", false},
		{"README.md", false},
		{"no-extension", false},
		{".DS_Store", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsVideoFile(tc.name); got != tc.want {
				t.Errorf("IsVideoFile(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestScanVideos_Flat(t *testing.T) {
	root := t.TempDir()
	mustWriteFiles(t, root, map[string]string{
		"a.mp4":             "",
		"b.MOV":             "",
		"README.md":         "",
		"sub/nested.mp4":    "",
		".DS_Store":         "",
		"node_modules/x.mp4": "",
	})

	got, err := ScanVideos(root, false)
	if err != nil {
		t.Fatalf("ScanVideos: %v", err)
	}
	want := []string{
		filepath.Join(root, "a.mp4"),
		filepath.Join(root, "b.MOV"),
	}
	sort.Strings(got)
	sort.Strings(want)
	assertEqualStrings(t, got, want)
}

func TestScanVideos_Recursive_PrunesIgnoredDirs(t *testing.T) {
	root := t.TempDir()
	mustWriteFiles(t, root, map[string]string{
		"top.mp4":                      "",
		"sub/nested.mov":               "",
		"sub/deeper/still.mp4":         "",
		".hidden/secret.mp4":           "",
		".git/objects/pack.mp4":        "",
		"node_modules/lib/clip.mp4":    "",
		"hero/base.mp4":                "",
		"hero_workspace/produced.mp4":  "",
		"docs/README.md":               "",
	})

	got, err := ScanVideos(root, true)
	if err != nil {
		t.Fatalf("ScanVideos: %v", err)
	}
	want := []string{
		filepath.Join(root, "top.mp4"),
		filepath.Join(root, "sub/nested.mov"),
		filepath.Join(root, "sub/deeper/still.mp4"),
	}
	sort.Strings(got)
	sort.Strings(want)
	assertEqualStrings(t, got, want)
}

func TestScanVideos_NonexistentRoot(t *testing.T) {
	_, err := ScanVideos(filepath.Join(t.TempDir(), "missing"), false)
	if err == nil {
		t.Fatal("expected error for missing root, got nil")
	}
}

func TestIsIgnoredDir(t *testing.T) {
	cases := map[string]bool{
		".git":         true,
		".hidden":      true,
		"node_modules": true,
		"hero":         true,
		"hero_foo":     true,
		"hero_":        true,
		"src":          false,
		"lessons":      false,
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isIgnoredDir(name); got != want {
				t.Errorf("isIgnoredDir(%q) = %v, want %v", name, got, want)
			}
		})
	}
}

func mustWriteFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

func assertEqualStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %d, want %d\n got: %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
