# P8 notes: authentication and Grants (iteration 1)

Date: 2026-11-20. Branch `main`, base commit 9ce85a8. Nothing is committed (the approval step commits).

## What was built

- **`internal/auth`** (new): users, Argon2id password hashing, database sessions, Roles and Grants, the Access Log.
  - Argon2id through `golang.org/x/crypto/argon2` (now a direct dependency, v0.59.0). Reason: the standard library has no Argon2; `x/crypto` is maintained by the Go team, pure Go, no cgo, and was already in `go.sum`. The wrapper libraries add only PHC encoding, which is about 40 lines here.
  - Settings: 64 MiB, t=3, p=4 (RFC 9106 option 2), 16-byte salt, 32-byte key, stored as a PHC string. Stored parameters are bounded. Comparison is constant time. A hash made with older defaults is rehashed after sign-in. At most 4 hashes run at once. Unknown, disabled and locked users verify a dummy hash and get the same message.
  - Policy: 12 to 256 characters, and the password may not equal the username. Five wrong passwords lock the user for 15 minutes.
  - Sessions: a 256-bit random token in the `athena_session` cookie (HttpOnly, SameSite=Strict, Secure on HTTPS). Only its SHA-256 is stored. A session ends after 8 h idle, after 7 days, on sign-out, on a password change (other sessions) or reset (all sessions), and when the user is disabled.
  - CSRF: `X-CSRF-Token` = HMAC-SHA-256 keyed by the session token.
- **System migration 9** `users_sessions_and_grants`: `athena_users`, `athena_roles`, `athena_grants`, `athena_user_roles`, `athena_sessions`, `athena_access_log`.
- **Permission gate** `internal/server/gate.go`: a deny-by-default ConnectRPC interceptor with a per-procedure policy table. A procedure missing from the table is denied, and `TestEveryProcedureHasAPolicy` checks the table against the generated services. It also adds the security headers (CSP, nosniff, frame deny, referrer policy) and refuses cross-origin state-changing requests.
- **Grant enforcement in the handlers** `internal/server/access.go`:
  - Row conditions use the P7 expression language with `user.name`, are translated to SQL, and apply to `ListRecords`, `LookupRecords` and every single-Record operation. A filter cannot widen them.
  - Field permission levels: a field above the user's level is removed from Records, from the Record Type definition sent to non-builders, from filter and expression scope, and from preview output. Writing it is denied.
  - Audit masking (P6 carry-over): masked `FieldChange` in `ListRecordHistory`, and masked snapshots and diffs in `ListRecordVersions`. The stored hash chain is untouched, and the test verifies the chain afterwards.
  - Separate duties: the creator cannot submit a Record.
  - Reading history, versions and activity needs the `audit` action. `VerifyAuditLog` needs the `audit` system permission.
- **Actor**: the signed-in username replaces "Local administrator" in Records, the Audit Log, versions and activity. The CLI stays "Local administrator" (the operator) and applies no Grants. An API call without a principal is refused.
- **`AuthService`** (`proto/athena/v1/auth.proto`, `internal/server/auth_rpc.go`): Login, Logout, GetSession, ChangePassword, ListUsers, CreateUser, UpdateUser, ResetPassword, ListRoles, SaveRole (checks Record Type and condition), DeleteRole, ListAccessLog.
- **Parent rollup in one save** (P7 carry-over): `SaveRecordTypeRequest.parent` saves a child and its parent's new definition in one request. The parent's expressions are checked against the new child first. The desk sends the parent automatically when a saved parent's formula reads `doc.<child>.`. Limit: the parent must already be saved once; if the parent save fails after the child saved, the child stays saved and the error says so.
- **CLI** (`cmd/athena/access.go`):
  - `athena user create|list|set|reset-password`, `athena role list|show|save|delete`, `athena access-log`, all with `--workspace`, `--json` and help text.
  - Passwords come from `ATHENA_USER_PASSWORD` or a prompt.
  - Only `user create --owner` can create an instance owner.
  - The prompt asked for `--project`; P2a renamed it to `--workspace`, and `terms_test.go` forbids `--project`, so `--workspace` is used. `--addr` does not apply, because no server starts, as with the other kernel commands.
  - `init`, `new`, `run` and `serve` are unchanged.
