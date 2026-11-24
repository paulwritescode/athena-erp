# Review brief: C5 slice 1 — migration 20 + ADR-0031 (7.1)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c5-r (detached at f466742;
verify clean on arrival). Brief:
../athena-erp/.agents/tasks/beta-c5/brief.md (slice 1 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and AGENTS.md.
Design: batch-2 design.md G7 + Data Models row `party_ledger`;
Requirement 7 criteria 1–2.

Review the diff (git diff main...HEAD — 5 files: sysmigrations.go,
sysmigrations_test.go, ADR-0031 new, decision-log + README rows):

1. Migration 20 `party_ledger`: creates `l_party`,
   `athena_party_items` (open items per instalment, instalment
   currency), `athena_settlement_applications` (append-only,
   both endpoints, both currencies, base amount, reversal
   link); unapplied cash separate per criterion 7. Number 20
   matches the reservation; migration applies cleanly up and
   verifies on a throwaway database (the sysmigrations test
   covers it — rerun).
2. ADR-0031 (Proposed): tables, append-only rule, endpoint
   semantics, ceiling/lock strategy preview honest about
   what slices 2+ still build; nothing seeded; no invented
   policy.
3. Decision-log + README rows point at ADR-0031 with intact
   links; no stale references.
4. Confirm: no other tables/columns, no proto/gate/web/cmd
   changes, Z2 untouched (sentinels intact, no window/policy
   invented), no other Z touched.
5. Gates: gofmt -l cmd internal (empty), go build ./..., go
   vet ./..., .agents/bin/affected by the ABSOLUTE main-repo
   path (same lock; migration changed so expect the full
   fallback — reuse of the worker's green run on f466742 is
   fine, read that log), make generate twice with no diff.
   Tree clean before any gate. No browser pass (no Desk).

Merge-small rule: only blockers stop the merge (wrong
tables/columns, migration that does not apply cleanly,
ADR promising what the slices do not build, red gates);
the rest are follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-c5/review-slice-1.md with
a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`,
blockers first with file:line. Then report: say master
"C5 SLICE 1 REVIEW: PASS or NEEDS_CHANGES <one line>".
Then wait for instructions. Do not edit product code,
commit, merge, push or start agents. Send UPDATE every 10
minutes.
