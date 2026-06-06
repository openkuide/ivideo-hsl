package pipeline

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// VideoExtensions is the fixed allowlist of container extensions that ffmpeg
// reliably handles as source material for this pipeline. The list is
// deliberately conservative: adding a random "*.*" match would invite
// encode attempts on unrelated files (.DS_Store, README.md, etc.).
var VideoExtensions = []string{
	".mp4", ".mov", ".m4v", ".mkv", ".webm",
	".avi", ".3gp", ".3g2", ".flv", ".wmv", ".ts",
}

// IsVideoFile reports whether `name` has one of the allowlisted extensions.
// Comparison is case-insensitive so `FOO.MP4` matches.
func IsVideoFile(name string) bool {
	return slices.Contains(VideoExtensions, strings.ToLower(filepath.Ext(name)))
}

// ignoredDirs are subdirectory names skipped during recursive scans. These
// are directories a user almost never means to include: scratch workspaces,
// VCS internals, and node_modules.
var ignoredDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"hero":         true,
}

// isIgnoredDir returns true for hidden dirs (leading `.`), ivideo-hls's own
// per-video workspaces (`hero_*`), and the fixed allowlist above.
func isIgnoredDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	if strings.HasPrefix(name, "hero_") {
		return true
	}
	return ignoredDirs[name]
}

// ScanVideos finds all video files under `root`. When `recursive` is false,
// only the immediate directory is scanned. When true, the tree is walked
// but ignored directories are pruned.
//
// Returns absolute paths. Errors from Stat on individual files are skipped —
// one unreadable file should not abort the whole scan — but errors reading
// the root directory itself do propagate.
func ScanVideos(root string, recursive bool) ([]string, error) {
	if !recursive {
		return scanFlat(root)
	}
	return scanRecursive(root)
}

func scanFlat(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !IsVideoFile(e.Name()) {
			continue
		}
		out = append(out, filepath.Join(root, e.Name()))
	}
	return out, nil
}

func scanRecursive(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// tolerate permission errors on sub-trees
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
		if IsVideoFile(d.Name()) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
