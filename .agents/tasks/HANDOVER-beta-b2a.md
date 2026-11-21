# Handover: beta-b2a (B2a table system, slice 1) — PAUSED for laptop restart

## Worktree and branch

- Worktree: `/Users/iampaulking/projects/duchess/athena-erp-b2a`
- Branch: `feat/b2a-tables`
- HEAD at pause: `ec98c5f` = `origin/main` ("feat(auth): idle warning, cross-tab sessions and passive background calls (B7 Desk)"). Includes B6. No commits made on the branch (per brief: do not commit).
- Brief: `/Users/iampaulking/projects/duchess/athena-erp/.agents/tasks/beta-b2a/brief.md`
- All slice-1 changes are uncommitted in the worktree (nothing stashed; `git stash list` is empty).

## What is done and verified (slice 1, code-complete)

The DataTable itself, Records wiring, and Gallery showcase are implemented:

- `web/src/components/DataTable.vue` (rewritten, ~800 lines): §21 frame, padding, select column (page checkbox + indeterminate, only when bulk actions exist), row numbers across pages, toolbar (search with clear-x, filter popovers with value chips, Clear filters link, Columns dropdown persisted per user), selection bar replacing the toolbar (desktop) / fixed bottom action bar (phones), footer (range, rows-per-page persisted per user, Page x of y, Previous/Next with disabled reasons), client paging + server keyset mode (no total → "Page N"), loading/empty/filtered-empty/error states in-frame, resizable columns, stacked phone rows with `#N` prefix, long-press select mode, phone Filters drawer (sort + filters + columns). Old props kept working so RecordTypesPage (B9b's area) is untouched.
- `web/src/components/dataTableFeatures.ts` (new): shared TanStack feature set + `DataTableColumn<T>` type.
- `web/src/tablePrefs.ts` + `tablePrefs.test.ts` (new): per-user columns/page-size localStorage helpers (`athena-table-columns|page-size:<user>:<table>`).
- `web/src/dataTableModel.ts` + `dataTableModel.test.ts` (new): pure footer math (range/page/selection text, row numbers).
- `web/src/components/PrimaryCell.vue` (new): icon tile + title + second line.
- `web/src/components/records/recordColumns.ts` + `recordColumns.test.ts` (new): Record column builder (Status badge, primary label-field cell, link-label resolution, numeric/center alignment, phone stacking rule) + `combineRecordFilter` (CEL search ANDed with `doc.docstate` Status filter; `doc.docstate` verified filterable server-side in `internal/server/expressions.go`).
- `web/src/pages/RecordsPage.vue` (list section rewritten; editor untouched): DataTable with server keyset paging (token stack for Previous), debounced CEL search (500 ms, filter errors inline only to avoid toast storms), Status filter for submittable types, bulk delete with confirm (uses known revisions, fetches the rest, summary toasts, reloads page one), row Open/Delete menu (Delete disabled with reason for settings/non-drafts, per server rules), resizable with the existing `athena-entry-columns:<key>` width key preserved, per-type `:key` remount, per-user table id `records:<key>`.
- `web/src/components/RecordsTable.vue` (deleted, replaced by the above).
- `web/src/pages/GalleryPage.vue`: full client demo (32 customers, search + 2 filters + Columns + selection + bulk + row actions + paging + state buttons) and a 90-row keyset server-paging demo without total.
- `web/src/components/PageSwitch.vue`: one line — passes `username` to RecordsPage.
- `web/src/copy/glossary.test.ts`: one line — B6's View-files list pointed at `components/records/recordColumns.ts` after the RecordsTable deletion (this edit is required for tests to pass).
- `web/src/style.css`: only additive `.datatable-*` rules (frame max-height + sticky header, resizer, stacked `#N`/select/actions rules). Old `.entries-*` rules left in place (dead but harmless; cleanup is a follow-up).

Verified (all in this worktree, exit 0):

- `CI=true pnpm --dir web run typecheck` — pass.
- `CI=true pnpm --dir web run test` — 440 passed, 38 files.
- `CI=true pnpm --dir web run build` — pass (pre-existing chunk-size warning only).
- `go build -o bin/athena ./cmd/athena` — pass.

## What is in progress (file by file)

- Slice-1 browser pass — NOT STARTED. This is the next step (see below). No screenshots taken yet; `.cache/screens/b2a-*` does not exist.
- No file is mid-edit. Every file above is in its finished slice-1 state.

## Exact next steps (slice-1 browser pass)

1. Rebuild the binary if needed (embeds `internal/webui/dist`, already built): `go build -o bin/athena ./cmd/athena`.
2. Start the throwaway workspace server (port 3100 to avoid clashes):
   `cd /tmp/b2a-home/test_b2a_tables && ATHENA_SERVER_PASSWORD='74fdd471be62652ab073332c8957538e' /Users/iampaulking/projects/duchess/athena-erp-b2a/bin/athena run --addr 127.0.0.1:3100`
   Detach with `setsid -f ... </dev/null >>/tmp/b2a-server.log 2>&1`, then `curl 127.0.0.1:3100/healthz`. Watch stderr: if the root password was generated (rather than the DB password), it is printed once ("shown once").
3. Create Record Types via CLI from `/tmp/b2a-home` (needs `--workspace test_b2a_tables` or run inside the workspace dir). JSON shape: see `internal/server/record_types.go` (`label_field`, `kind`, `submittable`, fields). Suggested: master `customer` (name text required + label_field, email, plan enum, seats number) and submittable `order` (for the Status filter).
4. Seed 65+ customer records: loop `athena records create customer --values '{"name":"..."}'` (CLI acts as operator, no Grants needed). For orders: create several, then `athena records submit order <id> --revision N` on some (get revisions from `records list --json`).
5. Sign in as `administrator` (password = DB password above unless stderr showed a generated one).
6. Drive Chrome (installed at `/Applications/Google Chrome.app`, v158; NO playwright — use raw CDP from Node 26 at `~/.nvm/versions/node/v26.7.0/bin/node`, global WebSocket exists): sign in, open the Records list, exercise paging forward/back, cross-page selection, bulk delete with confirm, Status filter + search + clear, Columns chooser, at 375 / 768 / 1440 in light (`localStorage athena-theme=light`) and dark (`=dark`). Automation hooks: aria-labels "Select row N", "Select every row on this page", buttons Previous/Next/Columns/Clear filters, search placeholder text.
7. Save screenshots to `.cache/screens/b2a-*` in the worktree.
8. Re-run `typecheck`, `test`, `build` (exit 0), then report `hc send t6 "B2A SLICE 1: PASS"` (or BLOCKED with reason). Do not commit/merge/push.

Known API gaps to report with slice 1 (do not change Go): ListRecords has no total count and no free-text search (search keeps CEL syntax) and no sort parameters (sorting off in server mode); no bulk-delete RPC (desk loops single deletes).

## Commands to verify

- `git -C /Users/iampaulking/projects/duchess/athena-erp-b2a status --short` — expect only the files listed above (M: DataTable, PageSwitch, glossary.test, GalleryPage, RecordsPage, style.css; D: RecordsTable; ??: PrimaryCell, dataTableFeatures, records/, dataTableModel*, tablePrefs*).
- `export PATH="$HOME/.nvm/versions/node/v26.7.0/bin:$PATH"; CI=true pnpm --dir web run typecheck && CI=true pnpm --dir web run test 2>&1 | grep -E "Test Files|Tests " && CI=true pnpm --dir web run build 2>&1 | tail -2` (from the worktree).
- `go build -o bin/athena ./cmd/athena` (from the worktree).

## Fresh-session must-knows

- Node is NOT on default PATH: use `export PATH="$HOME/.nvm/versions/node/v26.7.0/bin:$PATH"` (node v26.7.0, pnpm 11.22.0). `web/node_modules` is installed.
- MariaDB was already running (Homebrew, `mysqld is alive`); I did not start or stop it — leave it alone.
- NO temp server is running (never started one). Nothing to stop.
- Temp artifacts (all outside the repo, safe to reuse): home `/tmp/b2a-home` (server login user `athena_app`), workspace dir `/tmp/b2a-home/test_b2a_tables`, database `test_b2a_tables`. The `test_` prefix is REQUIRED: `athena_app` can only create `test_*` databases (verified via SHOW GRANTS). Server password for the home: `74fdd471be62652ab073332c8957538e` (from the worktree's `.env.local` DSN — do not commit).
- File backup of my first five new files (paranoia copy): `/tmp/b2a-backup/`.
- B9b stray-edit incident (resolved): another worker briefly wrote B9b files into this worktree; master reverted them and this tree is clean. If foreign edits reappear, do not touch them — report via `hc send t6`.
- `hc` is at `/Users/iampaulking/.local/bin/hc`; master terminal is `t6`.
- Slices 2 (Access/Rules/Files) and 3 (Ledger/notifications, Record Types last after B9b) are NOT started; each merges and reports separately.
