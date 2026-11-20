# P8 review (iteration 2)
Verdict: APPROVED. The one iteration 1 blocker is fixed. No blockers remain.

## Prior blocker: verified fixed
- `internal/server/access_test.go:451` `TestSessionsEndAfterIdleAndAbsoluteTimeouts` uses the injected `Service.Now` clock. It checks a session is accepted just inside `IdleTimeout` and refused after it. It also checks a regularly used session stays alive but is refused after `AbsoluteLifetime`.
- `access_test.go:492` `TestPasswordChangeAndResetEndSessions` checks `ChangePassword` keeps the current session and revokes the other, and `ResetPassword` revokes all.
- Both tests assert `ErrUnauthenticated`, so a regression in either timeout or in revocation would fail them.
- The notes record `gofmt`, `go vet` and a targeted MariaDB run as passing. No production code changed, so the iteration 1 full-suite, web and browser results stand.

## Checked in iteration 1, unchanged
Deny-by-default gate with a per-procedure policy table and `TestEveryProcedureHasAPolicy`. Row scopes, field levels and audit masking enforced in handlers. Argon2id parameters, lockout, Role changes and no secrets in spans, logs or tables are tested. Vocabulary is current (CLI uses `--workspace` because `terms_test.go` forbids `--project`; `init/new/run/serve` unchanged). Docs, ADR-0014 (Proposed), decision log and AGENTS.md updated and consistent. OTel stays optional. ConnectRPC only. The desk uses shadcn components, semantic tokens and Sonner. No PostgreSQL claim, no secrets, no copies.
