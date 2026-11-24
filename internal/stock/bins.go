package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"athena-erp/internal/ledger"
	"athena-erp/internal/store"
	"athena-erp/internal/types"
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

// checkBinMatch rejects a movement whose base unit differs from the bin's:
// one bin holds one base unit, converted through the UoM masters.
func checkBinMatch(key binKey, bin binState, baseUoM string) error {
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
	if s.Dialect.Classify(err) != store.ErrDuplicateKey {
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

// RebuildBins recomputes every bin balance of a company and book from the
// stock ledger: signed quantities and values from l_stock including cancel
// mirrors, on-hand cost deltas joined through their rows, and active
// reservations. It joins the caller's transaction or owns one. Bins assume
// one base unit and currency per item, which the writer enforces.
func (s *Service) RebuildBins(ctx context.Context, db ledger.DB, company, book string) (int, error) {
	if tx, ok := db.(*sql.Tx); ok {
		return s.rebuildInTx(ctx, tx, company, book)
	}
	beginner, ok := db.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return 0, errors.New("stock: database does not support transactions")
	}
	tx, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	count, err := s.rebuildInTx(ctx, tx, company, book)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
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

func (s *Service) rebuildInTx(ctx context.Context, tx *sql.Tx, company, book string) (int, error) {
	bins := map[binKey]*rebuiltBin{}
	rows, err := tx.QueryContext(ctx, `SELECT owner, item_type, item_id, location_name, COALESCE(lot_no, ''), COALESCE(serial_no, ''), MIN(base_uom), MIN(base_currency), COALESCE(SUM(qty_base), 0), COALESCE(SUM(value_base), 0) FROM l_stock WHERE company_name = ? AND book = ? GROUP BY owner, item_type, item_id, location_name, COALESCE(lot_no, ''), COALESCE(serial_no, '')`,
		company, book)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var key binKey
		var itemType, uom, currency, qty, value string
		key.Company, key.Book = company, book
		if err := rows.Scan(&key.Owner, &itemType, &key.ItemID, &key.Location, &key.LotNo, &key.SerialNo, &uom, &currency, &qty, &value); err != nil {
			rows.Close()
			return 0, err
		}
		quantities, err := scanDecimal(qty)
		if err != nil {
			rows.Close()
			return 0, err
		}
		values, err := scanDecimal(value)
		if err != nil {
			rows.Close()
			return 0, err
		}
		bins[key] = &rebuiltBin{key: key, itemType: itemType, uom: uom, currency: currency, qty: quantities, value: values}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	deltas, err := tx.QueryContext(ctx, `SELECT s.owner, s.item_type, s.item_id, s.location_name, COALESCE(s.lot_no, ''), COALESCE(s.serial_no, ''), COALESCE(SUM(v.on_hand_delta), 0) FROM l_stock_value v JOIN l_stock s ON s.id = v.stock_row_id WHERE v.company_name = ? AND v.book = ? GROUP BY s.owner, s.item_type, s.item_id, s.location_name, COALESCE(s.lot_no, ''), COALESCE(s.serial_no, '')`,
		company, book)
	if err != nil {
		return 0, err
	}
	for deltas.Next() {
		var key binKey
		var itemType, delta string
		key.Company, key.Book = company, book
		if err := deltas.Scan(&key.Owner, &itemType, &key.ItemID, &key.Location, &key.LotNo, &key.SerialNo, &delta); err != nil {
			deltas.Close()
			return 0, err
		}
		amount, err := scanDecimal(delta)
		if err != nil {
			deltas.Close()
			return 0, err
		}
		if bin, seen := bins[key]; seen {
			bin.value = bin.value.Add(amount)
		}
	}
	deltas.Close()
	if err := deltas.Err(); err != nil {
		return 0, err
	}
	holds, err := tx.QueryContext(ctx, `SELECT owner, item_type, item_id, location_name, lot_no, serial_no, COALESCE(SUM(qty), 0) FROM athena_reservations WHERE company_name = ? AND book = ? AND status = 'active' GROUP BY owner, item_type, item_id, location_name, lot_no, serial_no`,
		company, book)
	if err != nil {
		return 0, err
	}
	for holds.Next() {
		var key binKey
		var itemType, qty string
		key.Company, key.Book = company, book
		if err := holds.Scan(&key.Owner, &itemType, &key.ItemID, &key.Location, &key.LotNo, &key.SerialNo, &qty); err != nil {
			holds.Close()
			return 0, err
		}
		quantities, err := scanDecimal(qty)
		if err != nil {
			holds.Close()
			return 0, err
		}
		if bin, seen := bins[key]; seen {
			bin.reserved = quantities
		}
	}
	holds.Close()
	if err := holds.Err(); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM athena_stock_bins WHERE company_name = ? AND book = ?`, company, book); err != nil {
		return 0, err
	}
	count := 0
	for _, bin := range bins {
		if _, err := tx.ExecContext(ctx, `INSERT INTO athena_stock_bins (company_name, book, owner, item_type, item_id, location_name, lot_no, serial_no, base_uom, on_hand, reserved_qty, value_on_hand, base_currency) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			bin.key.Company, bin.key.Book, bin.key.Owner, bin.itemType, bin.key.ItemID, bin.key.Location, bin.key.LotNo, bin.key.SerialNo,
			bin.uom, bin.qty.String(), bin.reserved.String(), bin.value.String(), bin.currency); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}
