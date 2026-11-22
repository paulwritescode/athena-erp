# Review: B2a slice 3, Ledger masters + Record Types on DataTable

Branch `feat/b2a-tables` in `../athena-erp-b2a`, commit `fcb4c66`
(diff `0431689...HEAD`; slices 1–2 already in main, not re-reviewed).
Reviewed against the slice-3 brief, design-system §21 (+ §7, §13, §14)
and tasks.md B2a. Read-only on product code; no commits or merges.

Verdict: PASS

## Blockers

None. No broken behavior, data loss, security hole or unusable screen
was found.

## Follow-ups (all non-blocking)

1. `web/src/pages/LedgerSetupPage.vue` companies table — the
   current-company marker is lost: the old list highlighted the
   working company (`chosen` class); the DataTable has no equivalent
   (no bulk selection without a bulk API). Switching still works
   (row click sets it) and the company still shows in section
   descriptions and the readiness select, but the at-a-glance marker
   is gone. Restore one when DataTable supports marker rows.
2. Worker-reported gaps, confirmed with evidence (not blockers per the
   brief): (a) the workspace CLI builds its DSN without ParseTime
   (`cmd/athena/main.go:228` vs the server's `utcConfig` at
   `internal/server/server.go:251`), so CLI period/date reads fail —
   reproduced: `ledger period save/list` → `sql: Scan error ...
   []uint8 into *time.Time`. The desk is unaffected (server path).
   (b) `proto/athena/v1/ledger.proto` has no bulk or delete RPCs
   (Save/List only, plus SetPeriodStatus) — the tables correctly show
   no select column, bulk bar or row menus.
3. B2a closure needs one master confirmation: the Access permissions
   matrix (`AccessPage.vue:1144`) stays a plain table per the slice-2
   brief, but the B2a task text names "permissions". It is a
   Role × RecordType action grid, not a list — DataTable does not fit
   it — so leaving it plain looks right, but the completed B2a box
   should record the decision. Notifications full list skipped per
   Stylus (verified: no notifications page exists). Other intentional
   plain tables: Rules versions/simulate, Files history, readiness
   checklist (all verified still plain and working).

## What was verified

Checks (all in `../athena-erp-b2a/web`, exit 0): `CI=true pnpm run
typecheck`, `test` (43 files, 490 tests, token guard green, updated
`recordTypeList` multi-select tests pass), and `build`. Tree left
clean.

Scope: 8 files. Shared changes are additive only: `TableFilter.hint`
rendered as a HelpHint in the phone drawer, `PrimaryCell` mono title
for ledger codes; `registry.ts` only threads `username`; dead
`.row-open`/`.ledger-new` CSS removed with zero remaining references
(grep); `filterRecordTypes` single→multi-select with tests.

Ledger (throwaway Workspace `test_athena_b2a_s3`, fake-HOME server,
Chrome via CDP; dev DB and other servers untouched): all 11 tables
render with search, sorting, client paging and per-user columns —
companies, branches, departments, cost centres, dimensions, fiscal
years, periods (Status filter: `soft_closed` → 1 row), currencies,
account types, accounts (30 rows, paging both ways, descending sort
verified), posting rules. New actions live in the toolbars; row
clicks open sheets/forms (company, currency, account sheets verified);
created branch/department/cost-centre/dimension/fiscal-year/period/
company through the UI; soft-close works. Empty and filtered-empty
states observed; readiness checklist stays a plain working table (7
rows). Column hide persists across reload.

Record Types: search, multi-select Kind (`Kind: Master +1`, both
rows) and Pack filters, sorting, `View records` row menu → the
Record's list, filtered-empty, and phone drawer with Kind/Pack hints
(verified visually).

Matrix viewed at 375/768/1440 px in dark and light on both pages:
§21 frame, toolbar, footer and stacked `#N` rows throughout.

B2a completion sweep: every table the B2a task lists is now on
DataTable except the permissions matrix (item 3 above) and the
skipped notifications page; no bespoke grids remain outside
DataTable, and no other page holds a list-table.

Screenshots (~20, ephemeral): `/tmp/athena-b2a-s3/screens/`. CDP
scripts: `/tmp/athena-b2a-s3/cdp/`. Throwaway server, Chrome profile,
home and database were removed after the pass.
