# Brief: F4 research + design (Muse, L5, design phase — NO code)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-fu. NOTE: your decisions commit 7d7b93a is in review; if master relays review fixes, they take priority over this research.
Read: tasks.md F4 (beta tasks.md line ~165); .agents/stylus/night-shift-orders.md L5 row; internal/expr (what exists); web Rules page + F2/F3 areas (read-only recon); C2 outbox (for the queued-email shape).

Task (research + design ONLY — write no product code, no tests, no migration):
1. Research how Airtable/Notion/HubSpot build no-code condition builders (operator sets, grouping, autocomplete, empty/not-empty, relative dates) and saved-list views with export. Note what fits Athena's Desk idiom (design-system rules, glossary words in View).
2. Design the Athena condition builder ("When [record] [field] [operator] [value/field]", AND/OR groups, autocomplete, compiles to internal/expr + round-trips, "Show as expression" toggle) for the Rules page (validation/guard/derivation without Starlark; Starlark stays for advanced), F3 conditions, F2 filters.
3. Design Lists (saved queries e.g. "RSVP yes but missing diet or parking"): live list, PDF export now, "Email them"/"Message them" queue via the C2 outbox (provider pending → button says queued). Audience rule excludes "invited but not responded" from day-of sends.
4. Write `.agents/tasks/beta-f4/design.md` (the full design) + ADR-**0034** "No-code conditions and lists" (Proposed — number assigned by the master; no other ADR) in a NEW branch `feat/f4-design` cut from latest main AFTER the decisions commit merges (master orders the cut; until then keep the design in your tree uncommitted or in /tmp and report DRAFT).

Report: say master "F4 DESIGN: DRAFT <one line>" with the design's location + key choices + open questions for Stylus/user. Do not commit until ordered. Never merge/push/spawn.
