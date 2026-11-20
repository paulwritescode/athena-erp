# P1 review: storage dialect layer

Verdict: APPROVED. No blockers.

Evidence (diff read, suites not re-run; coder recorded build, vet, gofmt, `go test -count=1 ./...` with MariaDB 11.8, and web typecheck/build/test (32 tests) all passing, with the 13 P0 characterization tests unedited).

- Behavior preserved: `internal/store/mariadb.go` emits the same DDL, GET_LOCK, information_schema queries and error numbers (1062/1213/1205/1451/1452/1048/1364/1406/1264/1366) the old inline code used. `writeError` maps the same codes to the same client errors. `recordTableName` now shortens, but record keys are capped at 50 chars (`recordKeyPattern`), so `athena_record_`+key is at most 64 bytes and never changes.
- Invariant tests: `store_test.go` checks shortening is deterministic, exactly 63 bytes, collision-free for near names, and unchanged when short. It also checks the MariaDB limit boundary, the classifier table including a wrapped error and ErrOther, exact statement strings, and column type/predicate coverage. `mariadb_db_test.go` runs the DDL, catalogue lookups and lock against MariaDB.
- Scope: no PostgreSQL code. ADR-0005 is Proposed, and the decision-log row, overview rows, README layout line and docs/README.md entry are consistent. The overview says PostgreSQL is unimplemented and untested. AGENTS.md needs no change because no decision was accepted. Metadata DDL in `server.go` `migrate` and `internal/project` SQL stay inline, and the overview records this.
- CLI: the roadmap P1 section defines no command. `init`, `new`, `run` and `serve` are untouched. The `athena <command>` parity item does not apply to this phase.
- Telemetry, API and UI: no span, proto, ConnectRPC or frontend change, so OpenTelemetry stays optional with no entry values in spans, and the Sonner, token, Build/View and `fieldRules.ts` constraints are unaffected. No new endpoints.
- Hygiene: no secrets or DSNs in the diff, no copies or backups, and the replaced helpers (`mariadbColumnType`, `incompatiblePredicate`, `tableExists`, mysql imports) are deleted.

Non-blocking: a few hard-coded backtick `id` and `revision` literals remain in `entries.go` WHERE clauses. This is not a correctness issue today and P10 can tidy it.
