# Review brief: TS slice 4 (shared-DB conversions, second wave)

Worker: w-ts. Branch `feat/test-speed`, tip 92055f8 (code a0741da + numbers).
Scope (test files + TestMains only, no production asserts touched): shared-DB
TestMain conversions for events/jobs/ledger/stock (+ server stragglers per the
slice-4 commit); keeper split with why-comments; measure.md numbers.

Claimed: affected 8 pkgs green exit 0 (events 8.1s ex-655, jobs 7.4s ex-653,
ledger 9.9s ex-857, stock 32.3s ex-202-408, posting 25.8s, server 228.2s
ex-435-551, auth 327.0s ex-639-657, cmd/athena 245.7s).

Verify: conversions real (migrate-once + Reset semantics preserved), zero
keepers hold or each documented, no test deleted or weakened (diff test bodies),
affected re-run or reuse through the main-repo suite by absolute path,
measure.md numbers match suite logs.
Full-suite confirmation rides the merge gate.

Verdict line: `Verdict: PASS` or `Verdict: NEEDS_CHANGES`.
Report to master with `say master "TS SLICE 4 REVIEW: PASS/NEEDS_CHANGES <one line>"`.
Reads only in the detached tree; never merge/push/spawn.
