# Brief: B10 art registry, hairlines and ASCII (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b10, branch feat/b10-art (from main f6f2fc0).
Read: tasks.md B10; docs/design/athena-design-system.md section 9 (this task — placement rules, sizes, motion, registry), sections 2, 7, 13; the `hairline-create` skill for custom figures.

`@lucasmarkes/hairline` 0.5.1 is already pinned in web/package.json (user decision Z9) — use it, add no other dependency. No Go except the CLI banner ASCII.

Slices, each merging on its own (report after each):
1. Registry + core placements: `web/src/art/registry.ts` (placement key → figure or ASCII block, size, label; `useArt(key)` returns nothing for unlisted keys; figures lazy-loaded so the bundle carries only the one in use), custom Athena figures in `web/src/art/figures/` made with the hairline-create skill, figures on sign-in, the 404 page and the offline page, ASCII mark for the CLI banner (`cmd/athena`) and the sign-in footer. Tests: at most one figure per screen (test over the registry), reduced motion rests every figure, registry rejects unknown keys. Figures: line color from `--app-text-subtle`, theme auto, intensity ≤0.3, play off.
2. Empty-state figures on settled Desk pages (AFTER B2a and B9-form land — other workers own those pages now; do not start slice 2 until the master says so): EmptyState figures per the §9 meaning table (Sieve for no results, Drawer for empty Files, etc.), ≤240 px (120 mobile), aria-hidden or labelled.

Rules: follow §9 exactly (where/how many/size/look/meaning/one-place). Never import a figure directly from a page — always through the registry. Use throwaway Workspaces only, never the dev database. Commit on your own branch feat/b10-art; you never merge, push or start agents.
Checks per slice: CI=true pnpm --dir web run typecheck, test (exit 0), build; gofmt/build/vet + make test for the CLI banner slice; browser pass on a throwaway Workspace (sign-in, 404, offline at 375 and 1440 in both themes, screenshots in .cache/screens/b10-*).
Report: hc send master "B10 SLICE n: PASS or BLOCKED <one line>".
