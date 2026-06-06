# Brand assets

One idea, four variants. The idea: a **play triangle sliced into four
bands** — the play shape everyone recognises for video, segmented to
suggest HLS.

## Files

| File | Panel | Use for |
|------|-------|---------|
| [`logo.svg`](logo.svg) | dark (`#111827`) rounded square | GitLab project avatar, dark docs |
| [`logo-light.svg`](logo-light.svg) | light (`#F9FAFB`) rounded square | GitHub README hero, light docs |
| [`logo-mark.svg`](logo-mark.svg) | transparent | places that supply their own chip |
| [`logo-favicon.svg`](logo-favicon.svg) | dark, 32×32 geometry, no gradient | browser favicon, 16–48 px icons |

All files are vector. Inline SVG in README rendering works out of the
box on GitLab and GitHub.

## Palette

| Role | Hex | Notes |
|------|-----|-------|
| Primary (violet) | `#7C3AED` | Gradient start; matches TUI `colPrimary`. |
| Accent (cyan)    | `#22D3EE` | Gradient end; matches TUI `colAccent`. |
| Panel (dark)     | `#111827` | Matches TUI `colBgSelect` family. |
| Panel (light)    | `#F9FAFB` | Neutral light grey; pairs with GitHub's default. |
| Outline (dark)   | `#E5E7EB` | For the triangle outline on the dark panel. |
| Outline (light)  | `#111827` | For the triangle outline on the light panel. |

The violet→cyan gradient is the same one `progress.WithScaledGradient`
uses for the run-dashboard bars. The logo intentionally looks like it
belongs to the tool, not a separate brand.

## Design constraints

- **Legible at 16×16.** The favicon variant uses solid colors only
  because gradient stops disappear at that size.
- **Two-color story maximum.** Violet and cyan, no third accent.
- **No text.** "ivideo-hls" at project-avatar scale is a blur.
- **Single idea at three scales.** Play button at 16 px, segmented
  stack at 48 px, stylised HLS graphic at 256+.

## Uploading to GitLab

1. Go to project **Settings → General → Project avatar**.
2. Upload `logo.svg` (GitLab accepts SVG for project avatars).
3. If GitLab rejects SVG, rasterise first:
   ```bash
   # requires `rsvg-convert` (brew install librsvg) or equivalent
   rsvg-convert logo.svg -w 512 -h 512 -o logo-512.png
   ```

## Using in README

```markdown
<p align="center">
  <img src="docs/brand/logo.svg" width="128" alt="ivideo-hls">
</p>
```

Swap `logo.svg` for `logo-light.svg` if the host renders dark-on-light.

## What this is NOT

- A wordmark. There's no text in any variant — that's intentional.
- A mascot. No film reels, clapperboards, or cameras.
- Final. If you find a version that reads better at 32×32 after seeing
  it in GitLab's sidebar, edit the SVG — the whole point of vectors is
  that iteration is free.
