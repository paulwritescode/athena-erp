# Brief: security settings RPC (small server task)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-security-rpc, branch feat/security-rpc. Work only there. No web/src changes except the generated web/src/gen/.
Read: AGENTS.md; internal/auth/security.go (settings, bounds, defaults); cmd/athena/security.go (athena security show|set); ADR-0026; proto/athena/v1/auth.proto; internal/server/gate.go.

Add to AuthService (or a new SecurityService if that is cleaner; say which):
- GetSecuritySettings: returns every setting, with its current value, default, bounds and unit, and a plain one-line description for the Desk ("Sign out after this many minutes without activity"). Permission: access:sessions or settings:security (add `settings:security` to auth.SystemPermissions if no existing permission fits; prefer a new one).
- SetSecuritySettings: partial update of named keys, validated against the same bounds as the CLI, and atomic. Permission: settings:security. Add it to destructiveProcedures, because it weakens security, so step-up is required. Write an Access Log row with the key names and old and new values (these are policy numbers, not secrets).
- Use the same Go function as the CLI. Do not create a second write path.
- procedurePolicies entries, `make generate` twice with no diff, and tests for bounds, permission, step-up required and the audit row.
- Update ADR-0026 (a short addendum), the decision log row and the README.

Checks: gofmt -l, go build, go vet, make test. Do not commit, merge, push or start agents.
Report: write .agents/tasks/security-rpc/report.md in the main repo, then hc send t6 "SECURITY RPC: PASS or BLOCKED".
