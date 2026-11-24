# Review brief: C1 6.10 — Desk voucher view (user GO)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c1-r (detached at the
commit in my ORDER; verify clean on arrival). Task brief:
../athena-erp/.agents/tasks/beta-c1-hooks/brief.md (slice 3 only).
Follow .kiro/specs/athena-beta/design.md section 5 reviewer workflow
and AGENTS.md. Design system is the authority (the impeccable /
emilkowalski-motion skills are not installed in this environment).

Review the single-commit diff (git diff main...HEAD — expect
RecordsPage.vue voucher view + PageSwitch one-line canLedger prop
+ tests):

1. Voucher view: Preview posting dialog (plans+legs, submit
   carrying the preview hash per R6.15 — verify the hash binds
   the submitted plan, stale hashes refused); ledger rows +
   Posting_Log on submitted vouchers; Sonner toasts for blocked
   posts (verify blocked paths toast instead of failing
   silently).
2. View vocabulary: "Recorded in the books" badge + plain words
   throughout; glossary words in View mode (no doc./field-key
   jargon visible).
3. Every state: rest, hover, focus-visible, pressed,
   disabled-with-reason, loading, error, empty — for the dialog
   and the entries sheet. Verify in code + shots.
4. Eyeball ALL worker shots (/tmp/c610-pass/, mapping in my
   ORDER message) with the image tool, both themes × 4 widths
   (375/768/1280/1440): each claimed state visible; no layout
   regression; dark coherent. The worker's disclosed gaps
   (light sheet widths, reversed mid-animation shot, 375
   credit tightness) must match what you see — any undisclosed
   breakage is a blocker.
5. Browser evidence: 25/26 with 1 harness fail (drive asserted
   capital Cash, sheet shows lowercase cash — verify from the
   log that this is the ONLY failure and it is harness-side;
   rule whether the lowercase itself violates any copy rule).
6. PageSwitch prop + mock: one-line can-ledger prop; the mock
   bug the worker's own gate caught (missing canLedger) is
   fixed with the mock green — verify the fix is principled,
   not mock-theatre.
7. Confirm: RecordsPage.vue + PageSwitch + tests ONLY (no
   server/proto/migration/gate changes beyond nothing);
   RecordsPage is 6.10-owned (F1 verified disjoint); Z2
   untouched (no posting behavior change — preview/submit go
   through the existing reviewed paths).
8. Gates: typecheck + touched Vitest + vite build (rerun);
   .agents/bin/affected by the ABSOLUTE main-repo path (same
   lock; reuse of the worker's green run on the same commit
   is fine — read that log); make generate twice stable.
   Tree clean before any gate. No full browser re-run
   (item 4–5 evidence + your eyeball is the verdict basis).

Merge-small rule: only blockers stop the merge (hash not
bound, blocked path silent, breakage in shots, jargon in
View, scope breach, red gates); the rest are follow-ups.
Write findings to ../athena-erp/.agents/tasks/beta-c1-hooks/
review-6.10.md with a verdict line `Verdict: PASS` or
`Verdict: NEEDS_CHANGES`, blockers first with file:line.
Then report: say master "C1 6.10 REVIEW: PASS or
NEEDS_CHANGES <one line>". Then wait for instructions. Do
not edit product code, commit, merge, push or start agents.
Send UPDATE every 10 minutes.
