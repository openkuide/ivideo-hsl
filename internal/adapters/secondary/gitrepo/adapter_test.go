package gitrepo_test

import (
	"testing"

	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/gitrepo"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// TestAdapter_SatisfiesPorts is a compile-time proof that *Adapter satisfies
// ports.GitRepository. The var _ declaration in adapter.go catches this at
// compile time; this test makes the intent explicit in test output.
func TestAdapter_SatisfiesPorts(t *testing.T) {
	var _ ports.GitRepository = gitrepo.New("git")
}
