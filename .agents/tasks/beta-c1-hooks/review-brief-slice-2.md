# Review brief: C1 slice 2 (6.9), posting RPCs + CLI

You are an independent reviewer (read-only on product code). Review
commit 40bcfbe on branch feat/c1-hooks (the worker stacks docs on
the branch, so work in your OWN detached worktree: `git worktree
add --detach ../athena-erp-c1-r 40bcfbe`, copy `.env.local` into
it, review and gate there, `git worktree remove` it after the
verdict is sent). Brief:
../athena-erp/.agents/tasks/beta-c1-hooks/brief.md (slice 2 only).
Follow .kiro/specs/athena-beta/design.md section 5 reviewer
workflow and AGENTS.md. Requirements R6.14/R6.18/R6.19.

Review the slice-2 diff (git diff 8fd7bef...40bcfbe — 13 files):

1. `proto/athena/v1/ledger.proto` (+149): PreviewPosting,
   GetPostingStatus, ListPostingLog, ListLedgerRows. Shapes sane,
   paged where lists can grow; regen clean (gen/ + web/src/gen
   match — verify generate-twice-no-diff yourself).
2. `internal/server/ledger.go` (+259) + tests
   (posting_reads_test.go): preview mints the hash that closes
   the R6.15 loop with submit (verify the loop: hash minted by
   PreviewPosting accepted by the 6.8 checker, stale refused);
   dryRun on postingPlans previews WITHOUT logging live runs
   (verify no live-run side effects); status/log/list correct.
3. `gate.go` (+4): every new procedure has a policy (the
   TestEveryProcedureHasAPolicy gate must pass — confirm the
   policies are the right grants, not just present).
4. CLI (`cmd/athena/post.go` + post_test + main.go wiring):
   preview|submit|cancel|status|log work; dryRun plumbed;
   Kernel.Ledger addition minimal and justified.
5. Small changes: hash-check reorder in postVoucher (verify the
   new order is correct + tested); lifecycle.go (+11) and
   rules_engine.go (+16) hunks in scope for this slice.
6. Z2: untouched — verify no window/policy/day-count/book
   defaulting in the diff and sentinels intact.
7. Confirm no Desk in this slice (only `web/src/gen` regen).

Run (in ../athena-erp-c1-r at 40bcfbe, tree clean): gofmt -l cmd
internal (empty), go build ./..., go vet ./..., `affected` by
the ABSOLUTE main-repo path
/Users/iampaulking/projects/duchess/athena-erp/.agents/bin/affected
(falls back to full make test on the proto change; it should
REUSE the worker's green run on clean 40bcfbe — read that log
instead of rerunning; SUITE_FORCE=1 only if you suspect a flake),
make generate twice with no diff. No browser pass.

Merge-small rule: only blockers stop the merge (broken
preview-hash loop, live side effects in dryRun, missing/wrong
gate policy, red suite); the rest are follow-ups (worker notes:
plan legs lack voucher-line linkage; stale rules.proto comment;
Desk Rules page — confirm non-blocking). Write findings to
../athena-erp/.agents/tasks/beta-c1-hooks/review-slice-2.md with a
verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers
first with file:line. Remove the -r worktree after sending.
Then report: say master "C1 6.9 REVIEW: PASS or NEEDS_CHANGES
<one line>". Then wait for instructions. Do not edit product
code, commit, merge, push or start agents.
