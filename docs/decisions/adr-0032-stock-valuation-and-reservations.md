# ADR-0032: Stock valuation and reservations

- **Status:** Proposed
- **Date:** 2026-11-24
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0022, ADR-0023, ADR-0029, ADR-0009, Requirement 11 (batch-2)

## Context

Stock control accounts post into `l_gl` today with no subledger behind them: nothing records which item, lot, serial or location a value movement belongs to, what each unit cost, what is reserved for a cart or an event, or whether the stock value still equals the books. The G6 engine carries quantity columns on `l_gl` rows, but quantities without layers, applications and bin balances cannot value issues or refuse negative stock. Batch-2 Requirement 11 asks for a stock ledger by owner, company, item, lot or serial, location and source line with signed base units and value, valuation layers with item applications, rebuildable bin balances, append-only cost deltas, reservations, serial and batch identity, quarantine, counts, and a stock-to-GL check that writes an Integrity and Incident Log row when it differs.

The beta plan left the valuation method and the negative stock policy open (Z3). The binding Z3 rule, decided 2026-11-24, settles them: perpetual weighted (moving) average for all stock with one formula for items of similar nature (IAS 2.25); perishables issue first-expiry-first-out as a picking order, not a cost formula; rental items are tracked but never consumed; negative stock is refused for valued items.

## Options considered

1. **Fact ledger, layers, applications, bins, reservations, identity, integrity log, rebuild state, all under weighted average (chosen).** `l_stock` holds one append-only row per movement with signed base units and signed base-currency value; `l_stock_value` holds append-only cost deltas referencing the original rows; `athena_stock_layers` holds one receipt layer per receipt row with a residual the writer locks; `athena_item_applications` holds the append-only issue-to-receipt and return-to-issue edges; `athena_stock_bins` holds the lockable bin balances the negative-stock refusal and reservation takes check; `athena_reservations` holds holds separate from on-hand; `athena_serials` and `athena_batches` hold unit and lot identity with expiry for FEFO; `athena_integrity_log` holds check rows with expected, observed and owner; `athena_stock_rebuilds` holds the rebuild watermark across restarts. The moving average for an item and location is bin value over bin quantity, so no separate average table is needed.
2. **FIFO layers as the cost formula with FEFO as one case of it.** Issues would consume the oldest receipt layer first and perishables would fall out naturally. Rejected: Z3 chose weighted average for all stock, and FEFO stays a picking order over batches, independent of cost.
3. **Facts only, bin balances derived by summation.** No bins table: on-hand comes from summing `l_stock` rows. Simpler storage, but the negative-stock refusal (Z3) and the one-holder reservation take (Requirement 11.10) both need a row to lock, and a sum has nothing to lock; concurrent issues would drive bins negative. Rejected, the same argument as ADR-0031 option 2.
4. **A cost method per item (FIFO for some, average for others).** More flexible for mixed businesses. Rejected: Z3 and IAS 2.25 require one formula for items of similar nature, and per-item methods would split Property 22 (stock value equals GL) into per-method proofs. The method interface in `internal/stock` (G11.2) keeps the door open if the sponsor ever revisits Z3.

## Decision (proposed)

