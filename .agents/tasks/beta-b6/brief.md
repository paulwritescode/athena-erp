# Brief: B6 slice 1, everyday words and help (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b6-words, branch feat/b6-words. Scope: web/src/copy/ (new), and copy-only edits in web/src pages and components. No layout or behavior changes, so other Desk work can merge around you. No Go.
Read: AGENTS.md; docs/design/athena-design-system.md sections 8 and 11; requirements R6; tasks.md B6; docs/revamp/Athena-ERP-Playbook.md (vocabulary); ADR-0008.

1. Create `web/src/copy/glossary.ts`: one entry per term a user meets, with an everyday label and a one-line explanation. Cover:
   - lifecycle (Draft, Submitted, Cancelled, Amended);
   - record kinds;
   - ledger (company, financial year, period, soft close, hard close, currency, exchange rate, account, account type, posting);
   - access (Role, Grant, Assignment, Scope, session, step-up, invite, owner, root administrator);
   - rules (Rule, validation, derivation, guard);
   - Packs, Workspace, and notifications kinds.
   Export a typed lookup.
2. `HelpHint.vue`: a circle-help icon that shows the explanation in a popover (a drawer on touch), with an accessible name.
3. View mode: labels, statuses and empty states come from the glossary. Build mode keeps builder words (Record Type, Pack, Rule, Grant) but adds HelpHint next to each term the first time it appears on a page.
4. Copy pass for internal or playful words that remain (sponsor, OPEN, kernel, ADR, "how hard the door is to knock on"). Sentences, not title case. Controls name their action.
5. `docs/design/naming.md`: the naming registry (product names from tasks.md section N mapped to CLI, proto service, Go package and UI label), marked Proposed. No code renames.
6. A Vitest test that every glossary key used in the code exists, and that no View-mode page string contains a builder-only word.
Checks: typecheck, test (exit 0), build; screenshots of 3 pages at 1440 and 375 with a HelpHint open. Do not commit, merge, push or start agents.
Report: hc send t6 "B6 SLICE: PASS or BLOCKED".
