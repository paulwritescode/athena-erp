# Review brief: C7 slice 1 — migration 21 + ADR-0032 (11.1)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c7-r (detached at 2625faf;
verify clean on arrival). Brief:
../athena-erp/.agents/tasks/beta-c7/brief.md (slice 1 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and AGENTS.md.
Design: batch-2 design.md G11 + Data Models row `stock_ledger`;
Requirement 11; Z3 decided (weighted-average, FEFO, rentals
tracked-not-consumed, no negative stock).

Review the diff (git diff main...HEAD — 5 files: sysmigrations.go,
sysmigrations_test.go, ADR-0032 new, decision-log + README rows):

1. Migration 21 `stock_ledger`: creates `l_stock`, layers, item
   applications, bins, reservations, serials, batches, integrity
   log per the design row (column names/types/keys match).
   Number 21 matches the reservation; migration applies cleanly
   up on a throwaway database (rerun the migration tests).
2. ADR-0032 (Proposed): tables, append-only/cost-method rules,
   Z3 decisions transcribed EXACTLY as decided (perpetual
   weighted-average all stock, FEFO picking, rentals tracked-
   not-consumed, negative refused); nothing seeded; no invented
   policy; honest about what slices 2+ still build.
3. Decision-log + README rows point at ADR-0032 with intact
   links; no stale references.
4. Evidence chain (IMPORTANT): worker's full `make test` green
   was 100% Go-cache (14s); the substantive evidence is the
   UNCACHED `go test -count=1 ./internal/server` exit 0 on
   2625faf (267.8s, zero FAILs). Verify that log exists and is
   clean; rerun the migration tests + the server package
   narrowly yourself if anything smells (through suite if
   unfiltered, same lock).
5. Confirm: no other tables/columns, no proto/gate/web/cmd
   changes, Z2 untouched, Z4 untouched (no secrets/retention
   code — that is ADR-0033/C2 work).
6. Gates: gofmt -l cmd internal (empty), go build ./..., go vet
   ./..., make generate twice with no diff. Tree clean before
   any gate. For the suite gate: reuse is INVALID here (nothing
   green ran on this exact content except the worker's logs you
   already verified in item 4 — say what you ran vs reused).
   No browser pass (no Desk).

Merge-small rule: only blockers stop the merge (wrong
tables/columns, migration that does not apply cleanly,
ADR mis-stating Z3, red gates, unproven migration);
the rest are follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-c7/review-slice-1.md with
a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`,
blockers first with file:line. Then report: say master
"C7 SLICE 1 REVIEW: PASS or NEEDS_CHANGES <one line>".
Then wait for instructions. Do not edit product code,
commit, merge, push or start agents. Send UPDATE every 10
minutes.
