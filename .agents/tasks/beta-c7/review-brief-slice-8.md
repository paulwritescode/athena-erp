# Review brief: C7 slice 8 (Property 22 stock-equals-GL)

Worker: w-c7. Branch `feat/c7-stock`, commit 94b3aab on origin/main ff18cb2.
Scope: `internal/stock/stock_test.go` phased histories (receipts/issues/returns/
deltas/cancels, two items + control accounts) asserting stock-equals-GL at every
op cutoff + company-wide check; in-slice fixes (LIFO cancel guards, value-sign
legs) + 5 regression tests. No proto.

Verify: property genuine (phased histories defeat rapid's small-value bias;
assertions real, 100/100 direct re-run or reuse), both in-slice bug fixes
correct and minimal, 5 regression tests fail-before/pass-after plausible,
affected green on identical content (internal/stock 332s zero FAILs — reuse or
re-run through the main-repo suite by absolute path), gofmt/build/vet clean.

Verdict line: `Verdict: PASS` or `Verdict: NEEDS_CHANGES`.
Report to master with `say master "C7 SLICE 8 REVIEW: PASS/NEEDS_CHANGES <one line>"`.
Reads only in the detached tree; never merge/push/spawn.
