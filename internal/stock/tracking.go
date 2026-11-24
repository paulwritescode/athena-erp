package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"athena-erp/internal/ledger"
	"athena-erp/internal/posting"
	"athena-erp/internal/types"

	"github.com/google/uuid"
)

// Serial statuses. A unit rests on hand, ships issued or on rent, and
// returns on hand; reservations mark it reserved while held.
const (
	SerialOnHand   = "on_hand"
	SerialReserved = "reserved"
	SerialIssued   = "issued"
	SerialOnRent   = "on_rent"
)

var (
	// ErrSerialState reports a serial transition from the wrong state, such
	// as issuing an already issued unit.
	ErrSerialState = errors.New("stock: serial is in the wrong state")
	// ErrDuplicateSerial reports a serial identity registered twice.
	ErrDuplicateSerial = errors.New("stock: serial already registered")
)

// RegisterSerials records unit identities for a receipt row: one serial per
// integral unit, each starting on hand at the receipt's location and owner.
// The serial count must equal the receipt quantity exactly.
func (s *Service) RegisterSerials(ctx context.Context, tx *sql.Tx, receiptRowID string, serials []string) error {
	row, err := loadStockRow(ctx, tx, receiptRowID)
	if err != nil {
		return err
	}
	if row.Event != posting.EventSubmit || row.Qty.Sign() <= 0 || row.Reversed != "" || row.Reversal {
		return fmt.Errorf("%w: row %s is not a submitted receipt", ErrInvalid, receiptRowID)
	}
	count, err := integralCount(row.Qty)
	if err != nil {
		return fmt.Errorf("%w: receipt quantity %s is not integral units", ErrInvalid, row.Qty.String())
	}
	if len(serials) != count {
		return fmt.Errorf("%w: receipt has %d units but %d serials", ErrInvalid, count, len(serials))
	}
	seen := map[string]bool{}
	for _, serial := range serials {
		if serial == "" || len(serial) > 140 {
			return fmt.Errorf("%w: serials must be 1-140 characters", ErrInvalid)
		}
		if seen[serial] {
			return fmt.Errorf("%w: serial %q listed twice", ErrInvalid, serial)
		}
		seen[serial] = true
	}
	for _, serial := range serials {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO athena_serials (id, company_name, book, item_type, item_id, serial_no, status, owner, location_name, lot_no, receipt_row_id) VALUES (?, ?, ?, ?, ?, ?, 'on_hand', ?, ?, ?, ?)`,
			id.String(), row.Bin.Company, row.Bin.Book, row.ItemType, row.Bin.ItemID, serial,
			row.Bin.Owner, row.Bin.Location, row.Bin.LotNo, receiptRowID); err != nil {
			if s.duplicate(err) {
				return fmt.Errorf("%w: serial %q", ErrDuplicateSerial, serial)
			}
			return err
		}
	}
	return nil
}

func integralCount(qty types.Decimal) (int, error) {
	whole, err := qty.Rescale(0)
	if err != nil {
		return 0, err
	}
	var count int
	if _, err := fmt.Sscan(whole.String(), &count); err != nil {
		return 0, err
	}
	if count < 0 {
		count = -count
	}
	return count, nil
}

// SerialOut ships one unit to issued (consumed) or on_rent (rented out,
// tracked but never consumed).
type SerialOut struct {
	SerialNo string
	Status   string
}

// IssueSerials ships units for an issue row, moving each serial from on
// hand (or reserved) to its out status at the issue's location and owner,
// and consuming any active hold on each serial. The serial count must equal
// the issue magnitude exactly.
func (s *Service) IssueSerials(ctx context.Context, tx *sql.Tx, issueRowID string, outs []SerialOut) error {
	row, err := loadStockRow(ctx, tx, issueRowID)
	if err != nil {
		return err
	}
	if row.Event != posting.EventSubmit || row.Qty.Sign() >= 0 || row.Reversal {
		return fmt.Errorf("%w: row %s is not a submitted issue", ErrInvalid, issueRowID)
	}
	count, err := integralCount(row.Qty)
	if err != nil {
		return fmt.Errorf("%w: issue quantity %s is not integral units", ErrInvalid, row.Qty.String())
	}
	if len(outs) != count {
		return fmt.Errorf("%w: issue has %d units but %d serials", ErrInvalid, count, len(outs))
	}
	for _, out := range outs {
		if out.Status != SerialIssued && out.Status != SerialOnRent {
			return fmt.Errorf("%w: out status %q must be issued or on_rent", ErrInvalid, out.Status)
		}
		unit, err := lockSerial(ctx, tx, row, out.SerialNo)
		if err != nil {
			return err
		}
		if unit != SerialOnHand && unit != SerialReserved {
			return fmt.Errorf("%w: serial %q is %s", ErrSerialState, out.SerialNo, unit)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE athena_serials SET status = ?, owner = ?, location_name = ?, issue_row_id = ?, revision = revision + 1 WHERE company_name = ? AND book = ? AND item_id = ? AND serial_no = ?`,
			out.Status, row.Bin.Owner, row.Bin.Location, issueRowID, row.Bin.Company, row.Bin.Book, row.Bin.ItemID, out.SerialNo); err != nil {
			return err
		}
		if err := s.consumeSerialHold(ctx, tx, row, out.SerialNo); err != nil {
			return err
		}
	}
	return nil
}

