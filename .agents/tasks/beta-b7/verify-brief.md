# Brief: B7 verification pass (Muse) — REPORT ONLY, no product changes

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b7verify, branch feat/b7-verify (from main f6f2fc0, incl. B4b/B9b/B2a-1/B5/B9form-1).
Read: tasks.md B7; .agents/tasks/beta-b7/leftovers.md (the 6-item checklist — this task); design.md section 4 (session/tab design); ADR-0026 (Proposed).

B7 (sessions, rate limits, tab management) looks essentially complete in main: server lifetimes/step-up/rotation/Clear-Site-Data/rate limits (8739e3d), Desk idle warning/cross-tab/step-up/session list (ec98c5f, 76210b3, B3a), CLI, ADR-0026, Bruno checks. Your job is to VERIFY each checklist item behaviorally, not to build:

1. Idle warning 2 min before expiry with "Stay signed in"; activity in any tab resets it (BroadcastChannel path).
2. "Keep me signed in on this device" offered only for non-builder sessions; remembered sessions get the 7-day absolute.
3. Workspace switch clears caches in all tabs.
4. Cross-tab sign-out covered by sessionChannel.test.ts (run it; confirm it passes and means what it says).
5. Expensive-read limits against a real expensive procedure if one is callable now (ReportService/RunReport are stubs until C6 — if untestable, say so; it becomes a C6 follow-up, not a B7 blocker).
6. Bruno 09-idle-expiry and 10-sign-in-limit pass against a throwaway Workspace (see docs/testing/bruno.md; never the dev database).

Rules: REPORT ONLY. Do not modify product code, tests, or docs outside your report file — other workers own the Desk files right now, and any fix goes through a later task. You MAY run builds, tests, Bruno, and throwaway servers (fake HOME only — the user server holds the real-HOME lock; never stop another agent's server) and create/drop test_athena_* databases. Write findings to /Users/iampaulking/projects/duchess/athena-erp/.agents/tasks/beta-b7/verify-report.md: one section per item with PASS or GAP (evidence: command + output, screenshot path, file:line), each gap with a suggested fix and the owning file. Then report: hc send master "B7 VERIFY: PASS or GAPS <one line>". Do not commit, merge, push or start agents.
