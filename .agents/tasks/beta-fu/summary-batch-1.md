# Summary: FU batch 1 — Desk polish (12 fixes)

Merged 2026-11-23 as 4cf2a0c..2d9c17d in train2 (built at
db6a7fd..425791b on 919bab4; parent is main, no rebase
needed; pre-train base b41077d rebased cleanly — train1
touched none of its files).

- B9a x2 (field-row icon spacing, dead rule removal) +
  B9a x3 verified already satisfied by B9b; B4 x1
  (Selected-page accent) + 2 verified pre-resolved; B4b x6
  (theme dots, dark ramp, CLI usage text, draft-accent
  fallback, Custom wheel, caption token); B9b x3 (draft
  comments, Change↔Auto toggle, Enter inline errors).
- Skips verified: --chart-palette flag (needs wiring,
  usage-text-only scope); B2a slice 1 deferred to batch 2
  (none half-built).
- Scope clean: 11 Desk/test files + CLI usage strings;
  no server/proto/gate/migration/L3-file changes.
- Gates: make test exit 0 pre-train; affected + 616/616
  Vitest post-train; browser exit 0 with 0 page errors
  (two driver-only timing flakes diagnosed and patched,
  app green throughout); 6/6 shots eyeballed.
- Review: PASS (review-batch-1.md) — 12/12 fixes verified
  against the design system, skips verified, shots match.
- Merge-time verification: train2 tip gate (full make test)
  on 2d9c17d.
- Follow-ups: Custom accent-row indicator; harden browser
  theme-menu predicate; capture wheel/ramp/Auto states in a
  future pass.
