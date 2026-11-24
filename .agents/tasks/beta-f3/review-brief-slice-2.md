# Review brief: F3 slice 2 (compact builder per condition)

Worker: w-f2. Branch `feat/f3-sheet`, commit 804c9fa on origin/main ff18cb2.
Scope: ConditionBuilder embeds (Builder/Written toggle per condition) for
required/read-only/must-be-true/show-when in FieldSettingsDialog.vue;
fieldSettings.ts changes; tests. Formula stays written-only BY DESIGN (builder
emits booleans incl. today(), which formulas forbid). No docs touched.

Verify: worker evidence (vue-tsc clean; affected 74 files/723 tests green;
vite build green; browser 59 oks/0 page errors, 6/6 shots 375/768/1280 x
light/dark eye-checked in /tmp/f3s2-shots; round-trip unit + browser), F4
files read-only (untouched), re-run affected + browser through the main-repo
suite by absolute path (or verify reuse), eyeball the shots.

Verdict line: `Verdict: PASS` or `Verdict: NEEDS_CHANGES`.
Report to master with `say master "F3 SLICE 2 REVIEW: PASS/NEEDS_CHANGES <one line>"`.
Reads only in the detached tree; never merge/push/spawn.
