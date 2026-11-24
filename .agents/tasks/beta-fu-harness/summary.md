# Summary: FU-H shared browser harness

Merged 2026-11-24 in train4 (tip bafe51d), 2 commits cb9a3c3 (driver + example + README) + 1e1450f (root-via-reset sign-in; owner lacked build, disclosed).

Scope: one dependency-free Node CDP driver at .agents/bin/browser (start athena on a free port with its own test DB + fake HOME, create first user, sign in, wait for the Desk by stable selector, set theme via class toggle, visit routes, screenshot, list console errors, always exit + close Chrome; exits 0 pass / 1 app failure / 3 harness failure) + example script + README. Every lane runs browser passes through `suite -- .agents/bin/browser <script>` from here on; no more /tmp drivers.

Evidence: self-test 16/16; 2 consecutive green example runs + both failure drills + second-worktree run through suite, no orphans (logs 1124-050940/051010 exit 0). Train4 tip gate: 17 Go pkgs ok, 632/632 web, zero FAIL.

Review: r-fuh PASS (review.md) — driver proven on the reviewer's own green suite run (97s, exit 0). 6 non-blocking follow-ups in tasks.md (CDP close handler + fetch timeouts, stale example comment, README wording, --db collision note, B11 owner echo, SIGINT/SIGTERM handlers).
