# Brief: B4b Appearance page redesign (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b4b, branch feat/b4b-appearance.
Reconstructed by the night master from tasks.md B4b and the shipped commit d91ad3b (the worker was briefed verbally pre-shift; no brief file existed).

Read: tasks.md B4b; docs/design/athena-design-system.md section 18 (revised 2026-11-21), sections 2, 7, 13, 14.

Desk: Settings → Appearance as a left column of choices (Accent with nine named presets — Ink (default), Slate, Blue, Sky, Green, Amber, Orange, Brown, Red — plus Custom with a color picker; Charts palettes; Your theme per user) beside one realistic preview card (invoices list with status badges, search with focus ring, selected sidebar item, six-month bar chart) shown in light and dark side by side on desktop. Choices open in popovers on desktop, bottom drawers on touch. Save enabled only when changed; Reset to default. Applied via [data-accent] / custom properties on <html> at load.

Server: extend the preset list and chart palettes in internal/appearance, validated against one shared list; contrast test for every preset in both themes (text on accent ≥ 4.5:1).

Checks: go build ./..., go vet ./..., make test, make generate stable; CI=true pnpm --dir web run typecheck, test, build; browser pass on a throwaway Workspace at 375, 768, 1280, 1440 in both themes. Review file: .agents/tasks/beta-b4b/review.md.
