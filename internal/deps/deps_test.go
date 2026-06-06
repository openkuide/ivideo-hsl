package deps

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable-bit semantics differ on Windows")
	}
	dir := t.TempDir()
	exec := filepath.Join(dir, "tool")
	if err := os.WriteFile(exec, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	nonExec := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(nonExec, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isExecutable(exec) {
		t.Errorf("%s should be executable", exec)
	}
	if isExecutable(nonExec) {
		t.Errorf("%s should not be executable (mode 0644)", nonExec)
	}
	if isExecutable(filepath.Join(dir, "missing")) {
		t.Errorf("missing file should not count as executable")
	}
	if isExecutable(dir) {
		t.Errorf("directory should not count as executable")
	}
}

func TestCacheBinDir_HonorsXDG(t *testing.T) {
	custom := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", custom)
	got, err := CacheBinDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(custom, "ivideo-hls", "bin")
	if got != want {
		t.Errorf("CacheBinDir = %q, want %q", got, want)
	}
}

func TestCacheBinDir_FallsBackToHomeCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("home dir semantics differ on Windows")
	}
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", home)
	got, err := CacheBinDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "ivideo-hls/bin") {
		t.Errorf("CacheBinDir = %q, expected path to include ivideo-hls/bin", got)
	}
	if !strings.HasPrefix(got, home) {
		t.Errorf("CacheBinDir = %q, expected prefix %q", got, home)
	}
}

func TestResolve_PrefersLocalBin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable-bit semantics differ on Windows")
	}
	wd := realPath(t, t.TempDir())
	if err := os.Chdir(wd); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(wd, "bin", "ffmpeg")
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Also divert cache to a different empty temp dir so we know local wins,
	// not a coincidental PATH hit.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	got := realPath(t, resolve(FFmpeg))
	if got != local {
		t.Errorf("resolve preferred %q over expected local %q", got, local)
	}
}

// realPath resolves symlinks so comparisons work across platforms where
// /tmp is a symlink to /private/tmp (macOS).
func realPath(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p
	}
	return resolved
}

func TestResolve_FallsThroughToCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable-bit semantics differ on Windows")
	}
	// Put CWD somewhere with no ./bin/ffmpeg.
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	cacheRoot := realPath(t, t.TempDir())
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	cacheBin := filepath.Join(cacheRoot, "ivideo-hls", "bin")
	if err := os.MkdirAll(cacheBin, 0o755); err != nil {
		t.Fatal(err)
	}
	cached := filepath.Join(cacheBin, "ffmpeg")
	if err := os.WriteFile(cached, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := realPath(t, resolve(FFmpeg))
	if got != cached {
		t.Errorf("resolve = %q, want cached %q", got, cached)
	}
}

func TestResolve_ReturnsLiteralWhenNothingFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path semantics differ on Windows")
	}
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	// Scrub PATH so exec.LookPath can't find a system ffmpeg.
	t.Setenv("PATH", "")

	got := resolve(FFmpeg)
	if got != "ffmpeg" {
		t.Errorf("expected literal fallback 'ffmpeg', got %q", got)
	}
}
