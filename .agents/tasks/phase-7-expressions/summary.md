# P7 summary: expressions (cel-go)

Review verdict: APPROVED, no findings. Nothing is committed yet.

## What changed
- New `internal/expr`: restricted CEL (cel.dev/cel-go v0.33.4). No macros, loops, floats, lists or maps. Exact decimal, money and date types. Deterministic evaluation. Translation of filters to SQL through four new `store.Dialect` methods.
- Field attributes `formula`, `required_if`, `read_only_if`, `validate`, `depends_on`. They are compiled at save, and cycles, self-reference and wrong result kinds are refused.
- Write path: formulas are stored in dependency order, and child changes recalculate the parent's rollups in the same transaction with a `recalculated` Audit Log row. A formula change adds a `recalculate` plan step.
- API: `ListRecordsRequest.filter`, `PreviewRecord`, `EvaluateExpression` (ConnectRPC only).
- CLI: `athena expressions check|eval|preview`, `records list --filter`.
- Telemetry spans carry no Record values (tested).
- Frontend: formula and condition fields in the field settings dialog, a debounced preview in the record form, and a filter box. Vitest cases cover `fieldRules.ts`.
- Docs: ADR-0013 (Proposed), D-017, decision log, overview, README.

## Verification (MariaDB 11.8)
- `go build ./...`, `go vet ./...` and `gofmt` are clean.
- `go test -count=1 ./...` passes in all packages.
- `make generate` run twice gives identical output.
- Web typecheck, Vitest (72 tests) and build pass.
- `make restart` gives a healthy `/healthz`.
- Headless Chrome checks in light and dark mode: 26 of 26 pass.

## Open points
- Parent rollups need a second save, because the child type must exist first.
- Compiled expressions are not cached.
- Size (2000) and recursion (32) limits are not pinned by a test.
- PostgreSQL is untested for the new dialect methods.
- Lookup fields, `hidden_if`, `default_from` and the `user`/`setting`/`ctx` scopes are not built.

## Next: P8
Authentication with argon2id password hashing, database-backed sessions and RBAC (Grants). This also closes the "no authentication" gap on Record CRUD.
