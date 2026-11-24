package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"athena-erp/internal/ledger"
	"athena-erp/internal/posting"
	"athena-erp/internal/types"

	"github.com/google/uuid"
)

// CostDelta allocates an after-the-fact cost, such as a late freight
// invoice, to one receipt row. The service splits the total between the
// receipt's units still on hand and its units already sold, in proportion
// to the receipt layer's residual, and appends one l_stock_value row: the
// historical receipt row is never rewritten (Requirement 11.6).
type CostDelta struct {
	// ReceiptRowID is the submitted receipt row the cost belongs to.
	ReceiptRowID string
	// Total is the signed base-currency cost to allocate. Positive adds
	// cost, negative (a credit note) removes it.
	Total types.Decimal
	// OffsetAccount is credited (or debited, for a negative total) with the
	// full amount, typically freight clearing. COGSAccount takes the sold
	// portion. Both must be enabled leaves. The stock control account comes
	// from the receipt row.
	OffsetAccount string
	COGSAccount   string
	Dimensions    map[string]string
	VoucherType   string
	VoucherID     string
	Revision      int64
	SourceLineNo  int
	PostingDate   string // YYYY-MM-DD, the allowed posting date
}

// PlannedDelta is a validated cost delta with its on-hand/sold split and
// balanced value legs.
type PlannedDelta struct {
	Delta     CostDelta
	Receipt   *stockRow
	OnHand    types.Decimal // signed on-hand portion
	Sold      types.Decimal // signed sold portion
	Legs      []posting.Line
	Bin       binKey
	Replayed  bool
	ReplayRow string
}

// AppliedDelta describes a committed cost delta.
type AppliedDelta struct {
	RowID          string `json:"row_id"`
	PostingGroupID string `json:"posting_group_id"`
	Replayed       bool   `json:"replayed"`
}

// PlanCostDelta validates a cost delta, locks the receipt's bin, splits the
// total by the receipt layer's residual fraction, and builds the balanced
// value legs: the on-hand portion debits stock, the sold portion debits
// cost of sales, and the offset takes the other side. It runs in the
// caller's transaction and must be followed by ApplyCostDelta in the same
// transaction once the caller has posted the legs.
func (s *Service) PlanCostDelta(ctx context.Context, tx *sql.Tx, d CostDelta) (PlannedDelta, error) {
	if err := checkDelta(d); err != nil {
		return PlannedDelta{}, err
	}
	if rowID, found, err := findDeltaRow(ctx, tx, d); err != nil {
		return PlannedDelta{}, err
	} else if found {
		return PlannedDelta{Delta: d, Replayed: true, ReplayRow: rowID}, nil
	}
	receipt, err := loadStockRow(ctx, tx, d.ReceiptRowID)
	if err != nil {
		return PlannedDelta{}, err
	}
	if receipt.Event != posting.EventSubmit || receipt.Qty.Sign() <= 0 || receipt.Reversed != "" || receipt.Reversal {
		return PlannedDelta{}, fmt.Errorf("%w: row %s is not a submitted receipt", ErrInvalid, d.ReceiptRowID)
	}
	masters := &ledger.Service{DB: tx, Dialect: s.Dialect}
	for name, account := range map[string]string{"offset": d.OffsetAccount, "cost of sales": d.COGSAccount} {
		stored, err := masters.GetAccount(ctx, account)
		if err != nil {
			if errors.Is(err, ledger.ErrNotFound) {
				return PlannedDelta{}, fmt.Errorf("%w: %s account %q does not exist", ErrInvalid, name, account)
			}
			return PlannedDelta{}, err
		}
		if stored.IsGroup {
			return PlannedDelta{}, fmt.Errorf("%w: %s account %q is a group", ErrInvalid, name, account)
		}
		if stored.Disabled {
			return PlannedDelta{}, fmt.Errorf("%w: %s account %q is disabled", ErrInvalid, name, account)
		}
	}
	layer, found, err := layerByReceipt(ctx, tx, d.ReceiptRowID)
	if err != nil {
		return PlannedDelta{}, err
	}
	if !found {
		return PlannedDelta{}, fmt.Errorf("%w: receipt row %s has no layer; deltas need a valued receipt", ErrInvalid, d.ReceiptRowID)
	}
	total, err := d.Total.Rescale(LedgerScale)
	if err != nil {
		return PlannedDelta{}, fmt.Errorf("%w: delta total needs rounding to 6 digits", ErrInvalid)
	}
	onHand, sold, err := splitDelta(total, layer)
	if err != nil {
		return PlannedDelta{}, err
	}
	planned := PlannedDelta{Delta: d, Receipt: receipt, OnHand: onHand, Sold: sold, Bin: receipt.Bin}
	if _, _, err := lockBin(ctx, tx, planned.Bin); err != nil {
		return PlannedDelta{}, err
	}
	if total.Sign() != 0 {
		legs := deltaLegs(d, receipt, onHand, sold, total)
		planned.Legs = legs
		fragment := posting.Plan{
			VoucherType: d.VoucherType,
			VoucherID:   d.VoucherID,
			Revision:    d.Revision,
			Event:       posting.EventSubmit,
			Book:        receipt.Bin.Book,
			PostingDate: d.PostingDate,
			Lines:       legs,
		}
		if err := posting.ValidatePlan(ctx, tx, fragment); err != nil {
			return PlannedDelta{}, fmt.Errorf("stock: delta legs: %w", err)
		}
	}
	return planned, nil
}

