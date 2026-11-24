# Review brief: C4 slice 13.3b — ledger saves recompute tree paths

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c4-r (detached at 3175005;
verify clean on arrival). Brief:
../athena-erp/.agents/tasks/beta-c4/brief.md (slice 3b only). Stylus
decision: ADR-0029 line 27 ("ledger saves recompute path and depth")
had no implementing slice; this slice adds it (no migration, ADR
untouched). Follow .kiro/specs/athena-beta/design.md section 5
reviewer workflow and AGENTS.md. Design: batch-2 design.md G13;
ADR-0029.

Review the diff (git diff main...HEAD — 4 files: accounts.go,
masters.go, NEW accounts_test.go, NEW masters_test.go):

1. Recompute: every ledger save path (accounts, branches,
   departments, cost centres) recomputes tree_path + tree_depth
   for the saved row AND its descendants inside the save
   transaction; path format matches ADR-0029 (/root/child/,
   root depth 0) and slice-2 semantics.
2. Parent change: reparenting rewrites the whole moved subtree;
   descendants' depths shift consistently; no row left with a
   stale path (the stale-path regression test must prove this:
   reparent, then read every descendant's path from storage).
3. Refusals: cycles and dangling parents refused exactly as
   today (same errors, same sites — verify no behavior change
   beyond the recompute).
4. Tests (accounts_test.go + masters_test.go, NEW files):
   create, reparent, subtree depth, stale-path regression;
   rerun; judge coverage. Confirm no test went into
   ledger_test.go/ledgers_test.go (slice-4 separation).
5. Confirm: no migration (columns from 18), no proto/gate/web/
   cmd changes, no quantity/determination/UoM changes (slice 3
   untouched), Z2/Z4 untouched.
6. Gates: gofmt -l cmd internal (empty), go build ./..., go vet
   ./..., .agents/bin/affected by the ABSOLUTE main-repo path
   (same lock; reuse of the worker's green run on 3175005 is
   fine — read that log), make generate twice with no diff.
   Tree clean before any gate. No browser pass (no Desk).

Merge-small rule: only blockers stop the merge (stale paths
after save/reparent, recompute outside the tx, changed
refusal behavior, red gates); the rest are follow-ups. Write
findings to ../athena-erp/.agents/tasks/beta-c4/review-13.3b.md
with a verdict line `Verdict: PASS` or `Verdict:
NEEDS_CHANGES`, blockers first with file:line. Then report:
say master "C4 13.3b REVIEW: PASS or NEEDS_CHANGES <one
line>". Then wait for instructions. Do not edit product
code, commit, merge, push or start agents. Send UPDATE every
10 minutes.
