# Screenshots

Real terminal screenshots live here once captured. Until then, the ASCII
mockups in [`../../README.md`](../../README.md#screens) are the canonical
layout reference.

## Shot list

Capture on a dark-themed terminal at 120×40 or wider. PNG, not gif — the
UI is static between keystrokes. Name the files exactly as listed so the
README can link to them without further edits.

| File                      | What to capture                                                                 |
|---------------------------|---------------------------------------------------------------------------------|
| `picker.png`              | A directory with 4–6 videos selected, cursor on one of them, filter unused.     |
| `picker-filter.png`       | Same picker with `/` filter active, matching a subset.                          |
| `settings.png`            | Settings screen with a real URL and masked token, cursor on "Public URL".       |
| `settings-confirm.png`    | Settings after edits, pressing `esc` — the save/discard prompt visible.         |
| `run.png`                 | Run dashboard with 2–3 active jobs, at least one `compress` and one `convert`.  |
| `run-done.png`            | Run dashboard after some jobs have completed (Done section populated).          |
| `doctor-ok.png`           | Doctor output on a fully-configured machine — all green ✓.                      |
| `doctor-warn.png`         | Doctor output on a fresh checkout — warnings on config file, ssh keys.          |

## How to take them

macOS Terminal: ⌘⇧4, drag to the window, save into this folder. Make the
window fullscreen-ish first so the layout doesn't wrap.

iTerm2: ⌘⇧S with "Selection" set to "Whole Window" for pixel-perfect crops.

## Why aren't they committed yet?

Mockups stay in sync with the code (they're pure text), screenshots
don't. The plan: commit screenshots once the TUI layout is stable and
you've signed off on the visual design. Until then, the mockups
document the intended look and the code owns the reality.
