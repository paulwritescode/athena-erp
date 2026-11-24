# ADR-0023: Posting engine and operations

- **Status:** Proposed
- **Date:** 2026-11-21
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0022, ADR-0006, ADR-0016

## Context

Documents become money through posting: a voucher's plan turns into balanced ledger rows exactly once, reversals mirror them, and every attempt stays visible. The requirements demand one transaction per post (voucher revision, ledger rows, balance deltas, audit event, operation result, outbox row), idempotent replay by Operation_ID, post-once per voucher revision, event and book, and a log that survives rollback.

## Options considered

1. **Journaled single transaction with idempotency keys (chosen).** `internal/posting` validates the plan, then writes the group, rows, balances, audit event, operation result and outbox row in one InnoDB transaction. Operation_ID plus payload hash replays safely; the posting-group key posts once; failures log through a separate connection.
2. **Saga with compensating entries.** Reversals already exist as business facts; using them for technical rollback would confuse corrections with retries and complicate the log.
3. **Queue now, post later.** Defers consistency the requirements want synchronously at submit; the outbox row keeps the door open for G8 consumers without delaying the post.

## Decision (proposed)

- Migration 14 `posting_engine` creates `athena_operations` (unique actor, command, Operation_ID), `athena_posting_groups` (unique voucher type, id, revision, event, book), `athena_posting_log` (every attempt with outcome, code and plan hash), `l_gl` (one row per leg with base and transaction amounts, dimensions, voucher link, reversal link), `athena_balance_account` (running debit/credit totals the posting transaction maintains; G9 adds generations and party balances) and a minimal `athena_outbox` (id, event type, payload) that G8 extends. The balance and outbox tables go beyond the plan's four because Requirement 6.5 names their writes in the posting transaction.
- `Post` runs inside the caller's transaction (the submit path owns it); `Preview` writes nothing and returns plan, resolved accounts, source lines, policy versions and rounding. A changed plan at submit re-runs preview comparison where approval depended on it. Posting Rules are a new Rule type with fixture replay, plan diff and named approval, not production-ready until accepted.
- Double submit posts once: the second identical Operation_ID replays the first result; a different payload under one Operation_ID conflicts; a different Operation_ID for one voucher revision returns the first result without posting again.
- Decided 2026-11-24 (Z2): post only into open periods, with no fixed day window (O-011). Each month soft-closes on the 10th of the next month: clerks can no longer post, while holders of `ledger:close` post adjustments with a reason. It hard-closes after the VAT return (due the 20th of the next month); hard-closed periods refuse everything. Reopening needs `ledger:close` plus a reason, reopens later periods too, and is audited. Corrections after close go into the current period as reversals, never edits. Books (O-012): one statutory book per company (KES); other books must be named, and unnamed books stay refused.

## Consequences

- Submit and cancel paths grow a posting step; documents need Posting Rules before money moves.
- The outbox shape is minimal on purpose: G8 may add columns but must keep id, event type and payload.

## Revisit when

G8 designs the event envelope, or G9 designs balance generations. (O-011/O-012 were decided 2026-11-24, Z2.)