func checkDelta(d CostDelta) error {
	for name, value := range map[string]string{
		"receipt row": d.ReceiptRowID, "offset account": d.OffsetAccount,
		"cost of sales account": d.COGSAccount, "voucher type": d.VoucherType,
		"voucher id": d.VoucherID,
	} {
		if value == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalid, name)
		}
	}
	if d.OffsetAccount == d.COGSAccount {
		return fmt.Errorf("%w: offset and cost of sales accounts must differ", ErrInvalid)
	}
	if d.Revision < 1 {
		return fmt.Errorf("%w: voucher revision %d is below 1", ErrInvalid, d.Revision)
	}
	if d.SourceLineNo < 1 {
		return fmt.Errorf("%w: source line %d is below 1", ErrInvalid, d.SourceLineNo)
	}
	if !validDate(d.PostingDate) {
		return fmt.Errorf("%w: posting date must be YYYY-MM-DD", ErrInvalid)
	}
	if !fitsAmount(d.Total) {
		return fmt.Errorf("%w: delta total exceeds DECIMAL(28,6)", ErrInvalid)
	}
	return nil
}

// splitDelta divides a freight total between on-hand and sold units by the
// receipt layer's residual fraction. A fully stocked layer keeps the whole
// total on hand; a consumed one sends it all to sold.
func splitDelta(total types.Decimal, layer stockLayer) (types.Decimal, types.Decimal, error) {
	if layer.Original.Sign() <= 0 {
		return types.Decimal{}, types.Decimal{}, fmt.Errorf("%w: receipt layer has no original quantity", ErrIntegrity)
	}
	if layer.Residual.Sign() < 0 || layer.Residual.Cmp(layer.Original) > 0 {
		return types.Decimal{}, types.Decimal{}, fmt.Errorf("%w: receipt layer residual %s is outside its original %s", ErrIntegrity, layer.Residual.String(), layer.Original.String())
	}
	zero := types.MustDecimal("0.000000")
	if layer.Residual.Equal(layer.Original) {
		return total, zero, nil
	}
	if layer.Residual.Sign() == 0 {
		return zero, total, nil
	}
	scaled, err := total.Mul(layer.Residual).Div(layer.Original, LedgerScale+6, LedgerRounding)
	if err != nil {
		return types.Decimal{}, types.Decimal{}, err
	}
	onHand, err := round6(scaled)
	if err != nil {
		return types.Decimal{}, types.Decimal{}, err
	}
	return onHand, total.Sub(onHand), nil
}

// deltaLegs builds the balanced legs of a non-zero delta: on-hand debits
// stock, sold debits cost of sales, and the offset takes the other side.
// Negative totals flip every side. Zero portions post no leg.
func deltaLegs(d CostDelta, receipt *stockRow, onHand, sold, total types.Decimal) []posting.Line {
	company := receipt.Bin.Company
	legs := make([]posting.Line, 0, 3)
	add := func(account string, amount types.Decimal, isDebit, source bool) {
		if amount.Sign() == 0 {
			return
		}
		line := posting.Line{Company: company, Account: account, Dimensions: d.Dimensions}
		if isDebit {
			line.DebitBase = amount.Abs()
		} else {
			line.CreditBase = amount.Abs()
		}
		if source {
			line.SourceType = receipt.ItemType
			line.SourceID = receipt.Bin.ItemID
		}
		legs = append(legs, line)
	}
	positive := total.Sign() > 0
	add(receipt.Control, onHand, positive, true)
	add(d.COGSAccount, sold, positive, false)
	add(d.OffsetAccount, total, !positive, false)
	return legs
}

