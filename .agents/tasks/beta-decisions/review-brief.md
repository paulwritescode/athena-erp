# Review brief: night-shift decisions docs (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-dec-r, detached at 7d7b93a (1 docs commit on a6dec5c). Verify clean + `git show --stat HEAD` is docs-only (7 files: tasks.md, decision-log, ADR-0023, LICENSE, NOTICE, README, AGENTS.md — no source, no ADR-0032/0033 files).
Authority: .agents/stylus/night-shift-orders.md (the 12 decisions Z1–Z7/Z10/Z11/Z13/B7/B10) + .agents/tasks/beta-decisions/brief.md.

Review (design.md section 5 adapted for docs; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Every Z row in tasks.md section Z transcribes its orders decision EXACTLY (no softened numbers/dates/scope); B7/B10/C3/E3 notes match.
2. Decision-log: 12 rows present + O-009/O-011/O-012 + ADR-0028/0031 note updates correct against the orders.
3. ADR-0023 amendment: O-011/O-012 text matches Z2 (soft-close 10th, hard-close after VAT return, reopen rules, current-period reversals, one KES book + named books); ADR stays Proposed.
4. LICENSE reads as complete Apache-2.0 (all 9 sections + appendix present, no holder filled in); NOTICE is exactly the one holder line 'Copyright 2026 Athena contributors' + the required Apache notice block; README licence section + AGENTS.md name match the brief.
5. No gate needed (docs-only) but run `make generate`-stable sanity? NO — docs-only, skip gates; instead re-read the full diff once for stray edits (worker notes section-Z header left OPEN per brief scope — confirm that is the only intentional leftover).
6. Spend remaining effort on transcription fidelity (item 1 is the crux: a wrong number here becomes law).

Report: say master "DECISIONS REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-decisions/review.md with the verdict line.
