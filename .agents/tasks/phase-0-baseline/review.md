# P0 review: APPROVED

Evidence read: roadmap P0, notes.md (recorded results; suites not re-run), test sources, Makefile and package.json diffs, docs diffs.

- Invariant tests are meaningful: stale revision on update and delete (a refused update leaves data unchanged), history in the same transaction (history table dropped, create, update and delete all roll back), keyset paging with cross-Record token refusal, exact decimals, SaveRecord/PlanRecordSave/DeleteRecord guards, and a 30-case shared table read by both Go (`bindValues`) and Vitest (`validateField`).
- `internal/testdb` skips when `ATHENA_TEST_DATABASE_URL` is unset and drops its `test_athena_*` database on cleanup. Recorded: Go build, vet and tests, typecheck, build, Vitest 31/31 and `make test` all pass.
- No CLI, proto, telemetry or UI change, so command parity and the OTel and span constraints are unaffected. `init/new/run/serve` are untouched by P0.
- Docs: AGENTS.md, README, overview, decision-log (D-012 renumber, D-013 Accepted) and ADR-0002/0003 stale statements are fixed. PostgreSQL stays "later / untested". No new ADR is needed.
- Secret scan: only `change-me` placeholders; `.env.local` is git-ignored. No backup or copy files.
- Deviation: the baseline commit and its exclusion scan were skipped because this run was told not to commit. The notes record it. Not a blocker.
- Non-blocking note: the server accepts `name@example` and the client rejects it. It is recorded as a known gap and is outside P0's no-behavior-change scope.
