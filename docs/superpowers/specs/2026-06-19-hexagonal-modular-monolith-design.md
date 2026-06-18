# Hexagonal Modular Monolith — Design Spec

**Date:** 2026-06-19
**Status:** Approved
**Scope:** Full big-bang refactor of `ivideo-hls` from a flat `internal/pipeline` package into a strict hexagonal architecture with ports, adapters, and a clean application service layer.

---

## 1. Goals

- **Testability:** every application service is testable with fake ports — no real ffmpeg, git, or filesystem in unit tests.
- **Replaceability:** any infrastructure adapter (encoder, git host, manifest format) can be swapped by implementing its port interface, with no changes to domain or app layer.
- **Clarity:** each package has one clear responsibility, one set of dependencies, and can be understood without reading its consumers.

---

## 2. Architecture

### 2.1 Layers (strict dependency direction)

```
cmd/              →  adapters/primary  →  app/  →  ports/  ←  adapters/secondary/
                                          ↓
                                       domain/
```

- `domain/` imports nothing outside stdlib.
- `ports/` imports `domain/` only.
- `app/` imports `domain/` and `ports/` only.
- `adapters/secondary/` imports `ports/` and its own external lib.
- `adapters/primary/` imports `app/` only (never ports or adapters/secondary directly).
- `cmd/` imports `adapters/primary/` and wires everything in `main.go`.

No layer may import a layer above it. No two adapters may import each other.

### 2.2 Single Go module

One `go.mod` at the repo root (`github.com/chamrong/ivideo-hls`). Modularity is enforced by interface contracts and import discipline, not by separate module files. `go.work` is not used.

---

## 3. Package Layout

```
internal/
  domain/
    video/
      video.go        # Video, Episode, Quality, Compression value types
      scan.go         # ScanVideos — pure logic, accepts []fs.DirEntry
    job/
      job.go          # Job, Result, Stage enum
      events.go       # Event, Emitter interface
    settings/
      settings.go     # Settings — unified persistent+runtime config value type

  ports/
    encoding.go       # Encoder, Prober, Splitter interfaces
    publishing.go     # GitRepository, ManifestWriter interfaces
    recovery.go       # WorkspaceFinder interface
    config.go         # ConfigStore interface
    filesystem.go     # Workspace interface

  app/
    encoding_service.go   # compress → split → convert → rename
    publishing_service.go # commit → push → manifest
    recovery_service.go   # retry / resume
    config_service.go     # load / save / merge settings
    runner.go             # Runner — fan-out, semaphores, parallel/serial
    app.go                # App — composition root, wires all ports

  adapters/
    primary/
      cli/            # cobra commands — thin, calls *app.App
      tui/            # bubbletea TUI — thin, calls *app.App
    secondary/
      ffmpeg/         # implements ports.Encoder + ports.Splitter
      ffprobe/        # implements ports.Prober
      gitrepo/        # implements ports.GitRepository
      jsonconfig/     # implements ports.ConfigStore (setting.json)
      workspace/      # implements ports.Workspace
      manifest/       # implements ports.ManifestWriter (urls.json)
      scanner/        # video discovery wrapping domain/video/scan.go + real os.ReadDir

  testutil/
    fakes/
      encoder.go      # FakeEncoder
      prober.go       # FakeProber
      splitter.go     # FakeSplitter
      git.go          # FakeGitRepository
      manifest.go     # FakeManifestWriter
      workspace.go    # FakeWorkspace
      finder.go       # FakeWorkspaceFinder
      configstore.go  # FakeConfigStore

cmd/
  ivideo-hls/
    main.go           # builds *app.App with real adapters, hands off to cli adapter
```

---

## 4. Domain Layer

**`domain/video/video.go`**
```go
type Video struct {
    Path   string
    Name   string   // stem without extension
    Branch string   // git branch name derived from Name
}

type Episode struct {
    Path   string
    Suffix string   // "" for unsplit, "a"/"b"/… for split parts
}

type Quality    string  // "low" | "medium" | "high"
type Compression string // "fast" | "balanced" | "best"
```

**`domain/video/scan.go`**
`ScanVideos(entries []fs.DirEntry, root string, recursive bool) []Video`
Pure function — no `os.ReadDir` call inside. The adapter injects real entries; tests inject fake ones.

