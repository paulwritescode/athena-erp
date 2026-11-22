# Brief: C1 G6 remaining — lifecycle/Rule hooks, posting RPCs/CLI, voucher view (6.8–6.12) (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-c1, branch feat/c1-hooks (from latest main).
Read: tasks.md C1; .kiro/specs/athena-batch-2/tasks.md G6.8–G6.12 (this task); batch-2 "Conventions"; .kiro/specs/athena-batch-2/design.md G6; requirements R6.4/R6.14–R6.16/R6.18/R6.19 (batch-2 requirements.md); internal/posting (slices 1–3 are merged — read what exists); internal/lifecycle; internal/rules + internal/server/rules*.go; proto/athena/v1/ledger.proto; web/src/pages/RecordsPage.vue.

Z2 RULE (binding, user decision open): the backdating window and book policy are UNDECIDED. Keep the existing failed_precondition sentinels (ErrCorrectionWindowOpen etc.) exactly as they are; do not invent a window, a policy, a day count or a default book. List anything you skipped for Z2 in every report.

No migration and no ADR in this task.

Slices, each merging on its own (report after each with say master "C1 6.x: PASS or BLOCKED <one line>"; one worker, sequential; rebase on main before reporting PASS and say if the rebase changed anything):
1. 6.8: hook into the lifecycle + Rule host. Edit `internal/lifecycle/lifecycle.go` + `internal/server/lifecycle.go` to call Post and the mirror. Edit `internal/rules/rules.go` + `internal/server/rules_engine.go` to add the Posting rule type with the plan API, fixture replay, plan diff and named approval in `internal/server/rules.go`.
2. 6.9: preview/status/log procedures + CLI. Edit `proto/athena/v1/ledger.proto` (PreviewPosting, GetPostingStatus, ListPostingLog, ListLedgerRows); `make generate` twice with no diff; edit `internal/server/ledger.go` + `gate.go` (append entries at the end of the list); create `cmd/athena/post.go`.
3. 6.10: Desk voucher view. Edit `web/src/pages/RecordsPage.vue`: Preview posting dialog, ledger rows and Posting_Log on a submitted voucher; Sonner toasts for blocked posts. Follow .kiro/steering/athena-design.md + the design system. GATE: do NOT start this slice until master says GO (another lane is polishing nearby Desk code; master will confirm the file is free).
4. 6.11+6.12: docs (`docs/architecture/overview.md`, README, AGENTS.md, decision log; mark Posting rules not production-ready until accepted) + checkpoint (generate, test, typecheck, build, browser pass of preview/submit/double-click-submit/cancel in both themes).

Files allowed: the files above + their tests + the docs. NOT allowed: any other lane's files (ledger.proto is yours; record_types.proto belongs to lane L3 — do not touch).

Rules: batch-2 conventions (gate policies for new procedures, generate twice no diff, property tests where the design demands them, spans without values, no Record values/credentials in spans). Telemetry stays off unless the endpoint is set. Use throwaway Workspaces/test_athena_* databases only, never the dev database; fake HOME for any server. Full suites and browser passes only through .agents/bin/suite (it queues; never bypass the lock). Package tests may run directly. Commit on your own branch feat/c1-hooks (restore internal/webui/dist/.gitkeep before committing); you never merge, push or start agents. If the remaining work has more than about three independent slices or one slice runs over about 90 minutes, ask master about splitting (AGENT_RULES.md).
Checks per slice: gofmt -l cmd internal (empty), go build ./..., go vet ./..., make test through suite, make generate stable; web typecheck/test/build for slice 3.
