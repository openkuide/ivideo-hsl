package workspacefinder_test

import (
	"testing"

	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/workspacefinder"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// TestAdapter_SatisfiesPorts is a compile-time proof that *Adapter satisfies
// ports.WorkspaceFinder.
func TestAdapter_SatisfiesPorts(t *testing.T) {
	var _ ports.WorkspaceFinder = workspacefinder.New("git")
}
