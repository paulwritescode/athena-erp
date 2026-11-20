# Phase P3 summary: exact types

Review verdict: APPROVED, no findings. Nothing is committed or pushed.

## What changed
- New package `internal/types`, standard library only (`go.mod` unchanged):
  - `decimal.go`: Decimal on `math/big`, `half_up` and `half_even` rounding, parse, format, add, sub, mul, div, round, rescale.
  - `money.go`: Currency and Money in `int64` minor units, with currency and overflow checks. Operations return the rounding difference. Includes Convert and Allocate.
  - `quantity.go`: Quantity with a unit-of-measure key and one-step Convert.
  - `dates.go`: strict `YYYY-MM-DD` dates. RFC 3339 timestamps require an offset and are normalised to UTC.
  - `allocate.go`: deterministic largest-remainder allocation. Ties go to the lowest index.
  - `types_test.go`: unit and property tests.
- Docs: `docs/decisions/adr-0007-exact-types.md` (status PROPOSED), a decision-log row, a link in `docs/README.md`, and a status row in `docs/architecture/overview.md` saying the package is not yet wired in.
- Not changed: proto, UI, CLI, API, AGENTS.md. The roadmap marks P3 as a library with no UI, so no `athena <command>` was added.

## Decisions
- Chose `math/big` over `shopspring/decimal`. It adds no dependency and avoids binary-fraction conversions.
- Property tests use `testing/quick`.
- No spans, because the package does no I/O. OpenTelemetry stays optional.

## Verification
- `go build ./...`, `go vet ./...` and `gofmt -l internal cmd` pass. gofmt printed nothing.
- `go test -count=1 ./...` against local MariaDB passes for cmd/athena, internal/server, internal/store and internal/types.
- Invariant tests cover:
  - allocation sums exactly and is repeatable;
  - splitting and re-adding over three rounds does not drift;
  - scale widen and narrow is lossless, and Money round-trips through Decimal;
  - a test fails if non-test package source uses the binary-fraction type.
- Not run, because nothing changed there: `make generate`, the pnpm typecheck, build and tests, and the browser pass.
- The reviewer read the code and confirmed rounding, sign handling, allocation ties, and currency and overflow checks.

## Open items
- Nothing is committed. `.agents/` and the P3 changes are still uncommitted.
- ADR-0007 stays PROPOSED until you accept it.

## Next: P4, record kinds and system fields
- Add record kinds and system fields to the Record model, and wire the exact types from P3 into Record fields and entries.
- This phase will touch the proto contract (so `make generate`), the Record and entry validation in Go, and the mirrored `web/src/fieldRules.ts`. It will also touch the shared `field_rules.json` table, the dashboard and the docs.
- It needs a plan first, and any platform-table change must be a new numbered system migration.
- Nothing starts until you approve.
