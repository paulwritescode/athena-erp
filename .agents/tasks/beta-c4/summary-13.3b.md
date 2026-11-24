# Summary: C4 slice 13.3b — ledger saves recompute tree paths

Merged 2026-11-24 as 8add95b in train3 (built at 764655e,
rebased cleanly twice — 312caf7, 3175005; patch identical).

- Added per Stylus: ADR-0029 line 27 ("ledger saves
  recompute path and depth") had no implementing slice.
- `accounts.go` + `masters.go`: every ledger save
  (accounts, branches, departments, cost centres)
  recomputes tree_path + tree_depth for the saved row and
  its descendants inside the save transaction; reparenting
  rewrites the moved subtree; cycles/dangling parents
  refused exactly as today.
- Tests in NEW accounts_test.go + masters_test.go only
  (slice-4 separation kept): create, reparent, subtree
  depth, stale-path regression.
- No migration (columns from 18), no proto/gate/web/cmd
  changes; ADR untouched; Z2/Z4 untouched.
- Review: PASS (review-13.3b.md) — recompute verified on
  all four save paths in-tx; gates green (one 20m timeout
  retry under load, then green).
- Merge-time verification: train3 tip gate (full make test
  exit 0, web 616/616) on 2377cb7.
- Follow-ups F1–F5: cross-level reparent test; reparent-to-
  root test; label-only save skips refresh (repair or ADR
  note); NULL-parent plain error → inputErrorf; hardcoded
  revisions in tests.
