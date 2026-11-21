# Brief: B9b, name-first New Record Type flow (Muse t1b)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b9b, branch feat/b9b-new-type. Scope: web/src (Record Types list and editor pages and their components). No Go unless an existing RPC cannot save a Record Type with one default field.
Read: docs/design/athena-design-system.md section 20 (this task), sections 7, 13 and 14; tasks.md B9b.
Do what section 20 says:
- "New Record Type" (the list header and the empty state) opens a ResponsiveDialog with Name (required, autofocused), What is it for? (optional), and Kind (with one-line explanations, default Master).
- The key is derived from the name with the existing naming rules, shown as "Saved as <key>" with a Change link.
- Create is disabled until the name is valid and unique. Errors appear inline.
- Create saves a real Record Type with one required Name text field, then opens /build/record-types/<key> on the Fields tab.
- The editor title is the name, editable in place, with Rename in the header menu.
- Remove the old unnamed-draft path, including /build/record-types/new opening a blank editor. That URL now opens the dialog over the list.
- Also fix the B9a follow-ups in these files: Delete separated from Save in the destructive slot, title as the h1, field-row action icons, Required checkbox alignment.
Checks: typecheck, test (exit 0), build. Vitest for key derivation and the create validation. Browser pass with a throwaway Workspace at 375 and 1440: create, duplicate name, rename. Do not commit, merge, push or start agents.
Report: hc send t6 "B9B: PASS or BLOCKED".
