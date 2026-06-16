package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// errSplitHandled is a sentinel returned by stepConvert when a file was split
// into per-part sub-jobs that each handled their own commit/push. The caller
// (processOne) treats this as a clean success, not an error.
var errSplitHandled = errors.New("split: per-part jobs complete")

type Result struct {
	Video   string
	Success bool
	Err     error
}

type Runner struct {
	cfg     *Config
	emitter Emitter
	cpuSem  *semaphore.Weighted
	netSem  *semaphore.Weighted
	cpuInUse atomic.Int32
	netInUse atomic.Int32
	mu      sync.Mutex
	results []Result
}

// SlotUsage is a snapshot of the runner's semaphore occupancy. Used by the
// TUI to show live "CPU 3/3 · NET 1/6" on the run dashboard.
type SlotUsage struct {
	CPUInUse, CPUCapacity int
	NetInUse, NetCapacity int
}

// Usage returns a snapshot of how many CPU and network slots are in use.
// Safe to call from any goroutine.
func (r *Runner) Usage() SlotUsage {
	return SlotUsage{
		CPUInUse:    int(r.cpuInUse.Load()),
		CPUCapacity: r.capacityOf(r.cpuSem, r.cfg.MaxParallel),
		NetInUse:    int(r.netInUse.Load()),
		NetCapacity: r.capacityOf(r.netSem, r.cfg.MaxParallel*2),
	}
}

// capacityOf returns the configured slot count for a semaphore, or 1 when
// serial mode has nil'd it out (everything runs through the bare fn).
func (r *Runner) capacityOf(sem *semaphore.Weighted, configured int) int {
	if sem == nil {
		return 1
	}
	if configured < 1 {
		return 1
	}
	return configured
}

func NewRunner(cfg *Config, e Emitter) *Runner {
	r := &Runner{cfg: cfg, emitter: e}
	if cfg.ParallelMode && cfg.MaxParallel > 1 {
		r.cpuSem = semaphore.NewWeighted(int64(cfg.MaxParallel))
		r.netSem = semaphore.NewWeighted(int64(cfg.MaxParallel * 2))
	}
	return r
}

func (r *Runner) Run(ctx context.Context) []Result {
	if r.isParallel() {
		r.runParallel(ctx)
	} else {
		r.runSerial(ctx)
	}
	return r.results
}

func (r *Runner) isParallel() bool {
	return r.cfg.ParallelMode && r.cfg.MaxParallel > 1
}

func (r *Runner) runSerial(ctx context.Context) {
	for _, v := range r.cfg.Videos {
		r.processOne(ctx, v)
	}
}

func (r *Runner) runParallel(ctx context.Context) {
	if err := prepareBaseHero(ctx, r.cfg, r.emitter); err != nil {
		errorf(r.emitter, BaseJob, StageWorkspace, err.Error())
	}
	g, gctx := errgroup.WithContext(ctx)
	for _, video := range r.cfg.Videos {
		g.Go(func() error {
			r.processOne(gctx, video)
			return nil
		})
	}
	_ = g.Wait()
}

func (r *Runner) addResult(res Result) {
	r.mu.Lock()
	r.results = append(r.results, res)
	r.mu.Unlock()
}

// jobContext holds the state for one video as it moves through the pipeline.
type jobContext struct {
	videoPath  string
	job        string
	branch     string
	workspace  string
	finalInput string // may differ from videoPath when pre-compression runs
	hlsDirs    []string // HLS output dirs, one per episode; populated by stepConvert
}

func newJobContext(videoPath string) *jobContext {
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	return &jobContext{
		videoPath:  videoPath,
		job:        base,
		branch:     base,
		finalInput: videoPath,
	}
}

func (r *Runner) processOne(ctx context.Context, videoPath string) {
	jc := newJobContext(videoPath)

	if err := r.ensureVideoExists(jc); err != nil {
		r.failJob(jc, err)
		return
	}

	info(r.emitter, jc.job, StageQueued, "starting "+filepath.Base(videoPath))

	steps := []func(context.Context, *jobContext) error{
		r.stepSetupWorkspace,
		r.stepCheckoutBranch,
		r.stepPreCompress,
		r.stepConvert,
		r.stepCommitAndPush,
	}
	for _, step := range steps {
		err := step(ctx, jc)
		if errors.Is(err, errSplitHandled) {
			// per-part sub-jobs already committed, pushed, and wrote manifests
			r.stepFinalize(jc)
			success(r.emitter, jc.job, StageDone, "complete (split into parts)")
			r.addResult(Result{Video: videoPath, Success: true})
			return
		}
		if err != nil {
			r.failJob(jc, err)
			return
		}
	}

	defaultManifestWriter.recordSuccess(r.cfg, jc.videoPath, jc.branch, jc.hlsDirs, jc.job, r.emitter)
	r.stepFinalize(jc)
	success(r.emitter, jc.job, StageDone, "complete")
	r.addResult(Result{Video: videoPath, Success: true})
}

