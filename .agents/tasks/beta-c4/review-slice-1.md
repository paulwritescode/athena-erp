# Review: C4 slice 1 — migration 18 + ADR-0029 (13.1)

Verdict: PASS

Worktree /Users/iampaulking/projects/duchess/athena-erp-c4, branch feat/c4-trees,
commit d9cee00, tree clean. Diff 9cf4005...HEAD is exactly the 5 briefed files
(232 insertions, 0 deletions): sysmigrations.go, sysmigrations_test.go,
ADR-0029, decision-log.md, docs/README.md. No product logic, proto or web.
Reviewed against brief slice 1, batch-2 design G13, R13.1/R13.4, and the
migration-18 reservation in .agents/stylus/migrations.md.

## Blockers

None.

## Checks (all pass)

- Migration 18 matches ADR-0029 + R13.1 (path/depth) + R13.4 (families, exact
  factors, item overrides): `athena_uom_families` (name PK, label, base_unit,
  disabled, revision), `athena_uom_conversions`
  (PK family/unit/item_key, item_key default '', factor DECIMAL(30,12), scale
  default 6, disabled, revision), tree_path VARCHAR(2048) + tree_depth with a
  prefix index on the four kernel tree masters, effective_from/effective_to
  DATE with an index on determinations.
- Anchors exist: `parent_name VARCHAR(140) NULL` on all four tables and
  `match_json` on determinations (migration 13). `ADD INDEX IF NOT EXISTS`
  already used by migration 17 (sysmigrations.go:556).
- Backfill correct and bounded: roots first, 100 passes max, fails naming the
  table and row count on leftovers. Idempotent per the Migration contract
  (IF NOT EXISTS, NULL guards; the struct has no Down — forward-only).
- Claims verified in code: names hold no slashes (`checkName`,
  internal/ledger/ledger.go:67, `^[a-z0-9][a-z0-9_-]{0,139}$`); saves refuse
  cycles and dangling parents (`checkNested` masters.go:310,
  `checkAccountCycle` accounts.go:135).
- Nothing seeded (no INSERT; test asserts both UoM tables empty). No other
  migration touched (entry appended only, sysmigrations.go:659).
- ADR-0029 Proposed with owner unassigned; decision-log and README rows
  accurate; D3 ADR-0030 rows intact — both rows present in numeric order.
- Test covers a 3-level account tree, a 2-level branch tree, empty UoM tables
  and a legacy determination with open-ended NULLs.

## Commands observed (this session)

- `gofmt -l cmd internal` empty; `go build ./...`, `go vet ./...` pass.
- Targeted: `go test ./internal/server/ -run
  'TestTreesEffectiveUoM|TestSystemMigrations|TestAccessModelMigration'` ok.
- Scoped suite per master's ORDER (full-suite wait interrupted, requeued via
  absolute `affected`): plan `Go: ./cmd/athena ./internal/server | web: no`;
  result `ok athena-erp/cmd/athena (cached)`, `ok athena-erp/internal/server
  156.791s`.
- `make generate` twice: clean tree, no diff. No browser pass (per brief).

## Follow-ups (non-blocking)

- Leftover-row error (sysmigrations.go, backfillTreePaths) says "cycle or
  missing parent" but also fires for chains deeper than 100 passes; a
  deeper-than-100 chain would misreport. Cosmetic; consider naming the depth
  bound in the message in a later slice.
