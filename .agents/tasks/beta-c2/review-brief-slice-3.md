# Review brief: C2 slice 3, job runner + occurrences + worker startup (8.4+8.5)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c2, branch
feat/c2-events-jobs (commit 3a9a3a1). Brief:
../athena-erp/.agents/tasks/beta-c2/brief.md (slice 3 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and
AGENTS.md. Design reference: .kiro/specs/athena-batch-2/design.md G8;
requirements R8.4/R8.5; ADR-0028.

Review the slice-3 diff (git diff 0ac599e...HEAD — 11 files, all Go):

1. `internal/jobs/` (new): jobs.go (enqueue, leased claims with
   expiry + fencing, backoff, owner assignment on exhaustion,
   restart recovery), occurrences.go (occurrence keys, timezone,
   catch-up policies), runner.go, jobs_test.go. Lease/fencing/
   backoff/occurrence semantics must match migration 17 + R8.4/R8.5.
2. Server startup: `internal/server/workers.go` (job drain,
   scheduled purge), `server.go` wiring, workers_test.go. Workers
   must not start in tests that do not ask for them; shutdown must
   be clean.
3. `internal/auth/purge.go` (PurgeExpired: spent Challenges, dead
   sessions, one minimal Access Log row) + purge_test.go +
   accesslog.go. Confirm the purge deletes only what it should and
   the log row carries no secret.
4. Slice-2 review fixes F1/F2 in `internal/events/dispatch.go`:
   verify they address the review comments and break nothing.
5. Property 15 occurrence uniqueness: rerun the property (Go suite
   needs -timeout 20m; default 10m can kill slow packages — a
   timeout alone is not a logic failure, but confirm green at 20m).
6. Confirm NO web/ or proto/ changes in the diff. Z4: no
   secrets/key-custody/retention policy invented; 8.8/8.9/8.10
   still deferred. Z2 untouched.
7. Run: gofmt -l cmd internal (empty), go build ./..., go vet
   ./..., the FULL make test through .agents/bin/suite (it queues
   behind the running B11 suite — wait for the lock, never bypass
   it), make generate twice with no diff. Tree must be clean at
   3a9a3a1 before any gate; if a new rapid .fail seed appears,
   report its content. No browser pass (no Desk in this slice).

Merge-small rule: only blockers stop the merge (wrong
lease/fencing/occurrence semantics, purge overreach, broken
startup/shutdown, failing property, red suite); the rest are
follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-c2/review-slice-3.md with a verdict
line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first with
file:line. Then report: say master "C2 SLICE 3 REVIEW: PASS or
NEEDS_CHANGES <one line>". Then wait for instructions. Do not edit
product code, commit, merge, push or start agents.
