package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chamrong/ivideo-hls/internal/adapters/primary/cli"
	"github.com/chamrong/ivideo-hls/internal/adapters/primary/tui"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/ffmpeg"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/ffprobe"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/gitrepo"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/jsonconfig"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/manifest"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/scanner"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/workspace"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/workspacefinder"
	"github.com/chamrong/ivideo-hls/internal/app"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	settingPath := filepath.Join(filepath.Dir(exePath()), "setting.json")
	store := jsonconfig.New(settingPath)
	cfg, _ := store.Load()
	if cfg.ScriptDir == "" {
		cfg.ScriptDir = wd
	}
	if cfg.SourceDir == "" {
		cfg.SourceDir = wd
	}

	enc := ffmpeg.New()
	prob := ffprobe.New()
	git := gitrepo.New("git")
	mw := manifest.New(cfg.PublicURLPattern)
	ws := workspace.New("git")
	finder := workspacefinder.New("git")
	scan := scanner.New()

	a := app.New(cfg, nil, enc, prob, enc, git, mw, ws, finder, store, scan)

	root := cli.Commands(a)
	root.AddCommand(newDoctorCommand(), newInstallDepsCommand())

	// No subcommand → launch the interactive TUI picker.
	root.RunE = func(cmd *cobra.Command, args []string) error {
		return runTUILoop(a)
	}

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runTUILoop(a *app.App) error {
	envToken := os.Getenv("IVIDEO_HLS_TOKEN")
	cfg, _ := a.Config.Load()

	for {
		pm, err := tui.RunPicker(a, cfg)
		if err != nil {
			return err
		}
		if pm.WantSettings {
			cfg, err = tui.RunSettings(a, cfg, envToken)
			if err != nil {
				return err
			}
			continue
		}
		if !pm.Confirmed {
			return nil
		}
		cfg = pm.UpdatedSettings
		videos := make([]video.Video, 0, len(pm.SelectedVideos))
		for _, p := range pm.SelectedVideos {
			videos = append(videos, video.NewVideo(p))
		}
		if _, err := tui.RunTUI(a, cfg, videos); err != nil {
			return err
		}
		return nil
	}
}

func exePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return exe
}
