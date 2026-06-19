package fakes

import (
	"context"
	"path/filepath"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

type SetupCall   struct{ V video.Video; JobName string }
type CleanupCall struct{ Dir, JobName string }

type Workspace struct {
	SetupFn       func(ctx context.Context, v video.Video, cfg settings.Settings, jobName string, e job.Emitter) (string, error)
	CleanupFn     func(workspaceDir string, e job.Emitter, jobName string)
	PrepareBaseFn func(ctx context.Context, cfg settings.Settings, e job.Emitter) error
	SetupCalls    []SetupCall
	CleanupCalls  []CleanupCall
}

func (f *Workspace) Setup(ctx context.Context, v video.Video, cfg settings.Settings, jobName string, e job.Emitter) (string, error) {
	f.SetupCalls = append(f.SetupCalls, SetupCall{V: v, JobName: jobName})
	if f.SetupFn != nil { return f.SetupFn(ctx, v, cfg, jobName, e) }
	return filepath.Join(cfg.ScriptDir, "hero_"+v.Name), nil
}

func (f *Workspace) Cleanup(workspaceDir string, e job.Emitter, jobName string) {
	f.CleanupCalls = append(f.CleanupCalls, CleanupCall{Dir: workspaceDir, JobName: jobName})
	if f.CleanupFn != nil { f.CleanupFn(workspaceDir, e, jobName) }
}

func (f *Workspace) PrepareBase(ctx context.Context, cfg settings.Settings, e job.Emitter) error {
	if f.PrepareBaseFn != nil { return f.PrepareBaseFn(ctx, cfg, e) }
	return nil
}
