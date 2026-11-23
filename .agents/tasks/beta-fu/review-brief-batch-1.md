# Review brief: FU batch 1 — Desk polish (12 fixes)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-fu-r (detached at 425791b;
verify clean on arrival). Brief:
../athena-erp/.agents/tasks/beta-fu/brief.md. Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and AGENTS.md.
Design authority: .kiro/steering/athena-design.md +
docs/design/athena-design-system.md (Desk rules that decide each fix).

Review the 12-commit diff (git log main..HEAD, oldest db6a7fd → 425791b;
11 files: appearance.ts/ts-tests, AppearanceSection/Preview, TextField,
RecordTypeCreateDialog, useDeskRoute, fields.test, style/theme.css,
cmd/athena/appearance.go usage text):

1. One fix per commit, each matching its bullet (B9a x2, B4 x1, B4b x6,
   B9b x3): verify the code does what the message claims, against the
   design system (tokens not literals, 4px spacing, Satori/Inter scope,
   Lucide-with-words, pointer cursors, class-based theme).
2. Skips (verify each): B9a x3 already satisfied by B9b (check the tree);
   B4 Security copy already plain on main; --chart-palette flag out of
   usage-text-only scope; B2a slice 1 deferred to batch 2 (confirm none
   of its items were half-built).
3. Scope: NO server logic, no proto/gate/migration, no
   RecordTypesPage/FieldSettingsDialog/fieldRules.ts (L3-owned), no
   App.vue, no api.ts beyond nothing. Confirm from the diff stat.
4. Eyeball all 6 shots (/tmp/fu-*.png, 1280px, both themes) with the
   image-reading tool: each claimed fix visible where claimed; no
   layout regression elsewhere in frame; dark theme coherent.
5. Worker's note: "theme menu stays open after a theme pick in the CDP
   run (worked around; may be a JS-click artifact, out of FU scope)" —
   judge: harness artifact or real Desk bug? If plausibly real, file as
   a follow-up with repro steps (do not block unless a shot shows
   user-visible breakage).
6. Gates: CI=true pnpm --dir web run typecheck, the touched Vitest files
   (appearance, fields), vite build; .agents/bin/affected by the ABSOLUTE
   main-repo path (same lock; reuse of the worker's green run on
   425791b is fine — read that log). No full browser re-run (worker's
   exit-0 pass + your shot eyeball is the evidence); tree clean before
   any gate. Node via the suite toolchain path if missing.

Merge-small rule: only blockers stop the merge (fix does not do what it
claims, design-system violation, scope breach into server/L3 files,
regression visible in a shot, red gates); the rest are follow-ups. Write
findings to ../athena-erp/.agents/tasks/beta-fu/review-batch-1.md with a
verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first
with file:line. Then report: say master "FU BATCH 1 REVIEW: PASS or
NEEDS_CHANGES <one line>". Then wait for instructions. Do not edit
product code, commit, merge, push or start agents. Send UPDATE every 10
minutes.
