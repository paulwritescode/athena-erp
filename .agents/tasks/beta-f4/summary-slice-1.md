# Summary: F4 slice 1 (conditions engine + builder + Rules modes + lists core)

Merged 2026-11-24 as 2a9c349 (17 picks d6cff00..6474b04 onto post-TS main; main.go registry-resolved + lists table entry, log/server unions), 17 commits, 43 files.

Scope: bottom-16 core — expr condition tree compile+parse round-trip + date() + TS mirror; condition builder component + Gallery story + Rules page Builder mode beside Starlark (builder exec without Starlark, Starlark regressions green); migration 23 saved_lists + migration 24 builder columns; ListService CRUD/run/export + gopdf renderer (Inter embed, golden 3-page) + CLI export --pdf; design.md + ADR-0034 (Proposed). gopdf v0.38.6 cleared after maroto failed size diligence (+14.9MB vs +1.2MB). F2/F3 embeds (slice 2) + QueueMessages/Desk UI (slice 3) stack above, unmerged. F4 text-search dropped as superseded by F2s1 query.

Evidence: worker full suite green + browser PASS (4 shots, 0 page errors). Merge gate: exit 0, 25m (21 Go ok incl. new internal/lists — posting 51s with TS shared-DB; 717/717 web; zero FAIL, log 1124-222112). Staging verified: 41/43 files identical to branch (main.go registry resolution + log union intentional); server files union incl. parseTime rename; generate stable.

Review: r-f4s1 PASS (review-slice-1.md) — engine round-trip + TS 25/25 + golden x3 + diligence + scope verified, full suite green 1448s on 6474b04.
