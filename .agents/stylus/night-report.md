# Night report — FINAL (clocked out 07:58)

## Summary

Eighteen merges in main and pushed to origin: B4b, B9b, B2a (complete),
B5 (complete), B9 (complete), B10 (complete), B8 (complete), B11 slice 1,
C1 slices 1–2, C2 slices 1–2. Three items finished review but did NOT
merge (see Pending). B7 verified 5/6 with one documented gap awaiting
the user's call. The :3000 server was left running, fresh on the
C2-2-complete main (722e183).

## Merged tonight

- B4b Appearance redesign (1568c0b + fix fb603a6). Try: Settings → Appearance → Blue + Ocean → Save → reload.
- B9b name-first flow (2468598). Try: Build → Record Types → New Record Type → "Vendor".
- B2a complete (slices 1–3: afc70a6, b4791a3+96edd68, f3c8c1b). Try: every Desk list.
- B5 complete (slice 1: 2e5595e + SMIL fix e80ad39 + union 4698f96; slice 2: bd49f75 + fix c2674c2). Try: Settings → Workspace identity.
- B9 complete (server 9260d04; Desk 99cc6ed, 4113b23, 7c7192d): layouts, renderer, builder, list thumbnails.
- B10 complete (slices 1–2: 82cf024, cf95819): art registry + placements.
- B8 complete (slice 1: 4d03467; slice 2: 48d804d + test fix d81473f): root guards + presentation. Try: sign in as root, mode switch, Preview-as.
- B11 slice 1 (b27b6c3): CLI install path + e2e-install.sh 28/28 + guide.
- C1 slice 1 (9d87fbf+a9e1d49): posting validation + writer + balanced-postings property.
- C1 slice 2 (f94fe09+367a56c): op-id contract + idempotent replay; one correctness fix (cross-actor winner by group id) re-reviewed PASS.
- C2 slice 1 (5d1e55f): migration 17 (events/jobs/workflows tables) + ADR-0028; Z4 secrets/lifetimes deferred.
- C2 slice 2 (6be771e): event bus (Emit/Claim/Declare/Match + dispatcher) + posting/lifecycle emit hooks + Property 13 inbox dedup. Z4/Z2 clean.

## Pending (left on branches — reviews PASS, merge-time suites red)

- C1 slice 3 (reversal mirrors + Property 10): review PASS at 5bbc94e;
  rebased to 981ee78 on feat/c1-posting (3 posting-file conflicts,
  unified insertOutbox signature over events.Emit). Merge-time
  `make test` EXIT 1. Left on branch per clock-out rule.
- B11 slice 2 (Setup wizard + readiness): re-review PASS at bdff521
  (B1 wedge fix verified in browser); rebased to be91324+5fbf685 on
  feat/b11-install (clean). Merge-time `make test` EXIT 1. Left on
  branch per clock-out rule.
- Both suites ran in parallel near the 20m budget the C2-2 review
  proved tight (auth/posting/server need 4–6 min each); both branches
  were green at review time. Leading hypothesis: contention timeouts,
  not logic. Morning: re-run each suite QUIETLY (one at a time); if
  green, merge --ff-only + records as usual. If C1-3 fails again in
  isolation, triage the insertOutbox unification first (C1-3's tests
  now run against the Emit path, not the direct INSERT they were
  written against).
- C2 slice 3 (job runner + leases + occurrences + Property 15,
  aec5366 on feat/c2-events-jobs): COMMITTED by the worker at 07:46
  but unreviewed, no PASS report (worker was mid-report-suite at
  clock-out; stop orders queued, no ack). Morning: confirm w-c2 idle,
  then review aec5366 normally before merging.

## Morning resume checklist

1. Confirm w-c2 idle (it may have finished its report suite overnight).
2. Quiet `make test` on feat/c1-posting (981ee78) → merge + records.
3. Quiet `make test` on feat/b11-install (5fbf685) → merge + records
   (rebase onto post-C1-3 main first; expect clean, web-only).
4. Review C2 slice 3 (aec5366) → merge.
5. Continue: C1 slice 4, B4a, C-group continuations, D1/D2 drafts.
6. Refresh the :3000 server after the queued merges land.

## Questions for the user

- B7's "a Workspace switch clears caches" has no trigger because the Desk has no Workspace switch (single-workspace servers). Recommended: descope that bullet until a switch flow is designed, and track it as a follow-up under B8 or B11. (Stylus agrees; B7 otherwise verified 5/6 and stays unticked until you decide.)
- B10 shipped with stock hairline figures; custom Athena figures need the `hairline-create` skill, which is not installed and cannot be fetched overnight (no-network policy). Recommended: install the skill in the morning and let a worker generate the custom set, or accept stock figures.
- Section Z decisions untouched.

## Errors and how they were handled

- Server panes t24 (node PATH) and t25 (install lock) failed at startup; all later exits were planned refresh kills.
- r-b4b's lock stall: fake-HOME nudge; r-b9b's hung script: restarted; both PASS.
- Fix loops, all re-reviewed PASS: B4b ramp/charts; B5-1 SMIL; B5-2 typecheck/CSP; B2a-2 stale delete targets (two data-loss bugs); B8-2 jsdom observer stub; C1-2 cross-actor op-id LIMIT 1; B11-2 stale years list.
- B5 slice-1 merge conflicts (5 files): union + regen + one test-precedence update, suite green.
- B9-form rebase import conflict: kept B2a's list + new image import, zero dangling refs.
- The B8-2 merge blocked once on a rebase-only test failure (B10 lazy art vs jsdom remount timing); bisected to B10-2's files, worker fixed with a test-only observer stub, re-reviewed PASS.
- A `rebase --continue` hung on an editor prompt; killed and retried with `GIT_EDITOR=true` (now standard).
- A B2a-2 worker nudged with a 15-minute timebox during a long browser pass (it then found, fixed, and reported).
- B2a-3 scope fork (no notifications page): Stylus chose skip + follow-up; recorded in tasks.md.
- B8/B11 commit messages reworded to Conventional style at merge (content identical).
- A stale redelivered review message recognized and ignored (B9 already complete).
- Early log timestamps were estimates; corrected 00:09, clock-stamped since.
- r-c1 ran B11-2 gates in the C1 tree with hung servers (nothing listening on :3222/:3223); redirected twice, recovered onto b11-tree binaries with healthz proof, and its review file carries explicit b11-tree provenance.
- Clock-out: both merge-time suites (C1-3, B11-2) went red running in parallel; per the clock-out rule both branches were left intact for quiet morning re-runs. w-c2 committed slice 3 instead of stopping uncommitted — safer than ordered, kept as is.

## Observations

- OTEL export errors (:4318 refused): endpoint set, no collector. Server healthy; changed nothing.
- Leftover `test_b9b_review` DB kept per policy (no `test_athena_` prefix); merged `b9-server` worktree still dirty (untracked testdata).
- Suite wall-time is now the binding constraint: three packages need 4–6 min each of the 20m budget. Never run two full suites at once again; before E2, raise the budget or parallelize packages.
