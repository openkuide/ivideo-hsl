# Development guide

Everything you need to build, run, test, and extend ivideo-hls.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Build](#build)
- [Project layout](#project-layout)
- [Architecture at a glance](#architecture-at-a-glance)
- [Running locally](#running-locally)
- [Testing](#testing)
- [Adding features](#adding-features)
- [Code conventions](#code-conventions)
- [Commit style](#commit-style)
- [Related docs](#related-docs)

---

## Prerequisites

| Tool | Minimum version | Install |
|---|---|---|
| Go | 1.25 | [go.dev/dl](https://go.dev/dl/) |
| git | any recent | `brew install git` / `apt install git` |
| ffmpeg + ffprobe | 6.x+ | `./ivideo-hls install-deps` or system |

---

## Build

```bash
# Clone
git clone git@github.com:ichamrong/ivideo-hls.git
cd ivideo-hls

# Build binary
go build -o ivideo-hls ./cmd/ivideo-hls

# Install ffmpeg/ffprobe into the local cache (first time only)
./ivideo-hls install-deps

# Verify environment
./ivideo-hls doctor
```

---

## Project layout

```
cmd/
  ivideo-hls/         ← CLI entry point (Cobra · flag wiring · dispatch)

internal/
  appconfig/          ← load · save · validate config.toml
  deps/               ← resolve ffmpeg/ffprobe binary paths
  doctor/             ← read-only environment diagnostics
  pipeline/           ← domain core (must not import tui or cli)
    config.go         ← Config struct · quality/compression enums
    events.go         ← Event · Emitter interface · level helpers
    exec.go           ← run / runQuiet / runCapture wrappers
    ffmpeg.go         ← compress · HLS convert · rename stages
    ffprobe.go        ← duration probe
    git.go            ← lock cleanup · remote config · branch pruning
    manifest.go       ← urls.txt writer (multi-episode aware)
    processor.go      ← Runner orchestrator · CPU/net semaphores
    redact.go         ← credential scrubbing patterns
    retry.go          ← retry-failed + resume-failed logic
    scan.go           ← video discovery · extension allowlist
    split.go          ← auto-split for files >2GB
    workspace.go      ← hero_* lifecycle · clone · reset · cleanup
  tui/
    picker.go         ← two-screen video checklist + config selector
    runner.go         ← live run dashboard (progress bars · log tail)
    settings.go       ← persistent config TUI editor
    styles.go         ← ALL Lipgloss color definitions (single source of truth)

docs/
  flows/
    pipeline/         ← fs_pipeline_*.md + assets/*.puml (HLS + split specs)
    tui/              ← fs_tui_*.md + assets/*.puml
    recovery/         ← fs_recovery_*.md + assets/*.puml
    config/           ← fs_config_*.md + assets/*.puml
  ARCHITECTURE.md
  CONFIGURATION.md
  DEVELOPMENT.md  ← this file
  PROCESS.md
  TROUBLESHOOTING.md
  USAGE.md
```

### Golden rule

> **`internal/pipeline` must never import `internal/tui` or `cmd/`.**

All pipeline output flows through `pipeline.Emitter`. The TUI is a consumer of events, not the other way around.

---

## Architecture at a glance

```
cmd/ivideo-hls  (Cobra · flag parsing · prereq checks)
       │
       ├── internal/tui  (Bubble Tea picker + run dashboard)
       │         │
       │         └── pipeline.Emitter  ← events flow up
       │
       └── internal/pipeline  (Runner · stages · semaphores)
                 │
                 ├── ffmpeg  (compress · convert · split)
                 ├── git     (workspace · branch · push)
                 └── fs      (manifest · scan · redact)
```

Full colored diagram: [`ARCHITECTURE.md`](ARCHITECTURE.md)

---

## Running locally

```bash
# Drop test videos into input/
mkdir -p input
cp ~/Downloads/test.mp4 input/

# Interactive TUI
./ivideo-hls

# Non-interactive (plain logs)
./ivideo-hls -a --no-tui

# Diagnose environment
./ivideo-hls doctor
```

---

## Testing

```bash
go test ./...
```

Existing test coverage:
- `internal/pipeline/ffprobe_test.go` — duration probe
- `internal/pipeline/parallel_test.go` — semaphore accounting
- `internal/pipeline/redact_test.go` — credential scrubbing
- `internal/pipeline/scan_test.go` — extension allowlist, recursive pruning
- `internal/pipeline/reuse_test.go` — reuse-compressed policy
- `internal/pipeline/finalize_test.go` — cleanup conditions
- `internal/pipeline/manifest_test.go` — multi-episode urls.txt writes
- `internal/pipeline/split_test.go` — auto-split threshold and suffix logic

Priority areas for new tests:
- `internal/pipeline/ffmpeg.go` — `compressArgs`, `hlsArgs`, `settingsFor`
- `internal/pipeline/processor.go` — full pipeline with fake-exec harness
- `internal/appconfig/` — TOML round-trip, precedence

---

## Adding features

### New quality preset

1. Add a constant in `internal/pipeline/config.go`.
2. Add a branch in `settingsFor()` in `internal/pipeline/ffmpeg.go`.
3. Add a label in `internal/tui/picker.go` config screen.

### New pipeline stage (e.g. thumbnail)

1. Add a `Stage*` constant in `internal/pipeline/events.go`.
2. Emit events from `internal/pipeline/processor.go`.
3. Add the stage to `stageProgress` so the progress bar covers it.
4. Add a weight to `stageRange` so the percentage math stays consistent.
5. Write a flow spec in `docs/flows/pipeline/`.

### New flag

1. Define the flag in `cmd/ivideo-hls/main.go`.
2. Wire it into `pipeline.Config` if it affects the pipeline.
3. Add a TOML key + env var if it should be persistent — update `internal/appconfig/`.
4. Document it in [`CONFIGURATION.md`](CONFIGURATION.md) and the flag table in [`../README.md`](../README.md).

### New color / style

> **All colors live in `internal/tui/styles.go`.**

Do not use `lipgloss.Color(...)` inline anywhere else — that is a bug. Add a new exported variable to `styles.go` and reference it from the component.

---

## Code conventions

| Rule | Detail |
|---|---|
| No `fmt.Println` in pipeline | All output via `pipeline.Emitter.Emit()` |
| Styles in one place | `internal/tui/styles.go` only |
| Atomic file writes | Write to `.partial`, rename on clean exit |
| Semaphore discipline | ffmpeg → `cpuSem`; git push → `netSem`; workspace clone → neither |
| Token hygiene | Scrub secrets before any `Emit()` call — see `redact.go` |
| Branch names | `basename(video) − ".mp4"` — invariant, no sanitization |
| Split branch names | `<base><suffix>` e.g. `lesson-01a` — suffix is the part letter |
| Rename step | `.ts → .married`, `.m3u8 → .single` — invariant, paired downstream |

---

## Commit style

```
type(scope): short description

# Types: feat · fix · docs · chore · refactor · test · perf
# Scope: pipeline · tui · cli · deps · appconfig · doctor · docs
```

Examples:
```
feat(pipeline): add thumbnail stage after convert
fix(tui): prevent spinner flicker on rapid events
docs: expand USAGE.md with settings screen reference
chore: bump go.mod to 1.25
```

---

## Related docs

- [`ARCHITECTURE.md`](ARCHITECTURE.md) — package layout, invariants, event surface
- [`CONFIGURATION.md`](CONFIGURATION.md) — all config keys, env vars, precedence
- [`PROCESS.md`](PROCESS.md) — end-to-end lifecycle and recovery decision tree
- [`flows/pipeline/fs_pipeline_01_hls_convert.md`](flows/pipeline/fs_pipeline_01_hls_convert.md) — pipeline stage spec
- [`flows/pipeline/fs_pipeline_02_split.md`](flows/pipeline/fs_pipeline_02_split.md) — auto-split spec
