# P9 notes: Starlark Rules host (iteration 1)
Date: 2026-11-20. Branch `main`, base 7985ef8. Nothing committed (the approval step commits). Existing uncommitted work (`.agents/`, `docs/Athena-ERP-Playbook.md`, `docs/Records-and-movements.md`, `docs/Research-and-audit.md`) was left alone.

## Deviations from the brief
- The brief says `--project`; AGENTS.md and ADR-0008 say `--workspace` (`--project` is a forbidden old term, enforced by `terms_test.go`). The CLI uses `--workspace`, `--json`, and no `--addr` (kernel commands start no server, like `athena files`).
- Added mid-phase by the parent: sign-in error handling. The brief calls it "with the Users & Roles page", but no such page exists (it is listed as not built in the overview), so only the sign-in and first-run work was done. A Users & Roles page is not part of this change.
- The desk without a database used to open signed in. It now shows the "Database not connected" card in every non-Connected state, including `Not configured`, because the sign-in work asked for that state. Say so if the offline desk should come back.

## What was built
### Starlark host
- Dependency: `go.starlark.net v0.0.0-20260908191801-89a6a09411d5` (no tagged release; pseudo-version pin, direct in `go.mod`). Why: the reference Go implementation, deterministic, no I/O or clock in the language, step counter and cancellation, pure Go (ADR-0016). Its module was already in the local module cache.
- `proto/athena/v1/rules.proto`: `RuleService` (ListRules, GetRule, SaveRule, DeleteRule, ListRuleVersions, SimulateRule, ListRuleRuns) and `AuthFailure` in `auth.proto`.
- `internal/rules` (new, no storage): `Compile` (parse, resolve, reject floats, `/`, `float`, `load`, wrong entry shape, unknown `doc.<attr>`, literal `set("x")` of a non-writable field, with line numbers), `Run` with limits (200,000 steps, 500 ms, 200 rows read, 50 sets, 50 messages, 64 KiB source), `Host` interface, `decimal`/`money` values over `internal/types`, `allocate`, `round`, `div`, `log`, `fail`/`warn`/`require`, input hash, `Stack` for cascade depth and cycles.
- `internal/lifecycle`: `Event.Doc` and `Document` (values, writes, `Set`), so any hook gets the document in the stored argument form.
- `internal/server/rules_engine.go`: lifecycle hook for every event; loads enabled Rules for the Record Type and event, orders derivation, validation, guard, priority, key; host reads as the acting user (Grant, row scope, field levels), writes through `fieldArgument`; Rule Run row on its own connection (survives rollback); span per run with ids, counts, duration, outcome only.
- `internal/server/rules.go`: handlers, definition checks, versions, audit rows (`athena:rule`, hash only), simulate (rolled back, diff), run log with keyset paging.
- `records.go`/`lifecycle.go`: build the pending document, fire, merge Rule writes with the state checks of a user's save (submitted, set-once). `loadRecordTypeFrom` extracted so Rules read through the transaction.
- Migration 11 `starlark_rules`: renames the unused first `athena_rules` to `athena_rules_legacy` (its row survived a test fixture, so it is kept, not dropped), creates `athena_rules`, `athena_rule_versions`, `athena_rule_runs`. `reads` is a reserved word in MariaDB, so the columns are `rows_read` and `rows_written`.
- Permissions `rules:read` and `rules:write` (`auth.SystemPermissions`), policies in `gate.go`, CLI role help updated.
- CLI `athena rules list|show|save|rm|versions|simulate|runs` (`cmd/athena/rules.go`).
- Desk: `RulesPage.vue` (list, editor with shadcn Select/Switch/Textarea/Tabs, Runs and Versions tabs, Simulate dialog with diff, Sonner toasts), `ruleDefinitions.ts` (mirrors the definition rules), `ruleClient` in `api.ts`, Rules nav follows `rules:read`.
- ADR-0016 (Proposed), decision log, overview, README, docs/README, AGENTS.md updated.

### Sign-in error handling
- Server: `Login` checks the real hash for a locked or disabled account, so only the right password learns the state (FailedPrecondition plus `AuthFailure{reason, unlocks_at}`); wrong password and unknown user stay one identical Unauthenticated answer. Database failures map to Unavailable (`isConnectionError`, also in the gate). CSRF mismatch is PermissionDenied with `AuthFailure{reason: csrf}`.
- Web: `apiErrors.ts` (`classify`, one place for code to state mapping, request ID from `x-request-id`), `signInRules.ts` (empty fields, 12-character minimum), rewritten `SignInPage.vue` (no users with copy and Check again, database and server cards, locked and disabled cards, session-ended note, field errors, disabled and "Signing in…" while busy), `App.vue` `startDesk`/`recheck` (`/healthz` first, so a down database is told from a down server).

