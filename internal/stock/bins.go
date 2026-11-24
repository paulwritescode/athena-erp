package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"athena-erp/internal/ledger"
	"athena-erp/internal/types"

	"github.com/google/uuid"
)

// binKey is one bin balance grain: company, book, owner, item, location,
// lot and serial. LotNo and SerialNo are "" when untracked.
type binKey struct {
	Company  string
	Book     string
	Owner    string
	ItemID   string
	Location string
	LotNo    string
	SerialNo string
}

// BinKey is the exported bin grain for readers.
type BinKey = binKey

func binKeyOf(m Movement) binKey {
	return binKey{
		Company: m.Company, Book: m.Book, Owner: m.Owner, ItemID: m.ItemID,
		Location: m.Location, LotNo: m.LotNo, SerialNo: m.SerialNo,
	}
}

// binState is a stored bin balance: on-hand and reserved base units with
// the on-hand value.
type binState struct {
	Qty      types.Decimal
	Reserved types.Decimal
	Value    types.Decimal
	UoM      string
	Currency string
	ItemType string
	Revision int64
}

// BinState is the exported bin balance for readers.
type BinState = binState

// lockBin returns a bin balance, locking it. A missing bin returns the
// zero state with found false.
func lockBin(ctx context.Context, tx *sql.Tx, key binKey) (binState, bool, error) {
	var bin binState
	var qty, reserved, value string
	err := tx.QueryRowContext(ctx, `SELECT item_type, base_uom, on_hand, reserved_qty, value_on_hand, base_currency, revision FROM athena_stock_bins WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND lot_no = ? AND serial_no = ? FOR UPDATE`,
		key.Company, key.Book, key.Owner, key.ItemID, key.Location, key.LotNo, key.SerialNo).Scan(
		&bin.ItemType, &bin.UoM, &qty, &reserved, &value, &bin.Currency, &bin.Revision)
	if err != nil {
		if err == sql.ErrNoRows {
			return binState{}, false, nil
		}
		return binState{}, false, err
	}
	if bin.Qty, err = scanDecimal(qty); err != nil {
		return binState{}, false, err
	}
	if bin.Reserved, err = scanDecimal(reserved); err != nil {
		return binState{}, false, err
	}
	if bin.Value, err = scanDecimal(value); err != nil {
		return binState{}, false, err
	}
	return bin, true, nil
}

// checkBinMatch rejects a movement whose base unit or item type differs
// from the bin's: one bin holds one base unit of one item type.
func checkBinMatch(key binKey, bin binState, itemType, baseUoM string) error {
	if bin.ItemType != "" && bin.ItemType != itemType {
		return fmt.Errorf("%w: bin holds item type %s, movement names %s", ErrInvalid, bin.ItemType, itemType)
	}
	if bin.UoM != baseUoM {
		return fmt.Errorf("%w: bin holds %s, movement converts to %s", ErrInvalid, bin.UoM, baseUoM)
	}
	if bin.Currency == "" {
		return fmt.Errorf("%w: bin has no currency", ErrIntegrity)
	}
	return nil
}

