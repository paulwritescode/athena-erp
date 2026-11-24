# Review: C2 secrets slice 4 lifetimes + disposal (8.10)

Verdict: PASS

- Tree: `../athena-erp-c2-s4-r`, detached at `75bb9df` (on main `3cc6022`), clean at start and end (`git status --short --untracked-files=no` empty; only ignored `.env.local` + `.cache` present).
- Diff `3cc6022..75bb9df`: exactly 5 files — `internal/jobs/disposal.go` (+476), `internal/jobs/disposal_test.go` (+301), `internal/server/attributes.go` (+38), `internal/server/lifetime_test.go` (+93), `internal/server/record_types.go` (+40/−3). No proto, gate, CLI, Desk, or other lane's files.
- Read: `brief-secrets.md` slice 4 (8.10), Z4 RULE, ADR-0033, batch-2 design.md G8 secrets/retention (line 139 lifetime keys), migration 22 (`sysmigrations.go:980-1018`), worker PASS (`comms.jsonl` 13:47:43).

## 1. Lifetimes: carried per design, validated, preserved — PASS

- Keys match design.md exactly: `lifetime`, `retention_trigger`, `retention_class`, `disposal_action`, `archive_readers`, `hold`, flat in the definition JSON via the embedded `Lifetime` struct (`record_types.go`).
- `validateLifetime` (`attributes.go`): empty block always valid; set block requires a known class pinned to the `jobs` constants (validation and disposal share one value set), trigger created/updated, action anonymise/delete/none, profile ≤40 chars, readers ≤140 chars.
- `carryLifetime` runs in `prepareOne` before validation; the carried definition is what `commitOne` journals and persists. Create path is a no-op (`previous == nil`); empty-carries-empty for types without lifetimes.
- Server edits are append-only: additive struct/functions plus one call each in `prepareOne`/`validateRecordType`; the `Tree`/`EffectiveDated`/`Fields` reflow is whitespace-only. No behavior change to existing paths: `validateLifetime(empty)` is nil, and the live `TestSavePreservesLifetime` second save plans zero steps (empty `describeSteps`, revision 1→2, injected block intact).
- Live on clean `75bb9df`: `TestValidateLifetime`, `TestCarryLifetime`, `TestSavePreservesLifetime` all PASS (36.5s).

## 2. Disposal: preview/apply/holds/restore-reapply all real, Z4 exact — PASS

- Preview: `SystemCandidates` (access log >1yr by `at`; outbox dispatched/dead >30d; events >30d; jobs succeeded/dead >90d) and `RecordCandidates` (per-type clock, trigger-created default, guest action defaults to anonymise). Pending outbox rows and queued/leased jobs never list — seeded and asserted live.
- Holds: `PlaceHold`/`ReleaseHold`/`isHeld` against `athena_holds`; row holds skip in `Apply`, type holds skip in preview. Length guards (reason ≤600, placer ≤255) match the migration widths.
- Apply: one transaction, every outcome recorded in `athena_disposal_register` (register row count == outcome count, asserted); linked Records skip as dependent evidence; missing rows and unknown classes/actions record exceptions, never silent success.
- Restore: `ReapplyRegister` re-deletes `outcome = deleted` rows that came back with a backup; skips/exceptions reapply to nothing. Live-tested with a re-inserted event row (reapplied == 1).
- Z4 honored exactly: guest 90d, Access Log 365d, outbox/events 30d, job runs 90d. The 7-year set (ledger/invoice/stock/audit) is absent from `retentionPeriods` AND refused in both `recordCandidates` (skip) and `applyOne` (skip even when force-listed) — a forced invoice candidate skips, asserted live.
- Every disposal statement checked against migration 22 columns; `resolveTable` allow-lists system tables and regex-pins Record Type keys (injection-tested live with `nope; DROP`).
- Live on clean `75bb9df`: `TestSystemCandidates`, `TestRecordCandidates`, `TestHolds`, `TestApply`, `TestReapplyRegister` all PASS (173s).

## 3. Guest anonymisation gap: honest, markers-only — PASS

- `ActionAnonymise` records `OutcomeException` with "guest anonymisation needs field markers beta does not have yet" — never `OutcomeAnonymised`, never a silent no-op presented as done. Guest+delete is likewise an exception ("guest rows anonymise, never delete").
- The gap is the markers only: disposal takes no field-level input (`RecordProfile` carries class/trigger/action/hold; `LinksTo` is row-level), so per-field anonymisation cannot live here until L3/G18 define field markers. Worker flagged it; code agrees.

## 4. No secret material — PASS

- Zero secret-ish tokens in the new/edited files (`secret|password|private_key|decrypt` grep over `disposal.go`, `attributes.go`, both test files); `disposal.go` does not import `internal/secrets` — disposal never sees values or refs.
- Spans carry counts (`athena.candidates/deleted/skipped/exceptions`) and `athena.outcome` only. Error paths read (`deleteRow`, `recordOutcome`, `isHeld`, link check) carry Record Type + id identifiers only, no payloads.

## 5. Gates — PASS via verified carry (reuse path) + live narrow runs

- Carry claim verified: `git diff f5cbe4d..75bb9df -- <5 slice files>` is empty (byte-identical); the 50-file rebase delta is `.agents/` + `.kiro/tasks.md` docs, `internal/stock` (C7s4), and `web/` (F2s2/F3s1). None of the tested packages (`cmd/athena`, `jobs`, `secrets`, `server`) import `internal/stock` — zero importers outside `internal/stock/` itself (stock imports server, not vice versa) — so the four test binaries build from byte-identical Go inputs on both commits.
- Worker's affected run on `f5cbe4d` (`suite.jsonl` 13:46:52, log `1124-133655-w-c2.log`): exit 0, all four packages `ok` (386s/358s/161s/593s; wall 597s is consistent with parallel `-p` test binaries on 8 CPUs, not an anomaly).
- Caveat, mitigated: that run recorded `clean=False` (tracked dirt at end, or the head moved — the worker cites post-rebase `75bb9df` 11 min after run start — mid-run rebase is plausible). Mitigation: I re-ran all 8 slice tests live on the clean `75bb9df` tree (PASS, above), plus `gofmt -l cmd internal` empty, `go build ./...` OK, `go vet ./...` OK, `make generate` stable (buf ran, zero `git status` drift, no proto in scope). Residual risk (pre-existing-test drift under the worker's dirty tree) is further backstopped by the mandatory full merge gate.
- No full rerun queued: 6 waiters + a running merge gate ahead of the suite lock put a rerun hours out, and per AGENT_RULES nothing reaches `main` without the whole suite at merge anyway. No background run left behind (it would die with this session).
- Scope gate: the 5 files above ONLY — no proto/gate/CLI/Desk, no other lane's files. The `attributes.go`/`record_types.go` edits are the brief-allowed lifetime carries only.

## 6. Nits (non-blocking)

- `candidateCap = 500` is declared but the four preview queries use literal `LIMIT 500`. Harmless duplication; consider using the constant.
- Disposal entry points (`SystemCandidates`, `RecordCandidates`, `Apply`, `PlaceHold`, `ReleaseHold`, `ReapplyRegister`) have no callers outside their tests yet — expected; slice 5 (CLI) and restore-flow wiring come later.
