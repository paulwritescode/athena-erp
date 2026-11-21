# Report: B9a server part, Record Type list data

**Status: Final. Verdict: PASS.**

Worktree: `/Users/iampaulking/projects/duchess/athena-erp-b9a-server`, branch `feat/b9a-server`
(from main `d6859e4`). Nothing committed, merged or pushed, no agents started.

## What ListRecordTypes already returned

Name (label), description, kind and field count (from `fields`) were already present. Icon is a
Desk-side derivation (not requested as a field) and field count is `len(fields)`.

## What changed

- `proto/athena/v1/record_types.proto`: additive fields 16–20 on `RecordType` —
  `record_count` (int64), `count_hidden` (bool), `updated_at` (RFC 3339 string, matching the
  records/files convention), `updated_by` (display name), `pack_key` (empty until C3). No
  renumbering, no new procedure (so no `gate.go` change).
- `internal/server/migrations.go`: `saveOptions` gains `Actor` (display name, or
  `"Local administrator"` for the CLI), persisted in the journal's existing `options` JSON.
  No migration: the brief assigned no migration number, and the options column already
  carries per-operation metadata. Saves before this change report empty `updated_by`.
- `internal/server/access.go`: `displayActor(ctx)` helper (display name, falling back to
  username; operator literal for the CLI).
- `internal/server/record_types.go`:
  - Save paths (single, child+parent pair members, delete) stamp the actor into the journaled
    options. Resume reuses the stored row, so the original actor survives a crash resume.
  - `ListRecordTypes` enriches each item: latest completed `save` op per type in one window-
    function query (ties on the second-precision stamp break on the time-ordered op ID;
    `save_group` rows excluded since they carry no options), and Record counts in one
    `UNION ALL` query over existing tables (single `information_schema` pre-check; missing
    tables count 0). Only the actor is decoded from `options` — backfills may hold Record
    values and are never logged or decoded here.
  - Permissions: readable = operator or `Can(name, read)`. Anything else listed gets 0 plus
    `count_hidden`. In practice non-builders only ever see readable types (existing `CanAny`
    filter), so the flag triggers for builders lacking a read Grant. Row-scoped Grants still
    see the full-table count (brief asks for type-level read only); noted, not implemented.
- Generated: `gen/athena/v1/record_types.pb.go`, `web/src/gen/athena/v1/record_types_pb.ts`
  (`make generate` 3×, stable checksums, no diff). No other `web/src` changes.

## Tests

- `TestListRecordTypesShowsCountsAndLastSave`: populated type counts 2, empty type 0 (neither
  hidden), operator actor literal, recent RFC 3339 stamp, empty pack key.
- `TestListRecordTypesHidesCountsWithoutRead`: builder without read sees both types with hidden
  counts; a read Grant on one reveals only it; a non-builder sees only the readable type with
  its count.
- `TestListRecordTypesShowsDisplayNameOfSaver`: Desk save by "Ada Lovelace" reports her display
  name and a recent stamp.
- `TestRecordTypesShowReportsLocalAdministrator` (CLI): `record-types save` then `show --json`
  reports `"Local administrator"` and a timestamp.

## Checks

- `gofmt -l internal cmd` → empty.
- `go build ./...` → exit 0. `go vet ./...` → exit 0.
- `make test` → green: Go suite passed (web ran after the `&&`), web 9 files / 248 tests passed.
- Only throwaway `test_athena_*` databases used.

## Review loop 1 fixes (all 4 done, PASS)

All four review findings fixed in the same worktree, no commits:

1. **Scoped counts hidden.** `recordCounts` now includes a table only when
   `Principal.Scope(name, read)` is unrestricted; row conditions and Assignment scope yield 0
   plus `count_hidden`. Tests: conditioned Grant, `company=acme` scoped Assignment, and a
   second unrestricted Assignment restoring visibility.
2. **Compensated saves excluded.** New journal status `compensated`, set on the child member op
   by `abandonPair` after a successful compensation (the failure path still leaves the member
   completed, and the resume path still ends members failed — both verified by untouched
   existing tests). The last-save query already selected `completed` only, so no query change.
   Test: Ada saves the child, Bob's pair save fails and is compensated, the list shows Ada's
   revision 1, and exactly one member row is `compensated`.
3. **Live records only.** Count members reuse `liveClause` (masters exclude soft-deleted rows).
   Test: delete the only customer → visible 0. Also added: definition without a table → visible 0.
4. **Dialect + retry.** Existence precheck via `dialect.TableExists`, UNION placeholders via
   `dialect.Rebind`, and one retry when the UNION fails with a missing-table error classified
   narrowly through the dialect (new `store.ErrMissingTable`: MariaDB 1146, PG 42P01; the
   existing MariaDB Classify test's `1146 → ErrOther` case updated deliberately). Retry unit
   test uses injected scripted errors (miss-then-hit, miss-twice surfaces, other error never
   retries); Classify cases added for both engines.

Loop-1 checks: `gofmt -l` empty, `go build`/`go vet` exit 0, `make test` exit 0 (all
packages ok — auth 350 s, server 261 s, cmd 164 s, ledger 141 s, files 105 s; web 248/248).

## Review loop 2 fixes (both done, PASS)

Both blockers fixed in the same worktree, no commits:

1. **Old compensated groups excluded.** `lastSaves` now skips member saves journaled inside any
   pair group that never completed (pending, failed, or compensated), whatever the member's own
   status, by reading member IDs out of the stored group JSON — so pre-fix history ranks right
   with no backfill (the loop-1 `compensated` member mark stays as the explicit record for new
   rollbacks). The window-function ranking became a plain ordered scan with Go-side filtering,
   which is also more portable. Test forges the old shape directly (completed child op with a
   later stamp and sorting-last ID inside a failed/compensated group) and asserts the list
   still shows the surviving save's actor.
2. **Dialect-correct SQL.** PostgreSQL `TableExists` used a bare `?` (pgx needs `$1`) — fixed,
   with the probe SQL of both dialects extracted to asserted constants. `liveClause` takes the
   dialect as a parameter now (7 call sites updated, including the pre-existing records/rules
   flows the review suspected — same backtick bug, now fixed everywhere). New `countMemberSQL`
   builder is asserted per dialect (master/document/empty-kind) plus a rebound two-member UNION
   asserting `$1`/`$2`; the UNION itself rebinds through the dialect at execution as before.
   Note: the sibling PG catalogue methods (Column/IndexExists etc.) carry the same bare-`?`
   shape; they are off this task's path, so I left them untouched — flagging here.

Loop-2 checks: `gofmt -l` empty, `go build`/`go vet` exit 0, `make test` exit 0 (all
packages ok — auth 313 s, server 229 s, cmd 157 s, ledger 135 s, files 101 s; web 248/248).
