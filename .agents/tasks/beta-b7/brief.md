# Brief: B7 sessions, rate limits and tabs, server part (Muse Spark, taking over from Codex)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b7-sessions, branch feat/b7-sessions. Work only there.
**Handover:** Codex started this task and hit its usage limit at 12:50 on 2026-11-21. The worktree holds its partial, uncommitted, unverified work. Start with `git status` and `git diff`. Keep what is correct and matches this brief, fix or remove what is not, then finish. Note in your report what you kept and what you changed.

Read first: AGENTS.md; .kiro/specs/athena-beta/requirements.md R7; design.md sections 3, 4 and 5; tasks.md B7; internal/auth/service.go (current: IdleTimeout 8 h, AbsoluteLifetime 7 days, 5 failures lock 15 min); the OWASP Session Management Cheat Sheet.

Numbers assigned by the master: migration 16 `session_policy`; ADR-0026 Session lifetimes, step-up and rate limiting (Proposed, amends ADR-0014 and ADR-0017).

Defaults were approved by the user on 2026-11-21:
- idle 30 min for everyday users, 15 min for sessions that hold a build permission and for root;
- absolute 12 h; "keep me signed in" gives 7 days, for users without build permissions only;
- step-up after 15 min for destructive actions in Build or as root;
- sign-in limits: 10 per minute per IP and 5 per 15 min per username;
- writes: 60 per minute per session; Reports and export with their own limit.

## Do
1. Session policy: per-session idle and absolute expiry fixed at creation from Workspace settings within the R7.1 bounds (store the settings in the G5 Settings chain or migration 16). `Unauthenticated` carries a reason detail (`idle`, `absolute`, `revoked`). `Login` takes `remember_device`.
2. Step-up: `AuthService.StepUp(password)` records `step_up_at` (migration 16). Destructive procedures need a step-up within 15 min when the caller holds a build permission or is root; otherwise they answer `FailedPrecondition` with a `step_up_required` detail. List these procedures in gate.go next to procedurePolicies, and test that every destructive procedure is covered.
3. Session ID rotation at sign-in, on a permission change affecting the user (their sessions are re-issued or ended) and on step-up. `Clear-Site-Data: "cache", "cookies", "storage"` on Logout and ChangePassword responses.
4. Rate limiting: `internal/server/ratelimit.go` with `golang.org/x/time/rate` (already in go.mod, move it to direct). Limiters keyed by IP, username and session, in a bounded map with idle expiry. A ConnectRPC interceptor runs before the gate. It answers `ResourceExhausted` with a retry-after detail and a `Retry-After` header. Limits are Workspace settings. Each denial goes to the Access Log as `rate_limited` with no request values. Root is not exempt from the sign-in limits.
5. Proto and generated code: `make generate`. CLI: `athena security show|set` for the session and rate settings, and `athena user sessions` showing device, last seen and expiry.
6. Tests: each limit, idle and absolute expiry (with an injected clock), remember-device rules, step-up required and satisfied, rotation, Clear-Site-Data, bounded limiter map. Add Bruno checks under bruno/athena-api for 429 and expiry if the collection's pattern allows it without a password in files.

## Do not
- Do not touch web/src except the generated `web/src/gen/` files. The Desk part (BroadcastChannel tabs, idle warning, step-up dialog) is a later Muse task.
- Do not write to the dev database. Do not commit, merge, push or start agents.

## Checks
gofmt -l cmd internal (empty), go build ./..., go vet ./..., make test, make generate twice with no diff.

## Report
When done or stuck: write `/Users/iampaulking/projects/duchess/athena-erp/.agents/tasks/beta-b7/report.md` (PASS or BLOCKED, files changed, command output), then run `hc send t6 "B7 SERVER: <PASS|BLOCKED>, report at .agents/tasks/beta-b7/report.md"`.
