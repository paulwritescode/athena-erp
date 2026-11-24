# Summary: F3 slice 2 + fix (compact builder per condition)

Merged 2026-11-24 as 24bd35d + d5073e7 (2 commits).

Scope: ConditionBuilder embeds with per-condition Builder/Written toggle for
required/read-only/must-be-true/show-when in the field settings sheet; live
compile via the TS mirror into the stored expression; blank/unfinished builder
stores nothing; typed text stashed so peeks are lossless. Formula stays
written-only BY DESIGN (builder emits booleans incl. today(), which formulas
forbid; totals picker covers formulas). F4 files read-only. No docs touched.

Review: r-f3s2 NEEDS_CHANGES (1 real bug: Builder peek deleted the stored
expression via mirrorCondition on empty-builder mirror) → fix (empty builder
clears only when nothing written is kept + close-mid-peek regression test
proven fail-pre-fix) → re-review PASS (74/724 + 66oks/0err + 6/6 shots).
Merge gate: full `make test` exit 0 clean, 723s, 21/21 Go pkgs ok + 724 web
(log 1124-235605-master.log). Picks rebased onto post-C10s2 main (clean,
content-identical, zero shared files); build+vet green on the merged tip.

F3 COMPLETE (both slices merged). L4: w-f2 holding; F5 prep is the next
user-facing item (unbriefed).
