# Review: C2 secrets slice 1 (migration 22 + ADR-0033)

Verdict: PASS

Tree: `../athena-erp-c2-s1-r`, detached at `e3ba394`, clean. Base `6c871c9`. Slice diff is 6 files only, all in scope: `internal/server/sysmigrations.go` (+46), `internal/server/sysmigrations_test.go`, `docs/decisions/adr-0033-secrets-and-retention.md` (new), `docs/decisions/decision-log.md` (+1 row), `docs/README.md` (+1 row), `README.md` (Z4-decided line on the C2-owned events/jobs bullet).

## 1. Migration 22

- Versions 1–22 contiguous in `SystemMigrations()`; 22 `secrets_retention` sits directly on merged 21 `stock_ledger`. `TestSystemMigrationsAreValid` passes.
- Creates `athena_secret_refs` (id, name, provider, provider_ref, created/updated_at; unique name), `athena_key_versions` (id, key_name, version, status, created_at; unique name+version), `athena_disposal_register` (id, record_type, record_id, action, outcome, detail, actor, created_at; record index), `athena_holds` (id, record_type, record_id, reason, placed_by, released_at, created_at; record index). Matches slice scope.
- Structure only: no seed rows (test asserts all four tables empty), no secret material in the migration. No data migration — nothing to migrate.
- "Down" is N/A by platform design: `store.Migration` (`internal/store/migrate.go`) has no Down path; system migrations are up-only and applied migrations are never edited.
- MariaDB (rerun by reviewer, `test_athena_*`): `TestSecretsRetentionMigrationCreatesTheSecretsTables` (21→22 upgrade, columns, unique keys, empty) PASS; `TestEventsJobsWorkflowsMigrationExtendsTheOutbox` (full fresh chain through 22, absence at 17, presence at 22) PASS; neighbour 20/21 tests PASS.

## 2. ADR-0033 transcription of Z4 (orders binding text)

Checked word-by-word against `.agents/stylus/night-shift-orders.md` Z4 row. All exact: 7 years ledger/invoices/stock/audit with NO beta auto-delete; guest personal data anonymised 90 days after the event with dietary-data consent model; Access Log 1 year; outbox/events 30 days; job runs 90 days; Workspace key from env or OS keychain, owner-held, never in database or backups; `athena secrets rotate`. Status Proposed, never accepted. Context/Options/Decision/Consequences/Revisit follow the template.

## 3. Rows, scope, secrets

- Decision-log ADR-0033 row and docs README row accurate; README line correctly flips "stay out until the sponsor rules on Z4" to decided-with-link. No other lane's files. Diff scanned for credential patterns: no secret values in code, docs or tests.

## 4. Gates

- Worker green `1f462ed` → `e3ba394`: verified with git that among slice files ONLY `docs/README.md` changed (one F6 ASCII-logo line); Go sources, ADR, log and README byte-identical, and 21+22 identical in both commits, so the worker's green covered this chain. No Go test reads `docs/README.md`.
- Reviewer reruns: `gofmt -l cmd internal` empty; `go build ./...` + `go vet ./...` clean; filtered migration tests on MariaDB all PASS (above); `affected --label "C2 secrets slice 1 review"` through `suite`: `ok cmd/athena`, `ok internal/server`.

## Nits (not blocking; fold into slice 2+)

- ADR-0033 omits Z4's "sessions follow the existing session policy" clause. No number is wrong and sessions are ADR-0026 territory, but a pointer sentence would close the transcription loop.
- Two parentheticals went stale when 21 merged before this commit: ADR impl note "is reserved and lands once migration 21 (C7) merges" and the log row "(reserved, lands after 21)". Substantively still true (22 lands after 21); refresh the tense when nearby.
