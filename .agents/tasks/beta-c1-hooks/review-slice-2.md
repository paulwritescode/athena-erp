# Review: C1 slice 2 (6.9), posting RPCs + CLI

Verdict: PASS

Reviewed commit 40bcfbe on feat/c1-hooks in detached worktree
../athena-erp-c1-r (tree clean), diff 8fd7bef...40bcfbe, 13 files.
Requirements R6.14/R6.18/R6.19. No blockers; follow-ups below are
non-blocking.

## Blockers

None.

## What was checked

1. Proto (`proto/athena/v1/ledger.proto` +149): PreviewPosting,
   GetPostingStatus, ListPostingLog, ListLedgerRows shapes sane;
   the two unbounded lists are keyset-paged, status is one
   voucher's groups (small, unpaged OK). `make generate` run twice
   in the review worktree, both exit 0, tree clean after each.
2. Preview-hash loop (R6.15) verified by code and by test:
   PreviewPosting builds plans at `record.revision + 1` with
   `pendingDocument(ctx, def, id, record, nil, nil)`
   (`internal/server/ledger.go`), exactly what submit's
   transition passes to postVoucher (`expected + 1`, same
   pendingDocument call, `internal/server/lifecycle.go:257,300`).
   Both sides hash `bare` plans with `posting.HashPlans`, and
   `ValidatePlan` (which preview runs before hashing) is pure —
   no assignment to plan/line fields in
   `internal/posting/posting.go` — so both sides hash identical
   bytes. `TestPreviewPostingShowsPlansAndHash` submits with the
   previewed hash; `TestPostCommands` refuses a stale hash and
   accepts the fresh one; `TestSubmitRefusesWhenRulesVanishedAfterPreview`
   covers the reorder.
3. dryRun / no live side effects: `logRun` commits Rule Run rows
   on its own connection (`internal/server/rules_engine.go:248-275`),
   so the preview's rolled-back tx still leaves exactly the
   dry-run rows (`dry_run=1`), and the `!r.dryRun` guard at
   rules_engine.go:266 suppresses SaveStopped notifications.
   Preview otherwise only reads (rules host, ValidatePlan,
   GetAccount) inside the rolled-back tx. Tests assert `l_gl`
   and `athena_posting_groups` stay empty after preview.
4. Gate (`internal/server/gate.go` +4): all four procedures are
   `policySystem` + `ledger:read`, appended at the end of the
   map; `previewable` additionally requires the voucher read
   Grant, so neither permission alone opens the other.
   `TestEveryProcedureHasAPolicy` passes, and
   `TestPostingReadsNeedLedgerReadAndRecordGrant` proves both
   halves (denied without ledger:read, NotFound without the
   record Grant) for all four reads.
5. CLI (`cmd/athena/post.go` new + test + `main.go` wiring):
   preview/submit/cancel/status/log work; submit/cancel require
   `--revision >= 1`; `--preview-hash` plumbs to
   `PostingPreviewHash`. `Kernel.Ledger` (`kernel.go`) is used
   only by the three read subcommands — minimal and justified.
   `athena post` with no args prints usage (`isHelp` covers
   empty args, same pattern as other commands); no panic.
   `log --limit` maps to page size (0 = server default 50).
6. Hash-check reorder (`lifecycle.go:336-350`): compare-before-
   empty-check is correct — an approval now refuses when the
   Rules vanished instead of silently posting nothing, while a
   no-plan preview still submits cleanly (empty hash matches,
   then early return). Tested (see 2).
7. `lifecycle.go` (+11) and `rules_engine.go` (+16) hunks are in
   scope: the reorder, the `dryRun` parameter threading, and
   `namedPlan.version` for the preview response. The `main.go`
   rules-usage touch-up ("posting" added to the Rule type list)
   is a correct one-word fix riding along. No migration, no ADR,
   per the brief.
8. Z2 untouched: the diff has no `internal/posting` files and
   no window/policy/day-count/book defaulting anywhere;
   `ErrBookPolicyOpen` / `ErrCorrectionWindowOpen` sentinels
   intact in `internal/posting/posting.go`.
9. No Desk in this slice: the only web change is
   `web/src/gen/athena/v1/ledger_pb.ts` regen.

## Gates (review worktree, clean 40bcfbe)

- `gofmt -l cmd internal`: empty.
- `go build ./...`: clean. `go vet ./...`: clean.
- `affected`: dedup hit — "already green on 40bcfbe (clean
  tree) by w-c1, log 1123-014741-w-c1.log; not rerun". Log read:
  0 FAILs, all Go packages ok (server, posting, ledger,
  cmd/athena, auth, events, files ran fresh, not cached),
  63 web files / 613 tests passed.
- Narrow re-runs with the suite's DB env in the review
  worktree (allowed `-run` filters): the 8 posting-reads
  server tests `ok`, `TestPostCommands` PASS,
  `TestEveryProcedureHasAPolicy` PASS.
- `make generate` twice: exit 0 both times, no diff
  (ran with node from ~/.nvm on PATH; plain `make generate`
  fails on web-deps here because node is not on the default
  PATH — environment nit, not a code issue).

## Follow-ups (non-blocking, for a later slice)

- `proto/athena/v1/rules.proto:43-44`: stale comment — "Posting
  arrives with the posting engine" has arrived (6.8). Refresh
  the rule-type list comment (worker-noted).
- Plan legs lack voucher-line linkage (worker-noted): lines
  carry source_type/source_id but no per-line voucher link.
  Design question for a later slice, not this one.
- Desk Rules page posting display (worker-noted): slice 3/4
  territory. Confirmed out of scope here.
- Test gap (minor): no test asserts a preview leaves
  `dry_run=1` rule-run rows and no `dry_run=0` rows; books are
  asserted empty but the run-row marking is not. Suggest one
  assertion in `posting_reads_test.go` next time the file is
  touched.
- `post log` has `--limit` but no `--page-token`, so it only
  ever shows the first page. Fine for an operator CLI; note if
  paging is ever needed.
- Observation, not a defect: submit fires lifecycle hooks
  before postVoucher, so a future transform that mutated the
  submit doc would conservatively refuse an older preview hash.
  Correct behavior (the plan did change); just noting the loop
  assumes hooks don't rewrite the doc.
