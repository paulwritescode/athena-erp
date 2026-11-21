Review B7 server (sessions, step-up, rate limiting), read-only, in /Users/iampaulking/projects/duchess/athena-erp-b7-sessions (uncommitted vs main).
Spec: .agents/tasks/beta-b7/brief.md, .kiro/specs/athena-beta/requirements.md R7, design.md section 4, ADR-0026 in the worktree. Worker report: .agents/tasks/beta-b7/report.md.
Migration decision by the master: B7 keeps migration 15 (the worktree's Version 15 is final); the ADR, decision log and overview that say 16 must be corrected to 15. Report each place as a finding.
Check, as a security reviewer:
1. Idle and absolute expiry are enforced on the server for every request, cannot be extended by the client, and use the per-session values fixed at creation. Bounds enforced on set.
2. remember_device only lengthens sessions for users without build permissions; the cookie attributes (HttpOnly, SameSite, Secure when TLS, Max-Age only when remembered) are right.
3. Step-up: the destructive procedure list in gate.go is complete (compare with every procedurePolicies entry), fails closed, and StepUp cannot be brute-forced (rate limited, counts toward lockout).
4. Session ID rotation at login, permission change and step-up; old tokens stop working at once.
5. Rate limiter: keyed correctly (IP from the connection, not a spoofable header, unless a trusted proxy setting exists), bounded memory, runs before the gate, ResourceExhausted with Retry-After, Access Log rows without request values. Root not exempt from sign-in limits.
6. Clear-Site-Data on logout and change password.
7. No password, token, CSRF value or hash in logs, spans, errors or tables.
8. Removing the hardcoded password from bruno/athena-api/03-login.bru did not break the collection's documented use.
9. Run: gofmt -l cmd internal, go build ./..., go vet ./..., and make test (it needs ATHENA_TEST_DATABASE_URL; it may need a long timeout; run go test -p 1 -timeout 40m ./... with ATHENA_TEST_DATABASE_URL set from .env.local ATHENA_DATABASE_URL if make test times out under load). Do not print the DSN.
Classify each finding BLOCKER or FOLLOW-UP. PASS if no BLOCKER.
Do not edit, commit or start agents. Reply: hc send t6 "B7 REVIEW: PASS or FAIL" with findings file:line, BLOCKERs first.
