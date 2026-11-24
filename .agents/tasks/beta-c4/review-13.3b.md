# Review: C4 slice 13.3b — ledger saves recompute tree paths

- Commit: 3175005 `feat(c4): recompute tree paths on ledger saves (slice 13.3b)`
- Worktree: /Users/iampaulking/projects/duchess/athena-erp-c4-r (detached, verified clean on arrival; `git status` empty)
- Diff: `git diff main...HEAD` — 4 files, as briefed: `internal/ledger/accounts.go`, `internal/ledger/masters.go`, NEW `internal/ledger/accounts_test.go`, NEW `internal/ledger/masters_test.go`
- Brief: `.agents/tasks/beta-c4/review-brief-13.3b.md` (+ `brief.md` slice 3b, ADR-0029, batch-2 design G13)

Verdict: PASS

## Blockers

None.

## Findings by brief item

1. **Recompute (all four save paths, inside the tx).** `SaveAccount` (accounts.go:118-155), `SaveBranch` (masters.go:131-167), `SaveDepartment` (masters.go:224-260), `SaveCostCentre` (masters.go:308-344) all wrap create+update in the existing `withTransaction` (ledger.go:102-121, joins a caller tx or owns one with rollback-on-error). Create computes path/depth via `treePathFor` reading the parent through the tx and inserts both columns. Update reads `oldParent` in-tx, CAS-updates, and on a parent change calls `refreshSubtreePaths` in the same tx. Path format (`/name/` root depth 0, children extend parent path, masters.go:434-449) matches ADR-0029 and slice-2 semantics (migration-18 backfill `sysmigrations.go:824`, record-tree move `records.go:512`); names cannot hold slashes (`namePattern`, ledger.go:67), so the join is unambiguous. The `UPDATE…JOIN…CONCAT` in `refreshSubtreePaths` (masters.go:479-483) repeats the migration-18 backfill pattern (`sysmigrations.go:828`) — no new dialect posture (MariaDB tested, PG untested per ADR-0019). Only writers to these four tables are the four saves plus migration-18 fixtures (grep-verified).
2. **Parent change rewrites the whole subtree.** `refreshSubtreePaths` (masters.go:456-504) rewrites the root from its new parent, then walks level by level (children's path from already-updated parents), capped at 100 levels per the ADR revisit note. Tests reparent and read every moved descendant's path/depth from storage via SQL (`treePosition`, masters_test.go:15-31): accounts liquid→funds with cash checked, branches metro→south with downtown checked, plus departments and cost centres. Old parents verified unchanged.
3. **Refusals unchanged.** Cycle checks (`checkAccountCycle`, `checkNested`) and dangling-parent/company/group checks are untouched and still run pre-tx at the same sites with the same errors (`would close a cycle`, `ErrNotFound`). Existing pins (`ledger_test.go:101`, `determination_test.go:49`) plus new refused-cycle-writes-nothing cases (accounts_test.go:41-55, masters_test.go:71-86) hold. One trivial message change, not a behavior change: updating a missing row now returns `not found: account "x"` (in-tx `notFound`) instead of cas's `not found: "x"` — both wrap `ErrNotFound`, and no test pins the string (grep-verified).
4. **Tests in NEW files only; rerun green.** `ledger_test.go`/`ledgers_test.go` untouched (diff stat confirms slice-4 separation). Coverage: create, reparent, subtree depth, stale-path regression from storage, label-only no-op, cycle-refused-writes-nothing. All four tests pass against a live DB (narrow `-run TreePaths` run: 4/4 PASS, ~36s, no skips).
5. **Scope clean.** No migration, no proto/gate/web/cmd, no quantity/determination/UoM changes, ADR untouched, Z2 (posting sentinels)/Z4 (events secrets) untouched — confirmed by the 4-file diff stat.
6. **Gates.** `gofmt -l cmd internal` empty; `go build ./...` ok; `go vet ./...` ok; `affected` reuses the worker's green run on the same clean commit 3175005 (`suite-logs/1124-021332-w-c4.log`: cmd/athena, internal/ledger, internal/posting, internal/server all ok; read, no FAIL); `make generate` twice with no diff and clean tree (needed node on PATH, same path the suite wrapper uses). No browser pass (no Desk), per brief. Tree clean before gates.

## Follow-ups (non-blocking)

- F1: No test reparents across levels — every move keeps the same depth (1→1, 2→2), so a depth *shift* is never asserted. Add one cross-level move (e.g. a root under another tree's child) to masters_test.go.
- F2: Reparent-to-root (`parent → ""`) is untested in all four suites.
- F3: A label-only save skips the refresh (masters.go:159 etc.), so a hand-corrupted NULL-path row stays stale until a parent change. Consider recomputing at least the saved row's own path on every save, or keep the documented top-down repair and say so in the ADR area.
- F4: `treePathFor`'s NULL-parent error (masters.go:446) is a plain `fmt.Errorf`, not an `InputError` — reachable only via hand-edited rows, but it would surface as an internal error. Consider `inputErrorf`.
- F5 (nit): New tests hardcode revisions (`SaveBranch(ctx, metro, 1)` etc.) instead of using the returned `Revision`.
