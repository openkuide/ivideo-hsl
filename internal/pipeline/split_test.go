package pipeline

import (
	"context"
	"path/filepath"
	"testing"
)

// TestPartSuffix verifies letter generation for split part naming.
func TestPartSuffix(t *testing.T) {
	cases := []struct {
		i    int
		want string
	}{
		{0, "a"},
		{1, "b"},
		{25, "z"},
		{26, "aa"},
		{27, "ab"},
		{51, "az"},
	}
	for _, c := range cases {
		if got := partSuffix(c.i); got != c.want {
			t.Errorf("partSuffix(%d) = %q, want %q", c.i, got, c.want)
		}
	}
}

// TestProbeSize returns the correct file size and zero for a missing path.
func TestProbeSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v.mp4")
	mustWriteBytes(t, path, 4096)

	if got := probeSize(path); got != 4096 {
		t.Errorf("probeSize = %d, want 4096", got)
	}
	if got := probeSize(filepath.Join(dir, "missing.mp4")); got != 0 {
		t.Errorf("probeSize(missing) = %d, want 0", got)
	}
}

// TestSplitIntoEpisodes_UnderThreshold verifies that a file under 2 GB is
// returned as a single episode with an empty suffix and the original path.
func TestSplitIntoEpisodes_UnderThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "episode_3.mp4")
	mustWriteBytes(t, path, 1024) // well under 2 GB

	eps, err := splitIntoEpisodes(context.Background(), path, "episode_3", nil)
	if err != nil {
		t.Fatalf("splitIntoEpisodes: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(eps))
	}
	if eps[0].path != path {
		t.Errorf("path = %q, want %q", eps[0].path, path)
	}
	if eps[0].suffix != "" {
		t.Errorf("suffix = %q, want empty (no split)", eps[0].suffix)
	}
}

// TestSplitIntoEpisodes_ThresholdExact verifies a file of exactly the
// threshold is not split (boundary is strictly greater-than).
func TestSplitIntoEpisodes_ThresholdExact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v.mp4")
	mustWriteBytes(t, path, splitThresholdBytes)

	eps, err := splitIntoEpisodes(context.Background(), path, "v", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 1 || eps[0].suffix != "" {
		t.Errorf("file at threshold should not be split; got %d eps, suffix=%q", len(eps), eps[0].suffix)
	}
}
