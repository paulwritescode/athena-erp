# Review: C1 6.11 — G6.8–6.9 docs (docs-only)

Commit: bdce628 (re-review; fix of 203b758, detached, tree clean)
Scope: AGENTS.md, README.md, docs/architecture/overview.md, docs/decisions/decision-log.md; delta 203b758..bdce628 is overview.md one line only (confirmed via `git diff --stat`)

Verdict: PASS

## Blocker (fixed at bdce628)

- docs/architecture/overview.md:98 said validation, derivation, guard, transform **and posting** Rules "run inside the save through the lifecycle registry" — false for posting Rules (registry hook skips them, `rules_engine.go:94-102`; submit runs them via direct `postVoucher` after the hooks, `lifecycle.go:295-302`). bdce628 drops "and posting" from the registry sentence and the posting-Rules sentence now reads "they run inside the submit transaction through the posting path, after the registry hooks accepted the business action" — verified verbatim against `rules_engine.go:94-96` and `lifecycle.go:295-300`. Blocker closed.

## Non-blocking nit (follow-up, do not hold the merge)

- docs/architecture/overview.md:101 — "failures through a separate connection after rollback" implies the log write happens after rollback; the code writes it on a separate connection during `Post` (`internal/posting/operations.go:219-232`, "so the row survives the posting transaction's rollback"), i.e. before the caller's rollback completes. Substance (survives rollback) is right; consider "on a separate connection, so the row survives rollback".

## What passed

Accuracy vs merged code on main (6.8/6.9 slices merged; this tree's code == main plus docs):
- Lifecycle hooks: submit calls `postVoucher` (`lifecycle.go:300`), cancel calls `reverseVoucher` (`lifecycle.go:305`); README:93, overview:106 match.
- Posting Rule type: `Type="posting"`, entry `post`, events `["on_submit"]` (`internal/rules/rules.go:46,76-77,92-93`); invoked as `post(doc, ctx)` (`rules.go:561`); `fixtures()` required (`rules.go:287-298`). Activation gate replays fixtures, lands disabled with the plan diff, re-save with enabled is the named approval (`internal/server/rules.go:218-303`). Matches AGENTS.md:15, README:107, overview:98, decision-log:23.
- RPCs: all four on `LedgerService` (`proto/athena/v1/ledger.proto:53-60`), 39 total (35+4, counted) — overview:100 correct. Gate entries are `ledger:read` (`gate.go:162-165`) and `previewable` adds the voucher read Grant (`ledger.go:847-869`) — "ledger:read plus the voucher read Grant" correct. Preview assumes revision+1 with the approvable hash (`ledger.go:871-875,894,926-930`); submit refuses on hash mismatch with FailedPrecondition (`lifecycle.go:339-345`); log newest-first with posted|replayed|conflict|rejected|error (`ledger.proto:610-612`, `operations.go:25-35`); status returns groups with reversal state. Matches AGENTS.md:17, overview:101.
- CLI: `cmd/athena/post.go:14-25` usage matches README:98 exactly (subcommands, `--revision`, `--preview-hash`, `--outcome`, newest-first log, plan hash print).
- Posting engine: `ValidatePlan` checks voucher/event/book/date, enabled company, leaf enabled in-scope account, open period per (company, book), dimensions, control sources, balance per company and book (`posting.go:136-144`); write set is group + operation result + log row + ledger rows + balance deltas + audit event + outbox row in tx (`write.go:122-203`); Operation_ID replay/conflict (`posting.go:105-108`, `ErrConflict`); post-once unique key on (voucher_type, voucher_id, revision, event, book) (migration 14, `sysmigrations.go:405-426`); reversals flagged (`is_reversal`, `write.go:242`). Matches AGENTS.md:17, overview:101. R6.5/R6.15/R6.18 citations match `requirements.md:167,177,180`.
- File paths, command names, RPC names, migration 14, ADR-0016/ADR-0020/ADR-0023 (all exist, Proposed) all correct.

Not production-ready + Z2:
- "Not production-ready until accepted" present in AGENTS.md:15,17, README:98,107, overview:98,101, decision-log:30. No production-ready claim anywhere.
- Z2 open with no invented default: AGENTS.md:17, overview:101 (O-011/O-012), decision-log:30; code returns `ErrBookPolicyOpen`/`ErrCorrectionWindowOpen` and "never invents a window or a default book" (`posting.go:9-13,155-156`). No window length, policy, day count or default book in the diff. No other Z/slice decision touched (decision-log touches ADR-0016 and ADR-0023 rows only).

Conventions/consistency:
- Decision-log rows point at ADR-0016 (line 23) and ADR-0023 (line 30) with intact links; AGENTS.md/README/overview agree on Rule types, CLI shape, RPC names, grants and readiness.
- No stale references: overview "Not built" list dropped `posting`; lifecycle row dropped "No ledger reversal".

Gates:
- `git status` clean at bdce628 (detached).
- `/Users/iampaulking/projects/duchess/athena-erp/.agents/bin/affected --print` and `affected`: "no Go or web change: nothing to run (docs only)", exit 0.
- No web gates (no `web/` changes).
