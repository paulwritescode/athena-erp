# Review brief: FU batch 2 — B2a-verify, B5, FU-1, C1 nit, chart fix (interim PASS)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-fu-r (detached at 58a8cf4;
verify clean on arrival). Brief:
../athena-erp/.agents/tasks/beta-fu/brief-batch-2.md. Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and AGENTS.md.
Design authority: .kiro/steering/athena-design.md + design-system.md.

This PASS was reported under the Stylus interim rule (browser harness
failures, zero app failures claimed). Your job is to rule whether that
claim holds. The branch is 9 commits on main (77971a2 → 58a8cf4).

1. Evidence chain: full `make test` exit 0 ran on 4a7d26b (pre-rebase
   tip, NOT an ancestor of HEAD). Verify content equivalence: diff
   (4a7d26b's base..4a7d26b) vs (main..58a8cf4) must show the same
   9-fix content modulo the rebase (main moved with C4s3/C5s1/
   train3/C4s4 since). The tip commit 58a8cf4 (one template attr)
   is covered by typecheck + live browser asserts only — judge
   whether that suffices for a one-word template change.
2. THE key question: three browser runs (exit 2) failed with
   "TIMEOUT waiting for invoice editor: .field-row" (save lands,
   editor renders no rows). Rule harness-vs-real: run the SAME
   editor flow on CLEAN MAIN in your own throwaway check — if
   main fails identically, it is environmental/pre-existing, not
   batch-2. Also consider batch-1's TextField Enter-emit change
   (shipped in train2) as a suspect: does the editor render rows
   on train2 (2d9c17d) without batch 2? If EITHER control shows
   this is a REAL batch-2 (or batch-1) regression → NEEDS_CHANGES
   with the culprit commit. If it is load/flake (tonight: 5x
   slowdowns, OOM kills) → PASS with a follow-up.
3. Eyeball all 3 shots (/tmp/fu2-settings-light.png,
   /tmp/fu2-custom-light.png, /tmp/fu2-charts-light.png, fresh
   03:33) with the image-reading tool: each claimed fix visible;
   no regression in frame. Note the missing records-flow shots
   (blocked by the editor issue) — acceptable only if (2) clears.
4. One fix per commit (B5 x6, FU-1, C1 nit, chart :value):
   verify each does what it claims against the design system.
5. B2a verify-only claim: "all six already fixed by b4791a3,
   code-verified" — verify in the tree (each of the six items).
   Skips: --chart-palette flag, B3/Security-RPC/B9f1/B2a-s2-3/
   B8/B10/B11/C-lane items, operations.go:21 comment — confirm
   none half-built.
6. Scope: no proto/gate/migration, no RecordTypesPage/
   FieldSettingsDialog/fieldRules.ts, no App.vue, RecordsPage
   untouched (no GO was asked — verify), api.ts untouched.
7. Gates: typecheck + touched Vitest + vite build (rerun the
   ones covering the tip commit); .agents/bin/affected by the
   ABSOLUTE main-repo path (same lock; reuse of any green run
   on the same commit is fine — read that log). No full
   browser re-run (evidence per items 1–3 is the verdict).

Merge-small rule: only blockers stop the merge (unverified
evidence chain, real editor regression, fix not as claimed,
scope breach, red gates); the rest are follow-ups. Write
findings to ../athena-erp/.agents/tasks/beta-fu/review-batch-2.md
with a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`,
blockers first with file:line. Then report: say master "FU
BATCH 2 REVIEW: PASS or NEEDS_CHANGES <one line>". Then wait
for instructions. Do not edit product code, commit, merge,
push or start agents. Send UPDATE every 10 minutes.
