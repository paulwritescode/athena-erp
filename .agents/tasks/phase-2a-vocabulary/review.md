# P2a review (pass 1)

Verdict: APPROVED. No blockers.

Evidence read: plan, roadmap P2a, notes.md (recorded verification: gofmt, build, vet, full Go suite with MariaDB, 32 Vitest tests, typecheck, build, `make generate` x3 stable), file status, term greps.

- Old terms: greps over code, proto, web, CLI, README, AGENTS.md, overview and docs/README find old names only in the allowed legacy files, Accepted ADR bodies, the D-005 row and history items in implementation.yaml. `terms_test.go` enforces this for source files.
- P0 tests: HEAD had 8 entries tests + 4 records tests; now 4 record-type + 8 record tests, assertions renamed, not dropped.
- Migration 5: guarded steps, tests for data survival, idempotency, crash-resume at every statement, merge refusal, legacy key colliding with a system table. Identifier test covers determinism, lowercase and 63 bytes.
- ADR-0008 and ADR-0009 are Proposed and match the decision log, overview, README, AGENTS.md. ADR-0007 and `internal/types` have no old terms. PostgreSQL stays "untested/later". No secrets seen; scratch scripts are under .agents.
- Proto and generated code: old entries files deleted, new generated files present, regeneration stable.

Non-blocking observations (documented trade-offs, not findings):
- CSS classes (`entries-*`, `entry-form`), element ids and the `athena-entry-columns:` localStorage key keep old spelling by design (ADR-0008, preserves saved column widths). AGENTS.md's list of places allowed to hold old names does not mention them.
- Browser pass (light/dark, Build/View) was not done by the coder; the bundle builds and is served. The migration ran on the dev database without a backup, with the user's go-ahead.
