# ASCII Athena logo

**Status:** Implemented 2026-11-24 (task F6). The live logo draws on the
sign-in, first-run, not-found and offline surfaces; the static banner heads
bare `athena` and `athena version` output.

## Source art

`web/src/assets/brand/athena-black.png` (light theme) and
`web/src/assets/brand/athena-white.png` (dark theme): the olive-tree mark,
ink on transparency, 1254 x 1254. Each variant is generated from its own
source file, cropped to its own content box (pixels with alpha above 8),
so the mark fills the frame in both themes.

## Pipeline (`tools/asciilogo`)

Go, standard library only (`image/png`). Every source pixel counts: each
cell composites its pixels over the theme background (white for light,
black for dark), averages the relative luminance (Rec. 709) and maps the
ink value — darkness on light, brightness on dark — through the ramp after
gamma. The run is deterministic: same sources and flags, same bytes.

Regenerate with, from the repository root:

```sh
go run ./tools/asciilogo
```

Flags: `-cols` (default 40), `-rows` (default 8, at most 8 per design
§9.6), `-step-ms` (default 180, allowed 120 to 240), `-light-gamma` and
`-dark-gamma` (default 1.0), `-ramp` (default `" #"`), `-light-src`,
`-dark-src`, `-out` (default `web/src/art/logo-frames.ts`). The generator
refuses a ramp with letters or non-ASCII, and prints how many cells the
variants differ in.

## Ramp and sizes

- Ramp `" #"` (two stops, background then ink; threshold 0.5). Longer
  ramps were tried and turned the fine branches to noise at 8 rows; the
  two-stop density ramp keeps the canopy and the trunk curve readable.
  The ramp holds no letters, so the art can never spell the name.
- 40 columns by 8 rows: the §9.6 maximum height, and the width that fits
  the 400 px sign-in column and 375 px phones in caption mono.
- 9 frames per theme (see below), light and dark, in
  `web/src/art/logo-frames.ts` with `LOGO_COLS`, `LOGO_ROWS`,
  `LOGO_STEP_MS`, `LOGO_RAMP` and the joined final blocks
  `LOGO_LIGHT_BLOCK` / `LOGO_DARK_BLOCK`.

## Frames and motion

Frame *k* shows the first *k* rows of the mark and leaves the rest blank;
frame 0 is empty and frame 8 is the whole mark. Every frame keeps the
full 8-line height, so the draw never moves the page. The desk steps one
frame per 180 ms (`--duration-base`, top to bottom), each new row fading
in through an opacity-only keyframe of the same length; the animation
plays once and rests on the full mark. Under `prefers-reduced-motion`
the full mark renders at once with no timer and no animation.

## CLI banner

`cmd/athena/main.go` keeps a copy of the light final frame as `asciiMark`
(the density map reads on light and dark terminals alike, since dense
cells always mean ink). After regenerating the frames, copy the light
final block over `asciiMark` and keep the banner above the version lines;
`web/src/art/logo-sync.test.ts` fails while the two differ.
