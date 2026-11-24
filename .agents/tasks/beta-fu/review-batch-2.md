# Review: FU batch 2 — B2a-verify, B5, FU-1, C1 nit, chart fix (interim PASS)

Reviewer: r-fu. Tree: `/Users/iampaulking/projects/duchess/athena-erp-fu-r`, detached at
58a8cf4, verified clean on arrival (`git status --porcelain` empty) and untouched by this
review (all browser controls ran in the throwaway `../athena-erp-fu-ctrl` tree).
Branch: 9 commits on main, 58948de..58a8cf4 (base 58948de is on main).

Verdict: PASS

## Blockers

None. All seven items clear.

## 1. Evidence chain — HOLDS

- Full `make test` exit 0 on 4a7d26b (pre-rebase tip), clean tree, logged in
  `suite.jsonl` (`1124-005027-w-fu.log`, 844 s): 17 Go `ok` packages, 63/63 Vitest
  files, 616/616 tests. The single `FAIL|panic` grep hit is a test name
  ("keeps the failed address in the bar"), not a failure.
- Pre-rebase base is 46bf4fd (8 commits de96abf..4a7d26b). Post-rebase minus tip is
  58948de..218803c (8 commits 77971a2..218803c).
- `diff` of the two patches (modulo `index` lines): BYTE-IDENTICAL. The rebase moved
  the same 9-fix content (8 here + tip) across C4s3/C5s1/train3/C4s4 with zero content
  change.
