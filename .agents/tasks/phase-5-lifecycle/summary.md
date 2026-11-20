# Phase P5 summary: document lifecycle

Review verdict: APPROVED, no findings (review.json, review.md).

## What changed
- New `internal/lifecycle` package: states (draft, submitted, cancelled), state machine, event order, Go hook registry.
- Record Type flags `submittable`, `amendable`, and field `allow_on_submit`. System columns `docstate`, `name`, `amended_from`.
- `RecordService` RPCs: `SubmitRecord`, `CancelRecord`, `AmendRecord`, `ListRecordVersions`, `ListRecordActivity`. ConnectRPC only.
- Each transition runs in one transaction (row lock, revision check, state, history, version, activity, in-transaction hooks). Submitted documents change only in `allow_on_submit` fields. Cancelled documents are fixed. Only drafts can be deleted. Cancel is blocked by a submitted downstream document. Amend creates a draft named `<root>-N`.
- Migration 7 `document_lifecycle`: `athena_document_versions`, `athena_activity`.
- Telemetry spans `athena.submit|cancel|amend` carry no values and stay off without `OTEL_EXPORTER_OTLP_ENDPOINT`.
- CLI `athena records submit|cancel|amend|versions|activity`.
- Frontend: Submittable and Amendable settings, status badge and column, Submit, Cancel and Amend with confirmations, Activity and Versions lists, locked inputs by state. No new dependency.
- Docs: ADR-0011 (Proposed), D-015, README, overview.

## Verification (recorded in notes.md; the reviewer did not rerun)
- `go build ./...`, `go vet ./...`, `gofmt -l`: clean.
- `go test -count=1 ./...` against MariaDB 11.8: all pass.
- `make generate` is idempotent.
- Web `typecheck`, `build`, `test`: pass (67 Vitest cases).
- `make restart` healthy. Browser pass in light and dark themes.

## Open points for the owner
1. Amend fails when a required field is also unique, read-only or no-copy.
2. `name` equals the Record ID until P6.
3. Cancel writes no ledger reversal (no ledgers yet).
4. Version snapshots need Record-level permissions once P8 lands.
5. Lines of a submitted document are edited by child Record ID.

## Next: P6, numbering and audit hash chain
Naming series that replace the placeholder `name`, plus a hash-chained audit trail over the activity and version records.
