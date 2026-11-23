# Review brief: C4 slice 3 — UoM families + determination effective ranges (13.3)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c4-r (detached at 8df6b0c;
verify clean on arrival). Brief:
../athena-erp/.agents/tasks/beta-c4/brief.md (slice 3 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and AGENTS.md.
Design: batch-2 design.md G13; requirements R13; ADR-0029.

Review the slice-3 diff (git diff main...HEAD — 8 files: new
internal/ledger/uom.go + uom_test.go, determination.go + test, quantity.go +
types_test, ledger.go server/CLI caller tweaks):

1. UoM: families, factors, overrides, cross-family rejection in
   uom.go/quantity.go match G13/R13; conversion math exact (no float drift
   where the design demands exactness); unknown-unit and cross-family errors
   typed, not strings.
2. Determination: effective-range lookup honors as-of; the required-as-of
   contract is total (no caller can skip it silently); server/CLI callers
   resolving "today" is the briefed behavior — verify it against G13.
3. Boundary semantics consistent with slice 2 (open/closed ends, unbounded);
   quantity round-trips (a Property-25 preview, not the full property —
   slice 4 owns that).
4. Tests: uom_test.go + determination_test.go + types_test.go cover
   factors/overrides/rejection/ranges; rerun; judge coverage.
5. Confirm: no proto changes (none in diff — verify), no gate.go, no
   migration (uses migration-18 tables), no web//cmd-logic changes beyond
   the as-of caller tweak, Z2/Z4 untouched. Slice-2 files untouched except
   where justified.
6. Gates: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   .agents/bin/affected by the ABSOLUTE main-repo path
   /Users/iampaulking/projects/duchess/athena-erp/.agents/bin/affected (same
   lock; reuse of the worker's green run on 8df6b0c is fine — read that
   log), make generate twice with no diff. Tree clean before any gate. No
   browser pass (no Desk in this slice).

Merge-small rule: only blockers stop the merge (wrong UoM math, range
semantics contradicting slice 2, as-of escape hatch, red gates); the rest
are follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-c4/review-slice-3.md with a verdict line
`Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first with file:line.
Then report: say master "C4 SLICE 3 REVIEW: PASS or NEEDS_CHANGES <one
line>". Then wait for instructions. Do not edit product code, commit, merge,
push or start agents. Send UPDATE every 10 minutes.
