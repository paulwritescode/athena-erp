# Review: B2a slice 2, Access + Rules + Files on DataTable

Branch `feat/b2a-tables` in `../athena-erp-b2a`, commit `ed600ca`
(diff `e23de85...HEAD`; slice 1 already in main, not re-reviewed).
Reviewed against the slice-2 brief, design-system §21 (+ §7, §13, §14)
and tasks.md B2a. Read-only on product code; no commits or merges.

Verdict: PASS (fix loop 1 re-review; was NEEDS_CHANGES on ed600ca)

## Blockers (all three reproduced in the browser, throwaway Workspace)

The "explicit targets" refactor freezes the row-menu target in a ref that
the sheet/editor path does not reset. One instance breaks a save; the
other two delete the wrong object.

1. `web/src/pages/AccessPage.vue:523-528,605` — editing an Assignment
   always saves against the wrong user. `openEditAssignment` (reached
   from the sheet via `onAssignmentAction`, `:545`) never sets
   `actionUser` (only set at `:249`/`:254`), but `confirmSaveAssignment`
   saves with `username: actionUser.value` (`:605`) and the preview
   reads `Affects ${actionUser}` (`:1318`). Observed: open revu01's
   sheet, Edit its Report Viewer assignment, set Active from, Preview
   shows `Affects .`, Apply shows `Could not save the Assignment: not
   found: user ""`. On a fresh page the username is empty; after any
   row-menu action it is that stale user (the server's
   `user_id`-scoped update refuses it, so this breaks behavior without
   corrupting data). Fix: set `actionUser` from the sheet user in
   `openEditAssignment`.
2. `web/src/pages/RulesPage.vue:187,215,421,207,528` — cancelling a
   row-menu delete poisons the next sheet delete. `deletingRule` is set
   at `:215`, cleared only in `remove()` (`:208`); the sheet Delete
   (`:421`) opens the dialog without clearing it, and `remove()` (`:207`)
   plus the title (`:528`) prefer the stale value. Observed: row-menu
   delete review_rule_1, Cancel, open review_rule_2's editor, Delete
   shows `Delete review_rule_1?`; confirming deletes review_rule_1
   (CLI: rule_1 gone, rule_2 survives). Wrong-rule deletion is data
   loss. Fix: clear or set `deletingRule` when opening from the sheet
   (or on dialog close).
3. `web/src/pages/FilesPage.vue:329,332,444,178,561,638` — same shape as
   (2): `deletingPath` set by the row menu (`:444`), consumed in
   `remove()` (`:332`), never cleared by the sheet (`:178`) or editor
   (`:561`) Delete path; the title uses it (`:638`). Observed: row-menu
   delete packs/note-1.txt, Cancel, open note-2's sheet, Delete shows
   `Delete packs/note-1.txt?`; confirming deletes note-1.txt (CLI: 27
   notes left, note-1 gone, note-2 survives). Wrong-file deletion is
   data loss. Same fix.

## Follow-ups (non-blocking)

- API gaps, worker-reported and confirmed — no bulk RPCs anywhere in
  `proto/` (the converted tables correctly show no select column), and
  the runs list is hardcoded to `pageSize: 50` with the next token
  ignored (`RulesPage.vue:149`) and no total. Server work, not this
  slice.
- Centered columns stay centered in stacked phone rows (Rules "Enabled"
  sits centered in its card while siblings are left-aligned). The
  `.datatable-stacked` override in `style.css` covers only the legacy
  `.is-*` classes; extend it to the current alignment utilities.
- Mapping table: `onMapRowClick` ignores rows with no file, but those
  rows are still focusable and pointer-styled. Consider a non-clickable
  treatment for `filePath: null` rows.
- `DataTable` shows the Actions column whenever the `row-actions` prop
  is present, even when it returns `[]` (read-only Rules users, the
  administrator row). Cosmetic; consider passing `undefined` when no
  actions exist.
- The `px-4 py-3` to `pt-3 pr-4 pb-3 pl-4` rewrite in `DataTable.vue`
  (skeleton and body cells) is behavior-neutral but unexplained.

## Slice-1 follow-ups: all six fixed and verified

- Search padding + 480 px cap: measured 36 px left / 32 px right, text
  starts clear of the icon, wrapper max-width 480 px.
