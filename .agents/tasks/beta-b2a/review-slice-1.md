# Review: B2a slice 1, DataTable + Records + Gallery

Branch `feat/b2a-tables` in `../athena-erp-b2a`, one commit `7699b5a` against
`main`. Reviewed against the slice-1 brief, design-system §21 (+ §7, §13,
§14) and tasks.md B2a. Read-only on product code; no commits or merges.

Verdict: PASS

## Blockers

None. No broken behavior, data loss, security hole or unusable screen was
found. The six follow-ups below are all non-blocking under the merge-small
rule.

## Follow-ups (all non-blocking, for tasks.md)

1. `web/src/components/DataTable.vue:443` — the toolbar search icon overlaps
   the first ~16 px of text. The `Input` keeps its base `px-3`: computed
   `padding-left` measures 12 px, so `pl-9` loses the cascade and the text
   starts under the icon (measured: text at x=284, icon spans 284–300;
   visible in every toolbar screenshot). Same risk for `pr-8` under the
   clear button. Fix with an important padding (e.g. `pl-9!`) or equivalent.
2. `web/src/style.css:232-244,441-442` — `.entries-grid` / `.entries-resizer`
   rules are dead after the `RecordsTable.vue` deletion; no `.vue` file
   references them. Remove them (keep `.datatable-stacked`, which the new
   table uses).
3. `web/src/components/records/recordColumns.ts:14` and
   `web/src/pages/RecordsPage.vue:313` — status words are hardcoded
   (`Draft`/`Submitted`/`Cancelled`) where the old table and the editor
   (`RecordsPage.vue:258`) use `gloss()`. Same visible text today, but a B6
   copy change would leave the table behind. Use the glossary.
4. `web/src/pages/RecordsPage.vue:146-157` — clearing filters (or switching
   Record Types) fires two list requests: `onFiltersChanged` loads at once
   while the `filterText` watcher schedules a second `applyListFilter` 500 ms
   later. Same result, one wasted request. Skip the trailing call when the
   filter already applied.
5. `web/src/components/DataTable.vue:369-384` — nit: `pressTimer` is not
   cleared on unmount. Harmless one-shot; clear it for tidiness.
6. `web/src/components/DataTable.vue:441` — nit: search width caps at 448 px
   (`sm:max-w-md`); §21 says up to 480 px. Within tolerance; align if touched.

## What was verified

Checks (all in `../athena-erp-b2a/web`, exit 0): `CI=true pnpm run
typecheck`, `test` (38 files, 440 tests, including `tokens.test.ts`,
`tablePrefs.test.ts`, `dataTableModel.test.ts`, `recordColumns.test.ts`),
and `build`. Tree left clean (`.gitkeep` restored after the build).

Scope: 16 files, all table-owned. No `RecordTypesPage` or record-type
component edits. Shared-file touches are minimal and additive: one
`:username` prop in `PageSwitch.vue`, `Minus` in `icons.ts`, additive
`.datatable-*` CSS, and a path update in `glossary.test.ts`. `RecordsTable`
is fully removed with no remaining references; column widths keep the
`athena-entry-columns:<key>` storage (resize drag wrote
`{"name":260}`; old column ids match, so saved widths carry over).

Browser pass on a throwaway Workspace (`test_athena_b2a_review`, 65
customers + 6 invoices: 3 draft, 2 submitted, 1 cancelled) served from a
fake-HOME server at `127.0.0.1:3457`, driven via CDP in Chrome. The dev
database and other agents' servers were untouched.

- Paging both ways: `Showing 1–25 / Page 1` → Next → `Showing 26–50 /
  Page 2` starting at row 26 → Previous back. No totals on keyset pages.
- Selection: page checkbox selects all 25 and clears; indeterminate minus
  shown; `2 selected` bar with Delete + Clear selection replaces the
  toolbar in place; selection survives paging.
- Bulk delete: `Delete 2 records?` confirm → `2 records deleted.` toast,
  selection cleared, CLI count confirms. Settled/setting rows are refused
  server-side (`internal/server/records.go:529-560`) with per-row failure
  counts, so the unguarded bulk path is safe.
- Search: `doc.balance > dec("8000")` debounces to 17 rows; Clear filters
  restores; no-match shows `No results for these filters` with Clear.
- Columns + page size persist per user
  (`athena-table-columns:administrator:records:customer`) across reload.
- Row menu: Open routes to the editor; submitted-row Delete is disabled
  with `Only a draft can be deleted.` Status filter chips
  (`Status: Submitted`, 2 rows); `doc.docstate` confirmed server-supported
  (`internal/server/expressions.go:119-120`), filter only shown when valid.
- Gallery: client demo (search, Status/Plan popovers, Columns, sorting,
  `Showing 1–25 of 32 / Page 1 of 2`) and keyset demo (`Page 1/2`, no
  total) plus loading/empty/filtered-empty/error states.
- Editor: row Open and New-record create both work through the UI
  (`Record created.`, CLI lookup confirms); lifecycle/template untouched
  apart from list-refresh lines.
- Matrix viewed at 1440/768/375 px in dark and light: bordered frame,
  sticky header, right-aligned tabular amounts; phone shows stacked `#N`
  rows, a Filters drawer (Columns for Records, Status options for
  Invoice, Sort rows for the Gallery client demo), long-press select
  with a bottom action bar, and Previous/Next with disabled reasons
  (wrapper tooltip + `aria-label`, `ActionButton.vue:118,123`).

Screenshots (30, ephemeral): `/tmp/athena-b2a-review/screens/`. CDP
scripts: `/tmp/athena-b2a-review/cdp/`. Throwaway server, Chrome profile,
home and database were removed after the pass.
