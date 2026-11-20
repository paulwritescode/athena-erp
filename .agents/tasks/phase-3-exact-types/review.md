# P3 review: exact types
Verdict: APPROVED. No blockers.
- Scope matches roadmap P3: new `internal/types` (decimal, money, quantity, dates, allocation) on `math/big`, stdlib only, `go.mod` unchanged, no proto/UI/CLI/API change. Roadmap says "No UI" and wiring is P4, so no `athena <command>` is required; init/new/run/serve untouched.
- Code read: rounding (`divRound`) handles sign, ties and both modes; `AllocateUnits` is largest-remainder with lowest-index ties and handles negative totals; Money checks currency and overflow and returns rounding difference; dates require strict format and offset.
- Invariant tests (`types_test.go`): allocation exact sum and repeatability via `testing/quick`, split/re-add drift, scale round trip, and a test that fails if package source contains "float". Meaningful. Recorded evidence shows go build/vet/gofmt/test passing.
- Docs: ADR-0007 PROPOSED (matches roadmap), decision-log row, docs/README.md link, overview status row stating not yet wired. No PostgreSQL claim, AGENTS.md need not change.
- No I/O, so no spans; OTel stays optional. No secrets, copies or leftover code.
