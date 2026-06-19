package fakes_test

import (
	"testing"

	"github.com/chamrong/ivideo-hls/internal/ports"
	"github.com/chamrong/ivideo-hls/internal/testutil/fakes"
)

func TestFakes_SatisfyPorts(t *testing.T) {
	var _ ports.Encoder        = &fakes.Encoder{}
	var _ ports.Prober         = &fakes.Prober{}
	var _ ports.Splitter       = &fakes.Splitter{}
	var _ ports.GitRepository  = &fakes.GitRepository{}
	var _ ports.ManifestWriter = &fakes.ManifestWriter{}
	var _ ports.Workspace      = &fakes.Workspace{}
	var _ ports.WorkspaceFinder = &fakes.WorkspaceFinder{}
	var _ ports.ConfigStore    = &fakes.ConfigStore{}
}
