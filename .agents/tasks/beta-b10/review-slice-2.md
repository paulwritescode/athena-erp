# Review: B10 slice 2, empty-state figures

Verdict: PASS

Branch `feat/b10-art` at 7a0a7de in `../athena-erp-b10`, diff `9fdad29...HEAD`
(13 files, slice 1 excluded). Reviewed against the slice-2 brief, tasks.md B10,
design-system §9 (plus §7, §13, §14), design.md §5, AGENTS.md and
`.kiro/steering/athena-design.md`. No product code edited; the only worktree
touch was `internal/webui/dist/.gitkeep`, deleted by the review build and
restored — `git status` is clean.

## Blockers

None.

## 1. Empty-state figures per the §9 meaning table — pass

- `web/src/art/registry.ts:66-77` adds twelve `empty-*` placements:
  sieve for no-results, drawer for Files, plot for Reports, stack for ledger
  masters, plus format (Record Types), basket (Packs), terminal (Rules), relay
  (runs), hub (mapping), lockers (users), vault (roles), router (sessions).
  Every figure matches its page within §9.5's meaning-first rule; the
  `stack`-for-ledger choice is reasonable while the Records list stays
  art-free (see §2 below).
- All rendering goes through the registry: `EmptyState.vue:12-15` resolves
  `art` via `useArt` and mounts one `ArtFigure`; `DataTable.vue:666-679`
  passes `emptyArt`/`filteredArt` into it; pages name static keys only. No
  page imports a figure directly (pinned by
  `registry.test.ts:109-126`, green).
- Size: empties are 200 px desktop / 120 px mobile (registry + test
  `:84-90`), inside §9.3's 160–240 / 120. Runtime-measured 200×160 at 1440
  and 120×96 at 375 in both themes.
- Quiet look: `artFigureOptions` fixes theme auto, intensity 0.3, play off
  (`registry.ts:88-90`, test `:64-72`); `art.css:4-9` paints lines from
  `--app-text-subtle` on `--background` (test `:174-178`). Browser-computed
  values resolve to subtle-token mixes with the plate matching the page
  background in both themes.
- A11y/motion: no empty placement sets `label`, so all render `aria-hidden`
  (`ArtFigure.vue:40`, observed `aria-hidden="true"` on every figure).
  Reduced motion rests figures (`art.css:25-32`, test `:169-172`); verified
  live under emulated `prefers-reduced-motion: reduce` —
  `pointer-events: none`, enter animation `none`.
- One figure per screen: `EmptyState.vue` is the only non-`art/` file that
  mounts `<ArtFigure>` (test `:128-134`). Runtime counts show exactly one
  `.art-figure` on every art-carrying page/tab, including ledger tabs that
  hold several icon-only empties alongside the one figure (e.g. Companies
  tab: 4 empty titles, 1 figure). One narrow exception is follow-up F1 below.

## 2. Records list carries no art — pass

`RecordsPage.vue:908-939` sets no `empty-art`/`filtered-art`, and a repo-wide
grep confirms no other Records empty path names art. Live `/view` on the
empty workspace shows "Nothing to view yet" with zero `.art-figure` nodes.
Stack-for-Records from the §9.5 examples stays unapplied, correctly left to
B9 follow-up 2.3.

## 3. Registry invariants and lazy loading — pass

- Unknown keys still return `undefined` (`registry.ts:82-84`, test `:40-43`
  green); new keys reuse the same dynamic-import `loader`, so the desk
  bundle still carries no figure until one mounts. `placementKeys`
  (`registry.ts:80`) only feeds the tests.
- Chunk spot-check: `vite build` emits one async hairline chunk,
  `dist-*.js` at 120.88 kB (gzip 48.9 kB) — unchanged from the known ~120 KB
  slice-1 follow-up, no balloon (expected: same single dynamic import).
  Not blocking per the brief.

## 4. Scope: empty states only — pass

- `EmptyState`: `icon` optional, new `art` prop with icon fallback for
  unknown/unset keys; `mt-8` spacing applies in the figure case only
  (`EmptyState.vue:20-24`). Icon-only states render byte-identical markup.
