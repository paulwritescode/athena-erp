# Review brief: C4 slice 4 — Property 25 (13.4)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c4-r (detached at f615096;
verify clean on arrival). Brief:
../athena-erp/.agents/tasks/beta-c4/brief.md (slice 4 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and AGENTS.md.
Design: batch-2 design.md G13; R13.2/R13.4/R13.6; ADR-0029.

Review ONLY the slice-4 delta (git diff 3175005...HEAD — 13.3b below
it merges separately; expect NEW internal/ledger/ledgers_test.go +
append to internal/types/types_test.go, tests only):

1. Property 25 (effective ranges + UoM round trip): period
   range-reject + as-of cases, DB-backed UoM round trip, pure
   Convert/FromBase bound + exactness. Properties must be REAL
   properties (randomized/property-style with meaningful
   generators + shrinking or documented enumeration, NOT a
   handful of hardcoded examples wearing property names).
2. Two-leg bound: slice-3 review follow-up 4 asked Property 25
   to pin the exact ConvertQuantity two-leg bound — verify it
   does (or file it as a blocker if the briefed bound is
   missing and the tests cover clean values only).
3. Effective-range cases must agree with slice-2 semantics
   (end-exclusive, NULL-open, touching bounds pass).
4. Tests rerun green; judge strength (would these catch an
   off-by-one in the range predicate? a rounding change? a
   cross-family leak?).
5. Confirm: tests ONLY (no non-test file in the delta);
   types_test.go append-at-end only (shared with L1 —
   verify no other hunk); no proto/gate/migration/web/cmd
   changes; Z2/Z4 untouched.
6. Gates: gofmt -l cmd internal (empty), go build ./..., go vet
   ./..., .agents/bin/affected by the ABSOLUTE main-repo path
   (same lock; reuse of the worker's green run on f615096 is
   fine — read that log), make generate twice with no diff.
   Tree clean before any gate. No browser pass (no Desk).

Merge-small rule: only blockers stop the merge (fake
properties, missing two-leg bound, non-test changes in the
delta, red gates); the rest are follow-ups. Write findings
to ../athena-erp/.agents/tasks/beta-c4/review-slice-4.md
with a verdict line `Verdict: PASS` or `Verdict:
NEEDS_CHANGES`, blockers first with file:line. Then report:
say master "C4 SLICE 4 REVIEW: PASS or NEEDS_CHANGES <one
line>". Then wait for instructions. Do not edit product
code, commit, merge, push or start agents. Send UPDATE every
10 minutes.