// applyBinDelta adds signed quantity and value to a bin, inserting the bin
// on first use. A duplicate insert (a racing first receipt) reselects and
// updates once.
func (s *Service) applyBinDelta(ctx context.Context, tx *sql.Tx, key binKey, itemType, uom, currency string, qtyDelta, valueDelta types.Decimal, bin binState, found bool) error {
	if found {
		_, err := tx.ExecContext(ctx, `UPDATE athena_stock_bins SET on_hand = on_hand + ?, value_on_hand = value_on_hand + ?, revision = revision + 1 WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND lot_no = ? AND serial_no = ?`,
			qtyDelta.String(), valueDelta.String(), key.Company, key.Book, key.Owner, key.ItemID, key.Location, key.LotNo, key.SerialNo)
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO athena_stock_bins (company_name, book, owner, item_type, item_id, location_name, lot_no, serial_no, base_uom, on_hand, reserved_qty, value_on_hand, base_currency) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		key.Company, key.Book, key.Owner, itemType, key.ItemID, key.Location, key.LotNo, key.SerialNo, uom, qtyDelta.String(), valueDelta.String(), currency)
	if err == nil {
		return nil
	}
	if !s.duplicate(err) {
		return err
	}
	if _, _, err := lockBin(ctx, tx, key); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE athena_stock_bins SET on_hand = on_hand + ?, value_on_hand = value_on_hand + ?, revision = revision + 1 WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND lot_no = ? AND serial_no = ?`,
		qtyDelta.String(), valueDelta.String(), key.Company, key.Book, key.Owner, key.ItemID, key.Location, key.LotNo, key.SerialNo)
	return err
}

// BinBalance reads one bin balance without locking it.
func (s *Service) BinBalance(ctx context.Context, db ledger.DB, key BinKey) (BinState, bool, error) {
	var bin binState
	var qty, reserved, value string
	err := db.QueryRowContext(ctx, `SELECT item_type, base_uom, on_hand, reserved_qty, value_on_hand, base_currency, revision FROM athena_stock_bins WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND lot_no = ? AND serial_no = ?`,
		key.Company, key.Book, key.Owner, key.ItemID, key.Location, key.LotNo, key.SerialNo).Scan(
		&bin.ItemType, &bin.UoM, &qty, &reserved, &value, &bin.Currency, &bin.Revision)
	if err != nil {
		if err == sql.ErrNoRows {
			return binState{}, false, nil
		}
		return binState{}, false, err
	}
	if bin.Qty, err = scanDecimal(qty); err != nil {
		return binState{}, false, err
	}
	if bin.Reserved, err = scanDecimal(reserved); err != nil {
		return binState{}, false, err
	}
	if bin.Value, err = scanDecimal(value); err != nil {
		return binState{}, false, err
	}
	return bin, true, nil
}

// rebuildChunkRows is the most l_stock rows one rebuild chunk walks.
const rebuildChunkRows = 500

// RebuildBins recomputes every bin balance of a company and book from the
// stock ledger: signed quantities and values from l_stock including cancel
// mirrors, on-hand cost deltas joined through their rows, and active
// reservations. Bins assume one base unit and currency per item, which the
// writer enforces.
//
// The rebuild walks l_stock id order in chunks, recomputing every touched
// grain fully and committing its watermark after each chunk. An
// interrupted rebuild restarts from its committed watermark: calling
// RebuildBins again resumes it, and PendingRebuild stays true until a run
// completes. Grain recomputes overwrite, so resume and overlap are
// idempotent; writers interleave safely because each chunk locks the bins
// it rewrites. Pass *sql.DB for crash-safe watermarking: inside a caller
// transaction the whole rebuild runs single-shot.
func (s *Service) RebuildBins(ctx context.Context, db ledger.DB, company, book string) (int, error) {
	if tx, ok := db.(*sql.Tx); ok {
		return s.rebuildAllInTx(ctx, tx, company, book)
	}
	beginner, ok := db.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return 0, errors.New("stock: database does not support transactions")
	}
	control, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	rebuildID, watermark, err := s.startOrResumeInTx(ctx, control, company, book)
	if err != nil {
		control.Rollback()
		return 0, err
	}
	if err := control.Commit(); err != nil {
		return 0, err
	}
	for {
		chunk, err := beginner.BeginTx(ctx, nil)
		if err != nil {
			return 0, err
		}
		bound, done, err := s.rebuildChunkInTx(ctx, chunk, company, book, rebuildID, watermark)
		if err != nil {
			chunk.Rollback()
			return 0, err
		}
		if err := chunk.Commit(); err != nil {
			return 0, err
		}
		if done {
			break
		}
		watermark = bound
	}
	finish, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer finish.Rollback()
	if _, err := finish.ExecContext(ctx, `UPDATE athena_stock_rebuilds SET state = 'completed', completed_at = CURRENT_TIMESTAMP WHERE id = ?`, rebuildID); err != nil {
		return 0, err
	}
	var count int
	if err := finish.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_stock_bins WHERE company_name = ? AND book = ?`, company, book).Scan(&count); err != nil {
		return 0, err
	}
	if err := finish.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

// rebuildAllInTx runs the whole rebuild inside a caller transaction: one
// running row, chunks without intermediate commits, then completion.
func (s *Service) rebuildAllInTx(ctx context.Context, tx *sql.Tx, company, book string) (int, error) {
	rebuildID, watermark, err := s.startOrResumeInTx(ctx, tx, company, book)
	if err != nil {
		return 0, err
	}
	for {
		bound, done, err := s.rebuildChunkInTx(ctx, tx, company, book, rebuildID, watermark)
		if err != nil {
			return 0, err
		}
		if done {
			break
		}
		watermark = bound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE athena_stock_rebuilds SET state = 'completed', completed_at = CURRENT_TIMESTAMP WHERE id = ?`, rebuildID); err != nil {
		return 0, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_stock_bins WHERE company_name = ? AND book = ?`, company, book).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// startOrResumeInTx returns the running rebuild for a company and book, or
// inserts one. Concurrent rebuilds share the running row: chunk overwrites
// are idempotent, so overlap only repeats work.
func (s *Service) startOrResumeInTx(ctx context.Context, tx *sql.Tx, company, book string) (string, string, error) {
	var id string
	var watermark sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT id, watermark_row_id FROM athena_stock_rebuilds WHERE company_name = ? AND book = ? AND state = 'running' ORDER BY started_at DESC LIMIT 1 FOR UPDATE`,
		company, book).Scan(&id, &watermark)
	if err == nil {
		if watermark.Valid {
			return id, watermark.String, nil
		}
		return id, "", nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	rebuildID, err := uuid.NewV7()
	if err != nil {
		return "", "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO athena_stock_rebuilds (id, company_name, book, state) VALUES (?, ?, ?, 'running')`,
		rebuildID.String(), company, book); err != nil {
		return "", "", err
	}
	return rebuildID.String(), "", nil
}

