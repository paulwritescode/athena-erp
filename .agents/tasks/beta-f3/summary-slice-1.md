# Summary: F3 slice 1 (field settings sheet)

Merged 2026-11-24 as a82d934 (cherry-pick of 661bcef onto post-train main), 1 commit.

Scope: DetailSheet shell (right sheet desktop/tablet, drawer phone, viewport-safe, inner scroll) + sections (Basics open, Rules/Advanced collapsed, reset per field) + shared field-settings module + label/type/required/target moved from the RecordTypeEditorPage table into Basics (granted minimal edit; editor row reads text + key) + permission/link-target/totals picks as live selects (totals composer writes sum/min/max/count formula) + plain-word labels + one-line help + 11 glossary terms, no doc. syntax in labels/help. Variances (all disclosed + ruled): help stays in LayoutItemSheet (no Field model), expression example placeholders kept as the slice-1 bridge, Money/Quantity/Enum blocks in Basics. No F4 builder code (slice 2 waits for the F4 builder merge).

Evidence: affected web green (70 files/670 tests); browser PASS (seed 3 types; sections/collapse/totals-preview/permission/link-target/fit asserts; 6 shots 375/768/1280 x light/dark inspected, 0 errors; vite build green); vue-tsc clean. Browser caught 1 real bug (section state persisted across field swaps — opener never unmounts) fixed with a field watcher + regression test. Merge gate on fe1f302: 1223s, 17 Go ok (server 1117s, posting 1161s DB-backed — the run that proved auth starves its siblings), 670 web passed, zero FAIL.

Review: r-f3s1 PASS (review-slice-1.md) — sheet/selects/plain-words verified, gates re-checked, 2 nits.
