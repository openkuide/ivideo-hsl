// Package scanner provides a helper for discovering video files on disk.
// It is not a port implementation — it is called directly by the CLI and TUI.
package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

// ignoredDirs mirrors the pipeline scan logic: skip hidden dirs, hero_*
// workspaces, and a small fixed set of well-known non-video trees.
var ignoredDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"hero":         true,
}

// Scanner finds video files on disk using the same rules as the pipeline.
type Scanner struct{}

// New returns a ready-to-use Scanner.
func New() *Scanner { return &Scanner{} }

// Scan returns all video files under root. When recursive is false only the
// immediate directory is scanned; when true the entire subtree is walked,
// skipping hidden directories, hero_* workspaces, and ignoredDirs.
func (s *Scanner) Scan(root string, recursive bool) ([]video.Video, error) {
	if !recursive {
		return scanFlat(root)
	}
	return scanRecursive(root)
}

func scanFlat(root string) ([]video.Video, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []video.Video
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !video.IsVideoFile(e.Name()) {
			continue
		}
		out = append(out, video.NewVideo(filepath.Join(root, e.Name())))
	}
	return out, nil
}

func scanRecursive(root string) ([]video.Video, error) {
	var out []video.Video
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != root && isIgnoredDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if video.IsVideoFile(d.Name()) {
			out = append(out, video.NewVideo(path))
		}
		return nil
	})
	return out, err
}

func isIgnoredDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	if strings.HasPrefix(name, "hero_") {
		return true
	}
	return ignoredDirs[name]
}
