# Review: F3 slice 1 field sheet (661bcef)

Verdict: PASS

Tree: athena-erp-f3-s1-r, detached at 661bcef on 96acb15, clean. Diff is
slice-1 scope only (6 files): FieldSettingsDialog.vue + test, fieldSettings.ts
+ test, glossary.ts, RecordTypeEditorPage.vue. No F4 code (zero
ConditionBuilder references), no F4 files, no proto/Go changes.

1. Sheet: DetailSheet right sheet on desktop/tablet (480 px,
max-w calc(100vw-2rem), inset-y-0 h-full), BottomDrawer on phones; body
.min-h-0 flex-1 overflow-y-auto scrolls inside. Mount test asserts the real
classes; browser fit assert checks bounding rect within viewport + computed
overflow-y auto. Basics open, Rules/Advanced collapsed. The field-identity
watcher resets sections + totals picks; the regression test swaps the field
without unmounting, which mirrors the opener (`settingsField = field` on an
always-mounted dialog). Genuine coverage. Note: the first browser FAIL was a
first-expansion timeout while the reported bug was cross-field persistence;
both behaviors are now covered (mount tests + 2 green browser runs), so this
is archaeology only, no action.
2. Editor grant respected: label/type/required/target moved into Basics; the
row reads label/type/required text + key input only; the inline Link
relationship editor is gone (moved to the sheet); helpers moved byte-identical
into the shared module (same keep-set, defaults, reserved keys); recordTypes +
currentTypeName passed. Identity chain verified: currentRecordType is a live
find() into the same library array, so siblings and unsaved types are the
editor's own objects — no self-load. No other editor logic changed; typecheck
proves no dangling refs.
3. Selects real: permission 0-9 (Level 0 deletes the attribute), link targets
over live passed types excluding the edited type with a stale-key fallback
row, totals composer (fn + child + field) over child numerics. sum/min/max/
count all exist in internal/expr with list(decimal)/list(money) overloads
matching the composed `fn(doc.child.field)` shape; server tests use the same
shape. Output parses.
4. Plain words: zero doc./dec( outside placeholder attributes (5 expression
placeholders, all exempt per variance); the scan test covers labels, notes
and h2s. The totals preview shows composed formula text (data, font-mono,
not help). 11 glossary terms, all wired; wording sane except one nit below.
5. Variances: help stays in LayoutItemSheet — confirmed, zero 'help' in the
dialog and no Field-model change. Money/Quantity/Enum blocks in Basics —
VERIFIED: kinds.go refuses Money without currency+scale, Quantity without
uom, Enum without a choice, and changeType seeds exactly those, so the blocks
are needed to create those types without opening Advanced (quantity cap 6
matches the input max). Expression placeholders bridge — verified: stored as
written, length-checked against the server's 1000, server parses on save;
writers are not stranded.
6. Gates: affected web green on clean 661bcef re-verified in the suite log
(70 files / 670 tests, exit 0); browser PASS re-verified (sections, totals
preview, permission, link-target, fit asserts; 6 shots at 375/768/1280 x
light/dark eyeballed by this reviewer, 0 page errors, make build incl. vite
green). Re-ran here: 24 slice tests green, vue-tsc clean, go build/vet clean,
gofmt empty, tree still clean.

Nits (non-blocking, suggest slice 2 or a fast follow-up):
- N1: the composer offers Count for Money targets, but count() returns int and
  formulaResultFits rejects int for Money — the server refuses on save with a
  clear message. Consider offering sum/min/max only for Money.
- N2: glossary `unique` explanation 'A deleted master keeps its value' leaks
  the 'master' kind word into user-facing help and reads cryptic; suggest
  'Deleted records keep their values.' if that is the meaning.
