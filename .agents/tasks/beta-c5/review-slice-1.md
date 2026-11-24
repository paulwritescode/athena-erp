# Review: C5 slice 1 — migration 20 + ADR-0031 (7.1)

Reviewer: r-c5. Worktree: ../athena-erp-c5-r, detached at f466742, verified
clean on arrival and at the end (`git status --short` empty both times).
Single commit on main: `f466742 feat(c5): migration 20 party_ledger +
ADR-0031 (slice 1, 7.1)`. Diff is exactly the 5 files the brief names.

Verdict: PASS

## Blockers

None.

## Checks (brief items 1–5)

1. Migration 20 `party_ledger` — PASS.
   - Tables match the Data Models row `party_ledger` (batch-2 design.md:170)
     and Req 7 criteria 1–2, 7: `l_party` append-only facts (no
     `updated_at`, `is_reversal`/`reversed_row_id` mirrors), party keys,
     `control_account_name`, NULL-able `item_id` for unapplied cash,
     base+txn `DECIMAL(28,6)` pairs identical to `l_gl`
     (sysmigrations.go:453-457); `athena_party_items` one row per source
     instalment with the unique key on (source type, id, revision,
     instalment_no), per-instalment currency, side, original+residual,
     `open` default status, revision; `athena_settlement_applications`
     one row per settlement line+item with the unique key on (settlement
     type, id, revision, event, line, item), payment and item
     currency/amounts, `base_amount`, reversal link, group.
   - Number 20 matches the reservation (migrations.md); entry appended
     after 19 at the end of the list; the sysmigrations.go diff is
     purely additive — no applied migration touched.
   - `TestPartyLedgerMigrationCreatesThePartyTables` builds to 19,
     upgrades, and checks columns, emptiness and both unique keys;
     follows the established Target-upgrade pattern (cf. the trees
     test). Green via the worker's full run (see 5).
2. ADR-0031 (Proposed) — PASS. Tables match the migration; append-only
   rule, endpoint semantics and unapplied-cash rule stated; the
   ceiling/lock strategy is honestly scoped to G7.2+ consequences;
   nothing seeded; no invented policy. Verified grounding:
   - "Existence check deferred to G7": engine comment says exactly that
     (internal/posting/posting.go:74-76).
   - Decimals follow implemented `l_gl`: sysmigrations.go:453-457.
   - Sentinels byte-identical: no posting file in the diff;
     `ErrBookPolicyOpen`/`ErrCorrectionWindowOpen` confirmed at
     posting.go:58-64 with unchanged messages.
   - G9 consequence ("party balances build on l_party") is consistent
     with tasks 9.2 (balances updated in the posting transaction).
3. Decision-log + README rows — PASS. Both link files resolve
   (`docs/decisions/adr-0031-party-ledger-and-settlements.md` exists;
   relative links correct from each file); numbering follows ADR-0030;
   no stale references.
4. Scope — PASS. No other tables/columns; no proto, gate, web or cmd
   changes; Z2 untouched (only "stays open" denials — no window, day
   count, policy or default book anywhere in the diff); no other Z
   touched (Z4/Z10 appear only in unchanged context lines).
5. Gates — PASS.
   - `gofmt -l cmd internal`: empty. `go build ./...`: ok.
     `go vet ./...`: ok.
   - Tests: reused the worker's full `make test` green run on f466742
     (suite.jsonl 2026-11-24T00:16:11+0300, exit 0, clean tree, log
     `1124-000058-w-c5.log`, read in full: every Go package ok
     including `internal/server` 773.9s, vitest 63 files / 616 tests
     passed, no FAIL or panic). Note: the brief expected `affected`
     to take the full fallback, but `affected` only falls back on
     FULL_PATHS (proto/go.mod/Makefile/testdata/buf) or >70% reach —
     sysmigrations.go scopes to `./cmd/athena ./internal/server`, a
     different suite cmd string, so auto-reuse did not trigger. My
     scoped run queued 30m, started, and I terminated it as redundant
     per the reuse rule; the lock passed cleanly to the next waiter.
   - `make generate` twice: stable, tree clean after (needed the nvm
     node on PATH; `suite` adds it automatically).
   - No browser pass: correct, no Desk changes.

Z2: nothing skipped — slice 1 has no posting behaviour; sentinels intact.

## Follow-ups (non-blocking)

1. ADR-0031 line 24 says "mirrors negate by convention, as in `l_gl`".
   The engine actually swaps debit/credit sides with non-negative
   magnitudes (reverse.go `mirrorPlan`: "every amount and rate swaps
   sides"; posting.go:70-72 "amounts never go negative"). Suggest
   "mirrors swap sides" wording next time the ADR is touched. Net
   effect is equivalent; no code impact.
2. Tooling note for master/Stylus (not a slice issue): consider adding
   `internal/server/sysmigrations.go` to `affected` FULL_PATHS — a
   system migration can affect every MariaDB-heavy package, so the
   full fallback the brief expected would be the safer default.