// ApplyCostDelta writes a planned cost delta in the caller's transaction:
// the l_stock_value row with the receipt's economic date and the allowed
// posting date, and the on-hand addition to the bin value. groupID is the
// posting group the caller created; when the plan posts no legs the caller
// passes "" and Apply claims a group of its own. Layers are untouched:
// the layer keeps the receipt's original unit cost while the delta rows
// carry every adjustment, so cancels never un-reblend. When the voucher
// line already has a delta row Apply returns the first write.
func (s *Service) ApplyCostDelta(ctx context.Context, tx *sql.Tx, p PlannedDelta, groupID string) (AppliedDelta, error) {
	d := p.Delta
	if p.Replayed {
		return replayDelta(ctx, tx, p.ReplayRow)
	}
	if len(p.Legs) > 0 && groupID == "" {
		return AppliedDelta{}, fmt.Errorf("%w: valued delta needs its posting group id", ErrInvalid)
	}
	if groupID == "" {
		claimed, err := s.claimGroup(ctx, tx, d.VoucherType, d.VoucherID, d.Revision, p.Receipt.Bin.Book, posting.EventSubmit)
		if err != nil {
			return AppliedDelta{}, err
		}
		groupID = claimed
	} else if err := lockGroup(ctx, tx, groupID); err != nil {
		return AppliedDelta{}, err
	}
	if rowID, found, err := findDeltaRow(ctx, tx, d); err != nil {
		return AppliedDelta{}, err
	} else if found {
		return replayDelta(ctx, tx, rowID)
	}
	bin, found, err := lockBin(ctx, tx, p.Bin)
	if err != nil {
		return AppliedDelta{}, err
	}
	if !found {
		return AppliedDelta{}, fmt.Errorf("%w: valued receipt has no bin", ErrIntegrity)
	}
	rowID, err := uuid.NewV7()
	if err != nil {
		return AppliedDelta{}, err
	}
	receipt := p.Receipt
	total, err := d.Total.Rescale(LedgerScale)
	if err != nil {
		return AppliedDelta{}, fmt.Errorf("%w: delta total needs rounding to 6 digits", ErrInvalid)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO l_stock_value (id, company_name, book, stock_row_id, item_type, item_id, control_account_name, delta_value, on_hand_delta, sold_delta, base_currency, economic_date, posting_date, voucher_type, voucher_id, voucher_revision, source_line_no, event, posting_group_id, is_reversal) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		rowID.String(), receipt.Bin.Company, receipt.Bin.Book, d.ReceiptRowID, receipt.ItemType, receipt.Bin.ItemID,
		receipt.Control, total.String(), p.OnHand.String(), p.Sold.String(), receipt.Currency,
		receipt.MovementDate, d.PostingDate,
		d.VoucherType, d.VoucherID, d.Revision, d.SourceLineNo, posting.EventSubmit, groupID); err != nil {
		return AppliedDelta{}, err
	}
	zero := types.Decimal{}
	if err := s.applyBinDelta(ctx, tx, p.Bin, receipt.ItemType, receipt.UoM, receipt.Currency, zero, p.OnHand, bin, found); err != nil {
		return AppliedDelta{}, err
	}
	return AppliedDelta{RowID: rowID.String(), PostingGroupID: groupID}, nil
}

// deltaRow is a stored l_stock_value row with its receipt's bin.
type deltaRow struct {
	ID           string
	ReceiptRowID string
	Bin          binKey
	ItemType     string
	Control      string
	Currency     string
	Total        types.Decimal
	OnHand       types.Decimal
	Sold         types.Decimal
	Economic     string
	SourceLine   int
	GroupID      string
}

