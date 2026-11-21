You are the reviewer for task B1, read-only. Worktree: this folder (branch feat/b1-design-tokens, uncommitted changes vs HEAD).
Spec: .kiro/specs/athena-beta/tasks.md task B1, docs/design/athena-design-system.md sections 2, 3, 6, .kiro/steering/athena-design.md.
Check:
1. Only web/src/theme.css, web/src/style.css, web/src/tokens.test.ts, docs/architecture/frontend-theme-and-type.md changed.
2. Every pre-existing color value in theme.css is unchanged, except dark --sidebar-primary is now var(--primary).
3. Tokens from sections 2.4, 2.5, 3 and 6 exist for light and dark. The ink and red [data-accent] presets exist and red changes only accent tokens.
4. tokens.test.ts fails on hex, rgb( or named colors outside theme.css, with no loophole.
5. Run CI=true pnpm --dir web run typecheck, test and build.
Do not edit files, commit or start agents.
When done run: hc send t6 "B1 REVIEW: PASS or FAIL, then findings with file:line"