- Tip 58a8cf4 is one template attribute on a native `<input type="color">`
  (`:model-value` → `:value`, AppearanceSection.vue:360). `:model-value` is a
  Vue-component prop and a no-op on a native input, so slot 0 fell back to black
  (#000000); `:value` binds the ramp color. One-word template change; typecheck
  (vue-tsc covers templates, rerun green in item 7) plus the live browser assert
  (`1124-033257-w-fu.log`: "ok empty chart slot 0 shows the ink ramp (#18181b)")
  suffices. New-base interaction risk for the other 8 is covered by item 7's
  `affected` rerun on HEAD.

## 2. Editor-no-rows question — FLAKE, not a regression (PASS with follow-up)

Failure history on batch-2 code (all `/tmp/fu2-browser.sh` through `suite`):

| Run | Commit | Result |
| --- | --- | --- |
| 01:39 | 218803c | chart slot 0 #000000 (real bug, found and fixed by 58a8cf4) |
| 02:04 | 58a8cf4 | TIMEOUT "popover closed" (driver/harness) |
| 02:26 | 58a8cf4 | **"ok invoice editor"**, "ok record form", then TIMEOUT "First saved" (wrong assertion, later fixed) |
| 02:30 | 58a8cf4 | TIMEOUT "invoice editor: .field-row" |
| 03:32 | 58a8cf4 | TIMEOUT "invoice editor .../build/record-types/invoice: .field-row" |

Decisive points:

1. The SAME clean commit 58a8cf4 passed the editor step once (02:26) and timed it
   out twice. A deterministic batch-2 regression cannot pass on identical code.
2. Different stages failed in different runs (chart value, popover toggle, record
   save, editor ×2) — the signature of an overloaded machine, matching the night's
   5× slowdowns and OOM kills.
3. Controls (same editor flow, `/tmp/fu2c-cdp.py` + `/tmp/fu2c-one.sh`, one `suite`
   run "FU b2 review: editor controls", 60 s editor timeout, all on throwaway DBs):
   - clean main 8f3bd9d: editor in 0.3 s, 1 row, PAGE_ERRORS 0 → exit 0
   - train2 2d9c17d (WITH batch-1's TextField enter-emit, WITHOUT batch 2): editor
     in 0.3 s, 1 row, PAGE_ERRORS 0 → exit 0
   - batch-2 tip 58a8cf4: editor in 0.3 s, 1 row, PAGE_ERRORS 0 → exit 0
   (`/tmp/fu2c-main-editor.png`, `/tmp/fu2c-train2-editor.png`,
   `/tmp/fu2c-batch2-editor.png`; the batch-2 shot eyeballed: Invoice editor with
   the Name row and "Invoice created." toast.)
4. No plausible mechanism: batch-2's only web change is AppearanceSection.vue
   (settings page); the failing flow is record-type creation → editor, which batch 2
   does not touch. Batch-1's 2d9c17d only adds an Enter path to the create dialog
   (the driver clicks Create; no Enter is pressed) and marks fields touched before
   the validity check — identical behavior on valid input — and the train2 control
   clears it empirically anyway.

Follow-ups (not blockers): the records-flow shots (records/dark/dialog-key) remain
uncaptured; FU-H (already prioritized) owns driver hardening. No data loss, no app
bug: the editor renders one row for the fresh Invoice type on every commit.

## 3. Screenshots — each fix visible, no regression in frame

- `/tmp/fu2-settings-light.png` (fresh 03:33): settings settled in light, Ink/Match
  accent/Light, invoice previews with charts. Clean.
- `/tmp/fu2-custom-light.png`: Custom accent picked, Accent row indicator shows the
  conic-gradient wheel (FU-1 fix visible); preset radios + oklch value in popover.
- `/tmp/fu2-charts-light.png`: Charts Custom picked, ramp options render, slot-0
  swatch dark ink (chart fix visible; live assert pinned #18181b byte-exact).
- Missing records-flow shots acceptable per item 2 (flake, and B2a had no batch-2
  code change to eye-verify). Popover overlaps in the shots are normal overlay
  timing, not regressions.

## 4. One fix per commit — each does what it claims

- 77971a2 B5 role help: help text gains exactly the three real system permissions
  (`settings:appearance/workspace/security`, `internal/auth/principal.go:74-80`,
  in `SystemPermissions` order). One line.
- 0ffed3a B5 new --logo: CLI pre-validates with `brand.ValidateLogo`, the exact
  function and args the server uses (`internal/server/workspace.go:207`) — no
  accept/reject divergence. Test covers bad content in three arg positions.
- c5cc932 B5 SetIdentity conflict: compare-and-publish inside the tx; stale read →
  `ErrIdentityConflict` → RPC `CodeAborted` with an everyday-words message. Old
  generations stay (test asserts count 2). Verified: `GetIdentityDoc` returns
  `IdentityDoc{}` on no-rows, matching the save's empty-latest fallback; the
  `read := doc` snapshot is safe because the merge only replaces `Light`/`Dark`
  pointers and the scalar name, never mutates through a `*LogoRef`
  (`internal/server/workspace.go:155-176`); JSON round-trip comparison is
  deterministic for this struct.
- b712790 B5 prune best-effort: post-commit prune errors log server-side instead
  of failing the saved identity. Test forces prune failure via a dead files-store
  DB and asserts the save commits with the logo cleared.
- d195c62 B5 favicon cache: `(hash, size)` mutex-guarded memo; only validated
  hashes reach it, hashed URLs are immutable so entries never stale; handler is
  built once at mux setup (`internal/server/server.go:153`), so the cache is
  shared and effective. Test asserts one entry + byte-identical repeat.
  Non-blocking note: entries for pruned logos linger until restart (bounded, KBs).
- 726a4f7 B5 unchanged set: `reflect.DeepEqual(doc, read)` short-circuits a
  no-change save (no flags, or the same name) with no new generation; test asserts
  generations stay 1 across both no-ops. A no-op concurrent with another writer's
  save loses nothing (there is nothing to lose); the answered identity can lag by
  one generation in that race — harmless.
- 80ec827 FU1 wheel: `v-else-if="customPicked"` keeps the solid swatch once a
  custom color exists (saved or picked this session), else the conic-gradient
  wheel — the same pattern the popover already uses (AppearanceSection.vue:304).
  Live browser assert green.
- 218803c C1 nit: "after rollback" → "during the attempt, surviving its rollback".
  Matches the code: `logFailure` writes via the pool (`e.DB`), i.e. a separate
  connection, during Post (`internal/posting/operations.go:219-230`). One line in
  `docs/architecture/overview.md` only.
- 58a8cf4 chart :value: as item 1; live assert green.

Desk commits against the design system: no new components or tokens; the wheel
reuses the existing conic-gradient idiom, decorative (`aria-hidden`); everyday
words untouched. Fine.

## 5. B2a verify-only — all six confirmed in the tree; skips clean

b4791a3 ("Also fixes the six slice-1 review follow-ups", ancestor of HEAD) plus
current-tree verification:

1. Search padding: `pl-9!` wins over Input `px-3` (`web/src/components/DataTable.vue:451`). ✓
2. Dead CSS: no `entries-grid`/`entries-resizer` anywhere in `web/src`. ✓
3. Glossary: status labels via `gloss(key).label` (`web/src/components/records/recordColumns.ts:19`); no hardcoded Draft/Submitted/Cancelled in records components or RecordsPage. ✓
4. Single list request on clear: `clearAllFilters` emits search+filters+clear;
   `onFiltersChanged` applies immediately (one request), `onClearFilters` kills the
   debounce timer, and the `filterText` watcher skips when the expression already
   applied (`web/src/pages/RecordsPage.vue:144-168`) — double-guarded, exactly one. ✓
5. 480 px cap: `sm:max-w-[480px]` (`web/src/components/DataTable.vue:449`). ✓
6. pressTimer: cleared on unmount, before reset, and after fire (`web/src/components/DataTable.vue:376-392`). ✓

Skips confirmed not half-built: no `--chart-palette` flag (`chartPalette` hits are
pre-existing `appearance.ts` type fields; batch touches no CLI flag and no
appearance.ts); no B3 RuleRun/server, Security RPC, B9 form, B2a-s2-3, B8, B10,
B11, or C-lane files in the diff; `internal/posting/operations.go:21` still reads
"after the rollback" — correctly untouched (C1 file, reported skip).

## 6. Scope — clean

11 files: `cmd/athena/main.go`, `cmd/athena/new_test.go`,
`docs/architecture/overview.md`, `internal/ledger/identity.go`,
`internal/ledger/identity_test.go`, `internal/server/brand.go`,
`internal/server/brand_test.go`, `internal/server/identity_test.go`,
`internal/server/public_test.go`, `internal/server/workspace.go`,
`web/src/components/settings/AppearanceSection.vue`. No proto, no gate.go, no
migrations; none of RecordTypesPage / FieldSettingsDialog / fieldRules.ts /
App.vue / RecordsPage / api.ts. RecordsPage untouched (no GO needed, none taken).
All within the brief's allowed list.

## 7. Gates

- `CI=true pnpm --dir web run typecheck` on HEAD: exit 0 (rerun by reviewer).
- `src/appearance.test.ts`: 19/19 pass (rerun by reviewer; covers appearance.ts —
  no component test exists for AppearanceSection.vue, so the tip commit rests on
  typecheck + the live browser assert, per item 1).
- `make build` on 58a8cf4: green (reviewer's batch2 control built this exact commit:
  web-build incl. typecheck + vite build, plus `go build`).
- `.agents/bin/affected` (absolute main-repo path) on clean 58a8cf4: exit 0, 709 s
  (`1124-051216-r-fu.log`): `go test` ok on all four packages (cmd/athena,
  internal/ledger, internal/posting, internal/server — MariaDB tests ran, not
  skipped) + full web suite 63 files / 616 tests passed; zero FAIL lines in log.
- No full `make test` rerun (evidence chain per item 1) and no full browser rerun
  (editor controls per item 2 supersede it).
