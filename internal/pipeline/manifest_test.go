package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// renderManifestEntry
// ---------------------------------------------------------------------------

func TestRenderManifestEntry_EmptyPattern_UsesLocalPath(t *testing.T) {
	hlsDir := "/workspace/x"
	got := renderManifestEntry("", "mybranch", hlsDir)
	want := filepath.Join(hlsDir, marriedSingle)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderManifestEntry_PlaceholdersReplaced(t *testing.T) {
	pattern := "https://raw.githubusercontent.com/org/repo/{branch}/{subdir}/{filename}"
	cases := []struct {
		branch string
		hlsDir string
		want   string
	}{
		{
			branch: "episode_3",
			hlsDir: "/ws/x",
			want:   "https://raw.githubusercontent.com/org/repo/episode_3/x/" + marriedSingle,
		},
		{
			branch: "episode_3a",
			hlsDir: "/ws/ep1/x",
			want:   "https://raw.githubusercontent.com/org/repo/episode_3a/x/" + marriedSingle,
		},
		{
			branch: "episode_3b",
			hlsDir: "/ws/ep2/x",
			want:   "https://raw.githubusercontent.com/org/repo/episode_3b/x/" + marriedSingle,
		},
	}
	for _, c := range cases {
		got := renderManifestEntry(pattern, c.branch, c.hlsDir)
		if got != c.want {
			t.Errorf("branch=%q hlsDir=%q:\n got  %q\n want %q", c.branch, c.hlsDir, got, c.want)
		}
	}
}

func TestRenderManifestEntry_NoBranchPlaceholder(t *testing.T) {
	// pattern without {branch} still works — {subdir} and {filename} replaced
	pattern := "https://cdn.example.com/{subdir}/{filename}"
	got := renderManifestEntry(pattern, "episode_1", "/ws/x")
	if strings.Contains(got, "{") {
		t.Errorf("unreplaced placeholder in output: %q", got)
	}
	if !strings.Contains(got, "x/"+marriedSingle) {
		t.Errorf("expected subdir/filename in output, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// discoverHLSDirs
// ---------------------------------------------------------------------------

func TestDiscoverHLSDirs_SingleEpisode(t *testing.T) {
	ws := t.TempDir()
	mustMkdir(t, filepath.Join(ws, "x"))

	dirs := discoverHLSDirs(ws)
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d: %v", len(dirs), dirs)
	}
	if dirs[0] != filepath.Join(ws, "x") {
		t.Errorf("got %q, want %q", dirs[0], filepath.Join(ws, "x"))
	}
}

func TestDiscoverHLSDirs_MultipleEpisodes(t *testing.T) {
	ws := t.TempDir()
	mustMkdir(t, filepath.Join(ws, "ep1", "x"))
	mustMkdir(t, filepath.Join(ws, "ep2", "x"))
	mustMkdir(t, filepath.Join(ws, "ep3", "x"))

	dirs := discoverHLSDirs(ws)
	if len(dirs) != 3 {
		t.Fatalf("expected 3 dirs, got %d: %v", len(dirs), dirs)
	}
	for _, d := range dirs {
		if !strings.HasSuffix(d, "/x") {
			t.Errorf("expected each dir to end in /x, got %q", d)
		}
	}
}

func TestDiscoverHLSDirs_FallbackOnEmpty(t *testing.T) {
	ws := t.TempDir()
	// no x/ or ep*/ subdirs at all
	dirs := discoverHLSDirs(ws)
	if len(dirs) != 1 || dirs[0] != filepath.Join(ws, "x") {
		t.Errorf("expected fallback to workspace/x, got %v", dirs)
	}
}

func TestDiscoverHLSDirs_MissingWorkspace(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "nonexistent")
	dirs := discoverHLSDirs(ws)
	if len(dirs) != 1 || dirs[0] != filepath.Join(ws, "x") {
		t.Errorf("expected fallback to workspace/x, got %v", dirs)
	}
}

// ---------------------------------------------------------------------------
// writeWorkspaceManifest / recordSuccess
// ---------------------------------------------------------------------------

func TestWriteWorkspaceManifest_WritesOneFilePerDir(t *testing.T) {
	ws := t.TempDir()
	ep1 := filepath.Join(ws, "ep1", "x")
	ep2 := filepath.Join(ws, "ep2", "x")
	mustMkdir(t, ep1)
	mustMkdir(t, ep2)

	cfg := &Config{PublicURLPattern: "https://raw.githubusercontent.com/org/repo/{branch}/{subdir}/{filename}"}
	w := &manifestWriter{}
	w.writeWorkspaceManifest(cfg, "episode_3", []string{ep1, ep2}, "episode_3", nil)

	for i, dir := range []string{ep1, ep2} {
		data, err := os.ReadFile(filepath.Join(dir, manifestFilename))
		if err != nil {
			t.Fatalf("ep%d urls.txt missing: %v", i+1, err)
		}
		line := strings.TrimSpace(string(data))
		if !strings.Contains(line, "episode_3") {
			t.Errorf("ep%d urls.txt missing branch: %q", i+1, line)
		}
		if !strings.Contains(line, marriedSingle) {
			t.Errorf("ep%d urls.txt missing filename: %q", i+1, line)
		}
	}
}

func TestRecordSuccess_AppendsOneLinePerDir(t *testing.T) {
	sourceDir := t.TempDir()
	source := filepath.Join(sourceDir, "episode_3.mp4")
	mustWriteBytes(t, source, 1)

	hlsDirs := []string{"/fake/ep1/x", "/fake/ep2/x", "/fake/ep3/x"}
	cfg := &Config{PublicURLPattern: "https://raw.githubusercontent.com/org/repo/{branch}/{subdir}/{filename}"}
	w := &manifestWriter{}
	w.recordSuccess(cfg, source, "episode_3", hlsDirs, "episode_3", nil)

	data, err := os.ReadFile(filepath.Join(sourceDir, manifestFilename))
	if err != nil {
		t.Fatalf("urls.txt not written: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines (one per episode), got %d:\n%s", len(lines), string(data))
	}
	for i, line := range lines {
		if !strings.Contains(line, "episode_3") {
			t.Errorf("line %d missing branch: %q", i, line)
		}
	}
}

func TestRecordSuccess_EmptyDirs_WritesNothing(t *testing.T) {
	sourceDir := t.TempDir()
	source := filepath.Join(sourceDir, "v.mp4")
	mustWriteBytes(t, source, 1)

	cfg := &Config{}
	w := &manifestWriter{}
	w.recordSuccess(cfg, source, "v", nil, "v", nil)

	if _, err := os.Stat(filepath.Join(sourceDir, manifestFilename)); !os.IsNotExist(err) {
		t.Error("urls.txt should not be created when hlsDirs is empty")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
