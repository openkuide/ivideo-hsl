package video_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

func TestNewVideo_DerivesNameAndBranch(t *testing.T) {
	v := video.NewVideo("/src/my episode 1.mp4")
	if v.Name != "my episode 1" {
		t.Errorf("Name = %q, want %q", v.Name, "my episode 1")
	}
	if v.Branch != "my_episode_1" {
		t.Errorf("Branch = %q, want %q", v.Branch, "my_episode_1")
	}
}

func TestScanVideos_FlatFindsMP4(t *testing.T) {
	entries := fakeEntries(t, "a.mp4", "b.txt", "c.MP4")
	got := video.ScanVideos(entries, "/root", false)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d: %v", len(got), got)
	}
}

func TestScanVideos_IgnoresHeroDir(t *testing.T) {
	entries := fakeEntries(t, "hero_foo/a.mp4")
	got := video.ScanVideos(entries, "/root", false)
	if len(got) != 0 {
		t.Fatalf("want 0, got %d", len(got))
	}
}

func fakeEntries(t *testing.T, names ...string) []fs.DirEntry {
	t.Helper()
	fsys := fstest.MapFS{}
	for _, n := range names {
		fsys[n] = &fstest.MapFile{}
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	return entries
}