func (r *Runner) ensureVideoExists(jc *jobContext) error {
	if _, err := os.Stat(jc.videoPath); err != nil {
		return fmt.Errorf("video not found: %s", jc.videoPath)
	}
	return nil
}

func (r *Runner) failJob(jc *jobContext, err error) {
	errorf(r.emitter, jc.job, StageFailed, err.Error())
	if jc.workspace != "" {
		warn(r.emitter, jc.job, StageFailed,
			"workspace preserved for inspection: "+filepath.Base(jc.workspace))
	}
	r.addResult(Result{Video: jc.videoPath, Success: false, Err: err})
}

func (r *Runner) stepSetupWorkspace(ctx context.Context, jc *jobContext) error {
	ws, err := setupWorkspace(ctx, jc.videoPath, r.cfg, jc.job, r.emitter)
	if err != nil {
		return err
	}
	jc.workspace = ws
	removeGitLocks(ws)
	if err := configureRemoteOrigin(ctx, ws, r.cfg.RemoteURL, r.emitter, jc.job); err != nil {
		warn(r.emitter, jc.job, StageWorkspace, err.Error())
	}
	return nil
}

func (r *Runner) stepCheckoutBranch(ctx context.Context, jc *jobContext) error {
	r.syncMainBestEffort(ctx, jc)

	info(r.emitter, jc.job, StageWorkspace, "checkout -B "+jc.branch)
	if err := runQuiet(ctx, jc.workspace, "git", "checkout", "-B", jc.branch); err != nil {
		return fmt.Errorf("checkout branch %s: %w", jc.branch, err)
	}
	return nil
}

// syncMainBestEffort brings the workspace to a fresh `main` before branching.
// Both commands are allowed to fail: a new workspace may not have a remote
// tracking branch yet, and `main` may not be the active branch. The next
// `checkout -B <branch>` is what guarantees correctness.
func (r *Runner) syncMainBestEffort(ctx context.Context, jc *jobContext) {
	_ = runQuiet(ctx, jc.workspace, "git", "checkout", "main")
	_ = runQuiet(ctx, jc.workspace, "git", "pull", "origin", "main")
}

func (r *Runner) stepPreCompress(ctx context.Context, jc *jobContext) error {
	if !r.cfg.PreCompress {
		return nil
	}
	compressed, err := r.runCPUWithResult(ctx, func() (string, error) {
		return compressVideo(ctx, jc.videoPath, jc.job, r.emitter)
	})
	if err != nil {
		return err
	}
	jc.finalInput = compressed
	return nil
}

func (r *Runner) stepConvert(ctx context.Context, jc *jobContext) error {
	return r.runCPU(ctx, func() error {
		episodes, err := splitIntoEpisodes(ctx, jc.finalInput, jc.job, r.emitter)
		if err != nil {
			return err
		}

		if len(episodes) == 1 && episodes[0].suffix == "" {
			// no split — convert into the normal workspace
			if err := convertToHLS(ctx, episodes[0].path, jc.workspace, r.cfg, jc.job, r.emitter); err != nil {
				return err
			}
			jc.hlsDirs = []string{filepath.Join(jc.workspace, "x")}
			return nil
		}

		// split — each part gets its own branch, workspace, convert, and push
		for _, ep := range episodes {
			if err := r.processEpisodePart(ctx, jc, ep); err != nil {
				return err
			}
		}
		// signal to the caller that per-part jobs handled everything
		jc.hlsDirs = nil
		return errSplitHandled
	})
}

