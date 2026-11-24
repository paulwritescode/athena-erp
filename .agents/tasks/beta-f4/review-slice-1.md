# Review: F4 slice 1 — conditions engine + builder + Rules modes + lists core

Tree: `athena-erp-f4-s1-r`, detached at `6474b04` (bottom 16 of feat/f4-design).
Reviewer: r-f4s1. Date: 2026-11-24.

Verdict: PASS

Slice 1 delivers the literals-only core end to end: the expr condition
tree with round-trip, the Desk builder beside Starlark, builder Rules that
execute without Starlark, and ListService CRUD/run/PDF export. No slice 2
or 3 surface is present.

## 1. Engine — pass

- `internal/expr/tree.go` (1012 lines): `CompileTree` renders the canonical
  subset for all 22 operators; `ParseTree` accepts exactly that shape and
  refuses anything else with "not a builder expression". Validation covers
  unknown fields, op-on-kind, bad decimal/date/day-count/int, money fit,
  value counts, empty root, nested singles.
- `tree_test.go`: 21 round-trip cases (all ops incl. quote/backslash
  escapes, contains regex-meta, nesting, any/none-of) plus JSON
  round-trip, single-choice normalization, 10 compile errors, 9 parse
  refusals. Ran: `go test ./internal/expr/` green.
- `date()` literal (bc6b071): eval + error cases + SQL `Translate` with
  args check. Green in the same run.
- TS mirror (`web/src/conditions.ts` + `conditions.test.ts`): 25/25 pins
  byte-identical to the Go expectations. Ran green.
- Note: design §2.3 writes `athena.today`; the code emits the `today()`
  macro, which expands to `athena.today` (`expr.go`). Same thing; doc
  wording only.

## 2. Builder — pass

- `ConditionBuilder`/`ConditionGroup`/`ConditionRow`: field → operator →
  value rows, per-group match-all/any, nesting, live compile, "Show as
  expression", inline errors, empty state. Component tests (6) green.
- Gallery story renders the builder with demo fields.
- Rules page: Builder/Starlark toggle for validation/guard/derivation
  only; message editor, derivation sets editor, one-way scaffold and
  confirmed-discard switch dialogs. Posting/reaction/scheduled stay
  Starlark-only. Client sends the tree, never the expression.
- Server (`rules.go` + `rules_engine.go`): save compiles the tree
  authoritatively and stores `checked.compiled` (client `condition_expr`
  ignored); execution routes on stored `ConditionExpr` through the
  existing lifecycle registry (`runBuilder`) — no new execution path;
  writes go through `doc.Set` with user-save checks. Starlark path
  untouched (default branch).
- End to end without Starlark: `rules_builder_test.go` — validation
  rejects with message and logs ok/rejected runs, guard blocks submit,
  derivation sets `total` to 6, 8 save-validation refusals, simulate
  rejected/ok. All green inside the full suite; Starlark regressions
  green (whole `internal/server` package).

## 3. Lists core — pass

- Migrations 23 (`athena_lists`) and 24 (builder columns) apply in the
  contiguous 1..24 chain — every DB-backed test in the suite migrates it
  green.
- `ListService`: Save/Get/List/Delete/Run/ExportListPdf; save compiles
  the tree server-side; run/export reuse `recordService.ListRecords`
  (permissions/Scopes/paging identical); export caps 200/500 rows,
  resolves Link labels via cache, degrades branding to name-only.
  `lists_test.go` (CRUD + revisions + validation + dup + run + export
  incl. U+0169) green.
- gopdf renderer: Inter Regular/Bold embedded (vendored TTF, OFL text
  kept), wrapped cells, header with workspace/logo/title/filter/time
  repeated per page, hand-rolled page breaks. Golden 3-page test has
  structural asserts (page count, FontFile2, U+0169 CMap) plus
  normalized-bytes compare — ran 3x green (deterministic).
- CLI `athena lists export <id> --pdf` wired in `main.go` with usage;
  arg tests + full export flow test green.

## 4. PDF diligence — pass

- `go.mod`: `signintech/gopdf v0.38.6` + indirect `gofpdi
  v1.0.14-0.20211212211723` + `pkg/errors v0.8.1` — exactly the closure
  ADR-0034 records. No `import "C"` in any of the three modules; gopdf
  LICENSE is MIT. Fonts are vendored SIL OFL, no network at render.

## 5. Proto — pass

- New `lists.proto` (6 RPCs, all with `procedurePolicies` + destructive
  entry for Delete) and `RuleDefinition` fields 9–12 + `DerivationSet`:
  additive only. `make generate` stable (no diff, tree clean).
- No `QueueMessages`, no `lists:send` anywhere in the slice (grep clean).

## 6. Gates — pass

- Full `make test` on clean `6474b04` through the suite lock: exit 0,
  1448 s, log `1124-214745-r-f4s1.log` — 18/18 Go packages ok (server,
  expr, lists, cmd/athena included), 70 web files / 679 tests pass.
- `go build ./...`, `go vet` (touched packages), `gofmt -l` empty,
  `vue-tsc --noEmit` exit 0, `vite build` exit 0 (1.39 s; dist restored
  after).
- Shots eyeballed: `rules-builder.png` shows the Rules page with the
  Builder/Starlark toggle rendering correctly. `reports-*.png` show the
  stacked-slice Reports lists UI in the worker tree (out of scope here)
  and confirm the slice-1 list run/export backend works through a UI.

## 7. Scope — pass

- Bottom-16 file list: expr, lists, server rules/lists, CLI, proto+gen,
  conditions components, RulesPage, Gallery story, ruleDefinitions,
  design + ADR-0034. No RecordsPage/F2/F3 embeds, no Desk lists UI
  (`ReportsPage.vue` untouched since B10/B6; zero lists matches), no
  QueueMessages. The lists Desk UI and queue commits sit above `6474b04`
  in the worker tree only.

## Nits (non-blocking, mostly fixed above already)

- Stale "stays red until 21/22 land" NOTEs (migration 23 comment,
  `lists_test.go`, `rules_builder_test.go`, CLI flow test): 21/22 are
  in-tree and everything is green. Worker drops one such note above
  (5aa7ab7); the rest can follow.
- ADR-0034 "Revisit when" still names maroto clearance; worker rewords
  above (8e9c082).
- `runBuilder` outcomes carry no input hash/steps/reads, unlike Starlark
  runs — cosmetic gap in run rows.