// ReturnSerials brings units home for a return row, moving each serial from
// issued or on rent back on hand at the return's location and owner. The
// issue link stays: it names the issue a return cancel restores.
func (s *Service) ReturnSerials(ctx context.Context, tx *sql.Tx, returnRowID string, serials []string) error {
	row, err := loadStockRow(ctx, tx, returnRowID)
	if err != nil {
		return err
	}
	if row.Event != posting.EventSubmit || row.Qty.Sign() <= 0 || row.Reversed == "" || row.Reversal {
		return fmt.Errorf("%w: row %s is not a submitted return", ErrInvalid, returnRowID)
	}
	count, err := integralCount(row.Qty)
	if err != nil {
		return fmt.Errorf("%w: return quantity %s is not integral units", ErrInvalid, row.Qty.String())
	}
	if len(serials) != count {
		return fmt.Errorf("%w: return has %d units but %d serials", ErrInvalid, count, len(serials))
	}
	for _, serial := range serials {
		unit, err := lockSerial(ctx, tx, row, serial)
		if err != nil {
			return err
		}
		if unit != SerialIssued && unit != SerialOnRent {
			return fmt.Errorf("%w: serial %q is %s", ErrSerialState, serial, unit)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE athena_serials SET status = 'on_hand', owner = ?, location_name = ?, revision = revision + 1 WHERE company_name = ? AND book = ? AND item_id = ? AND serial_no = ?`,
			row.Bin.Owner, row.Bin.Location, row.Bin.Company, row.Bin.Book, row.Bin.ItemID, serial); err != nil {
			return err
		}
	}
	return nil
}

// lockSerial returns a unit's status, locking it.
func lockSerial(ctx context.Context, tx *sql.Tx, row *stockRow, serial string) (string, error) {
	var status string
	err := tx.QueryRowContext(ctx, `SELECT status FROM athena_serials WHERE company_name = ? AND book = ? AND item_id = ? AND serial_no = ? FOR UPDATE`,
		row.Bin.Company, row.Bin.Book, row.Bin.ItemID, serial).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: no such unit %q", ErrInvalid, serial)
		}
		return "", err
	}
	return status, nil
}

// consumeSerialHold consumes the active hold on a shipped serial, if any.
// Availability allows at most one active hold per serial, so the hold is
// unambiguous.
func (s *Service) consumeSerialHold(ctx context.Context, tx *sql.Tx, row *stockRow, serial string) error {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM athena_reservations WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND serial_no = ? AND status = 'active' LIMIT 1 FOR UPDATE`,
		row.Bin.Company, row.Bin.Book, row.Bin.Owner, row.Bin.ItemID, row.Bin.Location, serial).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	return s.closeReservation(ctx, tx, id, types.Decimal{}, ReserveConsumed)
}

// holdSerial marks a unit reserved for a hold. The unit must exist on
// hand.
func holdSerial(ctx context.Context, tx *sql.Tx, key binKey) error {
	var status string
	err := tx.QueryRowContext(ctx, `SELECT status FROM athena_serials WHERE company_name = ? AND book = ? AND item_id = ? AND serial_no = ? FOR UPDATE`,
		key.Company, key.Book, key.ItemID, key.SerialNo).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: no such unit %q", ErrInvalid, key.SerialNo)
		}
		return err
	}
	if status != SerialOnHand {
		return fmt.Errorf("%w: serial %q is %s", ErrSerialState, key.SerialNo, status)
	}
	_, err = tx.ExecContext(ctx, `UPDATE athena_serials SET status = 'reserved', revision = revision + 1 WHERE company_name = ? AND book = ? AND item_id = ? AND serial_no = ?`,
		key.Company, key.Book, key.ItemID, key.SerialNo)
	return err
}