// rebuildChunkInTx recomputes the grains of the next chunk of l_stock rows
// after the watermark and advances the watermark to the chunk bound. It
// reports done when no rows remain.
func (s *Service) rebuildChunkInTx(ctx context.Context, tx *sql.Tx, company, book, rebuildID, watermark string) (string, bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM l_stock WHERE company_name = ? AND book = ? AND id > ? ORDER BY id LIMIT ? FOR UPDATE`,
		company, book, watermark, rebuildChunkRows)
	if err != nil {
		return "", false, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return "", false, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", false, err
	}
	if len(ids) == 0 {
		return "", true, nil
	}
	bound := ids[len(ids)-1]
	grains, err := tx.QueryContext(ctx, `SELECT DISTINCT owner, item_id, location_name, COALESCE(lot_no, ''), COALESCE(serial_no, '') FROM l_stock WHERE company_name = ? AND book = ? AND id > ? AND id <= ?`,
		company, book, watermark, bound)
	if err != nil {
		return "", false, err
	}
	var keys []binKey
	for grains.Next() {
		var key binKey
		key.Company, key.Book = company, book
		if err := grains.Scan(&key.Owner, &key.ItemID, &key.Location, &key.LotNo, &key.SerialNo); err != nil {
			grains.Close()
			return "", false, err
		}
		keys = append(keys, key)
	}
	grains.Close()
	if err := grains.Err(); err != nil {
		return "", false, err
	}
	for _, key := range keys {
		if _, _, err := lockBin(ctx, tx, key); err != nil {
			return "", false, err
		}
		rebuilt, err := aggregateGrain(ctx, tx, key)
		if err != nil {
			return "", false, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM athena_stock_bins WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND lot_no = ? AND serial_no = ?`,
			key.Company, key.Book, key.Owner, key.ItemID, key.Location, key.LotNo, key.SerialNo); err != nil {
			return "", false, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO athena_stock_bins (company_name, book, owner, item_type, item_id, location_name, lot_no, serial_no, base_uom, on_hand, reserved_qty, value_on_hand, base_currency) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			key.Company, key.Book, key.Owner, rebuilt.itemType, key.ItemID, key.Location, key.LotNo, key.SerialNo,
			rebuilt.uom, rebuilt.qty.String(), rebuilt.reserved.String(), rebuilt.value.String(), rebuilt.currency); err != nil {
			return "", false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE athena_stock_rebuilds SET watermark_row_id = ? WHERE id = ?`, bound, rebuildID); err != nil {
		return "", false, err
	}
	return bound, false, nil
}

// PendingRebuild reports whether a company and book has a valuation rebuild
// still running. Period close must refuse while it is true; the close-path
// wiring is a follow-up after the submit-path seam lands.
func (s *Service) PendingRebuild(ctx context.Context, db ledger.DB, company, book string) (bool, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT id FROM athena_stock_rebuilds WHERE company_name = ? AND book = ? AND state = 'running' LIMIT 1`,
		company, book).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

type rebuiltBin struct {
	key      binKey
	itemType string
	uom      string
	currency string
	qty      types.Decimal
	reserved types.Decimal
	value    types.Decimal
}

// aggregateGrain recomputes one bin grain from every l_stock row for it
// (submit and cancel mirrors alike), the on-hand cost deltas joined
// through their rows, and the active reservations.
func aggregateGrain(ctx context.Context, tx *sql.Tx, key binKey) (*rebuiltBin, error) {
	out := &rebuiltBin{key: key}
	var qty, value string
	err := tx.QueryRowContext(ctx, `SELECT MIN(item_type), MIN(base_uom), MIN(base_currency), COALESCE(SUM(qty_base), 0), COALESCE(SUM(value_base), 0) FROM l_stock WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND COALESCE(lot_no, '') = ? AND COALESCE(serial_no, '') = ?`,
		key.Company, key.Book, key.Owner, key.ItemID, key.Location, key.LotNo, key.SerialNo).Scan(
		&out.itemType, &out.uom, &out.currency, &qty, &value)
	if err != nil {
		return nil, err
	}
	if out.qty, err = scanDecimal(qty); err != nil {
		return nil, err
	}
	if out.value, err = scanDecimal(value); err != nil {
		return nil, err
	}
	var delta string
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(v.on_hand_delta), 0) FROM l_stock_value v JOIN l_stock s ON s.id = v.stock_row_id WHERE v.company_name = ? AND v.book = ? AND s.owner = ? AND s.item_id = ? AND s.location_name = ? AND COALESCE(s.lot_no, '') = ? AND COALESCE(s.serial_no, '') = ?`,
		key.Company, key.Book, key.Owner, key.ItemID, key.Location, key.LotNo, key.SerialNo).Scan(&delta)
	if err != nil {
		return nil, err
	}
	amount, err := scanDecimal(delta)
	if err != nil {
		return nil, err
	}
	out.value = out.value.Add(amount)
	var reserved string
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(qty), 0) FROM athena_reservations WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND lot_no = ? AND serial_no = ? AND status = 'active'`,
		key.Company, key.Book, key.Owner, key.ItemID, key.Location, key.LotNo, key.SerialNo).Scan(&reserved)
	if err != nil {
		return nil, err
	}
	if out.reserved, err = scanDecimal(reserved); err != nil {
		return nil, err
	}
	return out, nil
}
