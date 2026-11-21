Final review of B9a server (loop 3 of 3), read-only, in /Users/iampaulking/projects/duchess/athena-erp-b9a-server.
Background: Codex reviewed twice. Its open blockers were (1) a historical compensated pair group (old shape: a completed child operation inside a failed or compensated save_group) still ranked as the last save, record_types.go:262,481; (2) PostgreSQL SQL for the table precheck (bare ?) and the master count (backtick-quoted deleted_at), record_types.go:313,349, postgres.go:232, records.go:514.
Verify both with your own reproduction: write the old shape into a throwaway MariaDB and list; capture the generated SQL for both dialects (unit level is fine) and check $n markers and double-quoted identifiers.
Recheck the earlier fixes: scoped counts hidden (row condition and Assignment scope), visible with an extra unrestricted Grant; soft-deleted masters count 0; retry when a table disappears.
Run gofmt -l, go build, go vet, make test.
BLOCKER or FOLLOW-UP per finding; PASS if no BLOCKER. Do not edit, commit or start agents. Reply: hc send t6 "B9A SERVER FINAL: PASS or FAIL" with file:line.
