# Review brief: C2 secrets slice 4 lifetimes + disposal (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c2-s4-r, detached at 75bb9df (on 3cc6022). Verify clean + diff is slice-4 scope only: lifetime carries in internal/server/attributes.go + record_types.go (append-only) + new internal/jobs/disposal.go + tests.
Read: .agents/tasks/beta-c2/brief-secrets.md slice 4 (8.10) + Z4 RULE + ADR-0033.

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Lifetimes: attributes carried per design (flat JSON, validated, preserved across saves) — verify round-trip with a test or live run; server edits genuinely append-only (no behavior change to existing paths).
2. Disposal: preview/apply/holds/restore-reapply all real (exercise preview + a hold + one apply path); Z4 schedule honored exactly (7yr ledger/invoice/stock/audit NEVER auto-deleted in beta; Access Log 1yr; outbox/events 30d; job runs 90d).
3. Guest anonymisation gap: worker flags it honestly pending field markers (L3/G18). Confirm the code does not pretend to anonymise (no silent no-op presented as done) and the gap is the markers only.
4. No secret material: refs only in jobs/disposal paths; nothing observable in logs/spans/errors (grep + read one error path).
5. Gates: affected green (reuse-or-rerun cmd/athena + jobs + secrets + server through suite on 75bb9df; worker carried f5cbe4d green under the disjoint rule — verify the carry claim); gofmt/build/vet clean; generate stable (no proto expected).
6. Scope: the files above ONLY (no other lane's files, no proto/gate/CLI/Desk — fail if any).

Report: say master "C2 SECRETS SLICE 4 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c2/review-secrets-slice-4.md with the verdict line.