- **Desk**:
  - Sign-in page (shadcn Card, Input, Label, Button). When no users exist, it tells the person to run `athena user create <name> --owner`.
  - User menu: a shadcn-vue `dropdown-menu` added from the official reka-luma registry JSON. The shadcn-vue CLI failed on the pnpm store-dir mismatch while re-adding dependencies that are already present, so the registry files were written as-is. The menu has Change password (dialog) and Sign out.
  - Build is shown only with the build permission; View is kept.
  - "New record" is hidden without the create action, and history is skipped without the audit action.
  - Masked history lines read "<field>: hidden at your permission level".
  - Field settings gained "Permission level (0 to 9)".
  - Sonner toasts (bottom right) report sign-in failures, denials, session end, sign-in and sign-out.
  - A CSRF interceptor in `web/src/api.ts` adds the header and returns the desk to sign-in on Unauthenticated.
  - `fieldRules.ts` is unchanged: permission level is a Record Type attribute, not a field value rule, so there is no client rule to mirror.
- **Telemetry**: new kernel spans for login, logout, change password, and user and Role operations. They carry the operation, request ID and outcome only. OTel stays off unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set (unchanged).
- **Docs**:
  - ADR-0014 (**Proposed**).
  - Decision log: ADR-0014 row, D-019, and an updated O-008.
  - `docs/README.md`, `docs/architecture/overview.md` (status table, migration count, runtime, desk), `record-type-and-pack-model.md`, `README.md` (first-owner step, CLI, limits, layout).
  - `AGENTS.md`: only the no-auth and loopback statements, the API contract list and "permissions are Grants". The server still binds to loopback, because ADR-0014 is not accepted and TLS and deployment are undecided.

## Invariant tests (`internal/server/access_test.go`, `internal/auth/auth_test.go`, `cmd/athena/access_test.go`)

These run over real HTTP with cookies, the gate and CSRF:

- No Grant means deny, the denial is written to the Access Log with user, Record Type, action and request ID, and no Record Types are listed.
- Signed-out calls are refused, and system info stays open.
- A missing or forged CSRF token is refused, and a cross-origin POST gets 403 with CSP set.
- Lists, lookups (by text and by id) and get/update never reach rows outside the row condition, and a filter cannot widen the scope.
- A field above the user's level is hidden in get, list and the definition. Writing it is denied, and so are filtering and evaluating on it. History masks it, the chain still verifies, and the operator sees the value.
- The Audit Log needs its own permission.
- Separate duties: the maker is denied, and the checker submits with `modified_by` set to the checker.
- A Role change takes effect on the next call (grant and revoke).
- Logout ends the session, and disabling a user ends all their sessions.
- Five failed logins lock the user, the lock expires (injected clock), and the lockout and failure rows are written.
- Passwords, the session token and the CSRF token never appear in span attributes or status (in-memory exporter), the process log, the Access Log, the sessions table or the password hashes.
- A child and its parent rollup save in one request; a bad parent formula saves nothing.
- Every procedure has a policy.
- Unit tests: Argon2id round trip and salt, rehash flag, hostile hashes, password policy, additive Grants and scope, separate duties, CSRF binding, Role validation.
- CLI: owner create, duplicate, short password, role save rejects a missing Record Type, role list, user create with roles, list, set --disable --json, last owner protected, reset-password, delete --yes, access-log content without the password.

Updated existing tests:
- `audit_test.go`: an API-sourced call now carries an owner principal, because a principal-less API call is refused by design.
- `cmd/athena/migrate_test.go`: 8 changed to 9 migrations.

## Verification (exact commands, all from the repository root)

| Command | Result |
| --- | --- |
| `go mod tidy` | `golang.org/x/crypto v0.59.0` is direct. Side effect of `go get golang.org/x/crypto@latest`: `golang.org/x/net` v0.59.0→v0.62.0 and `golang.org/x/text` v0.42.0→v0.44.0 (indirect) |
| `gofmt -l cmd internal gen` | no output |
| `go build ./...` | ok |
| `go vet ./...` | ok |
| `make generate` twice, comparing `git diff --stat` and `git status --short` after each | identical (stable) |
| `set -a; . ./.env.local; set +a; ATHENA_TEST_DATABASE_URL=${ATHENA_TEST_DATABASE_URL:-$ATHENA_DATABASE_URL} go test -count=1 ./...` (MariaDB 11.8, brew `mariadb@11.8`) | all packages ok: cmd/athena 18.1s, internal/auth 1.4s, internal/server 176.0s, expr, lifecycle, numbering, store, types, workspace |
| `CI=true pnpm --dir web run typecheck` | ok |
| `CI=true pnpm --dir web run test` | 72 passed |
| `CI=true pnpm --dir web run build` | built; `internal/webui/dist/index.html` and `assets/` rebuilt (timestamp 00:46). The build deleted the tracked `internal/webui/dist/.gitkeep`; it was restored with `git checkout` |
| `make restart` | `/healthz` → `{"status":"ok","database":"Connected"}`. The dev database now has migration 9 |
| Browser: headless Chrome over CDP (`/tmp/p8/drive.mjs`, log `/tmp/p8/browser.log`, screenshots `/tmp/p8/*.png`) against `bin`-equivalent `/tmp/p8/athena serve` on 127.0.0.1:3000 with a throwaway database `test_athena_p8browser`, seeded through the CLI and dropped afterwards | 32 of 32 pass, 16 in light and 16 in dark: theme applied; sign-in page shown with the database connected; wrong-password toast; sign in as clerk; no Build without build; View lists Note; no New record without create; salary hidden; history masks salary and shows no value; denied delete shows a toast; role change via `athena user set --roles` shows New record after reload; sign out; owner sees Build and salary; user menu; change-password dialog. The server log contains no passphrase and no session cookie |

