# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

### Added
- Colored, emoji-annotated Mermaid diagrams in `docs/ARCHITECTURE.md`, `docs/PROCESS.md`, and `docs/USAGE.md`.
- `DEVELOPMENT.md`, `CONFIGURATION.md`, `TROUBLESHOOTING.md`, and `CHANGELOG.md` docs.
- Per-layer link colors in the package dependency diagram.

### Changed
- All architecture diagrams upgraded from plain Mermaid to C4-style with consistent color palette.
- `docs/USAGE.md` expanded with full keybinding tables, screen mockups, and recipe examples.

### Removed
- `plans/` directory.
- All Claude/Claude Code tooling references.

---

## [1.0.0] — Initial release

### Added
- Single Go binary CLI (`cmd/ivideo-hls`) built with Cobra.
- Bubble Tea TUI with two-screen picker (video checklist → config) and live run dashboard.
- Per-video pipeline: `workspace → branch → compress → convert → rename → push → cleanup`.
- Optional pre-compression pass (`libx264 crf=28`) before HLS segmentation.
- Quality presets: `low` (480p · 800k), `medium` (720p · 2.8M), `high` (1080p · 5M).
- Compression presets: `fast`, `balanced`, `best` (maps to ffmpeg `-preset`).
- Parallel execution with independent CPU semaphore (ffmpeg) and net semaphore (git push).
- `install-deps` — downloads pinned static ffmpeg/ffprobe builds; SHA-256 verified.
- `doctor` — read-only env diagnostics with ✓/!/✗ output and remediation hints.
- `retry-failed` — force-push workspaces that have a finalized playlist but unpushed commit.
- `resume-failed` — delete partial workspaces and re-run the full pipeline from source.
- Persistent config via `~/.config/ivideo-hls/config.toml` with TUI settings editor.
- Playback URL template (`{branch}`, `{filename}`) and per-directory `urls.txt` manifest.
- Token credential hygiene — secrets scrubbed from all log output.
- Recursive source directory scan with extension allowlist and pruned directories.
- `--no-push`, `--no-cleanup`, `--keep-source`, `--no-tui` escape hatches.
- Plain-log mode for CI (`--no-tui`) with exit code `0` / `1`.
