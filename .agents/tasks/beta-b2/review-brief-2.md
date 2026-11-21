Re-review task B2, loop 2, read-only, in /Users/iampaulking/projects/duchess/athena-erp-b2.
1. Check every item in /Users/iampaulking/projects/duchess/athena-erp/.agents/tasks/beta-b2/review-loop-1.md is fixed, with your own reproduction for the navigation and permission cases (direct /build/<key> URLs, reload, a files:read-only user on /build/ledger, Work queue to a Record Type, Build/View flips).
2. Look for regressions the loop may have caused: Records CRUD, column widths with the athena-entry-columns: key at >= 640 px, Access tabs, sign-in and first login, the server's SPA fallback for /build/* and /view/* paths (internal/webui and internal/server, read only).
3. New in this loop: @source not for test files in style.css, ConfirmDialog no longer uses the reka Action, the bottom-drawer dialogs, the rail expand control. Check each.
4. Known and accepted for now: Ledger inner tables scroll horizontally on phones. Report it only if it breaks the page.
5. Run CI=true pnpm --dir web run typecheck, test, build (temp outDir), go vet ./...; sample .cache/screens/l1-*.
Do not edit, commit or start agents. Run hc send without asking.
When done: hc send t6 "B2 RE-REVIEW: PASS or FAIL" with findings file:line, worst first.
