# Usage guide

## Table of Contents

- [First-run checklist](#first-run-checklist)
- [Interactive mode](#interactive-mode)
  - [Screen 1 — Video picker](#screen-1--video-picker)
  - [Screen 2 — Run config](#screen-2--run-config)
  - [Screen 3 — Run dashboard](#screen-3--run-dashboard)
- [Non-interactive (CI / batch)](#non-interactive-ci--batch)
- [Recovery commands](#recovery-commands)
- [Settings screen](#settings-screen)
- [Troubleshooting](#troubleshooting)

---

## First-run checklist

```
1. ffmpeg installed?      ffmpeg -version  (or run: ./ivideo-hls install-deps)
2. git identity set?      git config --get user.email
3. SSH auth works?        ssh -T git@github.com
4. hero/ base repo        exists in cwd
        │
        ▼
   ./ivideo-hls
```

Or run the built-in checker:

```bash
./ivideo-hls doctor
```

---

## Interactive mode

```bash
cd /path/with/videos
./ivideo-hls
```

### Screen 1 — Video picker

```
 ivideo-hls   HLS video pipeline ✦

┌───────────────────────────────────────────────────────────────┐
│ 📼 Videos in ~/Videos/2026-lessons  ·  flat  ·  11 types      │
│                                                               │
│ ▶ ● lesson-01.mp4        12.4 MB                              │
│   ● lesson-02.mp4        14.1 MB                              │
│   ○ lesson-03.mp4         9.8 MB                              │
│   ○ lesson-04.mp4        11.2 MB                              │
│                                                               │
│  2 / 4  selected · 4 shown                                    │
└───────────────────────────────────────────────────────────────┘
 ↑/↓ move · space toggle · a all · 1-9 first N · / filter · s settings · enter continue · q quit
```

| Key | Action |
|---|---|
| `↑` / `↓`, `j` / `k` | Move cursor |
| `space` | Toggle current video |
| `a` | Toggle all |
| `1`–`9` | Select first N videos |
| `g` / `G` | Jump to top / bottom |
| `/` | Filter by name |
| `s` | Open persistent settings |
| `enter` | Go to config screen |
| `esc` / `q` | Quit |

### Screen 2 — Run config

```
 ivideo-hls · run config ✦

  Parallel jobs   [ 2 ]    ←/→ adjust
  Quality         medium   ←/→ low · medium · high
  Compression     balanced ←/→ fast · balanced · best
  Pre-compress    [ off ]  space toggle
  Keep source     [ off ]  space toggle

  enter → start pipeline   esc → back
```

| Key | Action |
|---|---|
| `↑` / `↓` | Move between fields |
| `←` / `→` | Adjust value |
| `space` | Toggle boolean |
| `enter` | Start pipeline |
| `esc` | Back to picker |

Parallel jobs is capped at `[1, len(selectedVideos)]`.

### Screen 3 — Run dashboard

```
 ivideo-hls · processing · 3/8 done · 12:04 · ETA ~6m · → git@github.com:username/repo.git
 ─────────────────────────────────────────────────────────────────────────
  ████████████████████░░░░░░░░  56%  lesson-02          convert   2.1x  2800k
  ████████████░░░░░░░░░░░░░░░░  38%  lesson-05          compress  1.8x  1400k
  ██░░░░░░░░░░░░░░░░░░░░░░░░░░   8%  lesson-06          workspace
  ░░░░░░░░░░░░░░░░░░░░░░░░░░░░   0%  lesson-07          queued
 ─────────────────────────────────────────────────────────────────────────
 Done
  ✓ lesson-01              3m12s   pushed origin/lesson-01
  ✓ lesson-03              2m48s   pushed origin/lesson-03
 ─────────────────────────────────────────────────────────────────────────
 Log · tail
 12:04:13 [lesson-02] HLS convert @ medium / balanced
 12:04:10 [lesson-06] checkout -B lesson-06
 ─────────────────────────────────────────────────────────────────────────
  ctrl+c cancel  ·  q quit (after done)
```

- **Badge** — current stage: `workspace`, `compress`, `convert`, `rename`, `push`, `done`, `failed`
- **Progress bar** — filled by stage weight; percent live from ffmpeg `-progress` stream
- **Speed / bitrate** — encoding speed (`2.1x`) and current bitrate (`2800k`)
- **Activity log** — last 10 events, color-coded by level

`ctrl+c` cancels in-flight work. `q` exits after all jobs complete.

TUI flow spec: [`flows/tui/fs_tui_01_picker.md`](flows/tui/fs_tui_01_picker.md)

---

## Non-interactive (CI / batch)

```bash
# All videos, 4 parallel, pre-compress, high quality, plain logs
ivideo-hls -a -p -j 4 -q high --compress --no-tui
```

`--no-tui` prints plain `[job] message` lines. Exit `0` = all succeeded; exit `1` = any failure.

### Common recipes

```bash
# Single file, default quality
./ivideo-hls -i lesson-01.mp4

# All files in ./input/, 2 parallel
./ivideo-hls -a -p -j 2

# Dry-run: commit but don't push (inspect workspace first)
./ivideo-hls -a --no-push

# Keep source .mp4 after success
./ivideo-hls -a --keep-source

# Override remote for this run only
./ivideo-hls -a --remote git@github.com:org/other-repo.git

# CI pipeline — specific files, no TUI, fail fast
./ivideo-hls -i intro.mp4 -i outro.mp4 --no-tui
```

---

## Recovery commands

| Scenario | Command |
|---|---|
| Encoding done, push failed | `./ivideo-hls retry-failed` |
| ffmpeg died mid-encode | `./ivideo-hls resume-failed` |
| Not sure what's on disk | `./ivideo-hls doctor` |

See [PROCESS.md](PROCESS.md) for the full decision tree and step-by-step walkthrough.

---

## Settings screen

Press `s` on the picker or run `--settings`:

```bash
./ivideo-hls --settings
```

```
 ivideo-hls · settings ✦

┌───────────────────────────────────────────────────────────────┐
│ Remote                                                        │
│ ▶ Push URL          git@github.com:username/repo.git         │
│   Auth method       SSH  /  HTTPS + token                    │
│   Token             ••••••••  (from $IVIDEO_HLS_TOKEN)       │
│   Playback URL      https://raw.../{branch}/x/{filename}     │
│                                                               │
│ Source                                                        │
│   Default folder    ~/Videos                                  │
│   Recursive scan    ○ off                                     │
│                                                               │
│ Defaults                                                      │
│   Quality           medium                                    │
│   Compression       balanced                                  │
│   Pre-compress      ○ off                                     │
│   Keep source .mp4  ○ off                                     │
└───────────────────────────────────────────────────────────────┘
 ↑/↓ move · ←/→ adjust · space toggle · s save · t test · esc back
```

| Key | Action |
|---|---|
| `↑` / `↓` | Move between fields |
| `←` / `→` | Adjust value |
| `space` | Toggle boolean |
| `s` | Save to `~/.config/ivideo-hls/config.toml` |
| `t` | Test remote connection (`git ls-remote`, 10s timeout) |
| `d` | Reset field to default |
| `esc` | Back (prompts if unsaved) |

Settings spec: [`flows/config/fs_config_01_settings.md`](flows/config/fs_config_01_settings.md)

---

## Troubleshooting

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for the full guide.

Quick reference:

| Symptom | Fix |
|---|---|
| `git push rejected` | Bad SSH auth — `ssh-add ~/.ssh/id_ed25519` then `retry-failed` |
| `ffmpeg: not found` | `./ivideo-hls install-deps` |
| Workspace left after crash | By design — inspect `hero_<name>/`, then `resume-failed` |
| ffmpeg very slow | Reduce `-j` — CPU semaphore is the throttle |
| `doctor` shows ssh warning | SSH agent has no keys — run `ssh-add` |
