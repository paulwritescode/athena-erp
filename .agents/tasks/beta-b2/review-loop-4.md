# B2 loop 4: blockers only (user chose option A)

Fix only these five, each with a regression test (Vitest for logic, a scripted browser check for keyboard and layout). Do not change anything else. Follow-ups are listed at the end for B3 and must not be done now.

1. **Wrong Record Type on a stale URL** (useRecordTypes.ts:19, RecordsPage.vue:316). A URL naming an unknown or deleted Record Type (`/view/retired/new`, `/view/retired/<id>`, `/view/retired`) must never fall back to another type. Show a not-found state ("This list no longer exists") with a link to View home, and render no form. RecordsPage must take the Record Type key from the route, and CreateRecord must send that key or not run. Test: the stale URL renders the not-found state, and no CreateRecord call happens.
2. **Sidebar focus trap** (AppSidebar.vue:71). Trap focus only while the sidebar is modal: the tablet overlay and the phone sheet. At 1440, and at 768 while collapsed, Tab moves from the last sidebar control into the header and main content, and Shift+Tab moves back. Test both directions at 768 collapsed, 768 expanded (trapped) and 1440.
3. **Double history entry on Gallery → View** (useDeskRoute.ts:91, PageSwitch.vue:24). The first flip to View pushes `/view` and then `/view/task`. Resolve the final View URL first, including the default Record Type, then push once. Test: after any mode flip, `history.length` grows by exactly 1, and one Back returns to the previous page.
4. **Tablet rail pushes content** (ui/sidebar/Sidebar.vue:59,67). At 640 to 1023 px the expanded sidebar must overlay. The main column keeps its 56 px offset, and the document width stays equal to the viewport after the transition. Use a fixed-position panel for the expanded state, not the layout gap. Test: at 768, main's left is 56 and `document.documentElement.scrollWidth` is 768 before, during and after expansion.
5. **Access details need a mouse** (AccessPage.vue:667,715,776). In Users, Roles and Sessions, each row's primary cell is a real button or link that opens the detail drawer, reachable by Tab and opened by Enter or Space. A row-level click can stay for the pointer. In Sessions, keep End as a separate button. Test with the keyboard only on all three tabs.

Then: typecheck, test and build. Shots: l3-rail-768-{collapsed,expanded}-{light,dark}, l3-stale-url-375-light, l3-access-keyboard-1440-light. Report with `hc send t6`.

## Follow-ups (moved to B3, do not do now)

- The Permissions phone cards show one field per role; limit them to 2 or 3 key fields.
- The sign-in page has fixed 24 px padding; use 16/24/32 px by breakpoint.
- Stacked field labels should use caption weight 500.
- The Work queue phone label "Queue" can collide with a Record Type label; make the stub name unique.
- A stray focus box stays on the sidebar brand after pointer navigation; show focus-visible only.
- Ledger inner tables scroll horizontally on phones.
- The two residual token-guard limits.
