# Process — end-to-end lifecycle

Single source of truth for *what happens when you run ivideo-hls*. The README and PRD link here instead of duplicating it.

---

## Table of Contents

- [The headline: mp4 → HLS → git](#the-headline-mp4--hls--git)
- [Per-video state machine](#per-video-state-machine)
- [Three ways a batch starts](#three-ways-a-batch-starts)
- [Pick the right recovery command](#pick-the-right-recovery-command)
- [An example, step by step](#an-example-step-by-step)
- [Auto-split for large files](#auto-split-for-large-files)
- [Lifetime of _compressed.mp4](#lifetime-of-_compressedmp4)
- [In-progress filename discipline](#in-progress-filename-discipline)
- [Resume policy: reuse compressed (opt-in)](#resume-policy-reuse-compressed-opt-in)
- [What the tool will never do](#what-the-tool-will-never-do)
- [Related docs](#related-docs)

---

## The headline: mp4 → HLS → git

Each `.mp4` in `./input/` becomes a branch on your configured remote carrying the HLS playlist (`x/index.single`) and its segments (`x/*.married`). A player then fetches the playlist from the Playback URL (an HTTP(S) template you configure).

```
./input/lesson-01.mp4
       │
       ▼
  [ffmpeg] compress (optional) → convert → rename
       │
       ▼
  hero_lesson-01/x/
    index.single
    index_000.married  …
       │
       ▼
  git commit + force-push → origin/lesson-01
       │
       ▼
  HLS player fetches via Playback URL
```

Sequence diagram: [`flows/pipeline/assets/fs_pipeline_01_seq_hls_convert.puml`](flows/pipeline/assets/fs_pipeline_01_seq_hls_convert.puml)

---

## Per-video state machine

```
[queued]
    │
    ▼
[workspace]  — copy hero/ → hero_<name>/
    │
    ▼
[branch]     — git checkout -B <name>
    │
    ├── PreCompress on ──► [compress] ──► [convert]
    │
    └── PreCompress off ─────────────► [convert]
                                           │
                                           ▼
                                       [rename]  — .ts→.married / .m3u8→.single
                                           │
                                           ▼
                                       [push]    — commit + force-push
                                           │
                                           ▼
                                       [done]    — cleanup + delete source
```

Any stage → `[failed]`: workspace and source `.mp4` are preserved for recovery.

---

## Three ways a batch starts

### 1. Fresh batch — `./ivideo-hls`

Normal case. Scans `./input/`, picks videos via TUI, runs the full pipeline.

### 2. Continuing after success

Same command. Successfully processed videos are deleted from `./input/` on success, so only new drops appear.

### 3. Continuing after failures

The tool **never auto-recovers** on a normal run — surprise re-encodes of 30-minute videos are worse than making the operator type a verb.

Run `./ivideo-hls doctor` first:

```
!  pending retries          4 workspace(s) waiting: lesson-01, lesson-02, …
                             ↳ run `ivideo-hls retry-failed` to finish them
!  incomplete workspaces    1 stopped mid-pipeline: lesson-05 (convert)
                             ↳ run `ivideo-hls resume-failed` to delete and re-run from source
```

---

## Pick the right recovery command

```
[failure]
    │
    ▼
[doctor]
    │
    ├── x/index.single present? YES ──► retry-failed
    │     (encoding done, push failed)    force-push existing commit
    │                                     no re-encoding
    │
    └── x/index.single absent?
          │
          ├── source .mp4 on disk? YES ──► resume-failed
          │     (ffmpeg died mid-encode)    delete partial workspace
          │                                 re-run full pipeline
          │
          └── source .mp4 missing? ──► manual: restore source, then resume
```

**retry-failed** — fast path. Push what's already committed. No ffmpeg.

**resume-failed** — slow path. Delete partial state, re-run from source.

Sequence diagrams:
- [`flows/recovery/assets/fs_recovery_01_seq_retry.puml`](flows/recovery/assets/fs_recovery_01_seq_retry.puml)
- [`flows/recovery/assets/fs_recovery_02_seq_resume.puml`](flows/recovery/assets/fs_recovery_02_seq_resume.puml)

---

## An example, step by step

```bash
cp ~/Downloads/lesson-{01,02,03,04,05}.mp4 ./input/
./ivideo-hls -a -p -j 2
```

Lesson-03 push failed (bad PAT), lesson-04 SSH keys dropped, lesson-05 battery died mid-encode. On disk next morning:

```
./input/
  lesson-03.mp4   # kept — push failed
  lesson-04.mp4   # kept — push failed
  lesson-05.mp4   # kept — encode didn't finish

./hero_lesson-03/   # x/index.single present, commit ready
./hero_lesson-04/   # x/index.single present, commit ready
./hero_lesson-05/   # x/ has .ts files, no index.single
```

Recovery:

```bash
# 1. Diagnose
./ivideo-hls doctor
# → ssh agent warning; pending retries: 03, 04; incomplete: 05

# 2. Fix environment
ssh-add ~/.ssh/id_ed25519

# 3. Push what's ready
./ivideo-hls retry-failed

# 4. Re-run what never finished
./ivideo-hls resume-failed
```

After step 4: `./input/` is empty, all five branches on the remote, `doctor` is green.

---

## Auto-split for large files

Files **larger than 2 GB** are automatically split into equal-duration parts before encoding:

- Part count: `ceil(fileBytes / 2GB)`
- Split method: `ffmpeg -c copy` (stream-copy, no quality loss)
- Part suffix: `a`, `b`, `c`, … (files: `lesson-01a.mp4`, `lesson-01b.mp4`, …)
- Each part runs a full independent pipeline: own workspace, own branch (`lesson-01a`, `lesson-01b`, …), own commit+push
- Temp split files are deleted after each part's convert succeeds

See [`flows/pipeline/fs_pipeline_02_split.md`](flows/pipeline/fs_pipeline_02_split.md) for the full spec.

---

## Lifetime of `_compressed.mp4`

The compressed file survives until the **whole job succeeds end-to-end**. Any of these keep it alive:

| Condition | Effect |
|---|---|
| Push failed | File kept |
| `--no-push` set | File kept |
| `--no-cleanup` set | File kept |
| `--keep-source` set | File kept |

This mirrors the rule for the source `.mp4` and enables the opt-in **Reuse compressed on resume** policy.

---

## In-progress filename discipline

Each stage writes to a `.partial`-suffixed name first, then renames atomically on clean exit:

- Crash anywhere mid-stage → visible `.partial` file on disk
- `doctor` can see exactly which stage was interrupted without trusting tool state
- Clean outputs never share a filename with killed ones

Today only compress uses this (`<name>_compressed.partial.mp4`). Convert and rename use per-segment writes (ffmpeg) and `os.Rename` respectively.

---

## Resume policy: reuse compressed (opt-in)

`resume-failed` re-runs all stages by default. The setting **Reuse compressed on resume** (`resume_reuse_compressed`, default off) lets it skip the compress stage when **all** of these hold:

1. `_compressed.mp4` exists next to the source
2. File size > 1 KiB
3. `ffprobe` reports duration > 1 second
4. **No `_compressed.partial.mp4` sibling** (partial = last compress was killed)

When active: delete only the workspace, run pipeline with `PreCompress=false` and the compressed file as input.

Convert and rename are always re-run — segment output is never trusted across runs.

---

## What the tool will never do

| Behaviour | Reason |
|---|---|
| Silently resume on a normal run | Recovery is explicit — surprise re-encodes are worse |
| Resume mid-ffmpeg | No safe native resume for single-file outputs; partial segments risk corrupt playlists |
| Delete sources when recovery might still be possible | Source deleted only when push *and* cleanup both succeed on the same run |

---

## Related docs

- [`flows/pipeline/fs_pipeline_01_hls_convert.md`](flows/pipeline/fs_pipeline_01_hls_convert.md) — full pipeline stage spec
- [`flows/pipeline/fs_pipeline_02_split.md`](flows/pipeline/fs_pipeline_02_split.md) — auto-split spec
- [`flows/recovery/fs_recovery_01_retry_failed.md`](flows/recovery/fs_recovery_01_retry_failed.md) — retry-failed spec
- [`flows/recovery/fs_recovery_02_resume_failed.md`](flows/recovery/fs_recovery_02_resume_failed.md) — resume-failed spec
- [`PRD.md`](PRD.md) — functional + non-functional requirements
- [`../README.md`](../README.md#recovering-from-a-failed-run) — command-line reference
