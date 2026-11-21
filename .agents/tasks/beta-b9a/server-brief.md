# Brief: B9a server part, Record Type list data (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b9a-server, branch feat/b9a-server. Work only there. No web/src changes except the generated `web/src/gen/`.
Read: AGENTS.md; docs/design/athena-design-system.md section 19; tasks.md B9a; proto/athena/v1/record_types.proto; internal/server/record_types.go.

The new Record Types list (design-system section 19) shows these for each Record Type: name, icon, description, kind, number of fields, number of Records, Pack (or none), when it last changed, and who changed it.
1. Find which of these `ListRecordTypes` already returns. For the rest, add fields to the proto, additive only (new field numbers, no renumbering): `record_count` (int64), `updated_at` (timestamp of the last save from the journal), `updated_by` (display name of the actor, or "Local administrator" for the CLI), and `pack_key` (empty for now, because Packs are not installed until C3).
2. Record counts: one COUNT per table is acceptable for the beta; batch them in one query with UNION ALL if there are many. Only count Record Types the caller can read. A caller without read on a type gets no count (0, plus a `count_hidden` flag).
3. Respect existing permissions. Add no new procedure unless needed. If you add one, it needs a policy in gate.go.
4. Tests: counts for empty and populated types, hidden counts, last-changed actor after a Desk save and after a CLI save.
5. Run make generate twice with no diff.
Checks: gofmt -l, go build, go vet, make test (about 5 minutes now). Do not commit, merge, push or start agents.
Report: write .agents/tasks/beta-b9a/server-report.md in the main repo, then hc send t6 "B9A SERVER: PASS or BLOCKED".
