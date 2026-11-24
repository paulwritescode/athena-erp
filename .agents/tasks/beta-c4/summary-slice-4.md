# Summary: C4 slice 4 — Property 25 (13.4)

Merged 2026-11-24 as 094610d (built at f615096 on 58948de;
single-commit cherry-pick, clean).

- Tests-only delta (2 files, +213): NEW
  `internal/ledger/ledgers_test.go` (period range-reject +
  as-of + DB-backed UoM round-trip properties) + append to
  `internal/types/types_test.go` (pure Convert/FromBase
  bound + exactness).
- Genuinely randomized properties (rapid; row scales 0–6,
  both modes through the real ToBase/FromBase code), not
  example lists. Period oracle matches the implementation
  exactly; touching-bounds direction tested.
- No non-test changes; types_test.go append-at-end only;
  no proto/gate/migration/web/cmd changes; Z2/Z4 untouched.
- Review: PASS (review-slice-4.md) — real properties, exact
  half-ulp bound verified; gates green.
- Merge-time verification: full make test exit 0 (Go ok,
  web 616/616) on 094610d.
- Follow-ups: composed two-leg bound formula still unpinned
  (pin suggested); ledgers_test comment overclaims period
  proof of record effective-dating (periods are inclusive,
  ranges end-exclusive); rapid bound redrawn per check
  (harmless nit).
