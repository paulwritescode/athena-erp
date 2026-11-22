# Summary: C1 slice 5 — posting RPCs + CLI (6.9)

Merged 2026-11-23 as cf2f674 (built at 40bcfbe on 64bb3d2,
rebased cleanly onto 1271dc7; patch byte-identical).

- `proto/athena/v1/ledger.proto` (+149): PreviewPosting,
  GetPostingStatus, ListPostingLog, ListLedgerRows; the two
  unbounded lists keyset-paged; regen clean.
- Preview-hash loop closed (R6.15): PreviewPosting builds plans
  at revision+1 exactly as submit does; both sides hash `bare`
  plans with HashPlans; ValidatePlan pure so bytes match.
  Stale hashes refused; no-plan previews submit cleanly.
- dryRun: preview rolls back its tx; Rule Run rows land with
  `dry_run=1` (own connection), SaveStopped notifications
  suppressed; `l_gl` + posting groups stay empty (asserted).
- Gate: all four procedures `policySystem` + `ledger:read`,
  appended at map end; preview additionally needs the voucher
  read Grant; TestEveryProcedureHasAPolicy + both-halves test.
- CLI: `athena post preview|submit|cancel|status|log`
  (+ tests, main.go wiring); Kernel.Ledger minimal.
- Hash-check reorder: approval refuses when Rules vanished
  instead of silently posting nothing (tested).
- Z2 untouched (no posting-package files in the diff).
- Review: PASS, no blockers (review-slice-2.md); reviewer
  ran gates in a detached -r worktree, reused the worker's
  green run via dedup + narrow reruns.
- Merge-time verification on cf2f674 (identical patch): full
  make test exit 0. Merged the slice-2 commit only; 6.11 docs
  stay stacked on the branch.
- Follow-ups: stale rules.proto comment; plan legs lack
  voucher-line linkage; Desk Rules page; dry_run-row assertion
  gap; `post log` first-page-only; hooks-don't-rewrite-doc
  assumption noted.
