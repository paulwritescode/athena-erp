# Review brief: D3 slice 2 RE-CONFIRM after rebase (then slice 3)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-d3-r (detached; you start at
pre-rebase 6f2ad1b for READING ONLY — do not gate it). Brief:
../athena-erp/.agents/tasks/beta-d3/brief.md (slice 2 only for now).
Round-1 verdict: ../athena-erp/.agents/tasks/beta-d3/review-slice-2.md
(PASS at 10ce8cb). Contract: ADR-0030. Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and AGENTS.md.

PRE-START (now, while the worker rebases and gates): read the brief slice 2
and round-1 review-slice-2.md in full. Reads only — no suite-lock gates on
this stale commit.

ON MY ORDER (I will send the final rebased SLICE-2 commit): move your tree
to it (git checkout --detach <commit>), verify clean, then:

1. Content check: diff the slice-2 tree against reviewed 10ce8cb for the 7
   slice-2 files (public.go, server.go wiring, 3 templates, public_test.go,
   ADR-0030) — expect rebase-only differences; any content change gets a
   full re-review of that hunk against the brief.
2. Re-gate: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   .agents/bin/affected by the ABSOLUTE main-repo path
   /Users/iampaulking/projects/duchess/athena-erp/.agents/bin/affected (same
   lock; reuse of the worker's green run on the same commit is fine — read
   that log), make generate twice with no diff. Tree clean before any gate.
3. Spot render check only if (1) found content changes: serve a throwaway
   Workspace from this tree's binary on a free loopback port, read /home raw
   bytes (title, canonical, JSON-LD, noindex absence).
4. Confirm: still no migration, no .proto, no web//cmd/ changes, Z2/Z4
   untouched.

Write findings to
../athena-erp/.agents/tasks/beta-d3/review-slice-2-reconfirm.md with a
verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first
with file:line. Then report: say master "D3 SLICE 2 RE-CONFIRM: PASS or
NEEDS_CHANGES <one line>". Then WAIT — slice-3 review follows in this same
tree (do not remove it). Do not edit product code, commit, merge, push or
start agents. Send UPDATE every 10 minutes.