// processEpisodePart runs the full convert→commit→push cycle for one split
// part. The part branch is named <base><suffix> (e.g. "episode_3a") and gets
// its own workspace so it pushes to a separate branch, identical to how an
// unsplit file would be handled.
func (r *Runner) processEpisodePart(ctx context.Context, parent *jobContext, ep episode) error {
	partBranch := parent.branch + ep.suffix
	partJob := parent.job + ep.suffix

	info(r.emitter, partJob, StageConvert, fmt.Sprintf("processing part %s → branch %s", ep.suffix, partBranch))

	pjc := &jobContext{
		videoPath:  ep.path,
		job:        partJob,
		branch:     partBranch,
		finalInput: ep.path,
	}

	// reuse a dedicated workspace for this part
	ws, err := setupWorkspace(ctx, ep.path, r.cfg, partJob, r.emitter)
	if err != nil {
		return fmt.Errorf("part %s workspace: %w", ep.suffix, err)
	}
	pjc.workspace = ws
	removeGitLocks(ws)
	if err := configureRemoteOrigin(ctx, ws, r.cfg.RemoteURL, r.emitter, partJob); err != nil {
		warn(r.emitter, partJob, StageWorkspace, err.Error())
	}

	r.syncMainBestEffort(ctx, pjc)
	if err := runQuiet(ctx, ws, "git", "checkout", "-B", partBranch); err != nil {
		return fmt.Errorf("part %s checkout: %w", ep.suffix, err)
	}

	if err := convertToHLS(ctx, ep.path, ws, r.cfg, partJob, r.emitter); err != nil {
		_ = os.Remove(ep.path)
		return fmt.Errorf("part %s convert: %w", ep.suffix, err)
	}
	pjc.hlsDirs = []string{filepath.Join(ws, "x")}
	_ = os.Remove(ep.path) // temp split file no longer needed

	defaultManifestWriter.writeWorkspaceManifest(r.cfg, partBranch, pjc.hlsDirs, partJob, r.emitter)
	if err := r.stageAndCommit(ctx, pjc); err != nil {
		return fmt.Errorf("part %s commit: %w", ep.suffix, err)
	}
	if r.cfg.Push {
		if err := r.forcePush(ctx, pjc); err != nil {
			return fmt.Errorf("part %s push: %w", ep.suffix, err)
		}
	}
	defaultManifestWriter.recordSuccess(r.cfg, parent.videoPath, partBranch, pjc.hlsDirs, partJob, r.emitter)
	if r.cfg.Cleanup {
		cleanupWorkspace(ws, r.emitter, partJob)
	}
	return nil
}

func (r *Runner) stepCommitAndPush(ctx context.Context, jc *jobContext) error {
	defaultManifestWriter.writeWorkspaceManifest(r.cfg, jc.branch, jc.hlsDirs, jc.job, r.emitter)
	if err := r.stageAndCommit(ctx, jc); err != nil {
		return err
	}
	if !r.cfg.Push {
		warn(r.emitter, jc.job, StageGitPush,
			fmt.Sprintf("push skipped (--no-push). Run manually: git -C %s push -u -f origin %s",
				filepath.Base(jc.workspace), jc.branch))
		return nil
	}
	return r.forcePush(ctx, jc)
}