// unholdSerial frees a held unit back on hand.
func unholdSerial(ctx context.Context, tx *sql.Tx, key binKey) error {
	var status string
	err := tx.QueryRowContext(ctx, `SELECT status FROM athena_serials WHERE company_name = ? AND book = ? AND item_id = ? AND serial_no = ? FOR UPDATE`,
		key.Company, key.Book, key.ItemID, key.SerialNo).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: held serial %q is gone", ErrIntegrity, key.SerialNo)
		}
		return err
	}
	if status != SerialReserved {
		return fmt.Errorf("%w: serial %q is %s, not reserved", ErrSerialState, key.SerialNo, status)
	}
	_, err = tx.ExecContext(ctx, `UPDATE athena_serials SET status = 'on_hand', revision = revision + 1 WHERE company_name = ? AND book = ? AND item_id = ? AND serial_no = ?`,
		key.Company, key.Book, key.ItemID, key.SerialNo)
	return err
}

// ReverseReceiptSerials deletes the units a cancelled receipt created. Any
// unit that moved past on hand blocks the cancel: release its hold or
// unwind its issue first.
func (s *Service) ReverseReceiptSerials(ctx context.Context, tx *sql.Tx, receiptRowID string) error {
	var moved int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_serials WHERE receipt_row_id = ? AND status <> 'on_hand' FOR UPDATE`, receiptRowID).Scan(&moved); err != nil {
		return err
	}
	if moved > 0 {
		return fmt.Errorf("%w: %d units moved past on hand", ErrCancelBlocked, moved)
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM athena_serials WHERE receipt_row_id = ? AND status = 'on_hand'`, receiptRowID)
	return err
}

// ReverseIssueSerials restores the units a cancelled issue shipped to
// toStatus (usually on hand): every unit the issue link names must still
// be issued or on rent.
func (s *Service) ReverseIssueSerials(ctx context.Context, tx *sql.Tx, issueRowID, toStatus string) error {
	if toStatus != SerialOnHand && toStatus != SerialReserved {
		return fmt.Errorf("%w: restore status %q must be on_hand or reserved", ErrInvalid, toStatus)
	}
	var total, away int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_serials WHERE issue_row_id = ? FOR UPDATE`, issueRowID).Scan(&total); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_serials WHERE issue_row_id = ? AND status IN ('issued', 'on_rent') FOR UPDATE`, issueRowID).Scan(&away); err != nil {
		return err
	}
	if away != total {
		return fmt.Errorf("%w: %d of %d units moved on", ErrCancelBlocked, total-away, total)
	}
	_, err := tx.ExecContext(ctx, `UPDATE athena_serials SET status = ?, issue_row_id = NULL, revision = revision + 1 WHERE issue_row_id = ?`, toStatus, issueRowID)
	return err
}

// ReverseReturnSerials puts back the units a cancelled return brought home,
// to the status the caller names (issued, or on_rent when the reversed
// issue was a rental): the return row's units must be exactly attributable,
// either by its serial or, for multi-unit returns, by matching the return
// quantity against the on-hand units of the reversed issue.
func (s *Service) ReverseReturnSerials(ctx context.Context, tx *sql.Tx, returnRowID, toStatus string) error {
	if toStatus != SerialIssued && toStatus != SerialOnRent {
		return fmt.Errorf("%w: restore status %q must be issued or on_rent", ErrInvalid, toStatus)
	}
	row, err := loadStockRow(ctx, tx, returnRowID)
	if err != nil {
		return err
	}
	serials, err := s.returnSerials(ctx, tx, row)
	if err != nil {
		return err
	}
	for _, serial := range serials {
		if _, err := tx.ExecContext(ctx, `UPDATE athena_serials SET status = ?, owner = ?, location_name = ?, revision = revision + 1 WHERE company_name = ? AND book = ? AND item_id = ? AND serial_no = ?`,
			toStatus, row.Bin.Owner, row.Bin.Location, row.Bin.Company, row.Bin.Book, row.Bin.ItemID, serial); err != nil {
			return err
		}
	}
	return nil
}

