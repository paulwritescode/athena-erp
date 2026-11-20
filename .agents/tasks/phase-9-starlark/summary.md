# P9 Starlark host: summary

Review verdict: APPROVED, no blockers (review.md, review.json).

## What changed
- Starlark Rules host (go.starlark.net, pinned pseudo-version): `internal/rules` compiles and runs Rules with limits (200,000 steps, 500 ms, 200 rows read, 50 sets, 50 messages, 64 KiB source). Floats, `load`, `while` and `/` are refused at compile. Rules reach data only through a Go host that checks Grants, row scope and hidden fields as the acting user.
- `RuleService` (ListRules, GetRule, SaveRule, DeleteRule, ListRuleVersions, SimulateRule, ListRuleRuns) in `proto/athena/v1/rules.proto`, with versions, audit rows (hash only), dry-run simulate with diff, and a run log that survives rollback.
- Lifecycle hook runs validation, derivation, guard and transform Rules on document events. A failing Rule rolls back the document.
- Migration 11 `starlark_rules`: old unused `athena_rules` renamed to `athena_rules_legacy`; new `athena_rules`, `athena_rule_versions`, `athena_rule_runs`.
- Permissions `rules:read` and `rules:write`; CLI `athena rules list|show|save|rm|versions|simulate|runs` (uses `--workspace`).
- Desk: Rules page (shadcn Select/Switch/Textarea/Tabs, Sonner toasts, simulate dialog), nav follows `rules:read`.
- Sign-in error handling: distinct states for no users, wrong password, locked, disabled, database down, server down, CSRF; no user enumeration.
- ADR-0016 (Proposed), decision log, overview, README, docs/README and AGENTS.md updated.

## Verification
- `gofmt -l`: clean. `go build ./...`, `go vet ./...`: ok.
- `go test -count=1 ./...` against MariaDB: all packages ok after fixing one regression (Rule reads inside the document transaction broke the numbering counter; now read on their own connection).
- `make generate` twice: identical output.
- Web `typecheck`, `build`, `test`: ok, 128 tests.
- Headless Chrome pass, light and dark: sign-in 30/30, Rules page all checks passed.

## Caveats
- No memory limit (starlark-go lacks allocation accounting). ADR-0016 stays Proposed; Rules are not production-ready.
- Rules attach to document Record Types only. Formula fields are stale at `validate`. `rules/*.star` files are not read at run time. Rule Run rows are not hash-chained.
- Offline desk (no database) now shows "Database not connected" instead of opening signed in.
- PostgreSQL is not implemented.

## Next
1. P10: PostgreSQL adapter (MariaDB and PostgreSQL interchangeable).
2. The file-management phase.
3. The remaining roadmap phases.

## Approved changes (supersedes "Next")
Batch 2 order: Users & Roles Build page, P10 (adapter code only, PostgreSQL untested), Starlark memory guard (caps on string/list/dict sizes and total values built, in host builtins; unblocks ADR-0016 acceptance), P11-P22 in roadmap order, P23 stays gated. File management (P8a) is done and not rescheduled.
Committed locally as 86fbec0 (not pushed). Left unstaged, not from P9: .gitignore, Makefile, internal/telemetry/telemetry.go, web/package.json, web/pnpm-lock.yaml, web/src/typography.css, docs/revamp/, .agents/.
