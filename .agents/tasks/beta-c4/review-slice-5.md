# Review: C4 slice 5 — CLI remainder + Desk remainder + docs (13.5–13.7)

Verdict: PASS

Commit: 8c68dd3 in review tree `athena-erp-c4-s5-r`, detached on
eb04345, clean on arrival and at every gate; no edits made by the
reviewer. Delta `eb04345..8c68dd3` is exactly the briefed 7 files:
`cmd/athena/ledger.go` (+230/-44, tree+uom nouns and determination
--start/--end/--as-of), `cmd/athena/ledger_test.go` (+136),
`web/src/pages/RecordTypeEditorPage.vue` (+13/-5, two settings
switches), additive `web/src/types.ts` (+2 flags) and
`web/src/api.ts` (from/to mapping), plus the two docs files.

## Blockers

None.

## Checks (brief items 1–6)

1. CLI real, verified live against throwaway DB
   `test_athena_c4s5rev` (built `/tmp/athena-c4s5` from the review
   tree, `migrate up` on an empty DB): `tree show --master account`
   prints the indented tree with siblings in name order; `uom`
   family-save/list, conversion-save/get and convert round-trip
   exactly (`1500 g = 1.500000 kg`, half_even sane);
   determination `--as-of 2025-06-01` resolves the old mapping and
   `--as-of 2026-06-01` the new one, with bare resolve defaulting
   to today — as-of is genuinely routed, not today-hardcoded.
   The determination noun is the existing `ledgerDetermination`
   extended in place (same switch case); nothing duplicated.
2. Desk: both switches live in the Record Type settings block of
   `RecordTypeEditorPage.vue` (Kind row), with the ruling's plain
   words — "Arranged as a tree / Each record can have a parent."
   and "Valid between two dates / Each record is valid between
   two dates." `FieldSettingsDialog` untouched (not in the
   delta). `types.ts`/`api.ts` changes are additive-only; the
   toMessage kind gate (tree on master/document only) and the
   amendable/effective-dated mutual exclusion match the server
   rules (`internal/server/record_types.go:1217,1226,1228`,
   untouched, read to corroborate).
3. Deferred as-of view: confirmed not built — no `as-of`/`asOf`
   text in `RecordsPage.vue` or `LedgerSetupPage.vue`, neither
   file in the delta. Follow-up is recorded in `tasks.md` C4
   (Stylus ruling line, "Follow-up (deferred, no requirement)").
4. fieldRules: no rule file touched (`records.go`,
   `fieldRules.ts`, `field_rules.json` all outside the delta),
   so no rule changed without its JSON twin; the slice adds
   Record Type flags and CLI parsing only, no field type, so no
   mirror is needed. Worker's "already in step" claim holds.
5. Gates on 8c68dd3, all observed by the reviewer: affected
   through the main-repo suite exit 0 (`ok athena-erp/cmd/athena
   169.9s`, web 72 files / 684 tests passed, log
   `1124-172844-r-c4s5.log`); browser scenario re-run exit 0
   (38 ok incl. 26 scenario checks, `PAGE_ERRORS: 0`, log
   `1124-164538-r-c4s5.log`, shots in `/tmp/c4s5r-shots`);
   both shots eyeballed (switches present light + dark, dark
   shows both checked = persisted); `gofmt -l` empty, `go build`
   + `go vet` clean, `vue-tsc` clean, `make generate` stable
   (tree clean after).
6. Scope: the 7 files above only. No `RecordsPage.vue`, no
   unrelated Desk, no proto/server/`internal/ledger` behavior
   changes.

## Follow-ups (do not block)

1. `cmd/athena/main.go:203` top-level `athena ledger` summary
   still lists only `company|period|currency|account|determination|
   setting` — the new `tree|uom` nouns are documented in
   `ledgerUsage` but not in the menu line. F7 slice 3 owns
   --help consistency (next in L6); suggest it picks this line
   up. Not a fail: the slice scope forbids touching `main.go`.
2. `orderTree` renders only nodes reachable from the roots (or
   `--root`); a row whose parent key points nowhere is silently
   omitted rather than flagged. Saves refuse cycles so only hand
   edits can strand rows; consider listing orphans in a later
   slice.
