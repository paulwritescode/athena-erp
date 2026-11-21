# Review: B4b Appearance page redesign (commit d91ad3b, fix loop 1: 52557d0)

**Verdict: PASS** (after fix loop 1; see Re-review below. First verdict was NEEDS_CHANGES with 2 blockers, both now fixed and behaviorally confirmed.)

**Scope reviewed:** `git diff main...HEAD` on branch `feat/b4b-appearance` (single commit `d91ad3b`), against tasks.md B4b, design-system §18 (plus §2, §7, §13, §14) and the brief in this folder. Read-only on product code; no product files touched.

## Blockers (must fix before merge)

### B1. The Match-accent chart ramp does not follow the draft in the preview panes or option dots

`web/src/theme.css` declares the ramp once on `:root`/`.dark`:

```css
--chart-1: var(--primary);
--chart-2: color-mix(in oklab, var(--primary) 80%, white); ...
```

`var(--primary)` inside a custom-property declaration is substituted at computed-value time **on the declaring element** (`:root`, i.e. the saved page accent). The preview panes and the Charts Match dots scope only `data-accent` on a subtree element, which sets `--primary` there but cannot change the already-resolved `--chart-1..5` they inherit. So whenever the draft accent differs from the saved accent, the preview's bar chart, legend and Match dots show the **saved** accent's ramp, not the draft's. Buttons/focus/badges in the preview are correct (they use `--primary` directly), and the live page is correct (ramp resolves on `<html>` itself) — only subtree scoping is broken.

Evidence (Chromium, throwaway workspace): with saved=ink, draft=Blue, both preview figures carry `data-accent="blue"` and `--primary: oklch(45% .15 260)`, but computed `--chart-1: oklch(21% .006 285.885)` (ink) and `--chart-5: color-mix(... oklch(21% ...) 38%, white)`. Screenshots `func-preview-blue.png` / `v2-blue-draft.png`: blue buttons, ink-gray chart bars. The same applies to custom-accent drafts (`paneVars` sets `--primary` only).

Suggested fix: declare the ramp on the scoped element too, e.g. a `[data-accent] { --chart-1: var(--primary); ... }` rule placed before the `[data-charts]` blocks (equal specificity, palettes keep winning by order), plus a class or inline vars carrying the same declarations for custom drafts (which have no `data-accent`). Named palettes are unaffected (they already declare `--chart-*` on the matched element). Add a regression test: computed `--chart-1` inside a `[data-accent="blue"]` subtree must equal the blue primary (jsdom computes custom properties; or assert the rule exists in theme.css next to the preset test).

### B2. Appearances saved before B4b lose their custom chart colors on apply

Server `Resolve` (`internal/appearance/colors.go:162`) deliberately reads empty `chart_palette` + stored `chart_colors` as custom, and the editor agrees (`AppearanceSection.vue:90` maps that shape to Charts=Custom). But `applyAppearance` (`web/src/appearance.ts:235`) and first-paint `web/public/paint.js:15` only pass chart colors through when `chartPalette === 'custom'`, so a pre-B4b save (`preset: custom`, colors, no palette) renders the accent ramp while the editor claims Custom.

Evidence: stored `{preset: custom, accent: oklch(0.5 0.1 250), chart1: oklch(0.4 0.12 20), chart2: oklch(0.6 0.12 140)}` via `athena workspace appearance set` (no palette flag, exactly the legacy shape); after reload, computed `--chart-1` on `<html>` is `oklch(0.5 0.1 250)` (the accent, i.e. the ramp) instead of `oklch(0.4 0.12 20)`, while the Charts row reads "Custom" (`v2-legacy.png`). Data is intact; rendering is wrong until the user re-saves.

Suggested fix: treat `chartPalette === '' && chartColors.length > 0` as custom in both `applyAppearance` and `paint.js` (mirroring the server comment), with a vitest case for the legacy shape.

## What passes

