# ADR-0007: Exact numeric types
- **Status:** Proposed
- **Date:** 2026-11-19
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0003, spec decision 17.5, roadmap phase P3
## Context
The kernel specification (3.1, 3.4) requires exact numerics, money as integer minor units plus a currency, quantities with a unit of measure, and allocation that always sums back exactly. Today `Number` and `Currency` fields are `DECIMAL(18,6)` decimal strings with no currency reference, no rounding rule and no allocation.
## Options considered
1. **Decimal strings in storage, `math/big` in code (chosen for user Number fields).** `Decimal` is a `big.Int` unscaled value plus a scale, parsed from and printed as strings. No new dependency and no way to reach binary fractions.
2. **Scaled integers everywhere.** Fast and exact, but every field needs a scale discipline and 64-bit limits. Chosen only for money, as `int64` minor units, which matches the spec and the roadmap's storage proposal.
3. **`github.com/shopspring/decimal`.** Convenient, but it is a new dependency and exposes conversions to and from binary fractions, which the package must never contain.
## Decision (proposed)
- `internal/types` is pure Go on the standard library (`math/big`). `go.mod` is unchanged.
- `Decimal`: parse, format, add, subtract, multiply, compare, divide and round. Rounding modes are `half_up` (ties away from zero) and `half_even` (bankers). Parse accepts at most 65 digits (the MariaDB DECIMAL bound) and rejects exponents and special values.
- `Money`: `int64` minor units plus a `Currency` (ISO-style code, exponent 0 to 9, rounding mode). Arithmetic checks the currency and overflow. Rounding returns the difference so callers can post an explicit rounding line (spec 3.4).
- `Quantity`: a `Decimal` plus a UoM key. Conversion takes a factor from the UoM master and rounds once.
- Dates: `date` is `YYYY-MM-DD`; `datetime` must carry an offset and is held in UTC.
- `Allocate`: floor shares on the absolute value, leftover units to the largest remainders, ties to the lowest index. Shares sum to the total exactly and the result is deterministic. Decimals allocate only at a scale that represents the total exactly.
- Tests use `testing/quick` (standard library) for the invariants: exact sum, no drift across repeated split and re-add, lossless scale round trip, and a test that fails if the package source mentions the binary-fraction type.
## Not decided here
Wiring into Record fields, columns and entry validation (P4), the Currency master (P11), the UoM Conversion master (P20), `period` and `duration` types, and the stored column types for money fields.
## Consequences
- Later ledger tables can store money as `BIGINT` minor units plus a currency code.
- Existing `Number` and `Currency` fields are unchanged until P4.
## Revisit when
P4 maps field types to columns, or a rate or quantity needs more than 65 digits.
