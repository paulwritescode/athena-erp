# Summary: TS slice 4 (shared-DB conversions, second wave)

Merged 2026-11-24 as ed8c89b + 919c187 (2 commits, test files + measure.md only).

Scope: shared-DB TestMain conversions for ledger/jobs/events/stock plus the
server stragglers (16 test files); keeper split with why-comments; affected
numbers recorded in `.agents/tasks/beta-ts/measure.md`. No test deleted or
weakened; no production file touched.

Gates: worker affected 8 pkgs green exit 0 (607s slot, -p 2).
Review: r-ts4 PASS (conversions real, zero undocumented keepers, no test bodies
touched, affected reuse + numbers match log 1124-225111).
Merge gate: full `make test` exit 0 clean, 836s, 21/21 Go pkgs ok + web green
(log 1124-230732-master.log).

Full-gate package times (this merge): auth 532s, server 223s, cmd/athena 256s,
files 111s, secrets 102s, stock 32.6s, posting 18.8s, events 16.4s, jobs 11.6s,
ledger 7.5s. Posting is no longer the pole (was 853s pre-slice-3/4); the lead
for slice 5 is auth-waits attribution (w-ts perf-schema probe sampled this very
gate) + files/cmd remaining per-test DDL, then the -p 2/4/8 shootout + GO_TEST_P.

Branch `feat/test-speed` kept for slice 5 (w-ts released to implement on merge).
