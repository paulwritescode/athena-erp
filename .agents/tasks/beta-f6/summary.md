# Summary: F6 live ASCII logo

Merged 2026-11-24 as 7911ac1/7c4f37a/f27e882/bcd2913 (cherry-picks of 4b3a667/3e3a7c1/09a5fe1/9d841c8 onto post-F1 main), 4 commits.

Scope: tools/asciilogo generator (deterministic, olive-tree art → 40x8 frames, light+dark, gamma-documented) + docs/design/ascii-logo.md; logo registry kind + AsciiLogo draw-in (150→180ms steps post-fix, opacity-only, static under reduced motion) on sign-in, first-run, 404, offline (≤1/screen); static CLI tree banner for bare `athena` + `athena version` (version line untouched for F8) + sync test; design-system 9.6 + overview rows.

Evidence: 32/32 full-page shots (4 surfaces x 4 widths x 2 themes, in-run asserts logos=1 figures<=1 rows=8, 8 eyeballed); 23/23 art vitest + Go banner/version + typecheck green; full make test green pre-merge. Browser war story: 5 harness-only attempts (DOM login timeout, null-body navigation, setup-readiness roles, slot held via waiter-subshell pipe — all fixed in-harness, zero app failures). Merge gate on b1f5bd4: 507s, 17 Go ok (server fresh 489s DB-backed, cmd/athena 209s), 646 web passed, zero FAIL. Pre-existing finding (not F6): offline plug figure never paints (lazy chunk vs offline); F6 logo renders (frames in main chunk).

Review: r-f6 NEEDS_CHANGES then PASS — the one issue (generator -step-ms default 150 vs doc/frames 180, regen would not reproduce) fixed one-line by w-f7 (9d841c8, regen byte-identical); re-review verified the delta + green affected with seeded env (an early 29s affected ran without .env.local in the -r tree and was re-run to confirm).
