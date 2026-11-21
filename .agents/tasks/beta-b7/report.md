# B7 server report: READY PASS (loop 1 + rebase onto main with B4/B12)

**Status: PASS.** Review loop 1 fixed all 3 blockers + 2 follow-ups, the branch
is rebased onto `origin/main` (now carrying B4/B12 migration 15 and the
test-speed fix), B7 is migration 16, and the literal `make test` is green.

## Review loop 1 fixes

1. **StepUp brute force (blocker).** `checkActorPassword` is replaced by
   `verifyActorPassword(ctx, actor, password, op, requestID)`: a wrong
   password counts toward the same failed-attempt counter and 5-strikes
   lockout as `Login` (root never counted/locked), every wrong attempt writes
   `step_up_failed` (user, request, `<op> wrong password`, never the
   password), the locking attempt additionally writes `lockout`, and a right
   password while locked returns `LockedError` (only the password owner
   learns the lockout, as in `Login`). A right password clears the counter.
   New sentinel `ErrWrongPassword` ("the password is wrong") maps to
   `Unauthenticated`; it names only the password. `StepUp` success writes
   `step_up`. `StepUp`/`TransferOwnership` service methods take `requestID`;
   the StepUp handler gained a kernel span.
2. **InviteUser step-up (blocker).** `InviteUser` is in `destructiveProcedures`
   with a conditional gate: step-up required only when `roles` is non-empty
   (SaveRecordType pattern, fail-closed). Coverage test extended with the
   `Invite` verb; new `TestInviteUserStepUpWhenRolesGranted` covers
   bare-ok / roles-required / satisfied.
3. **Migration number.** Final at **16** per the master's flip (B4/B12 took
   15): `sysmigrations.go` Version 16, ADR-0026, decision log and overview
   all say 16.
4. **Actor-password lockout (follow-up).** `TransferOwnership` and `StartReset`
   use the same `verifyActorPassword` (events `step_up_failed`/`lockout`,
   counter shared with sign-in and step-up). Transfer revokes sessions only
   when the grant/relinquish changed rows.
5. **Bruno 09/10 on a disposable workspace (follow-up).** Ran against
   `athena serve` on throwaway DB `test_athena_bruno_loop1` (5-min idle
   policy, port 3101, isolated HOME for the install lock; server stopped and
   database dropped afterwards). No passwords in files.
   - Idle run (`checkIdleExpiry=true`): **6/6 PASS** in 301 s —
     `09-idle-expiry` returned 401 with the `idle` detail after the wait.
   - Rate run (`checkRateLimit=true`): **17/17 PASS** — request 10 returned
     400 ×5 then **429 with Retry-After** on the 6th attempt (default 5/15
     min username bucket). An early attempt of this run failed only on the
     pre-existing `05-list-record-types` expectation (`recordTypes` array),
     because the fresh workspace had no Record Types; after seeding one via
     the CLI the full collection passes. No B7 code changed for this.

New tests: `TestStepUpFailuresLockTheAccount` (5 wrong → locked at login and
step-up, message names only the password, 6 `step_up_failed` + 1 `lockout`
rows, unlock, success writes `step_up`), `TestPrivilegedActorPasswordFailures-
CountTowardLockout` (3 reset + 2 transfer failures share one counter → locked).
Updated `TestStartResetNeedsTheActorPassword` to `ErrWrongPassword`.

## Rebase onto origin/main (dcd26c0)

Stashed, fetched, fast-forwarded `feat/b7-sessions` to `origin/main`
(B4/B12 server + test-speed fix), popped the stash, resolved 4 conflicts by
keeping both sides: `sysmigrations.go` (15 notifications, then 16
session_policy), `service.go` (B12 `notifyAccess` + B7 `revokeSessions` in
transfer/role paths), `docs/README.md` + `decision-log.md` (ADR-0024 and
ADR-0026 rows). `gate.go`, proto and notifications code merged cleanly.
Adapted the 7 B7 `revokeSessions` call sites to B12's new
`administrative` flag (`true`: an access change ending someone else's session
notifies them, per the B12 brief). Fixed 3 B12 tests for the intended merge
interaction (revoke-and-notify): `TestInboxAuthEvents` (extra `session_ended`
row + re-login before the revoke step), `TestPeriodCloseNotifiesCurrentLedger-
Writers` (extra `session_ended` rows), `TestWorkspaceAppearancePublicReadAnd-
ProtectedWrite` (re-login after role assign).

## Files changed vs origin/main (27 files, uncommitted)

Session/policy core: `internal/server/sysmigrations.go` (16),
`internal/auth/security.go` (new), `internal/auth/service.go`,
`internal/auth/assignment.go`, `internal/auth/principal.go`,
`internal/auth/accesslog.go`, `internal/server/ratelimit.go` (new),
`internal/server/gate.go`, `internal/server/auth_rpc.go`,
`internal/server/server.go`, `go.mod`.
Proto/gen (`make generate` stable, only auth files):
`proto/athena/v1/auth.proto`, `gen/athena/v1/auth.pb.go`,
`gen/athena/v1/athenav1connect/auth.connect.go`,
`web/src/gen/athena/v1/auth_pb.ts` (only `web/src` touch).
CLI: `cmd/athena/security.go` + `security_test.go` (new), `cmd/athena/main.go`.
Tests: `internal/server/session_policy_test.go` (new),
`access_test.go`, `access_rpc_test.go`, `assignments_test.go`,
`notifications_test.go`, `workspace_test.go`,
`internal/auth/ownership_test.go`, `internal/auth/root_test.go`.
Bruno: `03-login.bru`, `09-idle-expiry.bru` + `10-sign-in-limit.bru` (new).
Docs: `adr-0026-session-policy.md` (new), `docs/README.md`,
`decision-log.md`, `architecture/overview.md`, `docs/testing/bruno.md`,
`README.md`.

## Command output (exact, post-rebase)

- `gofmt -l cmd internal` → empty. `go build ./...` → ok. `go vet ./...` → ok.
- `make generate` ×3 total, last two byte-identical (`GENERATE-STABLE`).
- `CI=true pnpm --dir web run typecheck` → ok.
- `make test` → **exit 0**: cmd/athena, appearance, auth, expr, files, ledger,
  lifecycle, numbering, rules, server (104 s), store, types, workspace all
  `ok`; web **248/248** passed; zero `FAIL` lines.

No commits, merges, pushes, agents, or dev-database writes. The Desk part of
B7 (tabs, idle warning, step-up dialog) remains a later task, untouched.
