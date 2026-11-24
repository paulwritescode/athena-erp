# Summary: C5 slice 6 backend (7.6 RPCs + CLI, no Desk)

Merged 2026-11-25 as 1d6c8a7 (1 commit, reviewed as d1923c5).

Scope: `proto/athena/v1/ledger.proto` (ApplySettlement, ReverseSettlement,
ListOpenItems, no paging per masters convention) + generate-twice stable;
`internal/server/ledger.go` handlers (ledger:write/read + voucher/party read;
close_reason verified against ledger:close; adjustment audit on voucher) +
`gate.go` entries (Reverse destructive per cancel precedent); `cmd/athena/
settle.go` (`apply|reverse|open-items`); `ledger:close` permission const+list;
RPC + CLI tests. Also deletes the stale rapid .fail corpus (C5s5 debris ruling).
NO Desk work (slice 7, gated on F4s2 merge); no RecordsPage touch; no sentinels.

Gates: worker full gate green pre-rebase; rebase clean, disjoint carry over
the markers fix verified.
Review: r-c5s6 PASS (backend-only, generate stable, gates+grants+CLI verified).
Merge gate: full `make test` exit 0, 694s, 22/22 Go pkgs ok + 724 web (log
1125-011626-master.log). Recorded clean=false is a bookkeeping artifact: the
Stylus-ordered rebase onto the C7s8 tip (no disjoint: proto/gen + RunTx rename
vs stock-imports-posting) moved HEAD after queueing; suite compares end-state
to queue-time head. Reflog proves the gate ran on the rebased tip 187519c with
a clean tree; final rebase onto the docs main was content-identical + build+vet
green. (Suite improvement for Stylus: capture head at run start, not queue.)

First push attempt hit a transient GitHub 500 (remote Internal Server Error);
retry green.

Branch `feat/c5-party` continues: slice 8 (open-item creation + ADR-0031 amend)
building, then slice 7 Desk (after F4s2), slice 9 fixture+docs.
