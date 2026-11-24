# Review: F6 live ASCII logo

Verdict: NEEDS_CHANGES — generator default disagrees with the doc and the shipped frames (one-line fix).

Tree: `../athena-erp-f6-r`, detached at 09a5fe1, clean; 3 F6 commits (4b3a667, 3e3a7c1, 09a5fe1) on 4acb8d4. All files within the brief's allowed set; no gate.go/proto/migration/App.vue/api.ts touches. `internal/webui/dist/.gitkeep` intact.

## The one blocking defect

`tools/asciilogo/main.go` defaults `-step-ms` to 150, but `docs/design/ascii-logo.md` says the default is 180 and the committed `web/src/art/logo-frames.ts` has `LOGO_STEP_MS = 180`. The documented regen command does NOT reproduce the shipped file:

- Ran the generator twice to temp files: byte-identical (deterministic, good).
- Bare regen vs committed file: DIFFERS in exactly the step lines (`150 ms a row` / `LOGO_STEP_MS = 150` vs `180`).
- Regen with `-step-ms 180`: byte-identical to the committed file.

Fix: change the flag default 150 → 180 in `tools/asciilogo/main.go` (aligns code with the doc, the shipped value, the design-system 9.6 rule and `--duration-base: 180ms`). Then re-run bare `go run ./tools/asciilogo` and confirm zero diff. Re-verify cost is near zero (`tools/asciilogo` has no tests; nothing else reads the default).

## What passed

1. Generator: deterministic; frames 40x8 (LOGO_COLS/ROWS + registry test asserts rows ≤ 8); light+dark variants from separate sources; gamma 1.0/1.0 documented in doc + file header; art charset is space + `#` only (ramp letter/non-ASCII refusal in code, letter-free assertion in test). The mark reads as canopy/trunk/ground in every shot.
2. AsciiLogo.vue: 180 ms steps (in 120–240); keyframes opacity-only, no other animated property; reduced motion renders the full mark with no timer plus `animation: none` in CSS; `aria-hidden`; unknown/figure keys render nothing. Registry `logo` kind with 4 placements; ≤1 hairline figure per surface (signin 1, not-found 1, offline 1 mounted, first-run 0) + one logo each — the same figure-plus-mark pattern sign-in already had, now per design rule 6.
3. CLI: ran both — bare `athena` shows tree + usage, `athena version` shows tree above bare `0.1.0-dev` last line (F8's line untouched). `logo-sync.test.ts` compares the Go const bytes against `LOGO_LIGHT_BLOCK` — a real cross-file check, and the Go test pins the last line to `version`.
4. Shots: `/tmp/f6-shots/` holds 32/32; suite-logs/1124-050903-w-f6.log shows 32 wrote lines and 8/8 in-run checks (logos=1, figures≤1, rows=8). I eyeballed 10 the worker hadn't (768/1280 middle widths all surfaces × themes, plus signin-375-dark and offline-375-light): logo renders every surface/theme, 8 rows, no clipping or breakage at 375, dark shots true-dark. No undisclosed breakage.
5. Tests (all observed in this worktree): 23/23 art vitest green — behavioral, not shape-only (exact per-frame rows stepped on fake timers, rest check, exact dark final, reduced-motion full mark, keyframe body scan, per-file logo cap, Go↔TS byte sync). Go banner/version tests green; `pnpm typecheck` exit 0; affected gate green via suite (1124-063357-r-f6.log: `ok cmd/athena`, 65 web files / 626 tests pass); `make generate` exit 0 with tree still clean; `gofmt -l` empty; `go vet` clean.
6. Offline plug finding is pre-existing, confirmed against base 4acb8d4 (not caused by F6): the `loader()` dynamic-import path, ArtFigure.vue, the `offline` plug placement and OfflinePage's figure line are byte-identical between base and tip — F6 only added the logo below. Offline shots show the logo rendering with no plug, as reported.

## Follow-ups (filed, not blocking)

- F6-FU1: offline plug figure never paints — its lazy hairline chunk can't load while offline. Fix in B10 art ownership (inline the plug, or an offline-safe fallback); F6 logo already renders because frames ship in the main chunk.
- F6-FU2 (nit, optional): the logo block centers as a whole while ragged lines read slightly right-of-centre on wide 404/offline pages. Cosmetic; matches the old mark's centering.

## Re-review ask

After the one-line default fix + regen no-diff confirmation, this is a PASS from me with no further browser work needed (no visual or behavioral change).
