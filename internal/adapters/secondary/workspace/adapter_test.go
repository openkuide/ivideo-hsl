package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/workspace"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// TestAdapter_SatisfiesPorts is a compile-time proof that *Adapter satisfies
// ports.Workspace.
func TestAdapter_SatisfiesPorts(t *testing.T) {
	var _ ports.Workspace = workspace.New("git")
}

// TestAdapter_Cleanup_NoErrorOnMissing verifies that Cleanup does not panic
// or error when the workspace directory does not exist.
func TestAdapter_Cleanup_NoErrorOnMissing(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "hero_nonexistent")
	// Confirm it really doesn't exist
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("expected missing dir")
	}
	a := workspace.New("git")
	// Should not panic; os.RemoveAll is a no-op on missing paths
	a.Cleanup(missing, nil, "test-job")
}
