package portstest

import (
	"context"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
)

type RecordCall         struct{ SourceDir, Branch string; HLSDirs []string }
type WriteWorkspaceCall struct{ Branch string; HLSDirs []string }

type ManifestWriter struct {
	RecordFn            func(ctx context.Context, sourceDir, branch string, hlsDirs []string, jobName string, e job.Emitter) error
	WriteWorkspaceFn    func(ctx context.Context, branch string, hlsDirs []string, jobName string, e job.Emitter) error
	RecordCalls         []RecordCall
	WriteWorkspaceCalls []WriteWorkspaceCall
}

func (f *ManifestWriter) Record(ctx context.Context, sourceDir, branch string, hlsDirs []string, jobName string, e job.Emitter) error {
	f.RecordCalls = append(f.RecordCalls, RecordCall{SourceDir: sourceDir, Branch: branch, HLSDirs: hlsDirs})
	if f.RecordFn != nil {
		return f.RecordFn(ctx, sourceDir, branch, hlsDirs, jobName, e)
	}
	return nil
}

func (f *ManifestWriter) WriteWorkspace(ctx context.Context, branch string, hlsDirs []string, jobName string, e job.Emitter) error {
	f.WriteWorkspaceCalls = append(f.WriteWorkspaceCalls, WriteWorkspaceCall{Branch: branch, HLSDirs: hlsDirs})
	if f.WriteWorkspaceFn != nil {
		return f.WriteWorkspaceFn(ctx, branch, hlsDirs, jobName, e)
	}
	return nil
}
