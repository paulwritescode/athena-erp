Review fix/test-speed, read-only, in /Users/iampaulking/projects/duchess/athena-erp-test-speed (uncommitted vs its base). Brief: .agents/tasks/test-speed/brief.md. Report: .agents/tasks/test-speed/report.md.
Check that the speed-up keeps test isolation and meaning:
1. Shared migrated databases: every test or rapid iteration starts from a clean state (no leakage between tests, including tables created by Record Type saves, p_workspace__* tables, sessions, notifications, settings). Look for order dependence.
2. No assertion removed or weakened, rapid tests still run at least 100 iterations, and no property shrunk.
3. Lowering auth.DefaultParams in the server TestMain affects no assertion and cannot leak into production code.
4. The branch base is older than main (main now has B4/B12, migration 15 notifications). Say whether it rebases cleanly and whether the shared-DB reset covers athena_notifications.
5. Run: gofmt -l, go build, go vet, and go test -count=1 ./internal/auth ./internal/server with ATHENA_TEST_DATABASE_URL from .env.local ATHENA_DATABASE_URL (never printed), and report times.
BLOCKER or FOLLOW-UP per finding; PASS if no BLOCKER. Do not edit, commit or start agents. Reply: hc send t6 "TEST-SPEED REVIEW: PASS or FAIL" with file:line.
