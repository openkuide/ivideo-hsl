package portstest

import (
	"context"
	"time"
)

type Prober struct {
	DurationFn func(ctx context.Context, path string) (time.Duration, error)
	FileSizeFn func(path string) int64
}

func (f *Prober) Duration(ctx context.Context, path string) (time.Duration, error) {
	if f.DurationFn != nil {
		return f.DurationFn(ctx, path)
	}
	return 10 * time.Minute, nil
}

func (f *Prober) FileSize(path string) int64 {
	if f.FileSizeFn != nil {
		return f.FileSizeFn(path)
	}
	return 100 * 1024 * 1024 // 100 MB default
}
