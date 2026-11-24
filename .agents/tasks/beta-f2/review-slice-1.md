# Review: F2 slice 1 server free-text match

Verdict: PASS

Tree: /Users/iampaulking/projects/duchess/athena-erp-f2-s1-r, detached at df2b80d, clean. Diff 6c871c9..df2b80d is 7 files: records.proto, gen Go + TS, lookup.go, records.go, records_test.go, overview.md (1 line). Scope-only confirmed.

## 1. Match semantics

- Label + text fields: `matchPredicate` (lookup.go) ORs `labelExpression(d)` with every `textFieldTypes` column (Text, Long text, Email, Phone), skipping the label column when it is already a text field. Non-text label field (Enum state) still matches via the label expression — pinned by `TestRecordListQuerySearchesTheLabelField`.
- Hidden-safe by construction: called with `visibleDefinition(recordType, hidden)`, the same hidden set as `filterPredicate`; `visibleDefinition` drops hidden fields and clears a hidden `LabelField`, so a hidden value can neither match nor leak. No dedicated hidden-search test; same mechanism as filter and lookup.
- Paged, no unbounded scans: the predicate is AND'd into the existing keyset-paged query (`LIMIT pageSize+1` intact). Paging pinned: page_size=1 walk over "zed" equals the unpaged result.
- Hostile input: `likeEscaper` escapes `\`, `%`, `_` (same precedent as the lookup `query`); values are bound params, so quotes are inert. `%` and `_` pinned literal; case-insensitivity pinned ("ac"→Acme/Acorn, "ACME"→Acme, "OPEN"→open). Unicode not explicitly pinned; bound params plus a byte-safe replacer, acceptable.
- Empty query = today exactly: `TrimSpace`, empty skips the predicate, so the SQL is byte-identical to the old list. `""` and `"   "` pinned to return all.

## 2. Proto

- Additive: `string query = 7` on `ListRecordsRequest`; default empty preserves old behavior. No breaking change.
- Generated files match sources: `make generate` run twice, tree clean afterwards.
- Gate: `RecordServiceListRecordsProcedure` already has `{policyRecord, ActionRead}` (gate.go:112); the param rides the existing procedure, no new entry needed. Verified.

## 3. Tests REAL

- New tests run against real MariaDB via `newServices(t)`, run verbosely here: both PASS (0.23s, 0.32s).
- Worker full-suite green on e8efbbd confirmed in suite.jsonl (exit 0, clean, 1062s).
- Rebase code-disjoint verified with git: `e8efbbd..df2b80d` outside `.agents` touches only main-side F6 files (art, logo, tasks.md) plus the overview.md context line; all slice-1 code files identical between the two commits.
- Reviewer rerun: `affected` fell back to full `make test` (proto change) on df2b80d — exit 0, clean, 1105s, all Go packages `ok`, web 646 passed.
- `gofmt -l cmd internal` empty, `go build ./...` and `go vet ./internal/server/` clean.

## 4. Scope

- No RecordsPage.vue / DataTable.vue changes (slice 2 owns the Desk). No F4 builder work; the F4 item stays deferred. B2a gap note ("server API gaps: ... free-text search ...") is what this slice closes.