// returnSerials attributes on-hand units to a return row.
func (s *Service) returnSerials(ctx context.Context, tx *sql.Tx, row *stockRow) ([]string, error) {
	issue, err := loadStockRow(ctx, tx, row.Reversed)
	if err != nil {
		return nil, err
	}
	if row.Bin.SerialNo != "" {
		var status string
		var linkNull sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT status, issue_row_id FROM athena_serials WHERE company_name = ? AND book = ? AND item_id = ? AND serial_no = ? FOR UPDATE`,
			row.Bin.Company, row.Bin.Book, row.Bin.ItemID, row.Bin.SerialNo).Scan(&status, &linkNull)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("%w: serial %q is gone", ErrCancelBlocked, row.Bin.SerialNo)
			}
			return nil, err
		}
		if !linkNull.Valid || linkNull.String != issue.ID || status != SerialOnHand {
			return nil, fmt.Errorf("%w: serial %q moved on", ErrCancelBlocked, row.Bin.SerialNo)
		}
		return []string{row.Bin.SerialNo}, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT serial_no FROM athena_serials WHERE company_name = ? AND book = ? AND item_id = ? AND issue_row_id = ? AND status = 'on_hand' AND owner = ? AND location_name = ? ORDER BY serial_no FOR UPDATE`,
		row.Bin.Company, row.Bin.Book, row.Bin.ItemID, row.Reversed, row.Bin.Owner, row.Bin.Location)
	if err != nil {
		return nil, err
	}
	var serials []string
	for rows.Next() {
		var serial string
		if err := rows.Scan(&serial); err != nil {
			rows.Close()
			return nil, err
		}
		serials = append(serials, serial)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	count, err := integralCount(row.Qty)
	if err != nil {
		return nil, err
	}
	if len(serials) != count {
		return nil, fmt.Errorf("%w: %d units match a %d-unit return", ErrCancelBlocked, len(serials), count)
	}
	return serials, nil
}

// EnsureBatch records a lot's identity at receipt: one row per lot with
// expiry and received dates. The first receipt wins; later receipts of the
// same lot leave the record untouched.
func (s *Service) EnsureBatch(ctx context.Context, tx *sql.Tx, receiptRowID, lotNo, expiry, received string) error {
	if lotNo == "" || len(lotNo) > 140 {
		return fmt.Errorf("%w: lot must be 1-140 characters", ErrInvalid)
	}
	for name, date := range map[string]string{"expiry": expiry, "received": received} {
		if date != "" && !validDate(date) {
			return fmt.Errorf("%w: %s date must be YYYY-MM-DD", ErrInvalid, name)
		}
	}
	row, err := loadStockRow(ctx, tx, receiptRowID)
	if err != nil {
		return err
	}
	if row.Event != posting.EventSubmit || row.Qty.Sign() <= 0 || row.Reversed != "" || row.Reversal {
		return fmt.Errorf("%w: row %s is not a submitted receipt", ErrInvalid, receiptRowID)
	}
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id FROM athena_batches WHERE company_name = ? AND book = ? AND item_id = ? AND lot_no = ? FOR UPDATE`,
		row.Bin.Company, row.Bin.Book, row.Bin.ItemID, lotNo).Scan(&id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	batchID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO athena_batches (id, company_name, book, item_type, item_id, lot_no, expiry_date, received_date, status, receipt_row_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'on_hand', ?)`,
		batchID.String(), row.Bin.Company, row.Bin.Book, row.ItemType, row.Bin.ItemID, lotNo,
		nullDate(expiry), nullDate(received), receiptRowID); err != nil {
		if s.duplicate(err) {
			return nil
		}
		return err
	}
	return nil
}

func nullDate(date string) any {
	if date == "" {
		return nil
	}
	return date
}

// BatchPick takes part of a lot's available units.
type BatchPick struct {
	LotNo string
	Qty   types.Decimal
}

