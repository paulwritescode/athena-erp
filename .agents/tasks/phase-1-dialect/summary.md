# Phase P1 summary: storage dialect layer (MariaDB only)

Review verdict: APPROVED, no findings. Nothing committed or pushed.

## What changed
- New `internal/store` package: `Dialect` interface and `ShortenIdentifier` (`store.go`), `MariaDB` implementation (`mariadb.go`), plus unit and MariaDB tests.
- `internal/server/{records,entries,migrations,server}.go` now call a package-level `dialect` for quoting, DDL, catalogue lookups, schema lock, row lock, type mapping, data-loss predicates and error classification. Removed `mariadbColumnType`, `incompatiblePredicate`, `tableExists` and the `mysql` imports in `records.go` and `entries.go`.
- Docs: ADR-0005 (Proposed), decision-log row, `docs/README.md`, `docs/architecture/overview.md` status rows, README layout line. `AGENTS.md` unchanged.
- Behavior is preserved: same DDL, locks, information_schema queries and error numbers. No PostgreSQL code, CLI command, proto, telemetry or UI change.

## Scope notes
- Left inline until P2: metadata-table DDL in `server.go` `migrate`, `internal/project` SQL, and `LIMIT ?` pagination.
- `Upsert` and `JSONType` are in the interface but covered by unit tests only.
- Non-blocking: a few hard-coded backtick `id`/`revision` literals remain in `entries.go` WHERE clauses; P10 can tidy them.

## Verification (MariaDB 11.8)
- `go build ./...`, `go vet ./...`, `gofmt -l internal cmd`: pass, no output from gofmt.
- `go test -count=1 ./...`: pass. All 13 P0 characterization tests ran unedited, plus `internal/store` tests including `TestMariaDBCatalogueAndLock`.
- Web `typecheck`, `build`, `test`: pass (32 Vitest tests). The build deleted `internal/webui/dist/.gitkeep`, which was restored with git checkout.
- The reviewer read the diff and relied on the coder's recorded results; suites were not re-run in review.

## Next: phase P2, versioned migrations
Replace the inline metadata DDL in `server.go` `migrate` with versioned migrations running through the dialect layer. Rollback and module upgrades stay out of scope unless the roadmap says otherwise. P2 needs your approval before it starts. The changes from P1 are uncommitted and waiting for your decision on committing.
