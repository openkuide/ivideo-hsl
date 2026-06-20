// Package scanner provides a secondary adapter implementing ports.VideoScanner
// by traversing the filesystem using domain-owned filter rules.
package scanner

import (
	"os"
	"path/filepath"

	"github.com/chamrong/ivideo-hls/internal/domain/video"
	"github.com/chamrong/ivideo-hls/internal/ports"
)

// compile-time interface check
var _ ports.VideoScanner = (*Adapter)(nil)

// Adapter finds video files on disk. Recursive traversal respects the
// domain's IsIgnoredDir rules so filter logic stays in one place.
type Adapter struct{}

// New returns a ready-to-use Adapter.
func New() *Adapter { return &Adapter{} }

// Scan returns all video files under root. When recursive is false only the
// immediate directory is scanned; when true the entire subtree is walked,
// skipping directories rejected by video.IsIgnoredDir.
func (a *Adapter) Scan(root string, recursive bool) ([]video.Video, error) {
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
			return err
		}
		if d.IsDir() {
			if path != root && video.IsIgnoredDir(d.Name()) {
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
