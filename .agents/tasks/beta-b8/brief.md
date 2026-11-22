# Brief: B8 root administrator and Build/View presentation (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b8, branch feat/b8-root-buildview (from main 2714a12).
Read: tasks.md B8; ADR-0018 (root administrator, Accepted — confirm, do not change); AGENTS.md root paragraph; docs/design/athena-design-system.md sections 7 (mode switch), 14 (action buttons), 15 (sidebar), 16 (detail panels); internal/auth root guard code; web/src Access page + shell/mode files.

Slices, each merging on its own (report after each):
1. Server + CLI root confirmation: prove ADR-0018 end to end — root created at `athena new`/first boot with the database password, bypasses all gates, cannot be suspended, edited, role-changed or deleted through API, CLI or Desk (one test per path), every root action audit-logged as root, change-password rotates only root's own password. Fix any gap found (small, surgical; no redesign). CLI: confirm `athena access` refuses root edits with plain errors.
2. Desk: root sees a quiet "Root administrator" caption and a `lock` badge on its own user row with the reason; its row actions disabled with the reason. Fix the `?mode=build` fresh-load bug (lands in View because the `canBuildDesk` watch fires during startup). Mode switch per §7 (segmented control, words + icons, builders only); Build shows a persistent "Building" caption and hairline-separated sidebar group; View hides ALL Build navigation; a read-only "Preview as" control in Build shows View as a chosen Role.

Rules: follow the design system + steering (MUJI restraint, tokens, all states, both themes). No new permissions, migrations or ADRs expected — root behavior is specified; if a gap needs one, STOP and report BLOCKED with the reason instead of inventing it. Use throwaway Workspaces only, never the dev database. Commit on your own branch feat/b8-root-buildview; you never merge, push or start agents.
Checks per slice: gofmt -l cmd internal (empty), go build ./..., go vet ./..., make test, make generate stable; CI=true pnpm --dir web run typecheck, test (exit 0), build; browser pass on a throwaway Workspace at 375, 768, 1280, 1440 in both themes with screenshots in .cache/screens/b8-*.
Report: hc send master "B8 SLICE n: PASS or BLOCKED <one line>".
