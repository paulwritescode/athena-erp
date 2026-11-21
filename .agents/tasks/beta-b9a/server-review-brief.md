Review B9a server (Record Type list data), read-only, in /Users/iampaulking/projects/duchess/athena-erp-b9a-server (uncommitted vs its base). Brief: .agents/tasks/beta-b9a/server-brief.md. Report: .agents/tasks/beta-b9a/server-report.md.
Check:
1. Proto change is additive only (fields 16 to 20, nothing renumbered or removed).
2. Record counts: one UNION ALL query; table names come only from the stored definitions, are quoted through the dialect, and cannot be injected; missing tables count 0.
3. Permissions: a caller without read on a Record Type gets 0 and count_hidden, never a real count. Note the row-scope point in the report (full-table count for row-scoped Grants) and decide whether it leaks information that a row-scoped user should not have (it reveals how many rows exist beyond their scope). Call that a BLOCKER if it leaks and the fix is small (for example hide the count when the caller's Grant has a row condition), otherwise a FOLLOW-UP.
4. Actor: the journal options store only the display name; backfill values in options are never decoded, logged or returned; a crash-resumed save keeps the original actor.
5. updated_at and updated_by are correct for single saves, parent and child pair saves, and deletes.
6. Run gofmt -l, go build, go vet, make test (about 5 minutes).
BLOCKER or FOLLOW-UP per finding; PASS if no BLOCKER. Do not edit, commit or start agents. Reply: hc send t6 "B9A SERVER REVIEW: PASS or FAIL" with file:line.
