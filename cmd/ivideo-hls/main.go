package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chamrong/ivideo-hls/internal/adapters/primary/cli"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/ffmpeg"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/ffprobe"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/gitrepo"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/jsonconfig"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/manifest"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/workspace"
	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/workspacefinder"
	"github.com/chamrong/ivideo-hls/internal/app"
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

	a := app.New(cfg, nil, enc, prob, enc, git, mw, ws, finder, store)

	root := cli.Commands(a)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func exePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return exe
}
