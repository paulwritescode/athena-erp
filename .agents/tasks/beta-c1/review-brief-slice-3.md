# Review brief: C1 slice 3, reversal mirrors + correction-period rules (6.6+6.7)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c1, branch
feat/c1-posting (commit 5bbc94e). Brief:
../athena-erp/.agents/tasks/beta-c1/brief.md (slice 3 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and
AGENTS.md.

Review the slice-3 diff (git diff 3dece91...HEAD — 4 files, all in
internal/posting/):

1. `reverse.go` (new) + `write.go` refactor: Reverse unwinds a posted
   submit group by mirroring stored rows with swapped sides under the
   cancel event, flagged is_reversal with the reversed row identity.
   The mirror inherits the submit's validity (company/accounts/
   dimensions/control sources unchecked) so an unwind succeeds after
   master changes; only the reversal date's period must be open.
   Check the mirror-exactness claim against the code.
2. Replays/conflicts/post-once-per-cancel shared with the slice-2
   operation machinery; a retry replays even after the period closes
   — verify that path is tested, not just claimed.
3. Z2: hard-closed reversal period returns ErrCorrectionWindowOpen
   (failed_precondition); no correction target guessed. Confirm no
   window/policy invented and the sentinel is pinned.
4. Property 10 (submit+cancel net to zero with exact mirrors):
   rerun the property plus the 6.6 unit tests. The Go suite is slow
   (use -timeout 20m; default 10m can kill posting tests — a timeout
   alone is not a logic failure, but confirm green at 20m).
5. Run: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   make test with MariaDB enabled (throwaway test_athena_* only,
   never the dev database), make generate twice with no diff. If
   node is missing on PATH, source nvm first; do not skip checks.
   No browser pass (no Desk in this slice).

Merge-small rule: only blockers stop the merge (wrong mirror
semantics, broken replay sharing, invented Z2 policy, red suite);
the rest are follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-c1/review-slice-3.md with a verdict
line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first with
file:line. Then report: hc send master "C1 SLICE 3 REVIEW: PASS or
NEEDS_CHANGES <one line>". Do not edit product code, commit, merge,
push or start agents. Permission policy: stay inside athena-erp and
athena-erp-* folders, nothing destructive.
