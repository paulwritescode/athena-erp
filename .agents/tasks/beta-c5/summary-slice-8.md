# Summary: C5 slice 8 (open-item creation + ADR-0031 amend)

Merged 2026-11-25 as 9105f28 (1 commit).

Scope: posting a receivable/payable control leg on submit creates the open
item(s) in the same transaction (one per instalment via cross-plan offset;
single item default); cancel voids them first in the cancel tx (zeroes
residuals, keeps originals as cancelled; refuses on unreversed applications).
Identity voucher-scoped (type/id/revision + leg index); party keys copied from
the leg source; due date NULL (plan-channel follow-up filed). Replay-safe via
ErrItemExists pass-through. Invariant 7.8 tested at submit and cancel. ADR-0031
amended with the creation rule (Proposed kept). Extra files: lifecycle.go hook
(+97) + settlement.go void (+73) — justified, minimal, neither lane-owned. No
sentinels; items ride the GL tx so period policy inherits.

Gates: worker full gate green pre-rebase, disjoint carry (rebase clean, slice 6
dropped as merged, byte-identical).
Review: r-c5s8 PASS (creation/void/replay/invariant/ADR verified; affected
re-green at 24ecb55: athena 298s, posting 41s, server 327s, stock 65s; one
non-blocking note: cross-service voucher race, pre-existing architecture).
Merge gate: full `make test` exit 0 clean, 759s, 22/22 Go pkgs ok + 724 web
(log 1125-022646-master.log).

Live submit/cancel paths touched: server restarted + healthz green.
Follow-ups filed: instalment due dates need a plan channel; general
control-source existence for non-submit posts.

C5 does NOT tick yet: slice 9 (fixture+docs) in review, slice 7 Desk panel
still open after F4s2. C5 ticks when slice 7 merges.