## Invariant tests
- `internal/rules/rules_test.go`: rejection and warning; replay gives the same input hash, steps and writes; step and time limits stop a runaway rule; floats refused at compile and at the host; sandbox (no `set` in validation, no `load`, no `while`, no `open`, no `set()` type, recursion stops); references checked with line numbers; money and decimal math; hidden fields; read limit; `print(doc)` and `log` never print values; transform; host error stops the run; cascade stack.
- `internal/server/rules_test.go`: validation rejects a save and the Rule Run survives the rollback; derivation sets fields on create and update before validation; a rule cannot set `docstate` or an unknown field (compile-time and run-time); a failing rule rolls back the document and a derivation's writes; runaway rule logged `step_limit`; guard blocks submit only; disabled rule skipped; versions, conflicts, audit rows without source, chain verifies; simulate is a dry run with a diff and a replayable hash; reads are made as the acting user (hidden field, no Grant on `customer`); `rules:read` and `rules:write` and denials in the Access Log; cascade guard; spans and log carry no values. `TestEveryProcedureHasAPolicy` covers the new procedures.
- `internal/server/signin_errors_test.go`: distinct codes, no user enumeration, locked and disabled only for the right password, DB unreachable is Unavailable, CSRF detail, request ID header.
- `cmd/athena/rules_test.go`: every subcommand against a throwaway Workspace; a float rule is refused with a line number.
- `web/src/apiErrors.test.ts`, `ruleDefinitions.test.ts`: Vitest for the code-to-state mapping, sign-in rules, definition rules.

## Verification (repository root)
| Command | Result |
| --- | --- |
| `gofmt -l cmd internal gen` | no output |
| `go build ./...`, `go vet ./...` | ok |
| `go test -count=1 ./...` (`ATHENA_TEST_DATABASE_URL` from `.env.local`, never printed) | first full run: every package ok except `cmd/athena` (see below); after the fix `go test ./cmd/athena` and the Rule, numbering, lifecycle and sign-in tests in `internal/server` pass; other packages unchanged and ok (auth, expr, files, lifecycle, numbering, rules, store, types, workspace) |
| `make generate` twice, comparing `git status` hashes | identical |
| `CI=true pnpm --dir web run typecheck`, `build`, `test` | ok; 128 tests |
| Browser, headless Chrome over CDP against a throwaway Workspace (`test_athena_p9`, `athena run` on 127.0.0.1:3010), light and dark | sign-in states: 30 of 30 (no users, copy, Check again without reload, field validation, wrong password and unknown user identical, locked with unlock time and command, locked plus wrong password stays generic, disabled, session ended note, database stopped with `brew services stop mariadb@11.8` then recovered by Check again, server killed then sign-in shows the unreachable card). Rules page: all checks pass in both themes (list, open, simulate rejected, save toast v2, runs tab dry run, versions tab, rejected save in View shows the rule message in a toast). Screenshots in `/tmp/p9/*.png` |
One regression found by the full suite and fixed: the engine read `athena_rules` inside the document transaction, which started its snapshot early and made the numbering counter insert fail (MariaDB error 1020, `TestNumberingLookupAndAuditCommands`). The engine now reads Rules on its own connection. The Rules test also compared protojson text with fixed spacing, which protojson randomises; it now uses a regexp.
Two defects found by the browser pass and fixed: a stopped database showed "Server unreachable" (now `/healthz` decides first), and a fetch failure arrives as `Unknown` with a `TypeError` cause, not `Unavailable`.

## Caveats
- No memory limit: starlark-go has no allocation accounting. Recorded in ADR-0016 as the blocker for acceptance. Not production-ready.
- Rules attach to document Record Types only (lifecycle events fire only for documents). Cascades cannot occur yet, so the depth and cycle guard is tested directly.
- Formula fields are calculated after the events, so a Rule at `validate` sees their old value.
- The Rule that runs is the one saved through `RuleService` or `athena rules save`; `rules/*.star` files are not read at run time (needs the ADR-0015 publisher).
- Rule Run rows are append-only through the API but not hash-chained.
- `qty()`, `fx()`, `create_draft()`, `link()`, posting, reaction and scheduled Rules, Rule tests as Records are not built.
- Sign-in lock state is shown after the 5th wrong attempt only to the right password; the attempt that causes the lock still answers "wrong username or password".
- The dev server on 127.0.0.1:3000 was stopped for the browser pass (one Athena per user) and restarted with `make restart` at the end.

`make restart` ran at the end: `/healthz` returns `{"status":"ok","database":"Connected"}` and `internal/webui/dist` was rebuilt at 02:24. The throwaway database `test_athena_p9` was dropped and its server stopped.
