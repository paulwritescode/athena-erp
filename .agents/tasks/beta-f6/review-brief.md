# Review brief: F6 live ASCII logo (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-f6-r, detached at 09a5fe1 (3 commits on 4acb8d4). Verify clean + `git log` shows the 3 F6 commits. Read .agents/tasks/beta-f6/brief.md and the worker's evidence (night log 06:00 lines / w-f6 INFO). Design authority: docs/design/athena-design-system.md section 9.6 + ascii-logo.md.

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Generator: tools/asciilogo deterministic (run twice, byte-identical), frames 40x8 read as the tree, light+dark variants, gamma documented, no letters.
2. AsciiLogo.vue: draw-in 120-240ms steps, transform/opacity only, static under reduced motion; registry kind; exactly ≤1 figure per screen on all 4 surfaces (sign-in, first-run, 404, offline).
3. CLI banner: bare `athena` + `athena version` show the static tree; version line untouched (F8 owns it); sync test real.
4. Shots: eyeball /tmp/f6-shots/ (32: 4 surfaces x 4 widths x 2 themes) — logo renders every surface/theme, rows=8, no layout breakage at 375, dark true-dark. Worker eyeballed 8; spot-check the rest for undisclosed breakage.
5. Tests: 23/23 art vitest real (not shape-only), Go banner/version tests, typecheck/build green; affected or full gate per suite; generate stable.
6. Confirm the worker's finding is pre-existing (offline plug figure never paints — verify on clean main, not caused by F6). File follow-ups, don't block.

Report: say master "F6 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-f6/review.md with the verdict line.