// stageAndCommit stages all files and commits them. `git add` warnings are
// logged but not fatal. "Nothing to commit" is detected via a pre-check
// (`git diff --cached --quiet`) so a real commit failure (hook rejection,
// signing failure, corrupt index) propagates instead of being silently
// downgraded to a dim log.
func (r *Runner) stageAndCommit(ctx context.Context, jc *jobContext) error {
	info(r.emitter, jc.job, StageGitPush, "staging & committing")
	if err := runQuiet(ctx, jc.workspace, "git", "add", "."); err != nil {
		warn(r.emitter, jc.job, StageGitPush, "git add warning: "+err.Error())
	}
	if nothingToCommit(ctx, jc.workspace) {
		dim(r.emitter, jc.job, StageGitPush, "nothing new to commit")
		return nil
	}
	if err := runQuiet(ctx, jc.workspace, "git", "commit", "-m", "a"); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

// nothingToCommit returns true when the staging area has no changes vs HEAD.
// `git diff --cached --quiet` exits 0 when clean, 1 when there are diffs;
// any other error is treated as "assume there's something to commit" so the
// real commit attempt can surface the underlying problem.
func nothingToCommit(ctx context.Context, workspace string) bool {
	return runQuiet(ctx, workspace, "git", "diff", "--cached", "--quiet") == nil
}

func (r *Runner) forcePush(ctx context.Context, jc *jobContext) error {
	info(r.emitter, jc.job, StageGitPush,
		fmt.Sprintf("force-pushing to %s / %s", r.cfg.RemoteURL, jc.branch))
	pushURL := r.cfg.EffectivePushURL()
	err := r.runNet(ctx, func() error {
		// Pass the credential-bearing URL as the <repository> positional so
		// it never lands in `git remote -v` or the reflog.
		return runQuiet(ctx, jc.workspace, "git", "push", "-u", "-f", pushURL, jc.branch)
	})
	if err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	success(r.emitter, jc.job, StageGitPush, "push successful")
	return nil
}

func (r *Runner) stepFinalize(jc *jobContext) {
	if !r.cfg.Cleanup {
		warn(r.emitter, jc.job, StageDone,
			"workspace kept (--no-cleanup): "+filepath.Base(jc.workspace))
	} else {
		cleanupWorkspace(jc.workspace, r.emitter, jc.job)
	}
	r.maybeDeleteCompressed(jc)
	r.maybeDeleteSource(jc)
}

// maybeDeleteCompressed removes the <name>_compressed.mp4 sibling only when
// the whole job succeeded end-to-end. It reuses shouldKeepSource() so the
// rules stay in lockstep: any condition that keeps the source also keeps
// the compressed file — otherwise a failed push with resume_reuse_compressed
// would have nothing to reuse from.
func (r *Runner) maybeDeleteCompressed(jc *jobContext) {
	if !r.cfg.PreCompress || jc.finalInput == jc.videoPath {
		return
	}
	if keep, reason := r.shouldKeepSource(); keep {
		dim(r.emitter, jc.job, StageDone,
			"kept compressed ("+reason+"): "+filepath.Base(jc.finalInput))
		return
	}
	if err := os.Remove(jc.finalInput); err == nil {
		dim(r.emitter, jc.job, StageDone, "removed compressed temp")
	}
}

func (r *Runner) maybeDeleteSource(jc *jobContext) {
	if keep, reason := r.shouldKeepSource(); keep {
		info(r.emitter, jc.job, StageDone,
			"kept original ("+reason+"): "+filepath.Base(jc.videoPath))
		return
	}
	if err := os.Remove(jc.videoPath); err != nil {
		warn(r.emitter, jc.job, StageDone, "failed to delete original: "+err.Error())
		return
	}
	info(r.emitter, jc.job, StageDone, "deleted source "+filepath.Base(jc.videoPath))
}

// shouldKeepSource returns whether the original .mp4 must be preserved, and
// a human-readable reason suitable for a log line. Source deletion is the
// only irreversible action in the pipeline, so the reasons are ORed to fail
// safe: any one of them keeps the file.
func (r *Runner) shouldKeepSource() (bool, string) {
	switch {
	case r.cfg.KeepSource:
		return true, "--keep-source"
	case !r.cfg.Push:
		return true, "push skipped"
	case !r.cfg.Cleanup:
		return true, "cleanup skipped"
	}
	return false, ""
}

func (r *Runner) runCPU(ctx context.Context, fn func() error) error {
	return gateTracked(ctx, r.cpuSem, &r.cpuInUse, fn)
}

func (r *Runner) runNet(ctx context.Context, fn func() error) error {
	return gateTracked(ctx, r.netSem, &r.netInUse, fn)
}

func (r *Runner) runCPUWithResult(ctx context.Context, fn func() (string, error)) (string, error) {
	return gateResultTracked(ctx, r.cpuSem, &r.cpuInUse, fn)
}

// gateTracked is gate() plus an atomic counter so the TUI can show real-time
// occupancy. The counter only advances after Acquire succeeds, so cancelled
// or errored acquires don't pollute the gauge.
func gateTracked(ctx context.Context, sem *semaphore.Weighted, counter *atomic.Int32, fn func() error) error {
	if sem == nil {
		counter.Add(1)
		defer counter.Add(-1)
		return fn()
	}
	if err := sem.Acquire(ctx, 1); err != nil {
		return err
	}
	counter.Add(1)
	defer func() {
		counter.Add(-1)
		sem.Release(1)
	}()
	return fn()
}

func gateResultTracked[T any](ctx context.Context, sem *semaphore.Weighted, counter *atomic.Int32, fn func() (T, error)) (T, error) {
	if sem == nil {
		counter.Add(1)
		defer counter.Add(-1)
		return fn()
	}
	var zero T
	if err := sem.Acquire(ctx, 1); err != nil {
		return zero, err
	}
	counter.Add(1)
	defer func() {
		counter.Add(-1)
		sem.Release(1)
	}()
	return fn()
}

// Summary collects counts from results.
func Summary(results []Result) (ok, fail int) {
	for _, r := range results {
		if r.Success {
			ok++
		} else {
			fail++
		}
	}
	return
}
