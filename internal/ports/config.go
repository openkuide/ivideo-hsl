package ports

import "github.com/chamrong/ivideo-hls/internal/domain/settings"

type ConfigStore interface {
	Load() (settings.Settings, error)
	Save(settings.Settings) error
}
