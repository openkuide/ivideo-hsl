package portstest_test

import (
	"testing"

	"github.com/chamrong/ivideo-hls/internal/ports"
	"github.com/chamrong/ivideo-hls/internal/ports/portstest"
)

func TestFakes_SatisfyPorts(t *testing.T) {
	var _ ports.Encoder        = &portstest.Encoder{}
	var _ ports.Prober         = &portstest.Prober{}
	var _ ports.Splitter       = &portstest.Splitter{}
	var _ ports.GitRepository  = &portstest.GitRepository{}
	var _ ports.ManifestWriter = &portstest.ManifestWriter{}
	var _ ports.Workspace      = &portstest.Workspace{}
	var _ ports.WorkspaceFinder = &portstest.WorkspaceFinder{}
	var _ ports.ConfigStore    = &portstest.ConfigStore{}
	var _ ports.VideoScanner   = &portstest.VideoScanner{}
}
