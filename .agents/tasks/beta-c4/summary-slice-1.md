# Summary: C4 slice 1 — migration 18 + ADR-0029 (13.1)

Merged 2026-11-23 as d54d6e4 (built at d9cee00, rebased cleanly;
patch byte-identical).

- Migration 18 `trees_effective_uom`: `athena_uom_families` (name
  PK, label, base_unit, disabled, revision),
  `athena_uom_conversions` (PK family/unit/item_key, factor
  DECIMAL(30,12), scale, disabled, revision), `tree_path`
  VARCHAR(2048) + `tree_depth` with a prefix index on the four
  kernel tree masters, `effective_from`/`effective_to` DATE with
  an index on determinations. Backfill roots-first, 100 passes
  max, fails naming table + row count; idempotent, forward-only,
  nothing seeded. No other migration touched.
- ADR-0029 "Trees, effective dating and UoM" (Proposed) +
  decision-log/README rows (kept alongside the D3 ADR-0030 rows).
- Review: PASS, no blockers (review-slice-1.md). Gates: gofmt
  empty, build/vet clean, targeted migration tests ok, scoped
  affected run green (cmd/athena + internal/server, 169s),
  generate twice with no diff. One cosmetic follow-up (backfill
  error message), recorded in tasks.md.
- Merge-time verification on d54d6e4 (identical patch): full
  make test exit 0 in a detached worktree (slice 2 stacked on
  the branch meanwhile; merged the slice-1 commit only).
- Migration 18 marked merged in .agents/stylus/migrations.md.
- Open: slices 2–6 (attributes, UoM, Property 25, CLI + Desk,
  docs + checkpoint).