// PickFEFO lists lots oldest-expiry-first covering qty from the available
// (on-hand minus reserved) units at a location, and the uncovered
// shortfall. Lots without an expiry pick last. An empty owner picks across
// all owners.
func (s *Service) PickFEFO(ctx context.Context, q ledger.DB, company, book, owner, itemID, location string, qty types.Decimal) ([]BatchPick, types.Decimal, error) {
	if qty.Sign() <= 0 {
		return nil, types.Decimal{}, fmt.Errorf("%w: pick quantity must be above zero", ErrInvalid)
	}
	query := `SELECT b.lot_no, COALESCE(SUM(n.on_hand - n.reserved_qty), 0) FROM athena_batches b LEFT JOIN athena_stock_bins n ON n.company_name = b.company_name AND n.book = b.book AND n.item_id = b.item_id AND n.location_name = ? AND COALESCE(n.lot_no, '') = b.lot_no AND (? = '' OR n.owner = ?) WHERE b.company_name = ? AND b.book = ? AND b.item_id = ? GROUP BY b.lot_no, b.expiry_date HAVING COALESCE(SUM(n.on_hand - n.reserved_qty), 0) > 0 ORDER BY b.expiry_date IS NULL, b.expiry_date, b.lot_no`
	rows, err := q.QueryContext(ctx, query, location, owner, owner, company, book, itemID)
	if err != nil {
		return nil, types.Decimal{}, err
	}
	defer rows.Close()
	var picks []BatchPick
	remaining := qty
	for rows.Next() && remaining.Sign() > 0 {
		var lot, available string
		if err := rows.Scan(&lot, &available); err != nil {
			return nil, types.Decimal{}, err
		}
		have, err := scanDecimal(available)
		if err != nil {
			return nil, types.Decimal{}, err
		}
		take := have
		if take.Cmp(remaining) > 0 {
			take = remaining
		}
		remaining = remaining.Sub(take)
		picks = append(picks, BatchPick{LotNo: lot, Qty: take})
	}
	if err := rows.Err(); err != nil {
		return nil, types.Decimal{}, err
	}
	return picks, remaining, nil
}

// Transfer moves base-unit stock between bins: a location change, an
// ownership change, or a move into quarantine. The company value does not
// change: the issue takes the moving average and the receipt rebooks it,
// with offsetting legs on the transit account under one voucher and one
// posting group. Held units cannot move: the transfer quantity must be
// available at the source.
type Transfer struct {
	Company        string
	Book           string
	ItemType       string
	ItemID         string
	ControlAccount string
	TransitAccount string
	FromOwner      string
	FromLocation   string
	ToOwner        string
	ToLocation     string
	LotNo          string
	SerialNo       string
	Quantity       types.Quantity // positive entered quantity
	Valued         bool
	MovementDate   string
	PostingDate    string
	Dimensions     map[string]string
	VoucherType    string
	VoucherID      string
	Revision       int64
}

// PlannedTransfer is a validated transfer: the issue (line 1) and receipt
// (line 2) plans with their combined balanced legs.
type PlannedTransfer struct {
	Transfer Transfer
	Issue    PlannedMovement
	Receipt  PlannedMovement
	Legs     []posting.Line
}

// AppliedTransfer describes a committed transfer.
type AppliedTransfer struct {
	Issue          AppliedMovement
	Receipt        AppliedMovement
	PostingGroupID string
}

