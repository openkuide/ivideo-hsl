# Architecture

## Package dependency graph

The golden rule: **the pipeline core is free of UI and CLI concerns.**
`internal/pipeline` imports only the standard library and `internal/deps`.
The TUI, CLI, and settings packages all depend on the pipeline but not on
each other.

![Package dependency graph](assets/architecture_deps.svg)

> Source: [`assets/architecture_deps.puml`](assets/architecture_deps.puml)

No arrow points from `core` back into `ui` or `cli`. The TUI is an
optional consumer of `pipeline.Event`; the CLI is an optional driver.

## Per-video state machine

Every video is a state machine of eight states. Any failure short-circuits
to `failed`; the workspace is preserved for inspection and the source
`.mp4` is kept.

![Per-video state machine](assets/architecture_state.svg)

> Source: [`assets/architecture_state.puml`](assets/architecture_state.puml)

## Layered view

![Layered architecture view](assets/architecture_layers.svg)

> Source: [`assets/architecture_layers.puml`](assets/architecture_layers.puml)

## Package responsibilities

| Package | Responsibility |
|---|---|
| `cmd/ivideo-hls` | Flags, prereq checks, choosing TUI vs plain output, summary printing. |
| `internal/pipeline/config.go` | `Config`, quality/compression enums, defaults. |
| `internal/pipeline/events.go` | `Event`, `Emitter`, level helpers. Single log channel for TUI + plain modes. |
| `internal/pipeline/exec.go` | `run`, `runQuiet`, `runCapture` wrappers over `exec.CommandContext`. |
| `internal/pipeline/git.go` | Lock cleanup, remote config, branch pruning on base repo. |
| `internal/pipeline/workspace.go` | `hero_*` workspace lifecycle: clone, reset, cleanup. Mutex-guarded base prep. |
| `internal/pipeline/ffmpeg.go` | Pre-compression + HLS conversion + `.ts/.m3u8` rename step. |
| `internal/pipeline/processor.go` | `Runner` orchestrator — CPU/net semaphores, per-video pipeline. |
| `internal/tui/styles.go` | All Lipgloss color definitions. |
| `internal/tui/picker.go` | Two-screen selector: video checklist → configuration. |
| `internal/tui/runner.go` | Live run screen with progress bars, spinner, activity log. |

## Data flow (per video)

1. **Workspace** — clone base `hero/` into `hero_<sanitized_name>/`, set SSH remote.
2. **Branch** — `git checkout -B <name>` (force-reset if exists).
3. **Compress** (optional) — `libx264 preset=medium crf=28` → `<name>_compressed.mp4`.
4. **Convert** — HLS via `libx264 + aac`, segment `index_NNN.ts`, playlist `index.m3u8`.
5. **Rename** — `.ts → .married`, rewrite playlist, `.m3u8 → .single`.
6. **Commit & push** — `git add . && git commit -m "a" && git push -u -f origin <name>`.
7. **Cleanup** — remove `hero_*/`, delete original `.mp4`.

## Concurrency model

- `cpuSem` — `maxParallel` slots. Guards ffmpeg (compress + convert).
- `netSem` — `maxParallel * 2` slots. Guards `git push`.
- Workspace copy runs **outside** semaphores to overlap with ffmpeg work.
- `prepareBaseHero` is mutex-guarded; called once before parallel fan-out.

## Event surface

`pipeline.Event` carries `{Job, Stage, Level, Message, Percent, Speed, Bitrate}`.
`Percent` is parsed live from ffmpeg's `-progress pipe:1` stream (`out_time_ms`
over total duration from `ffprobe`). `Speed` and `Bitrate` ride along so the
TUI dashboard can show encoding speed per job without a separate channel.
Stage → per-job overall progress mapping lives in `stageRange`; `stageProgress`
is derived from it, so the fallback (no real percent available) and the
running-bar position can never drift. Adding a new stage = add a constant in
`events.go` + a range in `stageRange` + emit it from the processor.

## Run-screen layout

The run dashboard is a single live frame inspired by `htop` — no modal screens
after the pipeline starts. Sections are stacked between horizontal rules; the
key footer is always visible.

```
 ivideo-hls · processing · 3/8 done · 12:04 · ETA ~6m · → git@github.com:username/repo.git
 ───────────────────────────────────────────────────────────────────────────
  ████████████████████░░░░░░░░  56%  lesson-02            convert     2.1x  2800k
  ████████████░░░░░░░░░░░░░░░░  38%  lesson-05            compress    1.8x  1400k
  ██░░░░░░░░░░░░░░░░░░░░░░░░░░   8%  lesson-06            workspace
  ░░░░░░░░░░░░░░░░░░░░░░░░░░░░   0%  lesson-07            queued
 ───────────────────────────────────────────────────────────────────────────
 Done
  ✓ lesson-01              3m12s   pushed origin/lesson-01
  ✓ lesson-03              2m48s   pushed origin/lesson-03
  ✓ lesson-04              3m01s   pushed origin/lesson-04
 ───────────────────────────────────────────────────────────────────────────
 Log · tail
 12:04:13 [lesson-02] HLS convert @ medium / balanced
 12:04:10 [lesson-06] checkout -B lesson-06
 12:04:02 [lesson-05] compressed 12.4MB → 3.1MB (-74.8%)
 ───────────────────────────────────────────────────────────────────────────
  ctrl+c cancel  ·  q quit (after done)
```

Regions:

- **Header** — one line: status · counts · elapsed · ETA · remote.
- **Running** — active jobs with a bar, percent, name, stage badge, encoding
  speed (parsed live from ffmpeg's `-progress speed=` field), current bitrate.
- **Done** — completed (non-failed) jobs with duration and last message.
  Visible from the first success until quit.
- **Failures** — only rendered when a job fails; pins the full error.
- **Log tail** — bounded by terminal height: 3 lines on small (< 20 rows),
  6 on medium, 10 on large.
- **Footer** — constant key hint, changes only at `done`.

## Adding features

- **New quality preset:** add constant in `config.go`, branch in `ffmpeg.go`, label in `picker.go`.
- **New stage (e.g. thumbnail):** add `Stage*` constant, emit from processor, add to `stageProgress`.
- **Different remote strategy:** extend `git.go::configureRemoteOrigin`; expose via `--remote` flag (already wired).
