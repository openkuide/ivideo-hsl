package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sync/semaphore"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

// Runner fans out encoding and publishing across a list of videos,
// bounded by a semaphore of size maxParallel.
type Runner struct {
	encoding    *EncodingService
	publishing  *PublishingService
	maxParallel int
}

// NewRunner constructs a Runner. maxParallel <= 1 runs serially.
func NewRunner(enc *EncodingService, pub *PublishingService, maxParallel int) *Runner {
	return &Runner{encoding: enc, publishing: pub, maxParallel: maxParallel}
}

// Run fans out processing across videos using a semaphore and returns one
// job.Result per video.  cfg and e are per-run parameters so they can vary
// between calls on the same Runner.
func (r *Runner) Run(ctx context.Context, videos []video.Video, cfg settings.Settings, e job.Emitter) []job.Result {
	results := make([]job.Result, 0, len(videos))
	var mu sync.Mutex

	addResult := func(res job.Result) {
		mu.Lock()
		results = append(results, res)
		mu.Unlock()
	}

	if r.maxParallel <= 1 {
		// Serial path — no goroutines, simpler stack traces.
		for _, v := range videos {
			addResult(r.processOne(ctx, v, cfg, e))
		}
		return results
	}

	// Parallel path — bounded by semaphore.
	sem := semaphore.NewWeighted(int64(r.maxParallel))
	var wg sync.WaitGroup

	for _, v := range videos {
		v := v // capture loop variable
		if err := sem.Acquire(ctx, 1); err != nil {
			// Context cancelled; record failure for remaining videos.
			addResult(job.Result{VideoPath: v.Path, Success: false, Err: err})
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer sem.Release(1)
			addResult(r.processOne(ctx, v, cfg, e))
		}()
	}

	wg.Wait()
	return results
}

// processOne runs encoding then publishing for a single video and returns its Result.
func (r *Runner) processOne(ctx context.Context, v video.Video, cfg settings.Settings, e job.Emitter) job.Result {
	job.Emit(e, job.LevelInfo, v.Name, job.StageQueued, "starting "+filepath.Base(v.Path))

	wsDir, hlsDirs, err := r.encoding.Process(ctx, v, cfg, v.Name, e)
	// doCleanup is set to true only when it is safe to discard the workspace:
	// on encoding failure (workspace is incomplete) or full success (workspace
	// is committed and pushed). On push failure the workspace is retry-ready
	// and must be preserved for `ivideo-hls recover`.
	doCleanup := false
	if wsDir != "" {
		defer func() {
			if doCleanup {
				r.encoding.CleanupWorkspace(wsDir, cfg, e, v.Name)
			}
		}()
	}
	if err != nil {
		doCleanup = true // incomplete workspace — discard
		job.Emit(e, job.LevelError, v.Name, job.StageFailed, err.Error())
		return job.Result{VideoPath: v.Path, Success: false, Err: err}
	}

	pushURL := ""
	if cfg.Push {
		pushURL = cfg.PushURL
	}

	if err := r.publishing.Publish(ctx, v.Path, wsDir, v.Branch, pushURL, hlsDirs, v.Name, e); err != nil {
		// Push failed — preserve workspace so `recover` can push later.
		job.Emit(e, job.LevelError, v.Name, job.StageFailed, err.Error())
		return job.Result{VideoPath: v.Path, Success: false, Err: err}
	}

	doCleanup = true // committed and pushed — workspace no longer needed

	if !cfg.KeepSource {
		for _, p := range []string{v.Path, precompressedPath(v.Path)} {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				job.Emit(e, job.LevelWarn, v.Name, job.StageDone, "could not remove "+filepath.Base(p)+": "+err.Error())
			}
		}
	}

	job.Emit(e, job.LevelSuccess, v.Name, job.StageDone, "complete")
	return job.Result{VideoPath: v.Path, Success: true}
}

// Summary counts successes and failures in a result slice.
func Summary(results []job.Result) (ok, fail int) {
	for _, r := range results {
		if r.Success {
			ok++
		} else {
			fail++
		}
	}
	return
}
