# Product Requirements Document — ivideo-hls

| | |
|---|---|
| **Status** | Living document (v1.1 — auto-split added) |
| **Owner** | ichamrong |
| **Last updated** | 2026-06-17 |
| **Audience** | Contributors, future maintainers |

---

## Table of Contents

- [1. Overview](#1-overview)
- [2. Problem statement](#2-problem-statement)
- [3. Goals / non-goals](#3-goals--non-goals)
- [4. Users & use cases](#4-users--use-cases)
- [5. Functional requirements](#5-functional-requirements)
- [6. Non-functional requirements](#6-non-functional-requirements)
- [7. Architecture summary](#7-architecture-summary)
- [8. Assumptions & constraints](#8-assumptions--constraints)
- [9. Open questions / future work](#9-open-questions--future-work)
- [10. References](#10-references)

---

## 1. Overview

`ivideo-hls` is a single-binary Go CLI that batch-converts local `.mp4` files into HLS (HTTP Live Streaming) outputs and ships each result to a git remote on a per-video branch. Files larger than 2 GB are automatically split into named episode parts and each part gets its own branch.

It is operated by a single user from the directory where raw videos live. It assumes a trusted environment: SSH-authenticated git remote, `ffmpeg` on `PATH`, and write access to the current working directory.

### System context

```
         ┌─────────────┐
         │   Operator  │  (interactive TUI or CI job)
         └──────┬──────┘
                │ flags / keystrokes
                ▼
    ┌───────────────────────┐
    │      ivideo-hls       │
    │   (single Go binary)  │
    └───┬────────┬─────────┬┘
        │        │         │
        ▼        ▼         ▼
   ┌────────┐ ┌─────┐ ┌────────────┐
   │ *.mp4  │ │ git │ │   ffmpeg   │
   │  cwd   │ │ SSH │ │  (PATH)    │
   └────────┘ └──┬──┘ └────────────┘
                 │
                 ▼
        ┌─────────────────┐
        │  remote: fm.git │
        │ branch per video│
        └─────────────────┘
```

---

## 2. Problem statement

Preparing a batch of long-form videos for HLS delivery involves repetitive, error-prone work:

- Running `ffmpeg` with the right codec, bitrate, and segmentation flags per quality tier.
- Renaming segments (`.ts → .married`, `.m3u8 → .single`) so the downstream CDN/player stack accepts them.
- Committing each video's output into an isolated git workspace and force-pushing to a branch named after the source file.
- Cleaning up intermediate state without clobbering sibling jobs.
- Doing all of the above for many files at once without melting the CPU or racing on the git index.
- Handling files over 2 GB that cannot be processed as a single unit.

---

## 3. Goals / non-goals

### Goals

- Convert one or many `.mp4` files to HLS in a single invocation.
- Offer both an interactive TUI (for humans) and a plain-log mode (for CI / logs).
- Cap concurrent ffmpeg jobs and git pushes independently.
- Automatically split files >2 GB into episode parts, each pushed to its own branch.
- Publish each converted video to its own branch via force-push, with a predictable branch name.
- Surface progress per job, per stage, with a unified event stream.
- Clean up intermediate workspaces and source files only on success.

### Non-goals

- Serving HLS output (no HTTP server, no CDN integration).
- Multi-bitrate / adaptive ladder output (single rendition per run).
- Encrypted HLS (AES-128, SAMPLE-AES).
- Cross-repo or multi-remote publishing in a single run.
- Windows support (POSIX shell + SSH assumptions throughout).

---

## 4. Users & use cases

**Primary user:** a solo operator preparing educational or product video content for a self-hosted HLS delivery stack backed by git.

**Primary use cases:**
1. "I dropped five new `.mp4`s in this folder — convert and publish all of them."
2. "Re-encode just `lesson-03.mp4` at higher quality and push again" (force-push semantics expected).
3. "Run the whole batch from a CI job with no terminal attached."
4. "This 4 GB lecture needs to go up — split it automatically into parts."

---

## 5. Functional requirements

### 5.1 Input discovery

- The CLI scans the source directory for video files (allowlist: `.mp4 .mov .m4v .mkv .webm .avi .3gp .3g2 .flv .wmv .ts`).
- The user may select a subset via the TUI checklist, via `-i <file>` (repeatable), or auto-select all via `-a`.
- `-r` / `--recursive` walks subdirectories; `.git`, `node_modules`, `hero*`, and hidden dirs are always pruned.

### 5.2 Configuration surface

See [`CONFIGURATION.md`](CONFIGURATION.md) for the full key reference.

Key flags:
- `-j N` / `-p` — max concurrent jobs (default: serial)
- `-q low|medium|high` — quality preset
- `-c fast|balanced|best` — compression preset
- `--compress` — pre-compression pass
- `--remote URL` — override push destination
- `--token STR` — HTTPS PAT
- `--no-push` — commit locally, skip push
- `--no-cleanup` — keep workspace after success
- `--keep-source` — skip source `.mp4` deletion

#### 5.2.1 Configuration precedence

```
CLI flag  >  env var ($IVIDEO_HLS_REMOTE / _TOKEN / _SOURCE)  >  config file  >  built-in default
```

### 5.3 Per-video pipeline

| # | Stage | Behavior |
|---|---|---|
| 1 | `workspace` | Copy `hero/` → `hero_<sanitized>/`; set origin; remove stale `.git/index.lock` |
| 2 | `branch` | `git checkout -B <videoBasename>` — always reset |
| 3 | `compress` *(opt)* | `libx264 crf=28` → `<name>_compressed.mp4` (atomic write via `.partial`) |
| 4 | `split` *(auto)* | Files >2 GB: stream-copy into parts a, b, c…; each part runs its own sub-job |
| 5 | `convert` | HLS via `libx264+aac`, segments `index_NNN.ts`, playlist `index.m3u8` |
| 6 | `rename` | `.ts → .married`; rewrite playlist; `.m3u8 → .single` |
| 7 | `push` | `git add . && git commit -m "a" && git push -u -f <pushURL> <branch>` |
| 8 | `cleanup` | Remove workspace; delete source `.mp4` (success only) |

**Branch naming:** `basename(video) − extension`. For split parts: `<base><suffix>` (e.g. `lesson-01a`).

**Rename step is invariant:** downstream players expect `.married` / `.single`.

**Force-push is invariant:** each branch is owned by one source video.

Full pipeline spec: [`flows/pipeline/fs_pipeline_01_hls_convert.md`](flows/pipeline/fs_pipeline_01_hls_convert.md)
Split spec: [`flows/pipeline/fs_pipeline_02_split.md`](flows/pipeline/fs_pipeline_02_split.md)

### 5.4 Concurrency

- `cpuSem`: `maxParallel` slots — guards ffmpeg (compress + convert)
- `netSem`: `maxParallel × 2` slots — guards `git push`
- Workspace clone: no semaphore (I/O overlaps encoding)
- `prepareBaseHero`: mutex, runs once before fan-out

### 5.5 Observability

- All pipeline output flows through `pipeline.Emitter` — no direct `fmt.Println` from pipeline code.
- Events carry `{Job, Stage, Level, Message, Percent}`.
- **TUI mode:** per-job progress bars, stage badge, last 10 activity lines.
- **Plain mode:** `[job] message` per line — suitable for CI logs.

### 5.6 Cancellation & exit codes

- `ctrl+c` cancels in-flight ffmpeg and git processes via `exec.CommandContext`.
- `q` in the TUI exits only after all jobs reach a terminal state.
- Exit `0` = every selected video succeeded. Any `failed` job → exit `1`.

### 5.7 Failure handling

- On failure: `hero_<sanitized>/` workspace and source `.mp4` are **preserved**.
- Failing job's stage and error are surfaced; other jobs continue.
- Recovery: `retry-failed` (push failed) or `resume-failed` (encode failed).

---

## 6. Non-functional requirements

- **Platform:** macOS and Linux. No Windows support.
- **Runtime deps:** `git` on PATH; `ffmpeg`+`ffprobe` (cache or PATH); SSH key or HTTPS+PAT for remote.
- **Dependency bootstrap:** `install-deps` downloads pinned static ffmpeg/ffprobe, verifies SHA-256 (committed in `internal/deps/sources.go`), installs to `$XDG_CACHE_HOME/ivideo-hls/bin/`.
- **Diagnostics:** `doctor` runs 13 read-only checks (binary presence, git, config perms, remote URL, auth consistency, token source, SSH keys, source dir, remote reachability, playback URL shape, pending retries). Exit 1 when any check fails; warnings pass.
- **Recovery (push failure):** `retry-failed` finds committed workspaces, force-pushes, cleans up.
- **Recovery (mid-encode failure):** `resume-failed` finds partial workspaces, deletes them, re-runs from source. Opt-in compress reuse via `resume_reuse_compressed`.
- **Credential hygiene:** tokens and `https://TOKEN@host` URLs scrubbed before any emit. Patterns: URL credentials and GitHub/GitLab PAT formats.
- **Build:** `go build ./...` → single static binary. Go 1.25+.
- **TUI performance:** redraws at Bubble Tea's default tick rate; per-event cost O(1) in emitter.
- **Resource bounds:** concurrent ffmpeg ≤ `maxParallel`; concurrent pushes ≤ `maxParallel × 2`.

---

## 7. Architecture summary

```
cmd/ivideo-hls  (Cobra · flag parsing · prereq checks)
       │
       ├── internal/tui  (Bubble Tea · picker · run dashboard · settings)
       │         │
       │         └── pipeline.Emitter  (events flow up)
       │
       └── internal/pipeline
                 ├── processor.go   — Runner orchestrator + semaphores
                 ├── ffmpeg.go      — compress / convert stages
                 ├── split.go       — auto-split >2GB
                 ├── manifest.go    — urls.txt writer (multi-episode aware)
                 ├── retry.go       — retry-failed + resume-failed
                 ├── redact.go      — credential scrubbing
                 └── …             — config, events, exec, git, scan, workspace
```

Event flow:
```
pipeline stage → Emitter.Emit(Event)
                      │
            ┌─────────┴─────────┐
            ▼                   ▼
     TUI run screen        plain writer
     (progress bars)       "[job] message"
```

Full diagram: [`ARCHITECTURE.md`](ARCHITECTURE.md)

---

## 8. Assumptions & constraints

- A `hero/` base repo exists in the working directory — sacred template; only `prepareBaseHero` may write to it.
- The git remote accepts force-pushes to any branch.
- Source `.mp4` files are owned by the user — deletion on success is intentional.
- Colors are centralized in `internal/tui/styles.go`; inline `lipgloss.Color(...)` elsewhere is a bug.

---

## 9. Open questions / future work

- **Adaptive bitrate ladder:** 480p/720p/1080p variants with a master playlist.
- **Thumbnail generation:** `thumbnail` stage after convert.
- **Windows support:** would require rewriting workspace copy and SSH assumptions.
- **Broader test coverage:** fake-exec harness for `processor.go`; table-driven tests for `ffmpeg.go` arg construction.

---

## 10. References

- [`README.md`](../README.md) — quick-start and flag table
- [`docs/ARCHITECTURE.md`](ARCHITECTURE.md) — package layout and data flow
- [`docs/CONFIGURATION.md`](CONFIGURATION.md) — all config keys, env vars, precedence
- [`docs/USAGE.md`](USAGE.md) — operator guide and keybindings
- [`docs/PROCESS.md`](PROCESS.md) — end-to-end lifecycle and recovery
- [`docs/flows/pipeline/fs_pipeline_01_hls_convert.md`](flows/pipeline/fs_pipeline_01_hls_convert.md) — pipeline spec
- [`docs/flows/pipeline/fs_pipeline_02_split.md`](flows/pipeline/fs_pipeline_02_split.md) — split spec
- [`docs/flows/tui/fs_tui_01_picker.md`](flows/tui/fs_tui_01_picker.md) — TUI picker spec
- [`docs/flows/config/fs_config_01_settings.md`](flows/config/fs_config_01_settings.md) — settings spec
- [`docs/flows/recovery/fs_recovery_01_retry_failed.md`](flows/recovery/fs_recovery_01_retry_failed.md) — retry spec
- [`docs/flows/recovery/fs_recovery_02_resume_failed.md`](flows/recovery/fs_recovery_02_resume_failed.md) — resume spec