The browser run used a throwaway database so no users were written to the dev database.

**Action for the user:** the dev desk at `make run` now shows the sign-in page and needs a first owner: `set -a; . ./.env.local; set +a; go run ./cmd/athena user create <name> --owner`.

## Changed files

Modified: `AGENTS.md`, `README.md`, `cmd/athena/main.go`, `cmd/athena/migrate_test.go`, `docs/README.md`, `docs/architecture/overview.md`, `docs/architecture/record-type-and-pack-model.md`, `docs/decisions/decision-log.md`, `gen/athena/v1/record_types.pb.go`, `gen/athena/v1/records.pb.go`, `go.mod`, `go.sum`, `internal/server/{audit_test,auditlog,expressions,expressions_rpc,kernel,lifecycle,lookup,migrations,record_types,records,server,sysmigrations}.go`, `proto/athena/v1/record_types.proto`, `proto/athena/v1/records.proto`, `web/src/{App.vue,api.ts,style.css,types.ts}`, `web/src/components/FieldSettingsDialog.vue`, `web/src/pages/RecordsPage.vue`, `web/src/gen/athena/v1/{record_types_pb,records_pb}.ts`.

New: `internal/auth/{password,principal,service,accesslog,auth_test}.go`, `internal/server/{gate,access,auth_rpc,access_test}.go`, `cmd/athena/{access,access_test}.go`, `proto/athena/v1/auth.proto`, `gen/athena/v1/auth.pb.go`, `gen/athena/v1/athenav1connect/auth.connect.go`, `web/src/gen/athena/v1/auth_pb.ts`, `web/src/pages/SignInPage.vue`, `web/src/components/ChangePasswordDialog.vue`, `web/src/components/ui/dropdown-menu/*` (15 registry files), `docs/decisions/adr-0014-authentication-and-grants.md`, this file.

Not mine, left untouched: `docs/Athena-ERP-Playbook.md`, `docs/Records-and-movements.md`, `docs/Research-and-audit.md`. These untracked files appeared during the session.

## Open points for review

- Size: about 630 changed lines in existing files plus about 3,400 new lines (about 1,100 of them tests, ADR and the generated-style registry components). That is above the roadmap's 1,500-line split guideline. The phase was kept whole because the gate, the enforcement and the tests depend on each other.
- Not built (listed in ADR-0014): OIDC, MFA, company and user-permission data scopes, report and export permissions (no reports yet), a desk page for users and Roles (CLI and API only; adding a Build page changes the AGENTS.md Build list and needs approval), IP rate limiting, session listing.
- Browser OTLP forwarding at `/otlp/v1/traces` stays unauthenticated (loopback only, as before).
- Each request runs three small queries to load the principal; there is no cache.
- The parent rollup save is two journaled operations under one schema lock, not one atomic transaction. MariaDB DDL commits on its own.
- PostgreSQL is untested.
- Starlark is not involved; nothing here is Starlark.

## Iteration 2 (review fix)
Added `TestSessionsEndAfterIdleAndAbsoluteTimeouts` and `TestPasswordChangeAndResetEndSessions` to `internal/server/access_test.go`, using the injected `Service.Now` clock. They check the idle timeout, the absolute lifetime for an active session, that `ChangePassword` ends other sessions and keeps the current one, and that `ResetPassword` ends all.
Run: `gofmt -l internal cmd` (no output), `go vet ./internal/server/` (ok), `go test -count=1 -run 'TestSessionsEndAfter|TestPasswordChangeAndReset|TestLogoutEnds' ./internal/server/` (ok, MariaDB via .env.local). No production code changed, so the iteration 1 full-suite results stand.
