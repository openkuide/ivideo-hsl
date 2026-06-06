# Contributing to ivideo-hls

Thank you for your interest in contributing to `ivideo-hls`! This document provides guidelines and conventions to help you set up your development environment, understand the codebase, and write code that matches our standards.

---

## Codebase Layout

```
cmd/ivideo-hls/   # CLI command definitions (Cobra)
internal/appconfig # Persistent app settings (TOM-ish config parser)
internal/deps      # Pinned static dependencies manager (ffmpeg, ffprobe)
internal/doctor    # Environment diagnostic checks
internal/pipeline  # Core logic (Workspace, FFmpeg encoding, Git pushes)
internal/tui       # Bubble Tea pickers, dashboard, settings screens
docs/              # Product and architectural documentation
scripts/           # Helper scripts (such as git-hook installers)
```

---

## Coding Conventions & Standards

To keep the codebase maintainable and robust, please adhere to the following standards:

### 1. Language & Version
- **Go 1.25+** is required.
- Do not add external dependencies unless absolutely necessary. We prefer standard library packages where possible to keep the binary lightweight.

### 2. Error Handling
- Wrap errors using `fmt.Errorf("context: %w", err)` to preserve the original error chain.
- Do not use third-party packages like `pkg/errors` for error wrapping.

### 3. Concurrency Discipline
- Always use `golang.org/x/sync/errgroup` and `semaphore.Weighted` for concurrency management.
- CPU-bound tasks (like ffmpeg conversions) must respect the maximum concurrent jobs setting (`Runner.runCPU`).
- Network-bound tasks (like git pushes) must route through the push pool (`Runner.runNet`).

### 4. Logging & Output
- **Never write directly to `stdout` or `stderr`** from inside the core logic (`internal/pipeline/`). 
- All output must flow through the `pipeline.Emitter` interface. This allows the CLI to dynamically support both the Bubble Tea interactive TUI dashboard and a plain-log output mode.

### 5. Filesystem Boundaries
- Never mutate the baseline `hero` directory directly. All operations must go through the temporary workspace preparation routines (`prepareBaseHero`).
- Workspace directories prefixed with `hero_*` are disposable and should be kept clean.

### 6. Terminal User Interface (TUI)
- All Bubble Tea styling (colors, layout borders, text formats) must use definitions centralized in `internal/tui/styles.go`.
- Avoid writing inline `lipgloss.Color("...")` elsewhere in other TUI files to keep themes uniform and adjustable in one place.

---

## Getting Started

### Prerequisites
1. Install **Go 1.25+**.
2. Install **git**.
3. Install **ffmpeg** and **ffprobe** (you can boot-strap this locally inside your cache by running `./ivideo-hls install-deps` once you build the binary).

### Development Setup

1. **Clone the repository:**
   ```bash
   git clone git@github.com:username/repo.git
   cd ivideo-hls
   ```

2. **Build the binary:**
   ```bash
   go build -o ivideo-hls ./cmd/ivideo-hls
   ```

3. **Install pinned dependencies to cache:**
   ```bash
   ./ivideo-hls install-deps
   ```

4. **Verify your environment setup:**
   ```bash
   ./ivideo-hls doctor
   ```

5. **Run the TUI scanner:**
   ```bash
   ./ivideo-hls
   ```

---

## Testing & Quality Checks

Before submitting a Pull Request, please run the verification suite:

- **Run all unit tests:**
  ```bash
  go test ./...
  ```
- **Vet code formatting and style:**
  ```bash
  go fmt ./...
  go vet ./...
  ```

---

## Submitting Pull Requests

1. Create a new topic branch from `main`:
   ```bash
   git checkout -b feature/my-new-feature
   ```
2. Make your changes and write clear, atomic commits.
3. Ensure that your changes do not break the main playlist segment renames (`.ts` to `.married` and `.m3u8` to `.single`) as downstream media players rely on this behavior.
4. Verify your tests pass and the binary builds cleanly.
5. Push to your branch and submit a PR with a description of what the change solves and how it was tested.
