# Functional Specification — ivideo-hls

This document specifies the exact, code-verified operational requirements and execution logic of the `ivideo-hls` utility. It acts as the final, binding specification defining how inputs are processed, how state transitions are managed, and how the file system and external systems are mutated.

---

## 1. Document Control & Administration

### 1.1 Document Metadata
*   **Document ID**: FS-IVIDEO-HLS-001
*   **Classification**: Technical / Operational Specification
*   **Status**: Approved / Code-Verified
*   **Version**: 1.2
*   **Effective Date**: 2026-06-06
*   **Author**: Antigravity (Advanced Agentic Coding Team)

### 1.2 Revision History

| Version | Date | Author | Description of Change |
|---|---|---|---|
| 1.0 | 2026-06-05 | Core Dev | Initial landing of baseline system specifications. |
| 1.1 | 2026-06-06 | Antigravity | Code-verified alignment with picker, runner, settings, pipeline, and doctor logic. |
| 1.2 | 2026-06-06 | Antigravity | Converted to formal compliance and legal specification utilizing RFC 2119 requirement levels. |

### 1.3 Requirement Levels (RFC 2119)
The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**, **SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **MAY**, and **OPTIONAL** in this document are to be interpreted as described in [RFC 2119](https://www.ietf.org/rfc/rfc2119.txt).

---

## 2. Legal Notices & Compliance

### 2.1 Proprietary Rights & Licensing
The software logic described herein **SHALL** be governed by the project's license (MIT License). All structural modifications, layouts, and scripts remain the property of their respective contributors. 

### 2.2 Disclaimer of Warranty & Limitation of Liability
THE SOFTWARE SPECIFIED IN THIS DOCUMENT IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, AND NONINFRINGEMENT. IN NO EVENT **SHALL** THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES, OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT, OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

### 2.3 Regulatory & Standards Compliance
*   **HTTP Live Streaming (HLS)**: Playlist file outputs (`.m3u8` renamed to `.single`) and media segments (`.ts` renamed to `.married`) **SHALL** conform to the packaging recommendations defined in **RFC 8216**.
*   **Filesystem Security**: Persistent configuration storage **SHALL** restrict read and write access to the owner only (permissions `0600`) to prevent local privilege escalation or credential exposure.
*   **Transport Encryption**: Git pushes **SHALL** authenticate using SSH keys (leveraging `ssh-agent`) or secure HTTPS tokens (PATs) over TLS 1.3 or equivalent secure channels.

---

## 3. Executive Glossary

*   **Source Video**: The original input media file (typically `.mp4`) to be segmented into HLS format.
*   **Workspace**: A temporary subdirectory (prefixed with `hero_`) dedicated to a single job containing a standalone cloned git repository initialized to perform HLS encoding and commits.
*   **Base Hero**: The master templates directory (`hero/`) containing baseline repository configurations, git settings, and assets copied to spin up per-video workspaces.
*   **Segment Playlist (.single)**: The renamed output HLS playlist (`index.single`) where `.ts` references have been rewritten to `.married`.
*   **Segment File (.married)**: Individual HLS segment media files containing compressed stream fragments, renamed from `.ts` to `.married`.
*   **Push URL**: The credential-bearing target URL (containing tokens/usernames) used for git push authentication, kept separate from the remote config to protect credentials.
*   **Public URL Pattern**: A string template (e.g. `https://cdn.example.com/{branch}/x/{filename}`) used to map segment branch deployments to public CDN urls.

---

## 4. Module Specifications

### Group A: Configuration & Security

#### Module A.1: Persistent Settings (config.toml)
*   **Description**: Load and store user preferences locally to eliminate manual command-line argument redundancy.
*   **Storage & Access Boundary**:
    *   File Path: `$XDG_CONFIG_HOME/ivideo-hls/config.toml` (with fallback to `~/.config/ivideo-hls/config.toml`).
    *   File Permissions: **MUST** be set to `0600` (owner read/write only).
*   **Settings Fields Schema**:

| Field Name | Data Type | Default Value | Description |
|---|---|---|---|
| `remote_url` | String | `"git@github.com:username/repo.git"` | Standard repository target URL for remote push |
| `auth_method` | Enum (`"ssh"`, `"https"`) | `"ssh"` | Git authentication handshake protocol |
| `token` | String | `""` | HTTPS Personal Access Token (PAT), masked in TUI |
| `public_url_pattern` | String | `""` | CDN/HTTP prefix replacement template for branch files |
| `default_source_dir`| String | `""` | Folder scanner target; falls back to cwd if empty |
| `default_recursive` | Boolean | `false` | Enable/disable subfolder scans by default |
| `default_parallel` | Integer | `1` | Default threads allocation (1 = Serial) |
| `default_quality` | Enum (`"low"`, `"medium"`, `"high"`) | `"medium"` | Video resolution and bitrate scaling profile |
| `default_compression`| Enum (`"fast"`, `"balanced"`, `"best"`) | `"balanced"` | Encoder compression preset profile |
| `default_pre_compress`| Boolean | `false` | Enable/disable standard H264 compression pass |
| `default_keep_source` | Boolean | `false` | Prevent deletion of source video on success |
| `default_push_disabled` | Boolean | `false` | Disable branch pushing to the remote git repo |
| `default_cleanup_disabled` | Boolean | `false` | Keep local workspace directory post-success |
| `resume_reuse_compressed` | Boolean | `false` | Enable reuse of clean `_compressed.mp4` on resume |

*   **TUI Keybindings**:
    *   `↑` / `↓` or `j` / `k` **SHALL** move the cursor index.
    *   `←` / `→` or `h` / `l` **SHALL** cycle choices or scale parallel threads.
    *   `space` **SHALL** toggle boolean variables.
    *   `enter` **SHALL** trigger text editing for text fields, toggle the auth method, or advance slider states.
    *   `s` **SHALL** write the configuration buffer to disk.
    *   `t` **SHALL** trigger a connection check using `git ls-remote` (10s timeout).
    *   `d` **SHALL** reset all configuration values to system defaults.
    *   `esc` / `q` / `ctrl+c` **SHALL** exit the settings panel. If modifications exist, the system **MUST** render a confirmation prompt:
        *   `s` **SHALL** save and exit.
        *   `d` **SHALL** discard changes and exit.
        *   `esc` **SHALL** return to settings navigation.

#### Module A.2: Git Authentication & Credential Hygiene
*   **Description**: Authenticate remote transactions without exposing security tokens in local git configurations, logs, or system logs.
*   **Security Isolations**:
    *   The plain target repository URL (`RemoteURL`) **SHALL** be stored in the repository remote configuration.
    *   The credential-bearing URL (`PushURL`) **SHALL NOT** be written to local git configurations. It **MUST** be injected exclusively as a positional parameter at push execution time: `git push -u -f <PushURL> <branch>`.
*   **Credential Redaction**:
    *   Output streams and error handlers **MUST** redact secrets using regular expression matching:
        *   *URL credentials*: `(https?://)([^/@\s]+)@` → `https://***@`
        *   *Access tokens*: Pattern `(?:github_pat_[A-Za-z0-9_]+|gh[opsur]_[A-Za-z0-9]+|glpat-[A-Za-z0-9_.-]+)` → `***`

---

### Group B: Video Discovery & Manifesting

#### Module B.1: Video Discovery (scan)
*   **Description**: Locate processable video media files.
*   **Allowed Formats**: The scanner **SHALL** recognize files ending in: `.mp4`, `.mov`, `.m4v`, `.mkv`, `.webm`, `.avi`, `.3gp`, `.3g2`, `.flv`, `.wmv`, `.ts` (evaluated case-insensitively).
*   **Scan Constraints**:
    *   *Flat Scan*: Scans the root directory only. Subdirectories **MUST NOT** be traversed.
    *   *Recursive Scan (`-r`)*: Traverses subdirectories. The scanner **MUST** skip hidden directories (starting with `.`), active workspaces (starting with `hero_`), and standard blocklisted targets (`node_modules`, `hero`).
*   **Auto-Creation**: If an explicit directory path is configured but does not exist, the scanner **SHALL** invoke directory creation. Fallback paths (e.g. cwd) **SHALL NOT** trigger folder creation.

#### Module B.2: Output Manifests (urls.txt)
*   **Description**: Maintain indexes of processed HLS streams.
*   **System Integrity**:
    *   The source directory `urls.txt` append operations **MUST** be protected by a synchronization lock (`manifestWriter.mu`) to prevent parallel writer collisions.
    *   The workspace index file (`<workspace>/x/urls.txt`) **MUST** be written before the commit stage to include it in the published branch.
*   **String Replacements**: The engine **SHALL** substitute `{branch}` and `{filename}` in the template string with the active branch name and the HLS index file name (`index.single`) respectively.

---

### Group C: Processing Pipeline

#### Module C.1: HLS Conversion Pipeline
*   **Description**: Automate HLS stream packaging and git publication.
*   **Sequential Stage Flow**:
    1.  *Workspace Initialization*: Copy contents of `hero/` template to `hero_<sanitized_name>/`. Remove git locks (`.git/index.lock`) if older than 2 minutes. Configures remote `origin` to the display URL.
    2.  *Branch Checkout*: Synchronize `main` with the remote repository (best-effort). Force-create target branch: `git checkout -B <branch>`.
    3.  *Pre-Compression* (Optional): If enabled, downscale video to 1080p maximum (preserving aspect ratio) using `libx264` medium preset at CRF 28. Write output to `<name>_compressed.partial.mp4` and rename to `<name>_compressed.mp4` on success.
    4.  *HLS Encoding*: Run `ffmpeg` segmenter to convert video streams into 6-second segments (`index_NNN.ts`) and generate playlist metadata (`index.m3u8`).
    5.  *Target Renaming Invariants*: All `.ts` segment files **MUST** be renamed to `.married`. The `.m3u8` references **MUST** be replaced with `.married` extensions, and the playlist itself **MUST** be renamed to `index.single`.
    6.  *Publication*: Stage changes using `git add .`. If differences exist vs HEAD, commit with message `"a"` and push to the remote repository.
    7.  *Cleanup*: Delete the temporary workspace directory. Delete the source video (unless `KeepSource`, disabled push, or disabled cleanup is configured).
*   **Preset Profiles (Code-Verified)**:

| Quality | Target Resolution | Video Bitrate | Buffer Size | Audio Bitrate |
|---|---|---|---|---|
| `low` | `480p` (`-2:480`) | `800k` | `1600k` | `96k` |
| `medium` | `720p` (`-2:720`) | `2800k` | `5600k` | `128k` |
| `high` | `1080p` (`-2:1080`) | `5000k` | `10000k` | `192k` |

| Compression Preset | Encoder Preset | CRF Target |
|---|---|---|
| `fast` | `fast` | `23` |
| `balanced` | `medium` | `23` |
| `best` | `slow` | `26` |

#### Module C.2: Parallel Execution Model
*   **Description**: Limit concurrent CPU and network threads to protect system resources.
*   **Semaphore Guarding**:
    *   CPU-bound encoding tasks (compression, HLS conversion) **SHALL** be limited using `cpuSem` with a capacity of `MaxParallel`.
    *   Network-bound tasks (git push) **SHALL** be limited using `netSem` with a capacity of `MaxParallel * 2`.
*   **Serial Execution**: If `MaxParallel = 1` or parallel execution is disabled, the semaphores **SHALL** evaluate to `nil`, forcing synchronous loop execution.

---

### Group D: Interactive User Interface (TUI)

#### Module D.1: Interactive Picker TUI
*   **Description**: Bubble Tea TUI interface for checking target videos and configuring runtime presets.
*   **Key Controls (Screen 1 checklist)**:
    *   `↑` / `↓` or `j` / `k` **SHALL** move the selection cursor.
    *   `space` **SHALL** toggle target video selection.
    *   `a` **SHALL** toggle selection for all currently visible (filtered) videos.
    *   `1`-`9` **SHALL** select the first N visible videos.
    *   `g` / `home`, `G` / `end` **SHALL** jump the cursor to the boundaries of the visible list.
    *   `/` **SHALL** enter list filtering mode.
    *   `esc` (filter mode) **SHALL** close filtering and clear the filter query.
    *   `enter` (filter mode) **SHALL** lock the filter search and exit filter editing.
    *   `s` **SHALL** close the Picker and launch the Settings TUI.
    *   `enter` (nav mode) **SHALL** transition to the Run Config screen.
    *   `q` / `esc` / `ctrl+c` (nav mode) **SHALL** terminate the process.
*   **Key Controls (Screen 2 Run config)**:
    *   `↑` / `↓` or `j` / `k` **SHALL** navigate form inputs.
    *   `←` / `→` or `h` / `l` **SHALL** adjust parallel jobs, quality, or compression values.
    *   `space` **SHALL** toggle boolean checkboxes (Pre-compress, Keep source).
    *   `enter` **SHALL** save config options and start the conversion runner.
    *   `esc` **SHALL** return back to Screen 1.
    *   `q` / `ctrl+c` **SHALL** terminate the process.
*   **Constraint Boundaries**: The parallel jobs configuration value **MUST** be capped at `[1, len(selected_videos)]`.

#### Module D.2: Run Dashboard TUI
*   **Description**: Bubble Tea interface displaying real-time job execution telemetry.
*   **Telemetry Views**:
    *   *System Dashboard*: Display execution phase, elapsed time, ETA, occupied semaphore slots, and remote URL details.
    *   *Encoding Logs*: Display active percentage, speed ratio, and bitrate details parsed from the FFmpeg progress pipeline.
*   **ETA Logic**: The system **SHALL** compute estimated completion times using the average duration of completed jobs: `avgDuration * waves + avgDuration / 2` (waves = pending videos divided by parallel capacity).

---

### Group E: Troubleshooting & Diagnostics

#### Module E.1: doctor (Diagnostics)
*   **Description**: Analyze the local runtime environment to verify configuration correctness, network latency, and path tools.
*   **Validation Pipeline (In Order)**:
    1.  `ffmpeg`: Check bin location on PATH or cached dependencies directory.
    2.  `ffprobe`: Check bin location.
    3.  `git`: Check bin location.
    4.  `config file`: Validate TOML layout parsing and `0600` permissions.
    5.  `remote URL`: Validate scheme syntax (`git@`, `ssh://`, `https://`).
    6.  `auth method`: Verify URL syntax aligns with the selected authentication protocol.
    7.  `token`: Assert presence of access token if HTTPS auth is active.
    8.  `ssh keys`: Verify loaded SSH agent keys if SSH auth is active.
    9.  `source dir`: Assert folder presence on the local system.
    10. `remote reachable`: Verify remote responsiveness using `git ls-remote` (10s timeout).
    11. `playback URL`: Warn if HTTP scheme, SSH scheme, or missing templates are detected.
    12. `pending retries`: Scan current working directory for retryable output workspaces.
    13. `incomplete workspaces`: Scan current working directory for incomplete encoding workspaces.

#### Module E.2: install-deps (Bootstrapper)
*   **Description**: Download static FFmpeg and FFprobe binaries without sudo privileges.
*   **Support Profiles**:
    *   *macOS arm64 / amd64*: Evermeet.cx zip archives.
    *   *Linux amd64 / arm64*: John Vansickle tar.xz archives.
*   **Install Pipeline**:
    1.  Target bin directory is resolved to `cacheDir/ivideo-hls/bin` (macOS uses `~/Library/Caches`, Linux uses `~/.cache`).
    2.  Fetch archive via HTTP GET (10-minute timeout limit).
    3.  Assert archive SHA-256 matches the pinned checksum values in `sources.go`.
    4.  Extract target binary, move it to the bin cache, and grant execute permissions (`0755`).

---

### Group F: Recovery & Resilience

#### Module F.1: retry-failed (Push Recovery)
*   **Description**: Force-push HLS output files that were fully encoded and committed, but whose git push command failed.
*   **Qualification Checks**: A directory **MUST** meet all requirements:
    1.  Name begins with `hero_` prefix.
    2.  Contains a valid `.git/` folder.
    3.  Contains final output playlist (`x/index.single`).
    4.  Has unpushed local branch commits.
*   **Recovery Flow**: Find candidates, prompt user confirmation (unless `--yes` is set), execute force-push via safe `PushURL`, and trigger cleanup steps upon successful transfer.

#### Module F.2: resume-failed (Encode Recovery)
*   **Description**: Remove partial output files and restart pipeline execution for jobs that crashed or were terminated mid-encoding.
*   **Qualification Checks**: Workspaces starting with `hero_` that contain `.git/` but do not contain `x/index.single`.
*   **Resume Options**:
    *   *Reuse-Compressed*: If `resume_reuse_compressed` is set and a clean `_compressed.mp4` exists (size > 1 KiB, duration > 1s, and no `.partial.mp4` sibling), the system **SHALL** delete the workspace folder only, skip the compression step, and launch conversion using the compressed file as input.
        *   *Note*: The resulting job, branch, and workspace name **SHALL** append `_compressed` to their names (e.g. `hero_lesson-05_compressed` folder, `lesson-05_compressed` branch).
    *   *Fresh Re-run*: Delete the workspace and any partial compressed files, and re-run all pipeline encoding stages from the source video.

---

## 5. System Invariants & Safety Measures

1.  **Refusal to Overwrite Base**: The pipeline checks paths and **SHALL** refuse to delete or overwrite the baseline `hero/` template folder during setup and cleanup stages.
2.  **Atomic Write Renames**: Video compression and playlist modifications **MUST** be written to `.partial` files and renamed atomically upon successful completion to prevent partial reads or corrupt states.
3.  **Sanitization of Workspace Names**: Workspace names **SHALL** be constructed by stripping video file extensions and replacing non-alphanumeric characters with `_`.
4.  **Credential Shielding**: Credentials **MUST NOT** be written to any persistent git configuration file or logged to the terminal logs.
