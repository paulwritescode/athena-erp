Review the sessions slice plus the passive-request fix, read-only, in /Users/iampaulking/projects/duchess/athena-erp-desk-settings (uncommitted vs main 76210b3 or later). This touches security: be strict.
Server:
1. gate.go passiveProcedures (NotificationService UnreadCount and List, AuthService GetSession, WorkspaceService GetAppearance): AuthenticatePassive checks idle, absolute, revoked and disabled exactly like Authenticate but never moves last_seen_at. Both the gate and the rate limiter use it (the worker found a second path in the limiter). No other path touches the session on a passive call.
2. AuthService.Touch is active, signed-in only, has a policy, and cannot be abused to bypass the absolute lifetime.
3. TestPassivePollingEndsIdleSession and TestPassiveProcedureCoverage really test what they claim.
4. A passive procedure added later cannot silently keep sessions alive (the coverage test should catch it).
Desk:
5. Idle warning 2 minutes before expiry; Stay calls Touch and reschedules; a lapse shows the plain note; the BroadcastChannel carries sign-out, permission change and Workspace change to every tab, with the storage-event fallback; Keep me signed in only where it applies; notification polling pauses while hidden.
6. No password, token or session id in BroadcastChannel messages or localStorage.
Run gofmt -l, go build, go vet, make test, and CI=true pnpm --dir web run typecheck, test, build.
Merge-small rule: BLOCKERs only stop the merge. Reply: hc send t6 "SESSIONS REVIEW: PASS or FAIL" with file:line. Do not edit, commit or start agents.
