# Review: CLI parseTime fix (r-parse)

Verdict: PASS

Tree: `../athena-erp-parse-r`, detached at `3a26f4d` (`fix(cli): open the ledger service with the UTC connector`). Tree verified clean; diff is the fix ONLY: `cmd/athena/ledger.go` (+4/-1: import + `server.EndpointConnector` call + comment), `cmd/athena/ledger_connector_test.go` (+62: regression test), `internal/server/server.go` (+6/-3: `endpointConnector` → exported `EndpointConnector` + comment), `internal/server/sysmigrations.go` (1-line rename of the call site). Matches the Stylus ruling (night-log 19:30: export EndpointConnector for ledgerService, own commit, fail-before test, own review, ahead of F7s3).

## Brief items

1. Root cause fixed — PASS. `ledgerService` now opens through `server.EndpointConnector`, which for MariaDB goes through `utcConfig` (`server.go:269`: `ParseTime=true`, `Loc=UTC`, `time_zone='+00:00'`) — the same path the server uses, not a hand-rolled DSN. The Postgres branch still delegates to `endpoint.Connector()` with its own UTC handling, unchanged in behavior.
2. Regression test real — PASS. `TestLedgerServiceReadsDatesWithoutParseTime` opens the ledger service on a DSN without parseTime and reads a period and a determination. Fail-before verified: test file copied onto a clean parent (`9b1c070`) worktree → FAIL with exactly the reported error (`list periods: sql: Scan error on column index 5, name "start_date": unsupported Scan, storing driver.Value type []uint8 into type *time.Time`). Pass-after verified on `3a26f4d` → PASS (10.87s). Temp parent worktree removed afterwards.
3. Boundaries — PASS. `doctor.go` untouched (verified ping-only: `endpoint.Connector()` + `PingContext`, no scans). `internal/workspace` untouched globally (empty diff; `create.go` still on plain `endpoint.Connector()`). The only other change is the mechanical `endpointConnector` → `EndpointConnector` rename at its two call sites (`Serve`, `openMigrationDB`) — no behavior or flag changes.
4. Gates — PASS. `affected` green via suite dedupe on `3a26f4d` (clean tree): `ok athena-erp/cmd/athena 273.571s`, `ok athena-erp/internal/server 438.693s` (log `1124-203512-w-f7.log`, zero FAIL/panic lines, read not rerun). `gofmt -l cmd internal` empty, `go build ./...` OK, `go vet` on both changed packages clean (all rerun in the review tree). No proto/web changes, so no generate/web gates apply.
5. Scope — PASS. Fix + test only (4 files above). No F7s3 content (no completions, help-text, E2E-script or README-CLI changes).

## Notes (non-blocking)

- `affected --print` notes the tree is behind main (fix sits on `9b1c070`, main is at `74d242d`); expected for a reviewer checking the reported commit, and the deduped gate ran on exactly `3a26f4d`.
