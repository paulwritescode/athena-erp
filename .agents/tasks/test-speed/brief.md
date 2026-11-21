# Brief: make the MariaDB test suite fast enough for make test (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-test-speed, branch fix/test-speed, from main. Work only there.

Problem (measured by you in B4B12): `internal/auth` overruns the 20-minute `go test` timeout in `make test`. `TestLastOwnerProperty` (rapid, 100 iterations) creates, migrates and drops a fresh `test_athena_*` database per iteration: 535 s on main alone. Other property tests may do the same.

Do:
1. Find every test that creates a database per rapid iteration or per sub-case (grep `testdb.` and `rapid.Check` across internal/ and cmd/).
2. Give `internal/testdb` a way to reuse one migrated database per test, with a fast reset between iterations: truncate the tables the test touched, in foreign-key order, or use a template database cloned with `CREATE DATABASE ... ` plus a dump. Choose the simplest one that keeps each iteration isolated. A transaction rollback is not enough where the code under test commits.
3. Convert those tests. Keep the property and the 100 iterations (batch-2 convention: at least 100). Keep every assertion.
4. Target: `internal/auth` under 3 minutes and `internal/server` under 6 minutes on this machine, with `go test -count=1`. Report before and after times per package.
5. Do not raise the timeout in the Makefile.

Checks: gofmt -l, go build, go vet, make test green, with times. Do not commit, merge, push or start agents.
Report: write /Users/iampaulking/projects/duchess/athena-erp/.agents/tasks/test-speed/report.md, then hc send t6 "TEST-SPEED: PASS or BLOCKED, report at .agents/tasks/test-speed/report.md".
