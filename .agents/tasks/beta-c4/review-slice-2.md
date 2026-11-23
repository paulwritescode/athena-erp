# Review: C4 slice 2 — tree + effective-date attributes (13.2)

Verdict: PASS

- Commit: `b3940a3` (`feat(c4): tree and effective-date record attributes (slice 2, 13.2)`).
- Tree: `/Users/iampaulking/projects/duchess/athena-erp-c4-r`, detached at `b3940a3`, clean before and after all checks.
- Diff: exactly the 12 briefed files (2 protos, 4 generated, 5 hand-written Go, `trees_test.go` new). No other files.
- Blockers: none.

## 1. Attributes match G13/R13/ADR-0029

- Tree: `tree_parent_id`/`tree_path` (`/root/child/`, root depth 0)/`tree_depth` per ADR-0029; create stores path+depth, moves rewrite the subtree (`moveTreeSubtree`, LIKE prefix safe: UUIDs hold no wildcards or slashes), self-parent and descendant moves refused (`planTreeMove`, prefix check), deletes with live children refused (`ensureNoTreeChildren`), missing/deleted parents refused (`loadTreeParent` with `FOR UPDATE`). `repairTreePath` heals pre-path rows; its failure modes (hand-edited cycle, >100 chain) return errors inside the caller's tx, so nothing is half-written.
- Effective ranges are end-exclusive `[from, to)` everywhere, matching R13.2 and ADR-0029: `parseEffectiveRange` refuses `from >= to` (`attributes.go`); `checkEffectiveOverlap` uses `new.from < exist.to AND exist.from < new.to` with NULL as unbounded, so touching bounds pass; `asOfPredicate` is `from <= asOf < to`; proto comments say end-exclusive on all five sites. Overlap refusal keys entities off unique fields (NULL-tolerant), one entity when no unique field, child rows scoped by parent — the enable-time check (`overlappingEntitiesExist`) uses the same entity rule.
- Flag validation (`record_types.go:validateTreeEffective`): child/setting cannot be trees, amendable cannot be effective-dated. Correct per the kind model.
- Writes to tree/dated types serialize on the type row (`serializeTypeWrites`, always before row locks), closing the move/overlap race the way the unique index does for plain types.
- R13.1 "cross-company links": not applicable to generic Record Types (no company column; links stay inside one table). Ledger-side scoping is outside this slice.
- `planMigration` returns early on create, so `planTreeEffective` never sees a nil `previous` (no nil-deref); `migrateTreeEffective` nil-guards too.

## 2. Proto: additive only, generated in sync

- `record_types.proto`: `RecordType.tree = 22`, `effective_dated = 23`, plus the `MigrationStep` kind comment extended with the four new kinds. `records.proto`: `Record` gains 16–20, `ListRecordsRequest.as_of = 6`, `CreateRecordRequest` gains 5–6, `UpdateRecordRequest` gains 5–7 as `optional`. All numbers fresh, no collisions.
- The proto diff has 6 hunks; the only removed lines are the two extended comments. No `rpc` added/removed/changed (grep count 0), no other message touched.
- `make generate` run twice in this worktree (node via nvm on PATH): both exit 0, `git status` empty after each — generated Go/TS match.

## 3. Migration path: no silent loss, no bricking

- Flags off: behavior unchanged (`kernelColumns` adds nothing, `checkUnique`/`serializeTypeWrites` no-op; only a `parent_id` error-message wording change on the same code).
- Enable tree: columns added, existing rows backfilled as top-level — disclosed in the plan step, needs no confirmation (non-destructive). Enable effective: no backfill (NULL = open), refused with `FailedPrecondition` while two live rows share an entity.
- Disable tree/effective: drop steps marked destructive with counts, gated by `confirm_data_loss` (tree-disable refusal without confirmation is tested); disable-effective additionally refuses while history duplicates would violate the restored unique indexes.
- Cycles are impossible via the API (new rows, prefix-checked moves), so cycle rejection cannot strand saved trees.

## 4. Tests: `trees_test.go` (400 lines, 12 tests)

Covers path/depth store, move rewriting the subtree, self/descendant cycle refusal, delete-with-children refusal, flag validation, overlap rejection incl. touching bounds and backwards ranges, as-of listing incl. the exclusive end, enable/disable plan+confirm migration, all three enable-effective entity cases, child parent-scoping, and `parseEffectiveRange`/`parseAsOf` units. No randomized property tests, but none are required until slice 4 (Property 25, 13.4); boundary cases are covered by example.
- Reran filtered in this worktree: all 12 pass (plus slice-1 `TestTreesEffectiveUoMMigrationBackfillsPaths`), 19s against MariaDB.

## 5. Scope confirmations

- No new procedures → no `gate.go` change needed; `gate.go` untouched. No hand-written `web/` (only `web/src/gen/*`) or `cmd/` changes.
- Migration 18 `trees_effective_uom` is in `main` (`sysmigrations.go:661`, slice 1 merged) and untouched by this diff.
- Z2 (backdating window/book policy) and Z4 (retention/key custody) untouched and undecided by this code.

## 6. Gates

- `gofmt -l cmd internal`: empty. `go build ./...`: clean. `go vet ./...`: clean.
- `.agents/bin/affected` (absolute main-repo path): full-`make test` fallback (protos changed), reused worker green run on the same clean commit `b3940a3` (`1123-213711-w-c4.log`, exit 0). Log read: all Go packages `ok` (`internal/server` 561s, MariaDB tests ran), 613/613 web tests pass, zero FAIL lines.
- No browser pass: no Desk in this slice.

## Follow-ups (non-blocking, for a later slice)

1. No depth/path-length guard: `tree_path` is `VARCHAR(2048)`, about 53 UUID levels; deeper trees fail with a raw DB error instead of `invalid()`. ADR-0029 already flags path exhaustion as a revisit. Suggest a create/move depth cap with a friendly error.
2. Plan/apply asymmetry: enabling effective-dated drops unique indexes (`ensureIndexes`) with no plan step naming it (`planIndexes` only compares field attrs). No data loss — enforcement moves to the overlap check — but the `enable_effective` description should mention the index change.
3. `TestEnableEffectiveRefusesSharedEntities` (`trees_test.go:295`) tests empty-table success, not refusal; rename to e.g. `...AllowsEmptyTable`.
4. No test for the disable-effective path (duplicate-history refusal, confirmation gate).
