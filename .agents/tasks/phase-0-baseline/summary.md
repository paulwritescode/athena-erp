# Phase P0 summary: baseline tests and doc reconciliation

Review verdict: **APPROVED** (no findings). Nothing was committed or pushed.

## What changed

- **Test harness:** `internal/testdb` creates and drops a `test_athena_*` database from `ATHENA_TEST_DATABASE_URL` and skips when it is unset.
- **Go invariant tests** (`internal/server/*_test.go`):
  - stale revisions are rejected on update and delete;
  - history is written in the same transaction as the change;
  - keyset paging returns each entry once and refuses a page token from another Record;
  - decimals round-trip exactly;
  - `SaveRecord`, `PlanRecordSave` and `DeleteRecord` guards, including `confirm_data_loss`.
- **Shared field-rule table:** `internal/server/testdata/field_rules.json` (30 cases) is read by the Go test and by `web/src/fieldRules.test.ts`.
- **Vitest 5.1.1** added to `web/`. `make test` runs Go tests and then Vitest.
- **Client bug fixed:** `fieldRules.ts` accepted `2026-02-30`. It now checks the calendar date, as the server does.
- **Docs aligned:** AGENTS.md, README, overview, `implementation.yaml`, decision log (D-012 renumber, D-013 Accepted) and ADR-0002/0003 stale statements.

## Verification (recorded in notes.md; the reviewer did not re-run them)

| Check | Result |
| --- | --- |
| `go build ./...`, `go vet ./...` | pass |
| `go test -count=1 ./...` with the MariaDB test URL | pass |
| `go test ./...` without the URL | pass, MariaDB tests skip |
| `make generate` run twice | checksums identical |
| web typecheck and build | pass |
| Vitest | 31 of 31 pass |
| `make test` | pass |

No `test_athena_*` databases were left behind.

## Deviations and open notes

- The baseline commit and its secret and exclusion scan were skipped because this run was told not to commit. Run both before or with the next approval.
- The server accepts the email `name@example` and the client rejects it. This is a known gap, left as is because P0 changes no behavior. Decide whether the server should be stricter.
- I started the local MariaDB with `brew services start mariadb@11.8`.
- The 66 earlier uncommitted changes in the working tree are untouched.

## Next: P1, storage dialect layer (MariaDB only)

- **Goal:** move the inlined MariaDB SQL out of `internal/server/{records,entries,migrations,server}.go` into a new `internal/store` package: a `Dialect` interface plus `mariadb.go`. There is no behavior change, no PostgreSQL code and no PostgreSQL support claim (that is P10).
- **Extracted:**
  - identifier quoting, and length limit with deterministic hash shortening to 63 characters;
  - type mapping and DDL for create, add, alter and drop;
  - upsert, pagination and row lock clause;
  - schema or advisory lock, JSON column type, `now()` handling;
  - error classification (duplicate key, deadlock, serialization failure).
- **Docs:** `docs/decisions/adr-0005-database-dialect-layer.md` as **PROPOSED**, plus a decision-log row. ADR-0002 is only annotated after you accept the ADR.
- **Tests:** all P0 tests stay unchanged and green. New tests cover deterministic identifier shortening within 63 characters and the MariaDB error classifier.
- **Gate:** P2 (versioned migrations) depends on P1.
