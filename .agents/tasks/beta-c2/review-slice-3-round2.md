# Review: C2 slice 3 ROUND 2 — fix verification (B1, F1–F5)

Verdict: PASS

Scope: reviewer worktree athena-erp-c2-r, detached at c18ae67
(clean tree before and after every gate). Rebased stack is f186fe7
(slice 3) + b866c41 (B1) + 13c7973 (F1/F2/F4/F5) + c18ae67
(F3 migration 19) on current main. References: brief slice 3,
round-1 review-slice-3.md, batch-2 design G8, R8.4/R8.5, ADR-0028.

## Fix verification

1. B1 (blocker, fixed): `internal/jobs/runner.go` `Recover` parks
   dead with `attempts >= CASE WHEN max_attempts > 0 THEN
   max_attempts ELSE ? END` — row-first, mirroring `maxAttempts`.
   `TestRecoverHonorsRowMaxAttempts` (MaxAttempts 10, attempts 7,
   expired lease => released) passes; existing
   `TestRecoverReleasesExpiredLeases` and
   `TestRecoverParksExhaustedLeasesDead` still pass (all run
   directly on this tree, below).
2. F1 (fixed): `Runner.Errors` removed from the struct, its doc,
   `workers.go:49` and the test literal; no reference to it
   remains in `internal/jobs`; build and vet pass. `Classifier`
   stays for `EnsureOccurrence`.
3. F2 (fixed): `DispatchDue` uses the new `cloneMap`
   (`jobs.go`) before adding the occurrence identity; shallow
   copy is sufficient (only a top-level key is added).
   `TestDispatchDueLeavesCallerPayloadAlone` passes.
4. F4 (fixed as suggested): `Recover` release sets `attempts =
   attempts + 1` like the live reclaim path; the release test
   pins attempts == 1 and passes.
5. F5 (fixed): claim-order comment present (`runner.go`, retried
   jobs sort ahead of fresh NULL rows).
6. F3 (fixed): migration 19 `widen_job_occurrence_key` widens
   `athena_jobs.occurrence_key` to VARCHAR(700); reserved in
   `.agents/stylus/migrations.md`, contiguous after merged 18.
   `TestDispatchDueHandlesLongNames` dispatches a 602-char key
   end to end and passes; the test setup migrates a fresh
   throwaway database through 19, so the migration applies
   cleanly. The column carries full-column `KEY
   idx_jobs_occurrence`; 700 x 4 = 2800 bytes stays under the
   3072-byte InnoDB limit.

## Regression scan

- The rebased fix stack `f186fe7..c18ae67` touches exactly 6
  files: `internal/jobs/{jobs,runner,occurrences,jobs_test}.go`,
  `internal/server/workers.go`, `internal/server/sysmigrations.go`
  (migration 19 only) — identical set to the pre-rebase fix.
- Each rebased commit diffed against its pre-rebase twin
  differs only in the same 14 main-drift files (C1 6.9
  ledger/post + tasks.md), so the C2 content is byte-identical
  across the rebase and the rebase resolved nothing inside C2.
- `3a9a3a1..c18ae67` outside the 6 fix files is merged-main
  work only (C1 6.8/6.9, C4 migration 18, docs/agents, web
  setup, proto/gen); slice-3 semantics from round 1 unchanged.
- Fix delta holds no secret/custody/retention/disposal/
  encryption/backdating terms (grep, zero hits). Z4 still
  deferred (8.8/8.9/8.10 absent), Z2 untouched.

## Gates (all on c18ae67, clean tree)

- `gofmt -l cmd internal`: empty. `go build ./...`, `go vet
  ./...`: pass.
- `.agents/bin/affected`: plan is 7 Go packages, no web tests;
  reused the worker's green run on the same clean commit
  (w-c2, 2026-11-23T21:37, log `1123-212410-w-c2.log` read:
  7/7 `ok`, zero FAIL), no rerun per one-green-run-per-commit.
- Directly on this tree (filtered, no lock): 6/6 pass —
  TestRecoverHonorsRowMaxAttempts, TestRecoverReleasesExpiredLeases,
  TestRecoverParksExhaustedLeasesDead,
  TestExpiredLeaseReclaimCountsAnAttempt,
  TestDispatchDueLeavesCallerPayloadAlone,
  TestDispatchDueHandlesLongNames (141s, throwaway test DBs).
- `make generate` twice: exit 0 both, no diff, tree clean
  (first attempt failed only because `node` was off PATH;
  reran with the suite's nvm node path).
- No browser pass (no Desk in this slice, per brief).

## Database safety

Everything ran in throwaway `test_athena_*` databases. The dev
database was never migrated, served or written by this review.

## Follow-ups

None. No blockers under the merge-small rule.
