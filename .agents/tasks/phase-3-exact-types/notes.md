# Phase P3 notes: exact types
Nothing committed or pushed. The pre-existing untracked `.agents/` was left alone.
## What changed
- `internal/types` (new, standard library only; `go.mod` unchanged): `decimal.go` (Decimal on `math/big`, `half_up` and `half_even`, parse/format/add/sub/mul/div/round/rescale), `money.go` (Currency, Money in `int64` minor units, overflow and currency checks, rounding difference returned, Convert, Allocate), `quantity.go` (Quantity with UoM key, one-step Convert), `dates.go` (strict `YYYY-MM-DD`, RFC 3339 with offset normalised to UTC), `allocate.go` (deterministic largest-remainder allocation, ties to lowest index), `types_test.go`.
- Library choice: `math/big` over `shopspring/decimal` (no new dependency, no binary-fraction conversions). Property tests use `testing/quick`. Both recorded in ADR-0007 (PROPOSED).
- Docs: `docs/decisions/adr-0007-exact-types.md`, decision-log row, `docs/README.md`, `docs/architecture/overview.md` status row.
- No proto, UI, CLI or AGENTS.md change. The roadmap lists P3 as a pure library with no UI and no user-facing operation (wiring is P4), so no `athena <command>` was added; README and CLI help are unchanged. No spans, since the package does no I/O.
## Invariant tests
Allocation sums exactly and is repeatable (`testing/quick`); split and re-add over three rounds never drifts; scale widen and narrow is lossless and Money round-trips through Decimal; a test fails if any non-test source in the package mentions the binary-fraction type.
## Files changed
internal/types/{decimal,money,quantity,dates,allocate}.go, internal/types/types_test.go, docs/decisions/adr-0007-exact-types.md, docs/decisions/decision-log.md, docs/README.md, docs/architecture/overview.md, this file.
## Verification (local MariaDB, .env.local sourced, values not printed)
- `go build ./...`, `go vet ./...`: pass. `gofmt -l internal cmd`: no output.
- `ATHENA_TEST_DATABASE_URL=... OTEL_EXPORTER_OTLP_ENDPOINT= go test -count=1 ./...`: pass (cmd/athena, internal/server, internal/store, internal/types).
- Not run, nothing to verify: `make generate` (no proto change), pnpm typecheck/build/tests and `make restart` browser pass (no web change).
