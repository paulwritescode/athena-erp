# Review brief: C1 6.11 — G6.8–6.9 docs (docs-only)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c1-r (detached; you start at
pre-rebase edf94a1 for READING ONLY — do not gate it). Task brief:
../athena-erp/.agents/tasks/beta-c1-hooks/brief.md (slice 6.11+6.12 docs
part). Follow .kiro/specs/athena-beta/design.md section 5 reviewer workflow
and AGENTS.md.

PRE-START (now, while the worker rebases and gates): read the task brief and
the 4-file docs diff (AGENTS.md, README.md, docs/architecture/overview.md,
docs/decisions/decision-log.md) in your tree. Reads only.

ON MY ORDER (I will send the final rebased commit): move your tree to it
(git checkout --detach <commit>), verify clean, then:

1. Accuracy: every claim about 6.8 (lifecycle hooks, posting Rule type,
   activation gate) and 6.9 (preview/status/log RPCs, athena post CLI)
   matches the merged code on main; command names, RPC names and file paths
   correct.
2. Posting Rules are marked not production-ready until accepted; Z2 (window
   and book policy) stays open with no invented default; no other Z decision
   touched or claimed.
3. Docs conventions: decision-log rows point at the right ADRs (ADR-0016,
   ADR-0023); no stale references; overview README AGENTS.md consistent.
4. Gates: git status clean; .agents/bin/affected by the ABSOLUTE main-repo
   path /Users/iampaulking/projects/duchess/athena-erp/.agents/bin/affected
   (docs-only: expect a trivial plan — record what it ran). No web gates
   (no web/ changes).

Merge-small rule: only blockers stop the merge (false claims about merged
behavior, wrong commands/paths, Z2 overreach); wording nits are follow-ups.
Write findings to ../athena-erp/.agents/tasks/beta-c1-hooks/review-6.11.md
with a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers
first with file:line. Then report: say master "C1 6.11 REVIEW: PASS or
NEEDS_CHANGES <one line>". Then wait for instructions. Do not edit product
code, commit, merge, push or start agents. Send UPDATE every 10 minutes.
