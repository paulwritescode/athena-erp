# Summary: C4 slice 2 — tree + effective-date attributes

Merged 2026-11-23 as 919bab4 (train1 tip; built at b3940a3
on f02a4b7; rebased cleanly).

- 12 files: record_types.proto + records.proto (additive
  only, fresh field numbers, no new RPCs) + generated Go/TS
  in sync; attributes/records/record_types/migrations.go +
  store.go (path/depth store, prefix-checked moves, cycle +
  delete-with-children + parent refusal, end-exclusive
  ranges, overlap refusal, as-of listing, type-row
  serialization); trees_test.go (12 tests).
- Migration path: enable backfills top-level (disclosed,
  non-destructive); disable paths destructive-gated with
  counts; flags-off behavior unchanged.
- No gate.go change (no new procedures), no hand-written
  web/cmd changes; Z2/Z4 untouched.
- Review: PASS (review-slice-2.md); full-suite fallback
  reused green on b3940a3; 12/12 trees tests rerun green.
- Merge-time verification: train1 tip gate (full make test
  exit 0, web 613/613) on 919bab4.
- Follow-ups: depth/path-length guard; enable-effective
  plan text omits the index drop; test misnomer;
  disable-effective path untested.
