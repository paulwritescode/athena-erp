Review B3 and B3a (Desk experience), read-only, in /Users/iampaulking/projects/duchess/athena-erp-b3-experience (uncommitted vs its base; main has moved on with server-only commits).
Spec: .agents/tasks/beta-b3/brief.md; docs/design/athena-design-system.md sections 3 to 8 and 13 to 17; requirements R2, R3; tasks.md B3 and B3a. The worker reported five steps; the master's feedback after each step is in the brief's follow-ups and must be done.
Check:
1. No functional regression: Records create, edit, delete, submit, cancel, amend; Record Type save including destructive plans; Access invite, assign, revoke, suspend, reactivate, reset, sessions; Ledger forms; Rules save, simulate, delete; Files; sign-in and first login. Compare against main.
2. Mobile and tablet: every dialog, form, picker and menu is a bottom drawer below 1024; drawers are full width with a handle; the dirty-form guard works for Esc, outside click, X and swipe; floating nav; edge swipe does not fire inside drawers, inputs or horizontal scrollers; touch targets at least 44 px; no horizontal overflow at 320, 375, 640, 768, 1023.
3. Desktop: full-height detail sheets; action buttons follow section 14 (kind, icon, hover, spacing, destructive last); sidebar has three groups; consistent radius; toasts never cover actions.
4. Accessibility: keyboard reachability in sheets, drawers and the sidebar; focus return on close; accessible names; reduced motion respected for swipe and drawers.
5. New dependency vaul-vue and web/pnpm-workspace.yaml allowBuilds for vue-demi: justified, pinned, lockfile matches pnpm 11.22.0.
6. Run CI=true pnpm --dir web run typecheck, test, build (temp outDir). Sample .cache/screens/b3s5final-* and b3s4-*.
BLOCKER or FOLLOW-UP per finding; PASS if no BLOCKER. Do not edit, commit or start agents. Reply: hc send t6 "B3 REVIEW: PASS or FAIL" with file:line, BLOCKERs first.
