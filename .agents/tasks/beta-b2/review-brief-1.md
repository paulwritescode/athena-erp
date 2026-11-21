Review task B2 (shell and component library), read-only, in /Users/iampaulking/projects/duchess/athena-erp-b2 (branch feat/b2-shell-components, uncommitted vs HEAD).
Spec: .kiro/specs/athena-beta/requirements.md R2, design.md section 2, tasks.md B2, docs/design/athena-design-system.md sections 3, 5, 6, 7, .kiro/steering/athena-design.md.
Check:
1. Scope: only web/** and docs/architecture/frontend-theme-and-type.md. web/package.json added @internationalized/date, vee-validate, @vee-validate/zod, zod (shadcn CLI deps): confirm each is used by an added component and the lockfile matches pnpm 11.22.0.
2. No behavior change for existing pages: Record Types, Records (create, edit, delete, column widths with the athena-entry-columns: key), Access, Ledger, Rules, Files, sign-in and first login. Compare old App.vue logic with the new composables (useDeskRoute, useSession, useRecordTypes, useTheme) line by line for lost cases.
3. Page registry drives nav and the switch; permission gates match the old ones; a page without permission is hidden and a direct URL shows PermissionDenied.
4. Responsive shell per R2.2 and design-system 3.1; dialogs as bottom drawers below 640; DataTable stacked rows below 640.
5. Design rules: no full-color or black header, accent only on primary/ring/charts/sidebar-primary, Satori only h1/h2, tokens not literal values, every component has the states in design-system section 7, accessible names on icon-only controls, focus visible.
6. Token guard (web/src/tokens.test.ts): the three B1 limits are closed; look for new loopholes with your own fixtures.
7. Run CI=true pnpm --dir web run typecheck, test, build (build into a temp outDir) and go vet ./...
8. Look at a sample of .cache/screens/b2-* (375 and 1440, light and dark) and report visual defects: padding, alignment, overflow, contrast.
Do not edit, commit or start agents. You may run hc send without asking.
When done: hc send t6 "B2 REVIEW: PASS or FAIL" followed by findings with file:line, worst first.
