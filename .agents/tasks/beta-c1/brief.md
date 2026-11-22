# Brief: C1 posting engine G6.2–6.12 (Muse) — dispatched from main d426a3c

Worktree: /Users/iampaulking/projects/duchess/athena-erp-c1, branch feat/c1-posting (from latest main).
Read: tasks.md C1; .kiro/specs/athena-batch-2/tasks.md G6.2–6.12 (this task) + batch-2 "Conventions"; .kiro/specs/athena-batch-2/design.md G6; requirements R6 (batch-2 requirements.md); ADR-0023 (posting engine, OPEN backdating/book policy — see Z2 rule below); internal/ledger masters; internal/types money/allocate; internal/audit; internal/lifecycle; internal/rules; proto/athena/v1/ledger.proto.

Numbers: NO new migration and NO new ADR — 6.1 already created migration 14 (`athena_operations`, `athena_posting_groups`, `athena_posting_log`, `l_gl`) and ADR-0023. Reuse those tables. If you believe a new table is truly needed, STOP and report BLOCKED with the reason instead of inventing schema.

Z2 RULE (binding, user decision open): the backdating window and book policy are UNDECIDED. Implement only mechanical period/posting rules that do not depend on them (period exists; postings balance by company and book; leaf/valid accounts; dimensions; control accounts; reversals mirror with `is_reversal`). Anywhere the batch-2 text needs "allowed period", "correction window" or book-policy behavior: implement the mechanical part, and for the policy part return a plain `failed_precondition` naming the undecided policy (e.g. "book policy is not decided yet") with a test pinning that behavior — never invent a window. List every such point in each report.

Slices, each merging on its own (report after each; one worker, sequential):
1. 6.2+6.3: `internal/posting/` plan validation (balance by company/book, leaf accounts, periods, dimensions, control accounts) + single-transaction writer + balance deltas; property test for balanced postings.
2. 6.4+6.5: Operation_ID contract (canonical payload hash, replay result/conflict, uniqueness per voucher revision/event/book) + idempotent-replay property test with concurrent replays.
3. 6.6+6.7: reversal (mirror stored rows, `is_reversal`, correction in a mechanically-allowed period per the Z2 rule) + reversal-mirrors property test.
4. 6.8: lifecycle hooks (Post + mirror) and Posting Rule type with plan API, fixture replay, plan diff, named approval. Rules reach data only through Go host functions; no bypassing gates/invariants/transactions.
5. 6.9: `PreviewPosting`, `GetPostingStatus`, `ListPostingLog`, `ListLedgerRows` in ledger.proto + `make generate` + server/ledger.go + gate.go policies (every procedure needs a policy entry) + `cmd/athena/post.go` CLI. Spans without values.
6. 6.10+6.11+6.12: Desk voucher view in RecordsPage (Preview posting dialog, ledger rows + Posting_Log on a submitted voucher, Sonner toasts for blocked posts; DetailSheet + everyday words); docs (overview, README, AGENTS.md, decision log; mark Posting rules not production-ready until accepted); full checkpoint incl. browser pass (preview, submit, double-click submit, cancel, both themes).

Rules: batch-2 conventions (policies in gate.go, generate twice no diff, property tests, spans without values, Proposed ADRs only if needed — none expected). Use throwaway Workspaces/test_athena_* databases only, never the dev database; fake HOME for any server (user server holds the real lock). Commit on your own branch feat/c1-posting; you never merge, push or start agents.
Checks per slice: gofmt -l cmd internal (empty), go build ./..., go vet ./..., make test, make generate stable; web typecheck/test/build for slice 6.
Report: hc send master "C1 SLICE n: PASS or BLOCKED <one line>".
