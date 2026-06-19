package ports

import "github.com/chamrong/ivideo-hls/internal/domain/video"

// VideoScanner discovers video files on disk. Implementations handle
// filesystem traversal; the domain layer remains pure.
type VideoScanner interface {
	Scan(root string, recursive bool) ([]video.Video, error)
}
