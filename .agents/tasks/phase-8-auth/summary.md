# P8 summary: authentication and Grants

Review verdict: APPROVED (iteration 2, no open findings).

## What changed
- `internal/auth`: users, Argon2id hashing (64 MiB, t=3, p=4), database sessions (hashed token, idle 8 h, absolute 7 days), Roles, Grants, Access Log, lockout (5 failures, 15 minutes).
- Migration 9 `users_sessions_and_grants` adds six `athena_*` tables.
- `internal/server/gate.go`: deny-by-default ConnectRPC gate with per-procedure policies, CSRF (HMAC), security headers, same-origin check.
- `internal/server/access.go`: row conditions, field permission levels, audit masking (hash chain untouched), separate duties, `audit` action.
- `AuthService` (`proto/athena/v1/auth.proto`): login, logout, session, password, users, Roles, access log.
- Parent rollup saved with the child in one request (P7 carry-over).
- CLI: `athena user ...`, `athena role ...`, `athena access-log` (`--workspace`, since `--project` is forbidden).
- Desk: sign-in page, user menu (shadcn-vue dropdown-menu), change-password dialog, permission-aware Build and View, Sonner toasts, CSRF interceptor.
- Docs: ADR-0014 (Proposed), decision log, overview, model doc, README, AGENTS.md. Server stays on loopback.
- `golang.org/x/crypto` v0.59.0 is now direct; `x/net` and `x/text` bumped as a side effect.

## Verification
- `gofmt -l`, `go build ./...`, `go vet ./...`: clean.
- `make generate` run twice: stable output.
- `go test -count=1 ./...` with MariaDB 11.8: all packages pass (iteration 1). Iteration 2 added session timeout and password-change tests; targeted run passes.
- Web: typecheck ok, 72 tests pass, build ok.
- Browser (headless Chrome, throwaway database): 32 of 32 checks (light and dark). No passphrase or cookie in logs.

## Caveats
- Size is about 3,400 new lines plus 630 changed, above the 1,500-line split guideline.
- Not built: OIDC, MFA, data scopes, report permissions, a users and Roles desk page, IP rate limiting, session listing. PostgreSQL is untested.
- Dev desk now needs a first owner: `go run ./cmd/athena user create <name> --owner`.
- Untracked `docs/Athena-ERP-Playbook.md`, `docs/Records-and-movements.md`, `docs/Research-and-audit.md` are not part of this phase.

## Next: P9, Starlark host
Embed `go.starlark.net` for administrator-configured conditions, macros and bounded actions. It must go through Go permission checks, invariants and transaction boundaries, with step and time limits and no ambient I/O. Rule means Starlark code only.
