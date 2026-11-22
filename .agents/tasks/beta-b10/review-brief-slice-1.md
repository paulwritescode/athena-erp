# Review brief: B10 slice 1, art registry + core placements

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b10, branch feat/b10-art
(commit f1d620d). Brief: ../athena-erp/.agents/tasks/beta-b10/brief.md
(slice 1 only). Follow .kiro/specs/athena-beta/design.md section 5 reviewer
workflow, docs/design/athena-design-system.md (section 9 is this task; also
§7, §13, §14), AGENTS.md and .kiro/steering/athena-design.md.

Review the branch diff against main (git diff main...HEAD):

1. `web/src/art/registry.ts`: placement key → figure/ASCII, size, label;
   `useArt(key)` returns nothing for unlisted keys; figures lazy-loaded
   (bundle carries only the figure in use). No page imports a figure
   directly — all through the registry.
2. Placements: sign-in, 404, offline pages + ASCII mark for the CLI banner
   and sign-in footer. §9 rules: at most one figure per screen; empty
   160–240 px (120 mobile), sign-in 200–280 px; line color from
   `--app-text-subtle`, theme auto, intensity ≤0.3, play off; meaning table
   (Padlock sign-in, etc.); aria-hidden or labelled; ASCII ≤8 lines mono
   caption. Reduced motion rests every figure.
3. Tests: at most-one-figure-per-screen over the registry; unknown-key
   rejection; reduced-motion behavior; CLI banner shows the ASCII mark.
4. SCOPE NOTE (do not block on this): custom Athena figures
   (`web/src/art/figures/` via the hairline-create skill) are ABSENT — the
   skill is not installed and cannot be fetched under the no-network policy.
   Slice 1 correctly ships stock hairline figures for every placement; the
   custom set is a recorded follow-up. Verify stock coverage is complete and
   the registry accepts future custom figures without rework.
5. Run: CI=true pnpm --dir web run typecheck, test (exit 0), build; gofmt,
   go build, go vet, make test (CLI banner side); plus a browser pass on a
   throwaway Workspace (sign-in, 404, offline at 375 and 1440 in both
   themes). Never the dev database in .env.local. Note: only one Athena
   server runs per $HOME; the user server holds the real-HOME lock, so run
   any throwaway server with a fake HOME and never stop another agent's
   server.

Merge-small rule: only blockers stop the merge; the rest are follow-ups.
Write findings to ../athena-erp/.agents/tasks/beta-b10/review-slice-1.md
with a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers
first with file:line. Then report: hc send master "B10 SLICE 1 REVIEW: PASS
or NEEDS_CHANGES <one line>". Do not edit product code, commit, merge, push
or start agents. Permission policy: stay inside athena-erp and athena-erp-*
folders, nothing destructive.
