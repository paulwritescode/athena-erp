# ADR-0031: Party ledger and settlements

- **Status:** Proposed
- **Date:** 2026-11-23
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0022, ADR-0023, ADR-0009, Requirement 7 (batch-2)

## Context

Control accounts (receivables, payables) post into `l_gl` today with no subledger behind them: nothing records which customer or supplier instalment a leg settles, in which currency, or what remains open. The G6 engine carries `SourceType`/`SourceID` on each plan line for the control-account link, but the existence check was deferred to the party ledger (G7). Batch-2 Requirement 7 asks for an append-only party ledger with open items per instalment, line-level settlement applications, realised FX posted through the posting engine, mirrored applications on cancel, and unapplied cash kept separate from open items.

## Options considered

1. **Three tables: append-only facts, mutable items, append-only applications (chosen).** `l_party` holds one append-only row per party-side movement, mirroring the `l_gl` leg shape with party keys added. `athena_party_items` holds the mutable open-item state per instalment (residual, status) that allocation locks. `athena_settlement_applications` holds the append-only application edges between settlement lines and items. Amounts follow `l_gl` (`DECIMAL(28,6)` base and transaction pairs) so Property 12 compares like with like.
2. **Facts only, residuals derived by summation.** No items table: open balances come from summing `l_party` rows. Simpler storage, but allocation needs row locks on both residuals (Requirement 7.4) and a sum has nothing to lock; concurrent settlements would over-apply. Rejected.
3. **Applications inside voucher JSON.** Settlement lines stored on the payment Record itself. Loses cross-voucher queries (open items per party, applications per item), unique keys against double application, and the append-only audit trail. Rejected.

## Decision (proposed)

- Migration 20 `party_ledger` creates three tables and seeds nothing:
  - `l_party` (append-only): id (UUIDv7), company, book, party keys (`party_type` names the party's Record Type, `party_id` its Record id), `control_account_name`, `item_id` (NULL marks unapplied cash), `application_id`, posting date, base and transaction debit/credit pairs as in `l_gl`, voucher link (type, id, revision, event), `posting_group_id` into the G6 group, and `is_reversal`/`reversed_row_id` mirrors.
  - `athena_party_items` (mutable, writer-locked): one row per source instalment (unique on source voucher type, id, revision and instalment number) with party keys, control account, due date, instalment currency, side (`debit` or `credit`), original and residual amounts, `open`/`settled` status and revision.
  - `athena_settlement_applications` (append-only): one row per settlement line and item (unique on settlement voucher type, id, revision, event, line number and item), with amounts in the payment currency, amounts in the instalment currency, the base amount, `is_reversal`/`reverses_application_id` for cancel mirrors, and the G6 posting group.
- The settlement service (`internal/posting/settlement.go`, G7.2) is the only writer. Amounts are magnitudes; mirrors negate by convention, as in `l_gl`. Unapplied cash lives only in `l_party` rows with NULL `item_id`, never in the items table.
- Money storage follows the implemented `l_gl` (`DECIMAL(28,6)`), not the design doc's "integer minor units" line: `l_gl` as built already uses decimals, and the party/control reconciliation must compare identical representations.
- Z2 stays open: no backdating window, no book policy, no default book. The `failed_precondition` sentinels (`ErrBookPolicyOpen`, `ErrCorrectionWindowOpen`) stay byte-identical; settlement posts through the G6 engine inherit them unchanged.

## Consequences

- G7.2 implements open items, line-level applications, both-residual locks, the aggregate ceiling and unapplied cash in `settlement.go`, allocating through `internal/types/allocate.go`.
- G7.3 and G7.5 pin Property 11 (allocation sums) and Property 12 (signed party residuals equal the control balance at every cutoff).
- G7.4 posts realised gain/loss as plans through the G6 engine, mirrors applications on cancel, and adds the control-account source existence check the engine deferred.
- G7.6 adds `ApplySettlement`, `ReverseSettlement` and `ListOpenItems` to `LedgerService` with gate policies, `athena settle apply|reverse|open-items`, and the Desk open-items panel.
- G9 party balances build on `l_party`; no second subledger is needed.

## Revisit when

The sponsor decides the Z2 backdating window or book policy, confirms or rejects the D-007 relation of kernel masters, or asks for a party master in the kernel instead of Pack-owned party Record Types.