- Migration 21 `stock_ledger` creates ten tables and seeds nothing:
  - `l_stock` (append-only): id (UUIDv7), company, book, owner key, item keys (`item_type` names the item's Record Type, `item_id` its Record id), `control_account_name` for the stock-to-GL check, location, nullable lot and serial, base UoM key, signed `qty_base`, signed `value_base` (zero for quantity-only moves such as quarantine transfers and rental movements), explicit `base_currency`, movement (economic) and posting dates, dimensions JSON, the posting voucher link (type, id, revision, source line, event) into the G6 group, and `is_reversal`/`reversed_row_id` mirrors.
  - `l_stock_value` (append-only): one cost-delta row per corrected `l_stock` row (`stock_row_id`), with the delta split into `on_hand_delta` and `sold_delta` (late freight, Requirement 11.5), the original economic date and an allowed posting date (Requirement 11.7), the voucher link and reversal mirrors. Historical rows are never rewritten (Requirement 11.6).
  - `athena_stock_layers` (mutable, writer-locked): one layer per receipt row (unique on `receipt_row_id`) with original and residual base quantities, unit cost, and `open`/`consumed` status.
  - `athena_item_applications` (append-only): one edge per issue-to-receipt (`kind = 'issue'`) or return-to-issue (`kind = 'return'`) application with applied quantity and value, the voucher link and reversal mirrors.
  - `athena_stock_bins` (mutable, writer-locked, rebuildable): one row per company, book, owner, item, location, lot and serial (composite key) with on-hand, reserved and value-on-hand totals. Available equals on-hand minus active reservations (Requirement 11.9).
  - `athena_reservations` (mutable, writer-locked): one hold per holder and bin (unique on holder type, holder id, item, location, lot and serial) with quantity, `active`/`released`/`consumed`/`expired` status, holder link (cart, event, order) and optional expiry. Rental reservations name the event holder and the serials.
  - `athena_serials` (mutable, writer-locked): one row per integral unit (unique on company, book, item and serial) with status (`on_hand`, `reserved`, `issued`, `on_rent`, `quarantine`), owner, location, lot and receipt/issue row links. Rental units move to `on_rent` and back; they are never consumed.
  - `athena_batches` (mutable, writer-locked): one row per lot (unique on company, book, item and lot) with expiry and received dates for FEFO picking order.
  - `athena_integrity_log` (append-only): one row per stock-to-GL check break with check name, control account, cutoff, expected and observed values, responsible owner, `open`/`acknowledged`/`resolved` status and detail JSON (Requirement 11.16).
  - `athena_stock_rebuilds`: one row per bin rebuild with `running`/`completed`/`abandoned` state, the watermark row id (UUIDv7 ids order the rebuild), start and completion times and error text, so an interrupted rebuild restarts visibly and close stays blocked while valuation is pending (Requirement 11.13). This table is additional to the design row; the watermark needs persistence and no other migration in this task can add it.
- The Z3 rule is baked into the shape: weighted average needs only bin value over bin quantity; FEFO needs only the batch expiry index; rentals need serial rows plus event reservations with zero-value movements; the negative-stock refusal needs lockable bin rows. Counts post deltas against movements since the voucher cutoff from `l_stock` itself, so they need no table (Requirement 11.14). Quarantine is a location and ownership grain on rows and bins, so moves there carry value unchanged and need no table (Requirement 11.12).
- The stock service (`internal/stock`, G11.2) is the only writer. Quantities and values follow the implemented `l_gl` (`DECIMAL(28,6)`), not the design doc's "exact decimal strings" line: `l_gl` as built already uses decimals, and the stock-to-GL comparison must compare identical representations. UoM keys are `VARCHAR(32)` to fit any valid key from the ADR-0029 masters. Base currency is explicit on every value row so each row is self-describing at a cutoff.
- Z2 stays open: no backdating window, no book policy, no default book. The movement/posting date split is stored, but the window that constrains it is undecided, so the `failed_precondition` sentinels (`ErrBookPolicyOpen`, `ErrCorrectionWindowOpen`) stay byte-identical and stock posts through the G6 engine inherit them unchanged.

## Consequences

- G11.2 implements the ledger writer, layers, applications, bins and the cost-method interface with weighted average in `internal/stock`, refusing negative stock for valued items and posting value through the G6 engine.
- G11.3 implements append-only cost deltas with economic and posting dates, the freight on-hand/sold split, the rebuild watermark and the close block in `delta.go`.
- G11.4 pins Property 21 (append-only cost correction: historical row hashes unchanged, new cost only in appended deltas).
- G11.5 implements reservations with locked takes, serial/batch/FEFO tracking, quarantine and counts; rentals stay tracked-not-consumed.
- G11.6 pins Property 23 (availability: available equals on-hand minus active reservations, no two holds exceed on-hand).
- G11.7 implements the per-control-account stock-to-GL comparison with Integrity Log rows in `check.go`.
- G11.8 pins Property 22 (stock value equals GL at every cutoff).
- G11.9 adds `StockService` with gate policies, `athena stock balance|ledger|reserve|rebuild|check`, and the Desk stock-check view.

## Revisit when

The sponsor revisits the Z3 valuation rule (for example FIFO for an item class), decides the Z2 backdating window or book policy, confirms or rejects the D-007 relation of kernel masters, or asks for an item master in the kernel instead of Pack-owned item Record Types.
