# Summary: C10 slices 1–2 (ADR-0035 + provider inbox)

Merged 2026-11-24 as 90d0254 + 1aa6106 (2 commits).

Scope: slice 1 docs (ADR-0035 Proposed + decision-log/README rows; attachments
reuse B9 Image storage; migration 25 held for F4 23/24, still open) + slice 2
provider inbox (`internal/integrations/inbox.go`): HMAC-SHA256 verification with
bounded previous-key window, merchant/amount/identity claims, PK-atomic dedupe
via bus inbox, deterministic Operation_ID mapping, provider.received evidence
events; `secrets.ActiveVersionInfo`/`KeyForVersion` helpers. No mark-paid path
by construction; inbox unwired (RPC/CLI/Desk surface in later slices).

Gates: worker affected green (cmd/athena+integrations+secrets); disjoint carry
over the F4s1 push verified at PASS time.
Review: r-c10s2 PASS (HMAC window, dedupe, Operation_ID, secrets helpers;
affected green at 47dd6e4).
Merge gate: full `make test` exit 0 clean, 784s, 21/21 Go pkgs ok + 717 web
(log 1124-234256-master.log). Staged picks rebased onto post-C7s7 main with
byte-identical content (1 decision-log conflict, keep-both); build+vet green
on the merged tip.

L2 RESTING per the Stylus cap ruling (w-c2 stood down at C10s2 PASS). Slice 3
(Property 26) starts when a worker slot frees; migration 25 unblocked (23/24
merged).
