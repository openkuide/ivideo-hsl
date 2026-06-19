package app_test

import (
	"errors"
	"testing"

	"github.com/chamrong/ivideo-hls/internal/app"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
	"github.com/chamrong/ivideo-hls/internal/ports/portstest"
)

func TestConfigService_Load(t *testing.T) {
	want := settings.Default("/script")
	want.Push = true
	want.PushURL = "https://token@github.com/org/repo.git"

	store := &portstest.ConfigStore{
		LoadFn: func() (settings.Settings, error) { return want, nil },
	}
	svc := app.NewConfigService(store)

	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.PushURL != want.PushURL {
		t.Errorf("want PushURL %q, got %q", want.PushURL, got.PushURL)
	}
}

func TestConfigService_Load_Error(t *testing.T) {
	store := &portstest.ConfigStore{
		LoadFn: func() (settings.Settings, error) { return settings.Settings{}, errors.New("no config") },
	}
	svc := app.NewConfigService(store)

	_, err := svc.Load()
	if err == nil {
		t.Fatal("want error from Load, got nil")
	}
}

func TestConfigService_Save(t *testing.T) {
	store := &portstest.ConfigStore{}
	svc := app.NewConfigService(store)

	cfg := settings.Default("/script")
	cfg.MaxParallel = 4

	if err := svc.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(store.Saved) != 1 {
		t.Fatalf("want 1 saved config, got %d", len(store.Saved))
	}
	if store.Saved[0].MaxParallel != 4 {
		t.Errorf("want MaxParallel=4, got %d", store.Saved[0].MaxParallel)
	}
}

func TestConfigService_Merge_NonZeroOverrides(t *testing.T) {
	svc := app.NewConfigService(&portstest.ConfigStore{})

	base := settings.Default("/script")
	base.MaxParallel = 1
	base.Quality = video.QualityLow

	overrides := settings.Settings{
		MaxParallel: 4,
		Quality:     video.QualityHigh,
	}

	merged := svc.Merge(base, overrides)

	if merged.MaxParallel != 4 {
		t.Errorf("want MaxParallel=4 from overrides, got %d", merged.MaxParallel)
	}
	if merged.Quality != video.QualityHigh {
		t.Errorf("want Quality=%q from overrides, got %q", video.QualityHigh, merged.Quality)
	}
}

func TestConfigService_Merge_ZeroFieldKeepsBase(t *testing.T) {
	svc := app.NewConfigService(&portstest.ConfigStore{})

	base := settings.Default("/script")
	base.PushURL = "https://base-url"
	base.MaxParallel = 2

	overrides := settings.Settings{
		Quality: video.QualityHigh,
		// PushURL and MaxParallel are zero-value, base should win
	}

	merged := svc.Merge(base, overrides)

	if merged.PushURL != "https://base-url" {
		t.Errorf("want PushURL from base, got %q", merged.PushURL)
	}
	if merged.MaxParallel != 2 {
		t.Errorf("want MaxParallel=2 from base, got %d", merged.MaxParallel)
	}
	if merged.Quality != video.QualityHigh {
		t.Errorf("want Quality from overrides, got %q", merged.Quality)
	}
}
