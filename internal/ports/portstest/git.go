package portstest

import "context"

type InitCall     struct{ Dir, RemoteURL string }
type CheckoutCall struct{ Dir, Branch string }
type CommitCall   struct{ Dir, Message string }
type PushCall     struct{ Dir, PushURL, Branch string }

type GitRepository struct {
	InitFn           func(ctx context.Context, dir, remoteURL string) error
	CheckoutBranchFn func(ctx context.Context, dir, branch string) error
	StageAndCommitFn func(ctx context.Context, dir, message string) error
	ForcePushFn      func(ctx context.Context, dir, pushURL, branch string) error
	InitCalls        []InitCall
	CheckoutCalls    []CheckoutCall
	CommitCalls      []CommitCall
	PushCalls        []PushCall
}

func (f *GitRepository) Init(ctx context.Context, dir, remoteURL string) error {
	f.InitCalls = append(f.InitCalls, InitCall{Dir: dir, RemoteURL: remoteURL})
	if f.InitFn != nil {
		return f.InitFn(ctx, dir, remoteURL)
	}
	return nil
}

func (f *GitRepository) CheckoutBranch(ctx context.Context, dir, branch string) error {
	f.CheckoutCalls = append(f.CheckoutCalls, CheckoutCall{Dir: dir, Branch: branch})
	if f.CheckoutBranchFn != nil {
		return f.CheckoutBranchFn(ctx, dir, branch)
	}
	return nil
}

func (f *GitRepository) StageAndCommit(ctx context.Context, dir, message string) error {
	f.CommitCalls = append(f.CommitCalls, CommitCall{Dir: dir, Message: message})
	if f.StageAndCommitFn != nil {
		return f.StageAndCommitFn(ctx, dir, message)
	}
	return nil
}

func (f *GitRepository) ForcePush(ctx context.Context, dir, pushURL, branch string) error {
	f.PushCalls = append(f.PushCalls, PushCall{Dir: dir, PushURL: pushURL, Branch: branch})
	if f.ForcePushFn != nil {
		return f.ForcePushFn(ctx, dir, pushURL, branch)
	}
	return nil
}