- Dead `entries-*` / `files-tree` CSS removed; grep finds no remaining
  references (`.row-open` still defined and used by LedgerSetupPage,
  which converts in slice 3).
- Glossary status words: `recordColumns` and the Status filter use
  `gloss()`; Submitted/Draft badges render.
- No double list request: fetch hook counts exactly one `ListRecords`
  on filter and one on clear, none trailing after 1.5 s.
- `pressTimer` cleared in `onUnmounted` (code).

## What was verified

Checks (all in `../athena-erp-b2a/web`, exit 0): `CI=true pnpm run
typecheck`, `test` (39 files, 451 tests, token guard green, adapted
`accessRace` test passes), and `build`. Tree left clean.

Scope: 9 files, conversions plus the listed follow-ups only. Versions,
history, permissions-matrix and simulate tables stay plain (confirmed
in the diff); no RecordTypesPage or record-type component edits;
`registry.ts` only threads `username`.

Browser pass on throwaway Workspace `test_athena_b2a_slice2` (31
users, 10 roles, 8 rules, 18+ runs, 30 files), fake-HOME server at
`127.0.0.1:3457`, Chrome via CDP. Dev database and other servers
untouched.

- Users: paging 31 across 2 pages, search, State filter with chip, row
  menus (none on administrator), sheet opens; roles: search, New Role
  create, row Edit menu, row Delete with confirm (10→11→10, CLI
  confirms); sessions: search, filtered-empty, End session signs out
  with the proper message, re-login works.
- Rules: search, Type/Status filters with chips, row Delete menu,
  editor opens; runs: Outcome/Event filters, filtered-empty, row opens
  the run detail.
- Files: paging 30 across 2 pages, search, Folder filter with chip,
  row Open/Download/Rename/Delete menu, detail sheet, rename works
  (`Moved packs/note-3.txt to packs/renamed-3.txt`, CLI confirms);
  mapping: backed + No-file rows, search, backed-row jump to the file,
  No-file click is a no-op.
- Matrix viewed at 1440/768/375 px in dark and light on all three
  pages: §21 frame, toolbar, footer and stacked `#N` rows throughout;
  phone Filters drawer holds State + Columns.

Screenshots (~25, ephemeral): `/tmp/athena-b2a-review2/screens/`. CDP
scripts: `/tmp/athena-b2a-review2/cdp/`. Throwaway server, Chrome
profile, home and database were removed after the pass.

## Fix loop 1 re-review (commit 746af86): all blockers fixed

The fix sets `actionUser` from the sheet user in `openEditAssignment`,
clears `deletingRule`/`deletingPath` on the sheet/editor delete paths
and on dialog close, and captures the target in `remove()` before
closing. Checks re-run green: typecheck, 451/451 tests, build, tree
clean. All three reproductions re-run on throwaway Workspace
`test_athena_b2a_fix1` with CLI/DB confirmation:

1. Edit-assignment: preview now reads `Affects fixu01.`, Apply shows
   `Saved the Assignment.`; DB confirms `active_from 2026-11-01`,
   revision 2 on fixu01's Report Viewer assignment.
2. Cancel row-menu delete of fx_rule_1, then sheet delete with
   fx_rule_2's editor open: dialog correctly reads `Delete fx_rule_2?`;
   confirming deletes fx_rule_2 (CLI: fx_rule_1 and fx_rule_3 survive).
3. Cancel row-menu delete of packs/fx-1.txt, then sheet delete with
   fx-2's sheet open: dialog correctly reads `Delete packs/fx-2.txt?`;
   confirming deletes fx-2 (CLI: fx-1, fx-3, fx-4 survive).

Regression on the normal paths, all correct with CLI confirmation:
row-menu rule delete and direct editor rule delete (fx_rule_1,
fx_rule_3, ending in the `No Rules yet` empty state), row-menu file
delete and editor file delete (fx-1, fx-4), and a new Auditor
assignment via the sheet (`Affects fixu01.`, DB confirms the second
assignment on fixu01).

Observation, out of scope (CLI untouched by this web-only slice):
`athena user list` showed only `Report Viewer` for fixu01 while the
desk and the assignments table both show Auditor + Report Viewer —
pre-existing CLI display quirk, not a slice regression.

Scripts: `/tmp/athena-b2a-review3/cdp/`. Throwaway environment removed
after the pass.
