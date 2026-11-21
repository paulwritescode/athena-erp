# B7 review loop 1 (security review FAIL): fix all, same worktree

Blockers:
1. StepUp brute force (internal/auth/security.go:172-201, via checkActorPassword in service.go:338-353). A wrong StepUp password must count toward the same failed-attempt counter and lockout as Login, and be written to the Access Log (event step_up_failed, no password). Success writes step_up. The failure message names only the password, not the username.
2. InviteUser is missing from destructiveProcedures (gate.go:153-175). It assigns Roles and returns a single-use Challenge. Require step-up when roles are non-empty, following the SaveRecordType pattern (gate.go:268-275), and extend TestDestructiveProcedureCoverage.
3. The migration number is final at 15. Correct adr-0026-session-policy.md:11, decision-log.md:31, overview.md:93, and remove the renumber comment at sysmigrations.go:492-494.

Follow-ups, do them now too because they are the same class:
4. TransferOwnership and StartReset actor-password checks count toward the lockout and are logged, like step 1.
5. Bruno 09 and 10: run them once on a disposable Workspace per docs/testing/bruno.md, and paste the result.

Rerun: gofmt -l, go build, go vet, go test -p 1 -timeout 40m ./... (ATHENA_TEST_DATABASE_URL set). A separate task is making the auth suite faster; until it lands, the 40-minute serial run is the accepted check. Update the report, then hc send t6 "B7 LOOP1: PASS or BLOCKED".