- **Checks (all green, this session):** `make test` (Go incl. MariaDB appearance RPC test, 1.01 s, not skipped; vitest 35 files / 420 tests incl. new `appearance.test.ts` 15), `go vet ./...`, `gofmt -l cmd internal` (empty), `make generate` twice with no diff, `CI=true pnpm --dir web run typecheck`, `test`, `build`. No `.env.local` in the worktree; tests used `ATHENA_TEST_DATABASE_URL` against throwaway `test_athena_*` DBs only. The dev database was never touched.
- **Server:** one shared preset/palette list (`Presets`, `ChartPalettes`), validation errors, legacy read path, contrast gate with suggestion — covered by `colors_test.go` and the RPC round-trip test. Proto field 4 is additive; no gate change needed; no migration needed. Contrast ≥ 4.5 for all nine presets in both themes is enforced by the vitest that parses `theme.css`.
- **§18 layout:** 280 px choice column + live preview side by side ≥1024 px, stacked below; one pane following the theme switch on phones, light+dark side by side ≥640 px; popover on desktop, bottom drawer on coarse/narrow (verified at 375 px with touch); Save enabled only when changed (verified disabled on fresh load, enabled on draft, disabled after save); Reset restores ink/match; Your-theme row says it never changes the Workspace; preview has header actions, focused search, three badged rows, selected sidebar item, six-month chart + legend, and no "Focus ring"/"Selected page" labels.
- **Functional browser pass** (throwaway `test_athena_b4b_review` DB, server on 127.0.0.1:3411 with fake HOME alongside other agents' servers, since dropped): 10 accent options, 6 chart options, Blue+Ocean save → `data-accent`/`data-charts` applied → survive reload; custom accent round trip writes `#athena-appearance`; reset; zero console/page errors throughout.
- **Screenshots** at 375/768/1280/1440 × light/dark plus popover, drawer, custom, legacy states: worktree `.cache/screens/b4b/` (v2-*.png; the v1 set too). All were opened and inspected by the reviewer.

## Follow-ups (non-blocking; record in tasks.md if merged first)

- **F1. Accent dots/swatches ignore dark page mode.** The row dot, swatches and Match dots scope `data-accent` without `.dark`, so the light `[data-accent]` block wins and dots render light primaries on a dark page; the Ink dot is invisible on dark (`v2-375-dark.png`, `v2-1440-dark.png`). Consider scoping the pane's theme class onto the dots.
- **F2. Dark-mode ramp washes toward white.** `.dark` mixes `--primary` toward white, so for near-white primaries (ink dark) chart-2..5 are near-indistinguishable (visible in every dark preview). Consider a direction-aware ramp.
- **F3. CLI is stale:** `cmd/athena/appearance.go` usage still says `--preset ink|red`, and there is no flag for named chart palettes (it can still store the legacy custom shape, which is how B2 was reproduced).
- **F4 nits:** `RAMP_FALLBACK_HEX` always shows the ink ramp for empty custom slots even under another accent; the Custom swatch shows the ink fallback dot before a color is picked; preview month labels use `text-[11px]` outside the type tokens.

## Re-review: fix loop 1 (commit 52557d0) — PASS

Worker's fix (`fix(desk): scope match ramp to draft and apply legacy custom charts`): B1 declares the ramp on `[data-accent]` in `theme.css` (before the `[data-charts]` palettes so they win ties) plus `MATCH_RAMP_VARS` inline for custom drafts; B2 adds `chartsAreCustom()` (empty palette + stored colors) in `applyAppearance`/`paint.js`, with charts-only style blocks for the named-preset legacy shape. Two regression tests added.

**Behavioral confirmation** (rebuilt, throwaway `test_athena_b4b_review` DB on 127.0.0.1:3411 with fake HOME, since dropped; one login per script to respect the rate limiter; zero console/page errors):
- B1 fixed: unsaved Blue draft → light pane `--chart-1: oklch(45% .15 260)`, desktop dark pane `--chart-1: oklch(68% .14 260)`; Match dots follow the draft; custom `#1d4ed8` draft pane charts equal its `--primary`; saved blue+match applies the ramp live and survives reload (`r2b-blue-draft.png` shows blue ramps in both panes with Save still enabled).
- B2 fixed: CLI-stored legacy shape renders `--chart-1: oklch(0.4 0.12 20)`, `--chart-2: oklch(0.6 0.12 140)` after reload.
- No regressions: named Ocean palette still wins over the ramp in panes and live (`oklch(45% .1 250)` light / `oklch(64% .09 250)` dark); `make test` green (422 tests, +2 new), typecheck green. `paint.js` first-paint path verified by code (mirrors the tested helper); live-apply path verified behaviorally above.

Reviewer-harness footnote: an intermediate run reported 3 FAILs that were all harness artifacts, corrected in the final run — (a) figure index [1] is the hidden phone pane, not the desktop dark pane (there are 3 figures; the dark pane is [2]); (b) two ocean assertions compared against `0.45`-style decimals while Chrome serializes `45%`/`.1`. The built bundle was also inspected to rule out a minifier issue during that chase; it is correct.

The F1–F4 follow-ups from the first review stand as non-blocking.

## Reviewer notes (not findings)

- The v1 script's two failures were harness artifacts, both superseded by v2: the login rate limiter (B7 product behavior, "Too many tries") tripped on one-context-per-width logins, and a stale-toast race on the custom check. v2 uses one session with response-based waits; all v2 functional checks pass.
- `make build` deleted the tracked `internal/webui/dist/.gitkeep`; restored with `git checkout` after the pass — `git status` is clean.