// loadDeltaRow reads one l_stock_value row by id with the bin of its
// receipt row.
func loadDeltaRow(ctx context.Context, q ledger.DB, id string) (*deltaRow, error) {
	var row deltaRow
	var company, book, owner, itemID, location, lot, serial, total, onHand, sold string
	var economic time.Time
	err := q.QueryRowContext(ctx, `SELECT v.id, v.stock_row_id, s.company_name, s.book, s.owner, v.item_type, v.item_id, v.control_account_name, s.location_name, COALESCE(s.lot_no, ''), COALESCE(s.serial_no, ''), v.delta_value, v.on_hand_delta, v.sold_delta, v.base_currency, v.economic_date, v.source_line_no, v.posting_group_id FROM l_stock_value v JOIN l_stock s ON s.id = v.stock_row_id WHERE v.id = ?`,
		id).Scan(&row.ID, &row.ReceiptRowID, &company, &book, &owner, &row.ItemType, &itemID, &row.Control, &location, &lot, &serial,
		&total, &onHand, &sold, &row.Currency, &economic, &row.SourceLine, &row.GroupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: cost delta row %q not found", ErrInvalid, id)
		}
		return nil, err
	}
	row.Economic = economic.Format("2006-01-02")
	if row.Total, err = scanDecimal(total); err != nil {
		return nil, err
	}
	if row.OnHand, err = scanDecimal(onHand); err != nil {
		return nil, err
	}
	if row.Sold, err = scanDecimal(sold); err != nil {
		return nil, err
	}
	row.Bin = binKey{Company: company, Book: book, Owner: owner, ItemID: itemID, Location: location, LotNo: lot, SerialNo: serial}
	return &row, nil
}

// findDeltaRow returns the submitted delta row id of a voucher line,
// locking it.
func findDeltaRow(ctx context.Context, q ledger.DB, d CostDelta) (string, bool, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM l_stock_value WHERE voucher_type = ? AND voucher_id = ? AND voucher_revision = ? AND source_line_no = ? AND event = ? LIMIT 1 FOR UPDATE`,
		d.VoucherType, d.VoucherID, d.Revision, d.SourceLineNo, posting.EventSubmit).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return id, true, nil
}

func replayDelta(ctx context.Context, q ledger.DB, rowID string) (AppliedDelta, error) {
	row, err := loadDeltaRow(ctx, q, rowID)
	if err != nil {
		return AppliedDelta{}, err
	}
	return AppliedDelta{RowID: row.ID, PostingGroupID: row.GroupID, Replayed: true}, nil
}

// submitDeltaRowsOf loads the submitted, non-reversal delta rows of a
// voucher revision newest first, locking them.
func submitDeltaRowsOf(ctx context.Context, tx *sql.Tx, voucherType, voucherID string, revision int64, book string) ([]*deltaRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM l_stock_value WHERE voucher_type = ? AND voucher_id = ? AND voucher_revision = ? AND event = ? AND book = ? AND is_reversal = 0 ORDER BY id DESC FOR UPDATE`,
		voucherType, voucherID, revision, posting.EventSubmit, book)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []*deltaRow
	for _, id := range ids {
		row, err := loadDeltaRow(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// insertDeltaMirror writes one flagged reversal of a submitted cost delta
// row with negated splits under the cancel posting date. The bin unwound
// is the receipt's; layers stay untouched, as on the submit.
func insertDeltaMirror(ctx context.Context, tx *sql.Tx, p PlannedCancel, mirror deltaMirror, rowID, groupID string) error {
	orig := mirror.Orig
	bin := orig.Bin
	_, err := tx.ExecContext(ctx, `INSERT INTO l_stock_value (id, company_name, book, stock_row_id, item_type, item_id, control_account_name, delta_value, on_hand_delta, sold_delta, base_currency, economic_date, posting_date, voucher_type, voucher_id, voucher_revision, source_line_no, event, posting_group_id, is_reversal, reversed_row_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
		rowID, bin.Company, bin.Book, orig.ReceiptRowID, orig.ItemType, bin.ItemID, orig.Control,
		orig.Total.Neg().String(), orig.OnHand.Neg().String(), orig.Sold.Neg().String(), orig.Currency,
		orig.Economic, p.PostingDate,
		p.VoucherType, p.VoucherID, p.Revision, orig.SourceLine, posting.EventCancel, groupID, orig.ID)
	return err
}

// findCancelDeltaRows returns the cancel delta row ids of a voucher
// revision, locking them.
func findCancelDeltaRows(ctx context.Context, q ledger.DB, voucherType, voucherID string, revision int64, book string) ([]string, bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM l_stock_value WHERE voucher_type = ? AND voucher_id = ? AND voucher_revision = ? AND event = ? AND book = ? ORDER BY id FOR UPDATE`,
		voucherType, voucherID, revision, posting.EventCancel, book)
	if err != nil {
		return nil, false, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, false, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return ids, len(ids) > 0, nil
}
