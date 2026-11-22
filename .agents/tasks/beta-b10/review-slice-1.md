# Review: B10 slice 1, art registry + core placements

Branch `feat/b10-art` in `../athena-erp-b10`, commit `f1d620d` against
`main`. Reviewed against the slice-1 brief, design-system §9 (+ §7,
§13, §14) and tasks.md B10. Read-only on product code; no commits or
merges.

Verdict: PASS

## Blockers

None. No broken behavior, data loss, security hole or unusable screen
was found.

## Follow-ups (all non-blocking)

1. `web/src/composables/useDeskRoute.ts` (~`:164`) — "Back to home"
   from a directly-loaded 404 leaves the bad address in the bar.
   `readLocation` leaves `mode`/`page` at their `build`/`overview`
   defaults, so `openBuildPage('overview')` sets identical values, the
   location watcher never fires, and Overview renders under
   `/build/no-such-page` (observed; refresh coherently re-shows the
   404). Replace the address with the shown page when leaving 404.
2. `web/src/art/registry.ts:34-36` — the async chunk carries all 33
   figures (~120 KB) because `loader()` indexes the dynamic-import
   namespace by variable, which defeats tree-shaking (verified in
   `dist/`). The lazy boundary itself works (desk bundle carries no
   figure code). Try static per-figure member access so Rollup can
   split or shake the chunk.
3. `web/src/art/registry.ts:11` — `FigureName` is a closed union of
   the three stock figures; custom figures will need it extended (plus
   a placement entry with a `./figures/` loader, which the current
   `FigureLoader` shape already accepts). Consider `string` now so no
   type change is needed later.
4. Recorded per the brief scope note: the custom Athena figure set in
   `web/src/art/figures/` (hairline-create skill unavailable). Stock
   coverage is complete for slice 1 (padlock/query/plug + ASCII mark).

## What was verified

Checks (exit 0): `CI=true pnpm --dir web run typecheck`, `test` (46
files, 509 tests, token guard green), `build`; `gofmt` clean,
`go build ./...`, `go vet ./cmd/athena`, `go test ./cmd/...`
(including `TestUsageBannerShowsMarkAndVersion`). Tree left clean.

Registry (§9.7): `useArt` returns undefined for unlisted keys (unit
test + both components render nothing); package referenced only from
`web/src/art` (type imports + one dynamic import — enforced by a
source-scan test); at most one `<ArtFigure` per `.vue` (enforced by a
test); options intensity 0.3, theme auto, play off (unit test).
Verified against the installed 0.5.1 package: `padlock`/`plug`/`query`
exist; `theme: 'auto'` follows an ancestor `.dark` class, matching
the desk's class-based theme; the lib natively rests under
prefers-reduced-motion; `--hairline-plate/edge/mid` are honored (hi/lo
keep lib defaults — visually quiet in both themes).

Placements: sign-in padlock 240/160 px + 5-line ASCII footer,
not-found query 200/160 px, offline plug 200/160 px — one figure per
screen, `aria-hidden`, sizes in the §9.3 bands, meanings match their
pages. CLI banner shows the mark above the version; the Go and TS
drawings are byte-identical (5 lines, plain ASCII, no trailing space).

Browser pass on throwaway Workspace `test_athena_b10_s1`, fake-HOME
server, Chrome via CDP. Sign-in, 404 and offline at 375/1440 px in
dark and light: exactly one rendered figure each, measured widths
240/160, ASCII footer present, tones quiet in both themes. 404 keeps
the failed address and `Back to home` returns to Overview (modulo
follow-up 1); `/view/<unknown>` still takes the existing unknown-type
path. Offline appears on emulated disconnect, retry-while-offline
toasts gently, reconnect returns to the same screen. Reduced-motion
emulation: two frames 1.5 s apart byte-identical, `pointer-events:
none`, enter animation off. The hairline chunk loads on demand with
the first figure mount.

Scope: 19 files, all in slice (registry, art components, three
placements, route/offline plumbing, CLI banner + test, docs row,
token-only CSS). No new dependencies, no Go beyond the banner, no
other workers' pages touched.

Screenshots (17, ephemeral): `/tmp/athena-b10-review/screens/`. CDP
scripts: `/tmp/athena-b10-review/cdp/`. Throwaway server, Chrome
profile, home and database were removed after the pass.
