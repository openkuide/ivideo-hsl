package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chamrong/ivideo-hls/internal/adapters/primary/cli"
	"github.com/chamrong/ivideo-hls/internal/adapters/primary/tui"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
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
		// Auto-detect ./input/ as the default source dir (dev sandbox convention).
		inputDir := filepath.Join(wd, "input")
		if fi, err2 := os.Stat(inputDir); err2 == nil && fi.IsDir() {
			cfg.SourceDir = inputDir
		} else {
			cfg.SourceDir = wd
		}
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
	// Pass the already-defaulted cfg so runTUILoop doesn't reload from disk
	// and lose the ./input/ auto-detection applied above.
	root.RunE = func(cmd *cobra.Command, args []string) error {
		return runTUILoop(a, cfg)
	}

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runTUILoop(a *app.App, cfg settings.Settings) error {
	envToken := os.Getenv("IVIDEO_HLS_TOKEN")
	var banner string

	for {
		pm, err := tui.RunPicker(a, cfg, banner)
		banner = "" // consumed after first render
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
		results, err := tui.RunTUI(a, cfg, videos)
		if err != nil {
			return err
		}
		_, fail := app.Summary(results)
		if fail > 0 {
			// Return to picker with a banner so the user can adjust and retry.
			// Workspaces are preserved on push failure for `recover`.
			banner = fmt.Sprintf("⚠  %d video(s) failed — workspaces preserved · run `recover` to retry", fail)
			continue
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
