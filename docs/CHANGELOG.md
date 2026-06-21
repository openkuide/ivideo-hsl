# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.0.0] - 2026-06-21

### Added
- **Core Engine**: Single Go binary CLI (`cmd/ivideo-hls`) built using Cobra.
- **Interactive UI**: Multi-screen Bubble Tea TUI featuring:
  - An interactive video picker checklist with quick-select shortcuts.
  - A persistent configuration editor.
  - A real-time run dashboard with job progress bars, compression ratios, and active logging.
- **Pipeline Architecture**: Per-video execution pipeline: `workspace` ➔ `branch` ➔ `compress` ➔ `convert` ➔ `rename` ➔ `push` ➔ `cleanup`.
- **Media Processing**:
  - Optional pre-compression pass (using `libx264` at `crf=28`) to optimize size.
  - Streamlined HLS segmentation with customizable quality presets: `low` (480p), `medium` (720p), and `high` (1080p).
  - Configurable encoding speed presets: `fast`, `balanced`, and `best`.
  - Multi-job concurrency with separate semaphores for video encoding (CPU-bound) and git pushing (network-bound).
- **Git Publishing**: Automatic branch isolation—commits and pushes HLS outputs to dedicated video-specific Git branches.
- **Recovery Utilities**:
  - `doctor`: Diagnoses system dependencies (`ffmpeg`, `ffprobe`, `git`) and SSH/token credentials.
  - `retry-failed`: Re-attempts pushing finished workspaces on network failures.
  - `resume-failed`: Cleans and restarts interrupted conversion pipelines from source.
  - `recover`: A unified CLI command to triage and run both retry and resume phases sequentially.
- **CI/CD & Scripting Support**:
  - Non-interactive mode (`--no-tui`) with standard log streams and status exit codes.
  - Overrides for custom Git remotes, HTTPS authentication tokens, and directories.
  - Safe token scrubbing from console outputs and logs.
- **Documentation**:
  - Complete technical specs: `ARCHITECTURE.md` (featuring C4-style Mermaid diagrams), `FUNCTIONAL_SPEC.md`, and `PROCESS.md`.
  - User and developer guides: `USAGE.md`, `CONFIGURATION.md`, `TROUBLESHOOTING.md`, and `DEVELOPMENT.md`.