// PlanTransfer validates a transfer and plans its issue and receipt legs
// in the caller's transaction. The caller posts the combined legs once
// with posting.Engine.PostInTx and applies both lines under the group with
// ApplyTransfer.
func (s *Service) PlanTransfer(ctx context.Context, tx *sql.Tx, t Transfer) (PlannedTransfer, error) {
	if t.Quantity.Value.Sign() <= 0 {
		return PlannedTransfer{}, fmt.Errorf("%w: transfer quantity must be above zero", ErrInvalid)
	}
	if (t.FromOwner == t.ToOwner && t.FromLocation == t.ToLocation) || t.FromOwner == "" || t.ToOwner == "" || t.FromLocation == "" || t.ToLocation == "" {
		return PlannedTransfer{}, fmt.Errorf("%w: transfer needs two distinct non-empty bins", ErrInvalid)
	}
	issue := Movement{
		Company: t.Company, Book: t.Book, Owner: t.FromOwner,
		ItemType: t.ItemType, ItemID: t.ItemID,
		ControlAccount: t.ControlAccount, OffsetAccount: t.TransitAccount,
		Location: t.FromLocation, LotNo: t.LotNo, SerialNo: t.SerialNo,
		Quantity:     types.Quantity{Value: t.Quantity.Value.Neg(), UoM: t.Quantity.UoM},
		Valued:       t.Valued,
		MovementDate: t.MovementDate, PostingDate: t.PostingDate, Dimensions: t.Dimensions,
		VoucherType: t.VoucherType, VoucherID: t.VoucherID, Revision: t.Revision, SourceLineNo: 1,
	}
	plannedIssue, err := s.PlanMovement(ctx, tx, issue)
	if err != nil {
		return PlannedTransfer{}, err
	}
	if plannedIssue.Replayed {
		return PlannedTransfer{}, fmt.Errorf("%w: transfer voucher line 1 already posted", ErrInvalid)
	}
	receipt := Movement{
		Company: t.Company, Book: t.Book, Owner: t.ToOwner,
		ItemType: t.ItemType, ItemID: t.ItemID,
		ControlAccount: t.ControlAccount, OffsetAccount: t.TransitAccount,
		Location: t.ToLocation, LotNo: t.LotNo, SerialNo: t.SerialNo,
		Quantity:     t.Quantity,
		UnitCost:     plannedIssue.UnitCost,
		Valued:       t.Valued,
		MovementDate: t.MovementDate, PostingDate: t.PostingDate, Dimensions: t.Dimensions,
		VoucherType: t.VoucherType, VoucherID: t.VoucherID, Revision: t.Revision, SourceLineNo: 2,
	}
	plannedReceipt, err := s.PlanMovement(ctx, tx, receipt)
	if err != nil {
		return PlannedTransfer{}, err
	}
	if plannedReceipt.Replayed {
		return PlannedTransfer{}, fmt.Errorf("%w: transfer voucher line 2 already posted", ErrInvalid)
	}
	held, err := sumActiveHolds(ctx, tx, plannedIssue.Bin)
	if err != nil {
		return PlannedTransfer{}, err
	}
	if available := plannedIssue.BinBefore.Qty.Sub(held); available.Cmp(plannedIssue.BaseQty.Neg()) < 0 {
		return PlannedTransfer{}, fmt.Errorf("%w: source holds %s available", ErrInsufficientAvailable, available.String())
	}
	planned := PlannedTransfer{Transfer: t, Issue: plannedIssue, Receipt: plannedReceipt}
	planned.Legs = append(append([]posting.Line{}, plannedIssue.Legs...), plannedReceipt.Legs...)
	return planned, nil
}

// ApplyTransfer writes a planned transfer's issue and receipt lines under
// one posting group: the caller's group when the plan posts legs, or a
// claimed group for a quantity-only transfer.
func (s *Service) ApplyTransfer(ctx context.Context, tx *sql.Tx, p PlannedTransfer, groupID string) (AppliedTransfer, error) {
	if groupID == "" {
		if len(p.Legs) > 0 {
			return AppliedTransfer{}, fmt.Errorf("%w: valued transfer needs its posting group id", ErrInvalid)
		}
		claimed, err := s.claimGroup(ctx, tx, p.Transfer.VoucherType, p.Transfer.VoucherID, p.Transfer.Revision, p.Transfer.Book, posting.EventSubmit)
		if err != nil {
			return AppliedTransfer{}, err
		}
		groupID = claimed
	} else if err := lockGroup(ctx, tx, groupID); err != nil {
		return AppliedTransfer{}, err
	}
	issue, err := s.ApplyMovement(ctx, tx, p.Issue, groupID)
	if err != nil {
		return AppliedTransfer{}, err
	}
	receipt, err := s.ApplyMovement(ctx, tx, p.Receipt, groupID)
	if err != nil {
		return AppliedTransfer{}, err
	}
	if p.Transfer.SerialNo != "" {
		var status string
		err := tx.QueryRowContext(ctx, `SELECT status FROM athena_serials WHERE company_name = ? AND book = ? AND item_id = ? AND serial_no = ? FOR UPDATE`,
			p.Transfer.Company, p.Transfer.Book, p.Transfer.ItemID, p.Transfer.SerialNo).Scan(&status)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return AppliedTransfer{}, fmt.Errorf("%w: no such unit %q", ErrInvalid, p.Transfer.SerialNo)
			}
			return AppliedTransfer{}, err
		}
		if status != SerialOnHand {
			return AppliedTransfer{}, fmt.Errorf("%w: serial %q is %s", ErrSerialState, p.Transfer.SerialNo, status)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE athena_serials SET owner = ?, location_name = ?, revision = revision + 1 WHERE company_name = ? AND book = ? AND item_id = ? AND serial_no = ?`,
			p.Transfer.ToOwner, p.Transfer.ToLocation, p.Transfer.Company, p.Transfer.Book, p.Transfer.ItemID, p.Transfer.SerialNo); err != nil {
			return AppliedTransfer{}, err
		}
	}
	return AppliedTransfer{Issue: issue, Receipt: receipt, PostingGroupID: groupID}, nil
}
