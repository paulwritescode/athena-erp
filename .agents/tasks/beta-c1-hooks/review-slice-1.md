# Review: C1 slice 1 (6.8), lifecycle hooks + Posting rule type

Verdict: PASS

Scope: diff f28e953...a3691da on branch feat/c1-hooks (11 files, +1813/-18),
reviewed from committed blobs. Worktree /Users/iampaulking/projects/duchess/athena-erp-c1
was clean at a3691da when the review started but gained uncommitted slice-2 (6.9)
WIP during the review (8 files, incl. lifecycle.go and rules_engine.go); that work
was not touched. All gates below ran in pristine scratch worktree
/tmp/athena-c1-s1-gates (detached HEAD a3691da, clean tree, .env.local copied for
test DB URLs only, never committed).

Gates (scratch worktree at a3691da):
- gofmt -l cmd internal: empty.
- go build ./...: OK. go vet ./...: OK.
- affected (absolute main-repo path): falls back to full make test on the proto
  change; reused worker's green run on clean a3691da
  (suite-logs/1123-001831-w-c1.log, w-c1, 2026-11-23T00:28:03+0300): 16 Go
  packages ok, 0 FAIL, no skips (MariaDB tests ran); web 63 files / 613 tests
  passed. Log read in full instead of rerunning.
- Own uncached reruns with live MariaDB, all PASS: internal/posting
  TestHashPlans + TestCanonicalDecimalText; internal/rules TestPosting* +
  TestRunFixtures*; all 11 tests in internal/server/posting_hooks_test.go
  (submit posts, no-rule posts nothing, cancel reverses, preview-hash refuse +
  accept, named approval, broken plan refused + disabled draft + enable refused,
  voucher lines with isolation + leak checks, rejection refuses submit,
  shared-book refused, diff unit, operation-id stability).
- make generate twice (with suite's nvm node fallback on PATH): exit 0 both
  times, no diff. (First attempt without node failed at web-deps with
  "node: No such file or directory" — environment PATH only, not the slice.)

Blockers: none.

Findings by brief item:

1. Lifecycle hooks + worker deviation (internal/lifecycle untouched) — ACCEPT.
   internal/lifecycle/lifecycle.go is storage-agnostic by design ("knows nothing
   about storage"); importing the posting engine there would break layering, so
   the deviation is the better placement, not a gap. R6.4/R6.5/R6.11 hold
   equivalently: transition() calls postVoucher (internal/server/lifecycle.go:300)
   and reverseVoucher (:305) in the same tx before commit; PostInTx/ReverseInTx
   join the caller's tx and never commit (internal/posting/write.go:48,
   reverse.go:60), so voucher revision, ledger rows, balance deltas, audit,
   operation result and outbox commit together (R6.5) and a refused plan rolls
   the transition back (proven by TestSubmitRefusesAChangedPreviewHash: draft
   untouched, 0 ledger rows). One group per event+book via PostInTx uniqueness
   (R6.4, 1 group asserted); cancel mirrors every unreversed submit group of the
   voucher with reversal flags and nets to zero (R6.11, asserted). Only
   SubmitRecord/CancelRecord reach transition(), so no bypass path; posting runs
   after all in-tx hooks accept, which is stronger than registry-order
   dependence. Amend correctly posts nothing. Suggest master note the deviation
   as accepted in the slice summary.

2. Posting rule type — PASS. Posting type (internal/rules/rules.go:46) with
   post() entry, plan-dict collection judging shape only (collectPlan :669,
   values left to the engine), fixtures() required at compile, RunFixtures
   (:897) on an isolated host (reads/writes refused). Covered by
   internal/rules/posting_test.go (171 lines: plan collection, required
   defs, shape errors naming the leg, read-but-not-set, fixture cases +
   bad shapes).

3. Runner + SaveRule activation gate — PASS. Generic engine skips posting Rules
   (rules_engine.go hook filter; live-run count asserted = 1 in submit test);
   postingPlans (:319) runs enabled posting Rules in rule order, refuses
   duplicates per book, wraps in voucher identity (voucherPlan :353) with
   stable operation ids (operationID :376, 55 chars, tested). gatePostingSave
   (internal/server/rules.go:283) replays fixtures on activation, lands changed
   plans disabled with a capped author-worded diff, and same-source re-save with
   enabled is the named approval. Versions + audit recording VERIFIED to
   persist without migration: INSERT into athena_rule_versions (:244, table
   pre-exists in sysmigrations.go) + writeAudit (:254, appends in caller's tx,
   persists exactly on commit) run before commit on both the disabled landing
   and the approval; TestPostingRuleChangeNeedsNamedApproval asserts versions 1
   -> 2 disabled -> 3 enabled with a non-empty actor. Fixture children staging
   is symmetric (stage + delete, rollback-safe) with leak assertions in tests.

4. Preview-hash check (R6.15) — PASS. records.proto adds only
   posting_preview_hash field 4 (:210, +5 lines incl. comment); no new service,
   method or message, so no gate entry is needed — confirmed none added
   (gate.go untouched). Generate-clean (see gates). postVoucher recalculates
   via HashPlans and refuses stale hashes with FailedPrecondition; both
   directions tested. Empty hash posts current plans per the documented
   contract; PreviewPosting (6.9) will mint the hash.

5. Small additions — PASS. HashPlans (internal/posting/write.go:538) is
   C1-owned, canonical-hash based, scale-insensitive, tested both equal and
   changed cases. find() parent_id filter (rules_engine.go:620-631 + child row
   linkage :543-544) is needed: billLinesRule reads voucher lines through it,
   fixtures stage children for it, and TestPostingRuleReadsItsVouchersLines
   proves per-voucher isolation. Only follow-up: the two error branches
   (non-child def, non-string id) have no dedicated tests.

6. Z2 — worker's byte-identical claim VERIFIED. internal/posting/posting.go,
   reverse.go and operations.go have zero diff between f28e953 and a3691da
   (only write.go + canonical_test.go changed in that package); sentinel texts
   compared line-for-line identical. Full-diff grep for window/policy/day-count/
   book-defaulting finds only sentinel passthrough (postingError
   lifecycle.go:423, fixturePlanError rules.go:467) and one forward-reference
   comment. Nothing skipped for Z2 in this slice; nothing invented.

7. No Desk — CONFIRMED. Only web change is web/src/gen/athena/v1/records_pb.ts
   regen; no hand-written web/ file in the diff.

Follow-ups (non-blocking, for tasks.md):
- F1: proto/athena/v1/rules.proto:43-44 comment still says "Posting arrives with
  the posting engine" — now stale (comment only, zero behavior).
- F2: Desk Rules page lacks the posting type: web/src/ruleDefinitions.ts:8
  ruleTypes has 4 entries, no posting, so posting Rules are API-only until a
  Desk slice picks this up.
- F3: parent_id filter error branches untested (rules_engine.go:624-630).
- F4 (question for master/stylus, not a demand): R6.16 "named approval" is met
  literally (approver's name recorded in versions + audit), but the same user
  may author v2 and approve v3 — four-eyes separation is not enforced. Confirm
  whether same-user approval is acceptable or a later slice should require a
  different approver.

Z2 list: nothing skipped; sentinels kept byte-identical, no window, policy, day
count or default book introduced anywhere in the diff.