**`domain/job/job.go`**
```go
type Job struct{ ID, VideoPath, Branch string }
type Result struct{ Job Job; Err error }
type Stage string // compress | convert | commit | push | done

type IncompleteWorkspace struct {
    Name, Workspace, SourcePath, CompressedPath string
    Stage         Stage
    Hint          string
    SourceExists  bool
}

type RetryWorkspace struct {
    Name, Workspace, Branch string
}
```

**`domain/job/events.go`**
```go
type Emitter interface {
    Emit(Event)
}
// Event carries stage, job name, level (info/warn/error/dim), message.
```

**`domain/settings/settings.go`**
Merges `appconfig.File` and `pipeline.Config` into one canonical type. No file I/O — that lives in the `jsonconfig` adapter.

---

## 5. Ports

All interfaces live in `internal/ports/`. One file per capability.

### `encoding.go`
```go
type Encoder interface {
    Compress(ctx context.Context, video domain.Video, job string, e domain.Emitter) (compressedPath string, err error)
    ConvertToHLS(ctx context.Context, inputPath, outputDir string, cfg domain.Settings, job string, e domain.Emitter) error
    RenameHLSOutputs(outDir, job string, e domain.Emitter) error
}

type Prober interface {
    Duration(ctx context.Context, path string) (time.Duration, error)
    FileSize(path string) int64
}

type Splitter interface {
    Split(ctx context.Context, videoPath, job string, e domain.Emitter) ([]domain.Episode, error)
}
```

### `publishing.go`
```go
type GitRepository interface {
    Init(ctx context.Context, dir, remoteURL string) error
    CheckoutBranch(ctx context.Context, dir, branch string) error
    StageAndCommit(ctx context.Context, dir, message string) error
    ForcePush(ctx context.Context, dir, pushURL, branch string) error
}

type ManifestWriter interface {
    Record(ctx context.Context, sourceDir, branch string, hlsDirs []string) error
    WriteWorkspace(ctx context.Context, branch string, hlsDirs []string) error
}
```

### `recovery.go`
```go
type WorkspaceFinder interface {
    FindIncomplete(ctx context.Context, scriptDir string) ([]domain.IncompleteWorkspace, error)
    FindRetryReady(ctx context.Context, scriptDir string) ([]domain.RetryWorkspace, error)
}
```

### `config.go`
```go
type ConfigStore interface {
    Load() (domain.Settings, error)
    Save(domain.Settings) error
}
```

### `filesystem.go`
```go
type Workspace interface {
    Setup(ctx context.Context, video domain.Video, cfg domain.Settings, job string, e domain.Emitter) (workspaceDir string, err error)
    Cleanup(workspaceDir string, e domain.Emitter, job string)
    PrepareBase(ctx context.Context, cfg domain.Settings, e domain.Emitter) error
}
```

---

## 6. Application Services

All services live in `internal/app/`. They import `domain/` and `ports/` only — never adapters.

### `EncodingService`
Orchestrates: pre-compress → split decision → convert to HLS → rename outputs.
Returns `[]string` (hlsDirs — one per episode).

### `PublishingService`
Orchestrates: write workspace manifest → stage+commit → force-push → record to source manifest.

### `RecoveryService`
Orchestrates retry-failed and resume-failed flows. Delegates workspace discovery to `ports.WorkspaceFinder`.

### `ConfigService`
Wraps `ports.ConfigStore`. Merges flag/env/file/built-in precedence chain. Exposes `SaveRunConfig` for picker-confirmed choices.

### `Runner`
Fan-out engine. Holds CPU and network semaphores. Calls `EncodingService.Process` then `PublishingService.Publish` per video. Returns `[]domain.Result`.

### `App` — composition root
```go
type App struct {
    Encoding   *EncodingService
    Publishing *PublishingService
    Recovery   *RecoveryService
    Config     *ConfigService
    Runner     *Runner
}

func New(cfg domain.Settings, e domain.Emitter,
    enc   ports.Encoder,
    prober ports.Prober,
    split  ports.Splitter,
    git    ports.GitRepository,
    mw     ports.ManifestWriter,
    ws     ports.Workspace,
    finder ports.WorkspaceFinder,
    store  ports.ConfigStore,
) *App
```

`main.go` calls `app.New(...)` exactly once, injecting all real adapters.

---

## 7. Adapters

### Secondary (infrastructure)

