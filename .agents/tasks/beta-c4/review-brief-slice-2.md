# Review brief: C4 slice 2 — tree + effective-date attributes (13.2)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c4-r (detached; you start at
b3940a3 for READING ONLY — do not gate it until my ORDER, the worker may
still amend). Brief: ../athena-erp/.agents/tasks/beta-c4/brief.md (slice 2
only). Follow .kiro/specs/athena-beta/design.md section 5 reviewer workflow
and AGENTS.md. Design: batch-2 design.md G13; requirements R13;
internal/ledger determination + masters; ADR-0029.

PRE-START (now, while w-c4 gates): read the brief slice 2 and the 12-file
diff (git diff main...HEAD: record_types.proto + records.proto and generated
Go/TS, attributes.go, records.go, record_types.go, migrations.go, store.go,
trees_test.go). Reads only — no suite-lock gates on this commit.

ON MY ORDER (I will send the final commit): move your tree to it
(git checkout --detach <commit>), verify clean, then:

1. Attributes: tree path/depth/cycle checks + effective-date range checks in
   attributes.go/records.go/record_types.go/migrations.go/store.go match
   design G13 and R13; boundary semantics (open/closed ends, unbounded)
   consistent everywhere including the proto comments.
2. Proto: tree + effective-date fields on the right messages; NOTHING else
   changed in the .proto files; generated Go/TS match (make generate twice
   with no diff).
3. Migration path: existing Record Types/Records unaffected (no silent
   backfill, no dropped data); cycle rejection cannot brick saved trees.
4. Tests: trees_test.go covers path/depth/cycle/range + property-style cases
   per the brief; rerun; judge coverage.
5. Confirm: no gate.go change needed (no new procedures?) — if the slice
   adds procedures without policies, that is a blocker; no hand-written
   web/ or cmd/ changes; migration 18 untouched (slice 1 merged); Z2/Z4
   untouched.
6. Gates: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   .agents/bin/affected by the ABSOLUTE main-repo path
   /Users/iampaulking/projects/duchess/athena-erp/.agents/bin/affected (same
   lock; protos changed so expect the full fallback — reuse of the worker's
   green run on the same commit is fine, read that log). No browser pass
   (no Desk in this slice).

Merge-small rule: only blockers stop the merge (wrong tree/range semantics,
proto/generated drift, data-loss migration path, red gates); the rest are
follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-c4/review-slice-2.md with a verdict line
`Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first with file:line.
Then report: say master "C4 SLICE 2 REVIEW: PASS or NEEDS_CHANGES <one
line>". Then wait for instructions. Do not edit product code, commit, merge,
push or start agents. Send UPDATE every 10 minutes.
