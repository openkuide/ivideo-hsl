# Development guide

Everything you need to build, run, test, and extend ivideo-hls.

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
git clone git@github.com:iblogger855/ivideo-hsl.git
cd ivideo-hsl

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
    git.go            ← lock cleanup · remote config · branch pruning
    manifest.go       ← urls.txt writer
    processor.go      ← Runner orchestrator · CPU/net semaphores
    scan.go           ← video discovery · extension allowlist
    workspace.go      ← hero_* lifecycle · clone · reset · cleanup
  tui/
    picker.go         ← two-screen video checklist + config selector
    runner.go         ← live run dashboard (progress bars · log tail)
    settings.go       ← persistent config TUI editor
    styles.go         ← ALL Lipgloss color definitions (single source of truth)

docs/                 ← architecture · PRD · process · usage · this file
scripts/              ← install-hooks.sh · other dev helpers
```

### Golden rule

> **`internal/pipeline` must never import `internal/tui` or `cmd/`.**

All pipeline output flows through `pipeline.Emitter`. The TUI is a consumer of events, not the other way around. Violating this breaks the CI/plain-log path and the concurrency model.

---

## Architecture at a glance

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full colored diagram. High-level:

```mermaid
graph LR
    cli["🟣 cmd/ivideo-hls"]
    tui["🔵 internal/tui"]
    core["🟢 internal/pipeline"]
    libs["🟠 appconfig · deps · doctor"]

    cli --> tui
    cli --> core
    tui --> core
    core --> libs

    classDef cliStyle  fill:#EDE9FE,stroke:#7C3AED,stroke-width:2px,color:#3B0764;
    classDef uiStyle   fill:#E0F2FE,stroke:#0EA5E9,stroke-width:2px,color:#0C4A6E;
    classDef coreStyle fill:#D1FAE5,stroke:#059669,stroke-width:2px,color:#064E3B;
    classDef libStyle  fill:#FEF3C7,stroke:#D97706,stroke-width:2px,color:#78350F;
    class cli cliStyle;
    class tui uiStyle;
    class core coreStyle;
    class libs libStyle;
```

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

> **Note:** The test suite is minimal today — `go test ./...` runs clean but coverage is thin. A table-driven test for `ffmpeg.go` argument construction and a fake-exec harness for `processor.go` are the first targets for contributors.

Useful areas to cover:
- `internal/pipeline/ffmpeg.go` — `compressArgs`, `hlsArgs`, `settingsFor`
- `internal/pipeline/scan.go` — extension allowlist, recursive pruning
- `internal/pipeline/processor.go` — semaphore accounting (fake exec)
- `internal/appconfig/` — TOML round-trip, precedence

---

## Adding features

### New quality preset

1. Add a constant in `internal/pipeline/config.go`.
2. Add a branch in `settingsFor()` in `internal/pipeline/ffmpeg.go`.
3. Add a label in `internal/tui/picker.go` config screen.

### New stage (e.g. thumbnail)

1. Add a `Stage*` constant in `internal/pipeline/events.go`.
2. Emit events from `internal/pipeline/processor.go`.
3. Add the stage to `stageProgress` so the progress bar covers it.
4. Add a weight to `stageRange` so the percentage math stays consistent.

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
| Token hygiene | Scrub secrets before any `Emit()` call |
| Branch names | `basename(video) − ".mp4"` — invariant, no sanitization |
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

- [ARCHITECTURE.md](ARCHITECTURE.md) — package layout, invariants, event surface.
- [CONFIGURATION.md](CONFIGURATION.md) — all config keys, env vars, precedence.
- [PROCESS.md](PROCESS.md) — end-to-end lifecycle and recovery decision tree.
- [CONTRIBUTING.md](../CONTRIBUTING.md) — contribution guidelines and PR process.
