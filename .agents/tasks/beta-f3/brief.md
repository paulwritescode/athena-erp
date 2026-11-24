# Brief: F3 field settings sheet (Muse)

Worktree/branch: assigned by the master at dispatch (fresh tree off latest main). Lane L4 straight after F2 slice 2 merges — master orders the start, do not start early.
Read: tasks.md F3 (beta tasks.md); FieldSettingsDialog.vue (current dialog — L3-C4 owns it, no live worker); design-system sheets/drawers + glossary words in View; F4's ConditionBuilder component (read ONLY after it merges to main — never read or build on F4's live branch; master tells you when it lands).

Acceptance→slice mapping:
- "side sheet (drawer on mobile), never exceeds viewport, scrolls inside" → slice 1 (sheet shell + responsive + scroll; viewport-overflow tests).
- "Sections: Basics open (label, type, required, default, help); Rules for this field + Advanced collapsed" → slice 1 (section structure + collapse states; Rules section hosts the slice-2 builder).
- "Plain-word labels + one-line help, no doc. syntax" → slice 1 (every label + help line rewritten, glossary-backed).
- "existing-item references are selects (field keys, link/child Record Types, options source, permission levels); numeric child-field totals as picks" → slice 1 (all free-text references become selects/picks).
- "conditions use the F4 builder" → slice 2 (embed ConditionBuilder in 'Rules for this field'; WAITS for the F4 builder merge — if unmerged when slice 1 passes, hold slice 2 for the master's order).

Slices (report each: say master "F3 SLICE n: PASS/BLOCKED <one line>"; rebase on main before each PASS):
1. Sheet + sections + plain words + selects (no builder): the full sheet per above with mount tests + browser pass (sheet at 375/768/1280 x light/dark, overflow/scroll asserts).
2. Builder embed: ConditionBuilder in 'Rules for this field' (master-ordered start after F4 builder merges) + round-trip test + browser.

Files: FieldSettingsDialog.vue (+ its test, rename to sheet if the component renames) + glossary additions if needed. NOT allowed: RecordsPage.vue, DataTable.vue (F1/F2 own them), App.vue, any proto/server file, F4 files (until merged — read-only after master says so).

Rules: batch conventions; design-system sheet/drawer rules; glossary words in View; no Record values in spans (no new spans expected). Telemetry off unless set. Throwaway DBs + fake HOME. Full suites/browser only through .agents/bin/suite; package tests direct. Commit on the assigned branch; never merge/push/spawn.
Checks per slice: web typecheck/test/build, affected or full suite where it applies, browser pass per above.
