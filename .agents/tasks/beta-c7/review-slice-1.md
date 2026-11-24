# Review: C7 slice 1 — migration 21 + ADR-0032 (11.1)

Reviewer: r-c7. Worktree: `../athena-erp-c7-r`, detached at 2625faf, verified
clean on arrival (`git status` empty) and clean after every gate. Diff
`main...HEAD` is exactly the 5 reported files, +402/-0.

Verdict: PASS

## Item-by-item (review-brief-slice-1)

1. **Migration 21 `stock_ledger` — PASS.** `internal/server/sysmigrations.go:760`
   appends `{Version: 21, Name: "stock_ledger"}` last, after version 20; number
   matches the reservation (`migrations.md`: 21 C7 reserved 2026-11-24).
   Ten `CREATE TABLE` statements, no ALTERs, no seeds: `l_stock`,
   `l_stock_value`, `athena_stock_layers`, `athena_item_applications`,
   `athena_stock_bins`, `athena_reservations`, `athena_serials`,
   `athena_batches`, `athena_integrity_log`, `athena_stock_rebuilds`.
   All 9 tables of the design Data Models row
   (`design.md:174`) are present; the 10th (`athena_stock_rebuilds`) is
   disclosed in ADR-0032 as additional with its reason (rebuild watermark
   persistence). `l_stock` carries every common ledger column from the design
   doc (id, company, book, voucher type/id/revision, source line,
   posting group, business date, dimensions JSON, is_reversal/reversed id).
   `DECIMAL(28,6)` follows the implemented `l_gl` precedent
   (`sysmigrations.go:446+`), and the ADR says so explicitly rather than
   repeating the design doc's stale "decimal strings" line.
   Applies cleanly up: my own uncached run of
   `TestStockLedgerMigrationCreatesTheStockTables` (builds at 20, ups to 21,
   checks columns/keys/emptiness) passed in 12.88s; full `-run Migration`
   sweep passed uncached in 89s (see item 4).
2. **ADR-0032 (Proposed) — PASS.** Status Proposed, dated 2026-11-24, owner
   unassigned. Z3 transcribed exactly as decided in the brief: perpetual
   weighted (moving) average for all stock with one formula per IAS 2.25;
   FEFO as picking order not cost formula; rentals tracked-not-consumed
   (serial rows + event reservations, zero-value movements); negative stock
   refused for valued items (lockable bins). Nothing seeded; no invented
   policy (the rebuild-state addition and the decimal representation choice
   are both disclosed with reasons); Consequences honestly maps G11.2–G11.9
   onto slices 2+.
3. **Decision-log + README rows — PASS.** Both rows added, both link to the
   ADR file that exists (`docs/decisions/adr-0032-stock-valuation-and-reservations.md`);
   wording matches the ADR (ten tables, weighted average, FEFO, rentals,
   no negative, Z2 open, sentinels byte-identical). No other live doc
   references migration 21 or ADR-0032; the batch-2 spec files' printed
   numbers (migration 19 / ADR-0028 for stock) are the known-stale ones the
   brief already calls out.
4. **Evidence chain (crux) — PASS, verified independently.**
   - Worker's cached `make test` (log `1124-051201-w-c7.log`, head 1559078,
     14s): confirmed 17/17 Go packages `(cached)`, exit 0 — proves nothing
     about this content, as the brief says.
   - Worker's uncached `go test -count=1 ./internal/server` (log
     `1124-054007-w-c7.log`, head **2625faf**, clean tree, exit 0):
     verified the log exists — 2 lines, `ok athena-erp/internal/server
     267.809s`, zero FAIL/panic/race strings. Command in `suite.jsonl:295`
     confirms `-count=1`, so this is genuine uncached evidence on this
     exact commit.
   - My own reruns on 2625faf (filtered `-run`, direct, `-count=1`,
     throwaway `test_athena_*` DB): `TestStockLedgerMigrationCreatesTheStockTables`
     PASS 12.88s; all `-run Migration` tests `ok` 89.054s. Migration proven
     twice, not from cache.
5. **Scope containment — PASS.** No other tables/columns (pure CREATEs);
   no proto, gate, web, cmd, posting, ledger, quantity, or UoM changes
   (5-file diff). Z2 untouched: `ErrBookPolicyOpen` /
   `ErrCorrectionWindowOpen` still byte-identical in
   `internal/posting/posting.go:60-64`, no posting diff. Z4 untouched: no
   secrets/retention code.
6. **Gates — PASS, all run by me on this tree (nothing reused).**
   `gofmt -l cmd internal` empty; `go build ./...` ok; `go vet ./...` ok;
   `make generate` twice (with the suite's node path) both ok with zero
   `git status` diff afterwards. No browser pass (no Desk) — correct.
   Per the brief, suite reuse is invalid here: I ran everything above
   myself and reused only the worker's item-4 logs, which I read in full.

## Blockers

None.

## Follow-ups (non-blocking, for later slices / master)

- `athena_integrity_log.owner` is `VARCHAR(255)` while every other `owner`
  column in this migration is `VARCHAR(60)` (`sysmigrations.go:958` vs
  e.g. `:766`). Harmless but inconsistent; normalise in a later slice if
  the stock service wants one width.
- `uq_reservations_hold` omits company/book (holder UUID is globally
  unique, so this is sound, but it differs from the company-scoped
  `uq_serials_unit`/`uq_batches_lot` pattern). Slice 5 should confirm the
  intended grain when implementing takes.
- Numbering note for master: batch-2 `design.md:87` / `tasks.md:452-453`
  print ADR-0032 for G15 integrations — now taken by this slice. A future
  task needs a renumber there, not this one.
