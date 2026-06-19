package portstest

import "github.com/chamrong/ivideo-hls/internal/domain/video"

type VideoScanner struct {
	ScanFn func(root string, recursive bool) ([]video.Video, error)
}

func (f *VideoScanner) Scan(root string, recursive bool) ([]video.Video, error) {
	if f.ScanFn != nil {
		return f.ScanFn(root, recursive)
	}
	return nil, nil
}
