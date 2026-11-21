# Brief: Desk screens for B4, B12 and B7 (Muse t4)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-desk-settings, branch feat/desk-settings. Work only there. Scope: web/** and docs/architecture/frontend-theme-and-type.md. Do not touch RecordTypesPage.vue (another worker owns it in B9a).
Read: AGENTS.md, .kiro/steering/athena-design.md, docs/design/athena-design-system.md sections 13 to 18, requirements R4 and R7, tasks.md B4, B7 and B12, proto/athena/v1/{workspace,notifications,auth}.proto. Use impeccable (Operate) and emilkowalski-motion.

1. **Appearance (B4, section 18):** Settings → Appearance (permission settings:appearance; read-only view without it). Show `ink` and `red` as swatches, plus Custom with a color picker for the accent and optional chart 1 to 5. Show a live preview in light and dark side by side (primary button, focus ring, selected nav item, a small chart drawn with the chart tokens). Save calls SetAppearance. On a contrast failure, show the suggested value with "Use suggested". Add a reset to default. Apply the result at load and on the sign-in page (GetAppearance is public) by setting data-accent or the custom properties on <html>, cached per Workspace so the first paint is right. Add a per-user Light, Dark or System choice in the user menu.
2. **Notifications (B12, section 17):** the bell shows the unread count (UnreadCount). The inbox is a popover on desktop and a bottom drawer below 1024, grouped Today and Earlier, with read and unread state, Mark all as read, and a deep link per item (subject type and id to the right page). Poll every 30 s while the tab is visible. Show a quiet toast only for items that need action. Add empty and error states.
3. **Sessions (B7, design.md section 4):**
   - Handle the `Unauthenticated` reasons (idle, absolute, revoked) with plain copy on the sign-in page.
   - Warn 2 minutes before idle expiry with "Stay signed in" and "Sign out".
   - Use `BroadcastChannel("athena-session")` (falling back to the storage event) so sign-out, Workspace change and permission change update every tab within 2 s. Activity counts across tabs.
   - Step-up: when a call fails with step_up_required, open a password dialog, call StepUp, then retry the action once. Keep unsaved form state.
   - Add "Keep me signed in on this device" on sign-in, shown only when it applies.
   - Show the security settings (athena security values) read-only in Settings → Security, editable with permission.
4. Settings page: Settings becomes a real page with sections Appearance, Security, and (placeholders) Workspace identity (B5) and Notifications preferences.

Checks: typecheck, test (exit 0), build; Vitest for the session channel, idle timer, step-up retry and appearance application. Browser pass with a throwaway Workspace only, at 375, 768, 1280 and 1440 in both themes, including two tabs for the session sync. Screenshots in .cache/screens/ds-*.
Do not commit, merge, push or start agents. Report with hc send t6 "DESK SETTINGS: PASS or BLOCKED".
