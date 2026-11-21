Review B4 and B12 server (appearance, notifications), read-only, in /Users/iampaulking/projects/duchess/athena-erp-b4b12-server (uncommitted vs main).
Spec: .agents/tasks/beta-b4b12/brief.md; design-system sections 2, 8, 17, 18; requirements R4. Worker report: .agents/tasks/beta-b4b12/report.md.
Master decision: notifications take migration 16 at merge (B7 has 15). Note every place that says 15 as a FOLLOW-UP for the master to renumber; it is not a blocker.
Check:
1. Notifications: written in the same transaction as their event for each source in the brief; recipients right; a user can only list or mark their own; no Record values, passwords or tokens in rows, params, logs or spans; text in everyday words; keyset paging correct; indexes match the queries.
2. Appearance: GetAppearance needs no sign-in and returns only appearance; SetAppearance needs settings:appearance; OKLCH validation rejects bad input (no CSS injection, for example a value containing ; or } or url( ); contrast computed correctly against the light and dark foregrounds with a sensible suggested value; stored in the G5 Settings chain at Workspace scope.
3. Every new procedure has a procedurePolicies entry; SetAppearance belongs in destructiveProcedures only if B7 would classify it so (say which).
4. CLI commands, docs, ADR-0024, decision log.
5. Run gofmt -l, go build, go vet, and go test -p 1 -timeout 40m for internal/notifications, internal/appearance, internal/server, cmd/athena (ATHENA_TEST_DATABASE_URL from .env.local ATHENA_DATABASE_URL, never printed). internal/auth is known slow; run it only if time allows.
Classify findings BLOCKER or FOLLOW-UP; PASS if no BLOCKER. Do not edit, commit or start agents. Reply: hc send t6 "B4B12 REVIEW: PASS or FAIL" with findings file:line.
