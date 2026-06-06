package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMaybeDeleteCompressed_RespectsShouldKeepSource pins the invariant
// that the compressed sibling lives and dies with the source .mp4 —
// whatever condition keeps the source must keep the compressed file too,
// otherwise resume-reuse has nothing to reuse after a push failure.
func TestMaybeDeleteCompressed_RespectsShouldKeepSource(t *testing.T) {
	cases := []struct {
		name      string
		cfg       Config
		wantKeep  bool
		reasonHas string
	}{
		{
			name:     "happy path: push + cleanup + delete source",
			cfg:      Config{PreCompress: true, Push: true, Cleanup: true},
			wantKeep: false,
		},
		{
			name:      "push skipped keeps compressed",
			cfg:       Config{PreCompress: true, Push: false, Cleanup: true},
			wantKeep:  true,
			reasonHas: "push skipped",
		},
		{
			name:      "cleanup skipped keeps compressed",
			cfg:       Config{PreCompress: true, Push: true, Cleanup: false},
			wantKeep:  true,
			reasonHas: "cleanup skipped",
		},
		{
			name:      "keep-source keeps compressed",
			cfg:       Config{PreCompress: true, Push: true, Cleanup: true, KeepSource: true},
			wantKeep:  true,
			reasonHas: "keep-source",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "v.mp4")
			compressed := filepath.Join(dir, "v_compressed.mp4")
			mustWriteBytes(t, source, 1024)
			mustWriteBytes(t, compressed, 2048)

			runner := NewRunner(&c.cfg, nil)
			jc := &jobContext{
				job:        "v",
				videoPath:  source,
				finalInput: compressed,
			}
			runner.maybeDeleteCompressed(jc)

			_, err := os.Stat(compressed)
			kept := err == nil
			if kept != c.wantKeep {
				t.Errorf("compressed kept=%v, want %v", kept, c.wantKeep)
			}
		})
	}
}

// TestMaybeDeleteCompressed_NoOpWhenPreCompressOff guards against a bug
// where the cleanup runs even when the user never asked for pre-compress,
// which would delete whatever path jc.finalInput happens to hold (usually
// the source itself — disaster).
func TestMaybeDeleteCompressed_NoOpWhenPreCompressOff(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "v.mp4")
	mustWriteBytes(t, source, 1024)

	runner := NewRunner(&Config{PreCompress: false, Push: true, Cleanup: true}, nil)
	jc := &jobContext{
		job:        "v",
		videoPath:  source,
		finalInput: source, // same path — no compression ran
	}
	runner.maybeDeleteCompressed(jc)

	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source disappeared: %v", err)
	}
}

// TestMaybeDeleteCompressed_NoOpWhenFinalEqualsSource is the paranoid
// fallback: even if someone sets PreCompress=true by mistake but leaves
// finalInput pointing at the source, the guard still protects the source.
func TestMaybeDeleteCompressed_NoOpWhenFinalEqualsSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "v.mp4")
	mustWriteBytes(t, source, 1024)

	runner := NewRunner(&Config{PreCompress: true, Push: true, Cleanup: true}, nil)
	jc := &jobContext{
		job:        "v",
		videoPath:  source,
		finalInput: source, // should never happen, but if it does, don't delete
	}
	runner.maybeDeleteCompressed(jc)

	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source incorrectly deleted: %v", err)
	}
}