- `DataTable`: two optional props, default unset → icon empties unchanged.
- Pages: static `empty-art`/`filtered-art` attributes only, except —
- `PacksPage.vue`: custom `.empty-builder` block converted to `EmptyState`
  with `art="empty-packs"`; title, body and the secondary "Open Record
  Types" action are verbatim, notice row and disabled "New Pack" untouched
  (screenshot-confirmed). Within empty-state scope.
- `GalleryPage.vue`: demo copy "icon, title" → "figure or icon, title" plus
  an art demo; art-related copy only. `ReportsPage.vue`: icon → art swap.
- `docs/architecture/overview.md`: status-row doc update only.
- No token, behavior, layout (beyond the empty blocks) or dependency
  changes; `package.json` untouched. Token guard green:
  `tokens.test.ts`, 73 passed.

## 5. Checks run — all green

In `../athena-erp-b10` (node v26.7.0, pnpm 11.22.0):

- `CI=true pnpm --dir web run typecheck` → exit 0, no errors.
- `CI=true pnpm --dir web run test` → 48 files, 528 tests, all passed
  (incl. `registry.test.ts` 13/13, `tokens.test.ts` 73/73).
- `CI=true pnpm --dir web run build` → exit 0 in ~0.9 s.
- Browser pass on a throwaway workspace (`test_athena_b10_review`
  database, fake `$HOME`, throwaway server on 127.0.0.1:3457, headless
  Chrome over CDP, zero new dependencies): signed in as the throwaway
  root administrator; captured record-types, files, mapping, reports,
  packs, rules, rules-filtered, access users/roles/sessions, ledger
  readiness/companies/periods/currencies/accounts and `/view` at 375 and
  1440 in light and dark. Every art-carrying screen shows exactly one
  quiet figure at the specified size; filtered empties show the sieve
  with "No results for these filters"; non-empty tabs (users, roles,
  sessions) and `/view` show zero figures. Screenshots and the
  figure-metrics JSON are at `/tmp/athena-review-b10/screens/` (34 PNGs)
  and `/tmp/athena-review-b10/figure-metrics.json`.
- Motion: `play: false` + intensity ≤ 0.3 is code- and test-pinned; the
  enter fade and the reduced-motion rest were verified live. Looping
  decoration: none.
- Cleanup done: throwaway server stopped, `test_athena_b10_review`
  database dropped, fake home (held the server login) removed. The dev
  database in `.env.local` was never connected to; its DSN only supplied
  the MariaDB server login that the test rig itself uses for
  `test_athena_*` databases.

## B10 full coverage (last slice) — covered

- Registry per §9 with one-place rule, unknown-key rejection, lazy
  figures, quiet options: done (slices 1+2).
- Figures on sign-in, empty states, 404, offline: done and live-verified.
- ASCII mark for the CLI banner (`cmd/athena/main.go`, visible in
  `--help` output) and sign-in footer: done (slice 1).
- Accept criteria: at-most-one-figure test and reduced-motion rest:
  done, both test- and browser-verified.
- Not landed, both pre-existing structural follow-ups, neither a slice-2
  defect: no first-run placement (no first-run page exists yet — B11 owns
  that flow, add the placement when it lands) and no custom figures in
  `web/src/art/figures/` (carried slice-1 follow-up: `hairline-create`
  skill unavailable; stock figures used throughout).

## Follow-ups (non-blocking, for tasks.md)

- F1 (new, minor §9.2 edge): on Rules, a filtered-to-zero rules table
  (sieve) and an open runs tab (relay figure) can show two figures on one
  screen at once, since the rules table and the detail panel are visible
  together. Suggest the runs table keep the icon empty while the rules
  table shows its filtered figure, or vice versa. Not a blocker: both
  figures are quiet and the empty-action flow is unaffected.
  (`web/src/pages/RulesPage.vue:393-490`.)
- F2 (new, coverage note): `empty-users`, `empty-roles`, `empty-sessions`
  and `empty-runs` figures were not observed live — a fresh workspace
  always has the administrator row, seeded roles and live sessions, and
  runs need a saved Rule. Their loaders and mappings are unit-tested
  (`registry.test.ts:52-62,103-107`); suggest a master spot-check when a
  workspace with an empty users/roles view exists.
- Carried slice-1 follow-ups still open: 404 "Back to home" keeps the bad
  address; async chunk carries all figures (~120 KB); `FigureName` closed
  union; custom Athena figure set; first-run placement (see above).
