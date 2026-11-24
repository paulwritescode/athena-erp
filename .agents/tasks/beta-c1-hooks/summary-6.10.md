# Summary: C1 6.10 Desk voucher view

Merged 2026-11-24 in train4 (tip bafe51d), 1 commit 9a299ca (d13f4e0 rebased clean on 4acb8d4, content identical).

Scope: preview dialog (ResponsiveDialog, plans + legs, submit carries the plan hash per R6.15) on drafts; Books-entries DetailSheet (status groups, ledger lines, attempts log, show-more paging) on submitted/cancelled; Recorded-in-the-books badge; blocked-post Sonner toasts in plain words; one-line PageSwitch canLedger prop. Files: RecordsPage.vue (+445), RecordsPage.test.ts (new, 8/8), PageSwitch.vue (+1), pageSwitchBuild.test.ts (+1 mock).

Evidence: web suite 64 files / 624 tests green, typecheck/build/vet/gofmt/generate green; browser retry 6: 25/26 (the 1 fail is harness-class: drive asserted capital Cash, sheet shows lowercase cash), 15/15 shots eye-checked at 375/768/1280/1440 x light/dark with disclosed gaps (light sheet widths uncaptured, reversed shot mid-animation, 375 credit tightness). Train4 tip gate: 17 Go pkgs ok, 632/632 web, zero FAIL. No Z2 touch.

Review: r-c1 PASS (review-6.10.md), 5 non-blocking follow-ups in tasks.md (account_label server-side, engine-prefix leak, 375 credit clip, reversed badge variant, re-capture reversed settled).
