# Review brief: B9 form sub-slice 1, View renderer + Image field

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b9form, branch
feat/b9-form-builder (commit dcaef0f). Brief: ../athena-erp/.agents/tasks/
beta-b9/form-brief.md (sub-slice 1 only). Follow .kiro/specs/athena-beta/
design.md section 5 reviewer workflow, docs/design/athena-design-system.md
(§4, §7, §13, §14), AGENTS.md and .kiro/steering/athena-design.md.

Review the branch diff against main (git diff main...HEAD):

1. The View record form renders from the layout: sections, 1|2 columns,
   field order, widths, help text, per-section visibility CEL (display-only,
   inline type-check errors), static text and image blocks. Art blocks render
   a quiet placeholder (the art registry is B10; it must NOT be built here).
2. Image field: upload with preview, alt-text input required with inline
   error, display + thumbnail (via GetFile thumbnail), replace, clear — in
   the record editor and View form. Editor Fields tab offers the Image type
   with inapplicable attributes hidden.
3. Image value rules mirrored in web/src/fieldRules.ts plus the shared
   internal/server/testdata/field_rules.json table, Go and Vitest green
   together (layout round-trip tests present).
4. Scope: NO records-list edits — RecordsTable.vue untouched, no DataTable
   or record-column changes (list-image thumbnails are a post-B2a follow-up).
   Renderer code lives in new web/src/forms/ files; RecordsPage edits are
   editor-only. No hex/rgb/px outside tokens.
5. Run: CI=true pnpm --dir web run typecheck, test (exit 0), build; gofmt,
   go build, go vet, make test (field-rules sides); plus a browser pass on a
   throwaway Workspace at 375, 768, 1280, 1440 in both themes (layout
   sections/columns/visibility, image upload/alt/replace/clear, keyboard
   operability). Never the dev database in .env.local. Note: only one Athena
   server runs per $HOME; the user server holds the real-HOME lock, so run
   any throwaway server with a fake HOME and never stop another agent's
   server.

Merge-small rule: only blockers stop the merge; the rest are follow-ups.
Write findings to ../athena-erp/.agents/tasks/beta-b9/review-form-1.md with
a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first
with file:line. Then report: hc send master "B9 FORM 1 REVIEW: PASS or
NEEDS_CHANGES <one line>". Do not edit product code, commit, merge, push or
start agents. Permission policy: stay inside athena-erp and athena-erp-*
folders, nothing destructive.
