// Package deps resolves and installs the external binaries ivideo-hls relies
// on at runtime (ffmpeg, ffprobe).
//
// Lookup order:
//  1. ./bin/<name>            — development / manual drop-in
//  2. $XDG_CACHE_HOME/ivideo-hls/bin/<name>  — installed via install-deps
//  3. $PATH                   — homebrew / apt / system-installed
//
// ivideo-hls never redistributes ffmpeg. The install-deps command downloads
// static builds from the canonical upstream mirrors on the user's machine,
// verifies them against pinned SHA-256 checksums, and drops them into the
// user's cache directory. git is a separate hard prereq and is not managed
// here.
package deps

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Binary identifies an external tool.
type Binary string

const (
	FFmpeg  Binary = "ffmpeg"
	FFprobe Binary = "ffprobe"
)

// FFmpegPath returns the absolute path to use for `ffmpeg`, or the literal
// "ffmpeg" if nothing is found (letting exec.LookPath fail with its standard
// error at call time). Callers pass the result straight to exec.Command.
func FFmpegPath() string { return resolve(FFmpeg) }

// FFprobePath behaves like FFmpegPath but for ffprobe.
func FFprobePath() string { return resolve(FFprobe) }

func resolve(b Binary) string {
	name := string(b)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, dir := range searchDirs() {
		candidate := filepath.Join(dir, name)
		if isExecutable(candidate) {
			return candidate
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name // let exec fail with its own error
}

func searchDirs() []string {
	var dirs []string
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(wd, "bin"))
	}
	if cache, err := CacheBinDir(); err == nil {
		dirs = append(dirs, cache)
	}
	return dirs
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	// Windows doesn't carry +x in the mode bits; existence is enough.
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

// CacheBinDir returns the directory where install-deps places downloaded
// binaries. Honors $XDG_CACHE_HOME; falls back to ~/.cache on Linux and
// ~/Library/Caches on macOS.
func CacheBinDir() (string, error) {
	base, err := cacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ivideo-hls", "bin"), nil
}

func cacheRoot() (string, error) {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Caches"), nil
	case "windows":
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return v, nil
		}
	}
	return filepath.Join(home, ".cache"), nil
}

// ErrPlatformUnsupported is returned when install-deps is invoked on a
// GOOS/GOARCH combination we don't have a pinned build for.
var ErrPlatformUnsupported = errors.New("no prebuilt ffmpeg binary pinned for this platform")
