package app_test

import (
	"testing"

	"github.com/chamrong/ivideo-hls/internal/app"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/testutil/fakes"
)

func TestApp_New_SetsAllFields(t *testing.T) {
	cfg := settings.Default("/script")

	a := app.New(
		cfg,
		nil,
		&fakes.Encoder{},
		&fakes.Prober{},
		&fakes.Splitter{},
		&fakes.GitRepository{},
		&fakes.ManifestWriter{},
		&fakes.Workspace{},
		&fakes.WorkspaceFinder{},
		&fakes.ConfigStore{},
	)

	if a == nil {
		t.Fatal("want non-nil *App")
	}
	if a.Encoding == nil {
		t.Error("want Encoding non-nil")
	}
	if a.Publishing == nil {
		t.Error("want Publishing non-nil")
	}
	if a.Recovery == nil {
		t.Error("want Recovery non-nil")
	}
	if a.Config == nil {
		t.Error("want Config non-nil")
	}
	if a.Runner == nil {
		t.Error("want Runner non-nil")
	}
}
