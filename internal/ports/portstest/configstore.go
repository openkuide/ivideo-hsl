package portstest

import "github.com/chamrong/ivideo-hls/internal/domain/settings"

type ConfigStore struct {
	LoadFn func() (settings.Settings, error)
	SaveFn func(settings.Settings) error
	Saved  []settings.Settings
}

func (f *ConfigStore) Load() (settings.Settings, error) {
	if f.LoadFn != nil {
		return f.LoadFn()
	}
	return settings.Settings{}, nil
}

func (f *ConfigStore) Save(s settings.Settings) error {
	f.Saved = append(f.Saved, s)
	if f.SaveFn != nil {
		return f.SaveFn(s)
	}
	return nil
}
