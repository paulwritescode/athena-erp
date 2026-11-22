# Review: B8 slice 1, root administrator server + CLI

Verdict: PASS

Branch `feat/b8-root-buildview` at 6c702a3 in `../athena-erp-b8`, diffed against
`main`. Reviewed per `review-brief-slice-1.md`, the slice 1 text of `brief.md`,
`design.md` section 5, `AGENTS.md` (root paragraph) and ADR-0018 (Accepted —
behavior confirmed, no redesign proposed). Read-only on product code. No
blockers. Follow-ups are numbered below; none stops the merge.

## What was verified

Seeding and reservation (brief 1). `EnsureRoot` runs after migrations at both
seams — kernel open (CLI, `internal/server/kernel.go:63`) and server boot
(`internal/server/server.go:121`) — with the database password, generate-and-
print fallback, idempotency, and skip-with-log when a non-root user holds the
name. New CLI test proves first-command seeding and the seeded shape
(`Administrator`, `setup`, active, enabled, zero assignments). The reserved
name is refused at every creation path through one check (`validateNewUser` →
`IsRoot`, case-insensitive): `CreateUser`, `BootstrapOwner` (existing auth
tests), and `InviteUser` over the API and CLI (new tests, plain message
`reserved for the workspace root`).

Gate bypass over HTTP (brief 2). New `TestRootPassesEveryGateWithoutAssignments`
signs in as root with zero assignments and exercises system gates
(`ListUsers`, `ListAccessLog`), build (`SaveRecordType`), and record
create/list/read, asserting the level-2 field stays visible, record-type level
is max, and zero denials are logged. The bypass itself is centralized in
`Principal` (`HasSystem`, `Can`, `Level`, `Scope`, `MustSeparateDuties` all
branch on `Root`), so these procedures prove the shared evaluation the gate
consults.

Refusals per path (brief 3). Two new guards, both placed after the user lookup
and before any write, returning `ErrInvalid` (→ `InvalidArgument`) in plain
language: `updateUserOnce` refuses root display-name edits and role changes
(`internal/auth/service.go:898-905`), `saveAssignmentOnce` refuses root
assignments (`internal/auth/assignment.go:311`), joining the existing
suspension refusal (`service.go:924`). API, CLI, service, and desk tests each
cover their path: 5 RPC calls, 7 CLI invocations (one-line errors asserted),
auth-unit refusals with nothing-changed + still-signs-in checks, and 4 desk
tests asserting the existing UI offers root no row menu, no suspend/assign/
resend/reset/reactivate, and no delete (deletion is impossible by construction
— no `DeleteUser` RPC or CLI exists — and revoke on root's empty assignments
returns not-found before any write). CLI goes through the same service methods
(`access.go` calls `UpdateUser`/`SaveAssignment`/`CreateUser`/`BootstrapOwner`),
and `syncOwnerFlag` explicitly excludes the root username. No-op `disabled=false`
and `Unlock` on root remain harmless no-ops.

Audit and password (brief 4). New test asserts login, invite, record
create/update, and the latest `athena_audit_log` row all attribute
`administrator`; new test rotates root's password and proves the old password
dies while another account is unaffected.

Scope (brief 5). The diff is 16 lines of guards plus 4 test files — no new
permissions, migrations, proto, or ADRs, no scope creep.

## Commands observed (exit 0 unless noted)

- `gofmt -l cmd internal` — empty. `go build ./...`, `go vet ./...` — clean.
- `make test` — full suite green (Go all packages on throwaway `test_athena_*`
  databases; web 47 files / 513 tests, incl. new `rootAccess.test.ts`). Root
  tests re-run explicitly against the test DB: 9 auth, 3 server-over-HTTP,
  1 CLI, 4 desk — all PASS, none skipped.
- `make generate` twice — both succeed, `git status` clean after each (stable).
- No browser pass per the brief (slice 2 owns the Desk browser pass).

## Follow-ups (non-blocking)

1. `EnsureRoot`'s taken-name skip branch (`internal/auth/service.go`, the
   `default:` log-and-skip for pre-reservation databases) has no test.
   Consider one that plants a conflicting regular row via direct SQL.
2. Serve-boot seeding (`internal/server/server.go:121`) duplicates the
   kernel-open call, but only the kernel seam is test-covered (new CLI test).
   Consider a fresh-boot seeding assertion, or collapsing to one seam.
3. Desk `isRootSelected` (`web/src/pages/AccessPage.vue:224`) exact-matches
   lowercase while server `IsRoot` normalizes; a legacy mixed-case regular
   `Administrator` would be offered actions the server then refuses. Harmless
   (server is authoritative), one-line hardening if desired.

## Provenance

No servers started and no databases created outside the suites' own throwaway
`test_athena_*` databases; dev database untouched. Worktree left clean.
