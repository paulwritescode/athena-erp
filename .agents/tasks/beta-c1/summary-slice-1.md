# Summary: C1 slice 1, posting validation + writer (merged a9e1d49)

## What shipped

New `internal/posting/`: plan validation (presence, enabled company +
base currency, leaf/enabled/in-scope accounts, one-sided non-negative base
amounts, txn side, quantity/UOM pairing, dimension axes, control sources,
per-company open-period cover, balance per company+book in base plus
per-currency txn nets) and a single-transaction writer (21-column ledger
rows, balance deltas via DB-side increment with insert fallback, audit
append, outbox row, all atomic). Empty-book and hard-closed-period cases
return `failed_precondition` sentinels (Z2 undecided — nothing invented),
pinned by tests. Genuine rapid property test for balanced postings
(100 cases + +1¢ perturbation writes nothing). Reuses money/allocate
(where applicable), audit and the ledger service. Migration-14 tables
reused; no new migration or ADR.

## History

- Worker w-c1 committed 9d87fbf (validation + writer + tests) and a9e1d49
  (drop stale rapid repro). Three new files, zero edits elsewhere.
- Review round 1 PASS: line-by-line verification (validation rules,
  21-column DDL match, atomicity incl. empty-table assertions on failure,
  race reasoning, genuine property coverage, API reuse, safe dangling
  group id), Z2 grep-verified clean, full suite green, 6 follow-ups.
- No rebase needed (already on main d426a3c); all checks green; merged
  --ff-only, pushed, recorded slice line (slices 2–6 open).

## Follow-ups (in tasks.md)

fitsLedger digit tightness; calendar-date validation; multi-company test;
hashPlan Decimal normalization (matters in slice 2); Engine Dialect default.

## Try it

`go test ./internal/posting/ -run TestBalancedPostingsProperty -count=1`
with `ATHENA_TEST_DATABASE_URL` set; read `internal/posting/posting.go`
for the validation rules.