| Package | Port satisfied | External dep |
|---|---|---|
| `ffmpeg/` | `ports.Encoder`, `ports.Splitter` | ffmpeg binary via `os/exec` |
| `ffprobe/` | `ports.Prober` | ffprobe binary via `os/exec` |
| `gitrepo/` | `ports.GitRepository` | git binary via `os/exec` |
| `jsonconfig/` | `ports.ConfigStore` | stdlib `encoding/json` |
| `workspace/` | `ports.Workspace` | stdlib `os` |
| `manifest/` | `ports.ManifestWriter` | stdlib `encoding/json` |
| `scanner/` | — (called by cli/tui) | stdlib `os` |

Each adapter exports exactly one struct (`Adapter`) with a `New(...)` constructor.
Each adapter file has a compile-time interface check:
```go
var _ ports.Encoder = (*Adapter)(nil)
```

### Primary (entry points)

**`adapters/primary/cli/`** — thin cobra command wrappers. Receives `*app.App`. No pipeline logic — only flag parsing, calling a service method, and printing results.

**`adapters/primary/tui/`** — bubbletea models. Receives `*app.App`. No pipeline logic — only UI state and rendering.

---

## 8. TDD Strategy

| Layer | Test type | Tool |
|---|---|---|
| `domain/` | Pure unit — no mocks needed | `go test` |
| `ports/` | Compile-time interface checks only | `var _ ports.X = (*adapter)(nil)` |
| `app/` | Unit — fake ports injected | `go test` + `testutil/fakes/` |
| `adapters/secondary/` | Integration — real binaries | `go test -tags=integration` |
| `adapters/primary/` | Unit — fake `*app.App` | `go test` |

**Fake ports** (`internal/testutil/fakes/`) are hand-written stubs — one per port interface. Each fake records calls and exposes a `XxxFn` field for per-test overrides:
```go
type FakeEncoder struct {
    CompressFn      func(ctx, video, job, emitter) (string, error)
    CompressCalls   []CompressCall
}
```

No mock generation frameworks. Fakes are explicit, readable, and do not drift from the interface because the compiler enforces it.

**Test file rule:** every `app/` service has a `_test.go` file written before the implementation (red → green → refactor per Uncle Bob TDD).

---

## 9. Migration Sequence (big-bang)

```
Phase 1 — Scaffold new structure (no deletions)
  1. internal/domain/...        pure types + pure logic
  2. internal/ports/...         interface definitions
  3. internal/testutil/fakes/   fake implementations
  4. internal/app/...           services + runner (RED tests first)
  5. All app/ unit tests GREEN

Phase 2 — Secondary adapters (one commit each)
  6.  adapters/secondary/jsonconfig/
  7.  adapters/secondary/ffprobe/
  8.  adapters/secondary/ffmpeg/
  9.  adapters/secondary/gitrepo/
  10. adapters/secondary/workspace/
  11. adapters/secondary/manifest/
  12. adapters/secondary/scanner/

Phase 3 — Primary adapters
  13. adapters/primary/cli/
  14. adapters/primary/tui/

Phase 4 — Wire and delete
  15. cmd/ivideo-hls/main.go — switch to app.New(...) with real adapters
  16. Delete internal/pipeline/
  17. Delete internal/appconfig/
  18. Delete internal/tui/
  19. go test ./... — all green
  20. go build ./... — binary verified
```

**Constraint:** the old `internal/pipeline` package is untouched until step 16. `main.go` imports it until step 15. The new code compiles alongside the old code throughout phases 1–3.

---

## 10. Files Deleted After Migration

| Path | Replaced by |
|---|---|
| `internal/pipeline/` (entire package) | `app/` + `adapters/secondary/ffmpeg,git,workspace,manifest,scanner/` |
| `internal/appconfig/` | `domain/settings/` + `adapters/secondary/jsonconfig/` |
| `internal/tui/` | `adapters/primary/tui/` |

`internal/deps/` and `internal/doctor/` are unchanged — they have no pipeline dependency.

---

## 11. Invariants

- No adapter imports another adapter.
- No domain type imports `ports/` or `app/`.
- No `app/` service imports an adapter package.
- `cmd/` imports `adapters/primary/cli`, `app/` (for `app.New`), and all `adapters/secondary/` packages — it is the only place allowed to import concrete adapters, solely to construct them for injection.
- Integration tests are always behind `//go:build integration` — `go test ./...` (no tag) runs only unit tests and always passes without ffmpeg/git installed.
