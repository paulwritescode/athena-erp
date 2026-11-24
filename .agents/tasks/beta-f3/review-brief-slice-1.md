# Review brief: F3 slice 1 field sheet (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-f3-s1-r, detached at 661bcef (on 96acb15). Verify clean + diff is slice-1 scope only: DetailSheet shell + sections + shared field-settings module + editor table move + selects + glossary + tests.
Read: .agents/tasks/beta-f3/brief.md slice 1; the granted-scope ruling + variances (night log: editor grant, help variance, composer in, placeholders, blocks in Basics).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Sheet: right sheet desktop/tablet, drawer phone; never exceeds viewport, scrolls inside (verify the overflow asserts actually constrain); sections Basics-open + Rules/Advanced-collapsed; state resets per field (the fixed bug — verify the watcher + regression test genuinely cover the opener-never-unmounts case).
2. Editor grant respected: label/type/required/target moved into Basics via the shared module; editor row reads text + key input ONLY — fail on ANY other editor logic change.
3. Selects real: permission/link-target/totals picks are true selects over live data (incl. unsaved recordTypes passed down — verify no self-load staleness); totals composer writes valid sum/min/max/count formula text (verify output parses).
4. Plain words: labels + one-line help glossary-backed, no doc. syntax in labels/help (placeholders exempt per variance — verify they are ONLY placeholders); 11 glossary terms sane.
5. Variances: help-in-LayoutItemSheet (accepted — confirm no Field-model hack); Money/Quantity/Enum blocks in Basics (needed-to-create claim — verify or challenge); expression placeholders as bridge (verify they do not strand: writers can still write expressions as validated text).
6. Gates: affected web green (70 files/670 tests claimed — verify); browser PASS (6 shots 375/768/1280 x light/dark eyeball; asserts incl. fit verified in log); vue-tsc clean; NO F4 builder code (fail if present); NO F4 files touched.

Report: say master "F3 SLICE 1 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-f3/review-slice-1.md with the verdict line.
