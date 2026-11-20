# P2 review: versioned system migrations

Verdict: APPROVED. No blockers.

Evidence (diff and notes read; suites not re-run, coder's recorded results used):
- Runner `internal/store/migrate.go`: contiguous-version validation, schema lock on one connection, record row after each step, `--to`, dry run, refusal of newer databases, read-only status. Old `migrate()`, `addEntryRevisions(db)` and the `regexp` import are deleted from `server.go`; `utcConfig` is shared. No stale references found.
- Invariant tests are meaningful: idempotence (second `Up` applies 0), resume after a step whose DDL ran but whose record row did not, `--to`, newer-database refusal, status/dry-run write nothing, and fresh vs pre-P2 schemas compared column by column (including the legacy Record table gaining `revision`).
- CLI: `athena migrate status|up` with help, `--project`, `--json`, `--to`, `--dry-run`, tests in `cmd/athena/migrate_test.go`. `init/new/run/serve` unchanged except the extracted `resolveProject`. Not offering `--addr` is correct because no server starts, and the README says so.
- Telemetry: span `athena.migrate.up` carries only applied count and dry-run flag; no SQL values, DSN or entry data. Stays optional (no-op tracer when OTel is off). The CLI prints no DSN.
- API and UI: no proto, UI or dependency change, so ConnectRPC-only, shadcn-vue, Sonner, Build/View and `fieldRules.ts` parity are untouched.
- Docs: ADR-0006 Accepted (roadmap section 8, answer 2: accepted once P2 lands and is reviewed; this review satisfies that), decision log, O-003 narrowed (record-definition migrations, renames, rollback still open), ADR-0003/0005 notes, overview, README, docs/README, AGENTS.md all consistent. No PostgreSQL support claim; the ADR says statements are MariaDB syntax.
- No secrets, copies or leftover code.

Non-blocking: roadmap lists "definition migrations via `renamed_from` / `def_version`" under P2 work; the ADR explicitly defers them to P2a, and AGENTS.md/docs state this, so docs do not overclaim.
