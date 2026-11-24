# Review: F3 slice 2 compact builder per condition (804c9fa, fix 2510c6a)

Verdict: PASS (on the 2510c6a fix; was NEEDS_CHANGES on 804c9fa)

Tree: athena-erp-f3-s2-r, detached at 804c9fa on ff18cb2, clean.
Diff is slice-2 scope only (4 files): FieldSettingsDialog.vue + test,
fieldSettings.ts + test. No F4 files touched (ConditionBuilder and
compileTree used read-only via props/import); no Go, proto, migration
or docs changes. Formula stays written-only (line 348, no toggle),
per the brief's by-design rationale.

## Verified green

- Gates (reused through the main-repo suite on this exact clean
  commit; both logs read, not just trusted): affected web 74 files /
  723 tests green, exit 0 (1124-230900; FieldSettingsDialog.test.ts
  16 tests and fieldSettings.test.ts 14 tests both green, zero
  failures); browser 59 oks / PAGE_ERRORS 0 / BROWSER PASS
  (1124-231447; the one "fail" grep hit is the passing assert label
  "seed failed: seeded"). vue-tsc clean and vite build green are
  inside the browser run's `make build` (run.log).
- Worker failure archaeology: the two exit-1 pnpm builds forgot
  CI=true (environmental); the first browser FAIL ("four condition
  blocks, found 12") was the worker's first scenario draft — the
  final scenario asserts exactly 4 and passes twice on this code.
- Mapping claims hold exactly: `builderKindFor` matches server
  `exprKind` (internal/server/expressions.go: Number/Currency/
  Quantity-decimal, Money-money, Date-date, Checkbox-bool, else
  string) and is line-identical logic to RulesPage `fieldKind`;
  `builderFieldsFor` doc-qualifies names and carries
  currency/scale/choices like the Rules page.
- Round-trip covered in unit tests (build compiles live, Written
  shows it, tree survives, typed text survives a peek, stored text
  opens as Written, remove clears the attribute) and in the browser
  scenario (two-row Status-is-Paid AND Is-credit-checked build,
  compiled `(doc.status == 'paid' && doc.is_credit == true)`,
  round-trip, kept-note, viewport fit assert).
- Shots: 6/6 eyeballed (375/768/1280 x light/dark in
  /tmp/f3s2-shots). Light shows the built two-row condition +
  Written doc.paid; dark shows fresh empty builders (scenario
  re-navigates without saving — expected). Toggles are
  token-driven with aria-pressed/group labels; labels + help stay
  plain words (doc. syntax only in the slice-1-exempt
  placeholders); sheet fits, drawer on phone.

## F1 (blocking): a Builder peek deletes the stored expression

`switchConditionMode` to builder drops the tree and `mirrorCondition`
(FieldSettingsDialog.vue:76-87, the `delete` at line 80) deletes the
field attribute whenever the tree compiles to ''. So: Written text
(say saved `required_if = doc.paid`) -> click Builder -> the stored
value is deleted from the in-memory field immediately. It is restored
only by switching back to Written. Close the sheet or open another
field first, then save the Record Type, and a value the user never
edited is silently gone. The kept-note warns, but a mode tab should
not arm deletion, and the file's own comment ("a peek never loses
it", lines 43-51) is false for peek-then-close/save. The Rules page
model this follows uses confirmed-discard dialogs; the note-only
version does not guard it. Under design.md section 5, silent data
loss blocks.

Fix (3 lines, all existing tests trace green unchanged): in builder
mode with `compiled === ''`, restore kept text instead of deleting:

```js
if (compiled !== '') props.field[name] = compiled
else if (keptText(name)) props.field[name] = condTexts[name]
else delete props.field[name]
```

Clear-through-the-builder still deletes (setTree clears condTexts on
a finished build, so keptText is false there). Add a regression test:
stored `required_if = doc.paid`, switch to Builder, assert
`state.required_if` is still `doc.paid` while the kept note shows.

## Re-review of the 2510c6a fix: F1 closed

The fix (2 files, .vue + test) changes the builder branch to store
compiled output when non-empty, leave the field untouched when written
text is kept, and delete only when nothing is built and nothing is
kept. Leaving-as-is is sound: whenever condTexts is non-blank the
field already holds that text (set together on every keystroke, and
the text-mode compiled branch only runs with cleared condTexts), so
all paths keep or supersede correctly — build supersedes, un-complete
and remove clear, peek preserves. The comment and the kept-note copy
("A written version is stored — switch back to Written to see it.")
are now accurate.

Regression proof spot-checked and holds: pre-fix code deleted the
attribute unconditionally on empty-builder mirror, so the new
`expect(state.required_if).toBe('doc.paid')` immediately after
switching to Builder (round-trip test, clears test, and the new
close-mid-peek test) necessarily fails pre-fix. The `closed` helper
the new test uses is pre-existing. The new browser close/reopen
asserts are persistence coverage (they pass pre-fix too in the state
the scenario closes in) and the worker described them as such.

Gates re-verified through the suite on clean 2510c6a (reused, logs
read): affected 74 files / 724 tests green (dialog file now 17
tests); browser 66 oks / PAGE_ERRORS 0 / BROWSER PASS. The one
exit-3 browser run between was a viewport-sequencing timeout in the
new close/reopen block; the worker fixed the scenario (mtime 23:39)
and the final run is green. 6/6 regenerated shots eyeballed
(375/768/1280 x light/dark): built condition + Written doc.paid on
light, fresh empty builders on dark, no visual regression.

## Non-blocking (carried)

- N1 (follow-up for F4, not this slice): the empty-builder copy
  ("the rule matches every record") reads Rules-flavoured in the
  sheet, as the worker disclosed; F4 files are read-only here.
  Suggest F4 take a compact/copy prop later.
