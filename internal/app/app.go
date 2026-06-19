package app

import (
	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// App is the composition root that wires all application services together.
type App struct {
	Encoding   *EncodingService
	Publishing *PublishingService
	Recovery   *RecoveryService
	Config     *ConfigService
	Runner     *Runner
}

// New constructs a fully-wired App from port adapter implementations.
// cfg and e supply the initial runtime configuration and event emitter;
// they are passed down to the Runner so the caller can override them per-run
// via Runner.Run.
func New(
	cfg settings.Settings,
	_ job.Emitter, // reserved: callers pass e per-run via Runner.Run
	enc ports.Encoder,
	prober ports.Prober,
	splitter ports.Splitter,
	git ports.GitRepository,
	mw ports.ManifestWriter,
	ws ports.Workspace,
	finder ports.WorkspaceFinder,
	store ports.ConfigStore,
) *App {
	maxParallel := cfg.MaxParallel
	if maxParallel < 1 {
		maxParallel = 1
	}

	encoding := NewEncodingService(enc, prober, splitter, ws)
	publishing := NewPublishingService(git, mw)
	runner := NewRunner(encoding, publishing, maxParallel)
	recovery := NewRecoveryService(finder)
	config := NewConfigService(store)

	return &App{
		Encoding:   encoding,
		Publishing: publishing,
		Recovery:   recovery,
		Config:     config,
		Runner:     runner,
	}
}
