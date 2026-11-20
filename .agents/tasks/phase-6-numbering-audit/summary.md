# Phase P6 summary: numbering and audit hash chain

Review verdict: APPROVED (pass 2). Nothing committed or pushed yet.

## What changed
- New `internal/numbering`: series patterns (`{yyyy}`, `{yy}`, `{mm}`, one `{#####}`), counters in `athena_number_counters`. Gap-free numbers lock the counter row inside the caller's transaction. Non-gap-free numbers use their own transaction.
- New `internal/audit`: append-only `athena_audit_log` with `prev_hash`/`hash`, a locked chain head, `Append`, `Verify` (edits, deleted or inserted rows, reordering, deleted tail, optional `seq:hash` anchor) and `Rows`.
- Migration 8 moves `athena_record_history` into the chain and drops the old table. The old writer is deleted.
- Request correlation: one request ID per call, echoed in the response header and reused by telemetry spans.
- Record Type naming settings: `naming_series`, `gap_free`, `allow_manual_name`, `label_field`. The number is assigned at submit and is never reassigned. Manual names are unique and audited.
- Audit rows for create, update, delete, submit, cancel and amend. `ListRecordHistory` returns old/new values. `SystemService.VerifyAuditLog` is new.
- `RecordService.LookupRecords` (label search, keyset paging). Link values pointing to a missing, deleted or cancelled Record are refused.
- CLI: `athena records lookup|history`, `records create --name`, `athena audit verify [--anchor]`.
- Frontend: `RecordPicker.vue` (shadcn-vue Popover and Command) for every Link field and "Belongs to". Table Link cells show labels. The Record page has a manual Name input and an Audit Log list. The Build page has the naming controls.
- Docs: ADR-0012 (Proposed), D-016, README, overview.

## Verification (MariaDB 11.8)
- `go build`, `go vet` and `gofmt -l` are clean.
- `go test -count=1 ./...` passes, including 18 parallel submits that give exactly 1..12 with no gap, tamper detection and the concurrent-verify test.
- `make generate` twice gives byte-identical output.
- `pnpm typecheck`, `test` (70 Vitest cases) and `build` pass.
- Headless Chrome pass in light and dark mode: 17 checks each, all pass.
- Pass 2 re-ran Go build, vet, gofmt and tests after the fixes. The reviewer did not re-run the suites.

## Review fixes
1. `Verify` now reads the head and the rows in one REPEATABLE READ transaction, so a concurrent commit cannot look like tampering.
2. The ADR-0012 row in the decision log was repaired.

## Open points
One chain serializes audited writes. Anchoring is manual. Audit rows are unmasked until P8. Manual-name privileges wait for P8. The parent picker lists submitted documents. A manual name that clashes with a future series number fails that submit. Company and branch scopes wait for P11.

## Next: P7, CEL expressions with cel-go
Add cel-go for typed, bounded expressions (conditions, computed values, validations). Go permission checks and transaction boundaries still apply to them. Needs an ADR and decision-log entry before it is accepted.
