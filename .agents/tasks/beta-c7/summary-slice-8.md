# Summary: C7 slice 8 (Property 22 stock value equals GL)

Merged 2026-11-25 as 2ca8ca5 (1 commit, reviewed as 94b3aab, replay-identical).

Scope: `internal/stock/stock_test.go` phased histories (receipts/issues/
returns/deltas/cancels, two items + control accounts) asserting stock-equals-GL
at every op cutoff plus a company-wide check; phases defeat rapid's small-value
bias. Property found two real bugs, fixed in-slice: LIFO cancel guards
(issue-cancel blocked by live returns, receipt-cancel by live downstream,
same-voucher exclusion so transfers cancel) and value-sign legs (issues at
negative average mirror GL); 5 regression tests (4 fail-before proven by
reasoning, 1 characterization).

Gates: worker affected green on identical content (internal/stock 332s, zero
FAILs); property 100/100 direct.
Review: r-c7s8 PASS (property genuine, fixes minimal; one note: same-voucher
exclusion currently unreachable — sound defense-in-depth, message overclaims
slightly). No suite re-run (identical content + untouched deps; queued run
terminated as no-signal).
Merge gate: full `make test` exit 0 clean, 725s, 22/22 Go pkgs ok (new:
integrations 28s) + 724 web (log 1125-010418-master.log).

Production legs change is transitively unwired (stock seam waits slice 11):
no server restart, healthz only.
Follow-up scheduled: P21/P23 skewed kind-dispatch deserves the same phases
treatment (pass, but thinner than drawn ranges suggest).

Branch `feat/c7-stock` continues: slice 9 (stock RPC/CLI/Desk) building, then
docs (slice 10) + lifecycle wiring (slice 11).
