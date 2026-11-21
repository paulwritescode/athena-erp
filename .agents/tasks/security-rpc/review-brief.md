Review the security settings RPC, read-only, in /Users/iampaulking/projects/duchess/athena-erp-security-rpc. Brief: .agents/tasks/security-rpc/brief.md; report: .agents/tasks/security-rpc/report.md.
Check as a security reviewer:
1. Get and Set require the right permission. Set is in destructiveProcedures (step-up) and fails closed.
2. Set validates every key against the same bounds as the CLI, uses the same Go function (one write path), is atomic, and rejects unknown keys.
3. Each change writes an Access Log row with the key, old and new value, and the actor, and nothing secret.
4. A user cannot use Set to lock out the root administrator or disable rate limiting below the bounds.
5. proto additive only, procedurePolicies present, make generate stable, docs updated.
6. Run gofmt -l, go build, go vet, make test.
BLOCKER or FOLLOW-UP; PASS if no BLOCKER. Do not edit, commit or start agents. Reply: hc send t6 "SECURITY RPC REVIEW: PASS or FAIL" with file:line.
