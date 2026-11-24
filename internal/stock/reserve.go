package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"athena-erp/internal/ledger"
	"athena-erp/internal/types"

	"github.com/google/uuid"
)

// Reservation statuses. Only active holds reduce availability.
const (
	ReserveActive   = "active"
	ReserveReleased = "released"
	ReserveConsumed = "consumed"
	ReserveExpired  = "expired"
)

var (
	// ErrInsufficientAvailable reports a hold larger than on-hand minus
	// active reservations.
	ErrInsufficientAvailable = errors.New("stock: not enough available stock")
	// ErrReservationState reports a release, consume or expiry against a
	// reservation that is no longer active.
	ErrReservationState = errors.New("stock: reservation is not active")
	// ErrDuplicateHold reports a hold while the holder already actively
	// holds the bin.
	ErrDuplicateHold = errors.New("stock: holder already holds this bin")
)

// Reservation is one hold of base-unit stock for a holder such as a cart,
// an event or an order. LotNo and SerialNo narrow the hold to a batch or a
// unit; empty means the whole bin grain. ExpiresAt is "" for no expiry or
// a "2006-01-02 15:04:05" timestamp.
type Reservation struct {
	Company    string
	Book       string
	Owner      string
	ItemType   string
	ItemID     string
	Location   string
	LotNo      string
	SerialNo   string
	Quantity   types.Quantity
	HolderType string
	HolderID   string
	ExpiresAt  string
}

// HeldReservation is a stored active hold.
type HeldReservation struct {
	ID         string
	Bin        binKey
	UoM        string
	Qty        types.Decimal
	HolderType string
	HolderID   string
	ExpiresAt  string
}

// Availability breaks a bin into on-hand, actively held, and available
// (on-hand minus active holds).
type Availability struct {
	OnHand    types.Decimal
	Reserved  types.Decimal
	Available types.Decimal
}

// Reserve takes a hold in the caller's transaction: it locks the bin and
// the grain's active holds, refuses when the hold exceeds availability,
// and writes the hold with the bin's reserved total. Competing takes
// serialize on the bin lock, so one holder wins the last unit. A hold on a
// serial requires the unit to exist on hand and marks it reserved. A
// repeat hold after the earlier hold ended reactivates its row; holding
// twice while active stays a duplicate.
func (s *Service) Reserve(ctx context.Context, tx *sql.Tx, r Reservation) (HeldReservation, error) {
	if err := checkReservation(r); err != nil {
		return HeldReservation{}, err
	}
	masters := &ledger.Service{DB: tx, Dialect: s.Dialect}
	base, _, err := masters.ToBaseQuantity(ctx, r.Quantity, strings.ToLower(r.ItemID), LedgerRounding)
	if err != nil {
		return HeldReservation{}, fmt.Errorf("%w: quantity: %v", ErrInvalid, err)
	}
	qty, err := base.Value.Rescale(LedgerScale)
	if err != nil {
		return HeldReservation{}, fmt.Errorf("%w: base quantity %s needs rounding", ErrInvalid, base.Value.String())
	}
	if qty.Sign() <= 0 {
		return HeldReservation{}, fmt.Errorf("%w: hold quantity must be above zero", ErrInvalid)
	}
	key := binKey{Company: r.Company, Book: r.Book, Owner: r.Owner, ItemID: r.ItemID, Location: r.Location, LotNo: r.LotNo, SerialNo: r.SerialNo}
	bin, found, err := lockBin(ctx, tx, key)
	if err != nil {
		return HeldReservation{}, err
	}
	if !found {
		return HeldReservation{}, fmt.Errorf("%w: no stock at this bin", ErrInsufficientAvailable)
	}
	if err := checkBinMatch(key, bin, r.ItemType, base.UoM); err != nil {
		return HeldReservation{}, err
	}
	held, err := sumActiveHolds(ctx, tx, key)
	if err != nil {
		return HeldReservation{}, err
	}
	if available := bin.Qty.Sub(held); available.Cmp(qty) < 0 {
		return HeldReservation{}, fmt.Errorf("%w: bin holds %s of %s available", ErrInsufficientAvailable, available.String(), qty.String())
	}
	if r.SerialNo != "" {
		if err := holdSerial(ctx, tx, key); err != nil {
			return HeldReservation{}, err
		}
	}
	fresh, err := uuid.NewV7()
	if err != nil {
		return HeldReservation{}, err
	}
	heldID := fresh.String()
	var expires any
	if r.ExpiresAt != "" {
		expires = r.ExpiresAt
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO athena_reservations (id, company_name, book, owner, item_type, item_id, location_name, lot_no, serial_no, base_uom, qty, status, holder_type, holder_id, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?)`,
		heldID, r.Company, r.Book, r.Owner, r.ItemType, r.ItemID, r.Location, r.LotNo, r.SerialNo,
		base.UoM, qty.String(), r.HolderType, r.HolderID, expires); err != nil {
		if !s.duplicate(err) {
			return HeldReservation{}, err
		}
		resurrected, err := s.resurrectHold(ctx, tx, key, r, base.UoM, qty, expires)
		if err != nil {
			if r.SerialNo != "" {
				_ = unholdSerial(ctx, tx, key)
			}
			return HeldReservation{}, err
		}
		heldID = resurrected
	}
	if _, err := tx.ExecContext(ctx, `UPDATE athena_stock_bins SET reserved_qty = reserved_qty + ?, revision = revision + 1 WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND lot_no = ? AND serial_no = ?`,
		qty.String(), key.Company, key.Book, key.Owner, key.ItemID, key.Location, key.LotNo, key.SerialNo); err != nil {
		return HeldReservation{}, err
	}
	return HeldReservation{ID: heldID, Bin: key, UoM: base.UoM, Qty: qty, HolderType: r.HolderType, HolderID: r.HolderID, ExpiresAt: r.ExpiresAt}, nil
}

// resurrectHold reactivates a terminal hold row for a repeat hold by the
// same holder on the same bin. An already active row stays a duplicate.
func (s *Service) resurrectHold(ctx context.Context, tx *sql.Tx, key binKey, r Reservation, baseUoM string, qty types.Decimal, expires any) (string, error) {
	var id, status string
	err := tx.QueryRowContext(ctx, `SELECT id, status FROM athena_reservations WHERE holder_type = ? AND holder_id = ? AND item_id = ? AND location_name = ? AND lot_no = ? AND serial_no = ? FOR UPDATE`,
		r.HolderType, r.HolderID, key.ItemID, key.Location, key.LotNo, key.SerialNo).Scan(&id, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: %s %s", ErrDuplicateHold, r.HolderType, r.HolderID)
		}
		return "", err
	}
	if status == ReserveActive {
		return "", fmt.Errorf("%w: %s %s", ErrDuplicateHold, r.HolderType, r.HolderID)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE athena_reservations SET base_uom = ?, qty = ?, status = 'active', expires_at = ?, revision = revision + 1 WHERE id = ?`,
		baseUoM, qty.String(), expires, id); err != nil {
		return "", err
	}
	return id, nil
}

func checkReservation(r Reservation) error {
	for name, value := range map[string]string{
		"company": r.Company, "book": r.Book, "owner": r.Owner,
		"item type": r.ItemType, "item id": r.ItemID, "location": r.Location,
		"holder type": r.HolderType, "holder id": r.HolderID,
	} {
		if value == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalid, name)
		}
	}
	if !types.ValidUoM(r.Quantity.UoM) {
		return fmt.Errorf("%w: invalid unit %q", ErrInvalid, r.Quantity.UoM)
	}
	if r.ExpiresAt != "" {
		if _, err := time.Parse("2006-01-02 15:04:05", r.ExpiresAt); err != nil {
			return fmt.Errorf("%w: expiry must be YYYY-MM-DD HH:MM:SS", ErrInvalid)
		}
	}
	if len(r.LotNo) > 140 || len(r.SerialNo) > 140 {
		return fmt.Errorf("%w: lot and serial must fit 140 characters", ErrInvalid)
	}
	return nil
}

// sumActiveHolds totals the active holds on a bin grain, locking them.
func sumActiveHolds(ctx context.Context, tx *sql.Tx, key binKey) (types.Decimal, error) {
	var total string
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(qty), 0) FROM athena_reservations WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND lot_no = ? AND serial_no = ? AND status = 'active' FOR UPDATE`,
		key.Company, key.Book, key.Owner, key.ItemID, key.Location, key.LotNo, key.SerialNo).Scan(&total)
	if err != nil {
		return types.Decimal{}, err
	}
	return scanDecimal(total)
}

// Available reports a bin's on-hand, actively held and available base
// units from live reads (no locks): a point-in-time view. The take
// decision in Reserve relocks and recomputes.
func (s *Service) Available(ctx context.Context, db ledger.DB, key BinKey) (Availability, error) {
	var out Availability
	bin, found, err := s.BinBalance(ctx, db, key)
	if err != nil {
		return Availability{}, err
	}
	if !found {
		return Availability{}, nil
	}
	var total string
	err = db.QueryRowContext(ctx, `SELECT COALESCE(SUM(qty), 0) FROM athena_reservations WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND lot_no = ? AND serial_no = ? AND status = 'active'`,
		key.Company, key.Book, key.Owner, key.ItemID, key.Location, key.LotNo, key.SerialNo).Scan(&total)
	if err != nil {
		return Availability{}, err
	}
	if out.Reserved, err = scanDecimal(total); err != nil {
		return Availability{}, err
	}
	out.OnHand = bin.Qty
	out.Available = bin.Qty.Sub(out.Reserved)
	return out, nil
}

// ReleaseReservation frees part or all of an active hold in the caller's
// transaction: an empty qty frees the whole hold. The bin's reserved total
// drops with it, and a serial hold frees its unit back to on hand.
func (s *Service) ReleaseReservation(ctx context.Context, tx *sql.Tx, id string, qty types.Decimal) error {
	return s.closeReservation(ctx, tx, id, qty, ReserveReleased)
}

// ConsumeReservation converts part or all of an active hold into an issue
// in the caller's transaction: the caller posts the issue movement for the
// same quantity in the same transaction. An empty qty consumes the whole
// hold.
func (s *Service) ConsumeReservation(ctx context.Context, tx *sql.Tx, id string, qty types.Decimal) error {
	return s.closeReservation(ctx, tx, id, qty, ReserveConsumed)
}

func (s *Service) closeReservation(ctx context.Context, tx *sql.Tx, id string, qty types.Decimal, terminal string) error {
	held, err := lockReservation(ctx, tx, id)
	if err != nil {
		return err
	}
	if held.Qty.Sign() <= 0 {
		return fmt.Errorf("%w: hold quantity must be above zero", ErrInvalid)
	}
	part := qty
	if part.Sign() == 0 {
		part = held.Qty
	}
	if part.Sign() < 0 || part.Cmp(held.Qty) > 0 {
		return fmt.Errorf("%w: cannot close %s of held %s", ErrInvalid, part.String(), held.Qty.String())
	}
	rest := held.Qty.Sub(part)
	if rest.Sign() == 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE athena_reservations SET status = ?, revision = revision + 1 WHERE id = ?`,
			terminal, id); err != nil {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, `UPDATE athena_reservations SET qty = ?, revision = revision + 1 WHERE id = ?`,
		rest.String(), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE athena_stock_bins SET reserved_qty = reserved_qty - ?, revision = revision + 1 WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND lot_no = ? AND serial_no = ?`,
		part.String(), held.Bin.Company, held.Bin.Book, held.Bin.Owner, held.Bin.ItemID, held.Bin.Location, held.Bin.LotNo, held.Bin.SerialNo); err != nil {
		return err
	}
	if held.Bin.SerialNo != "" && rest.Sign() == 0 {
		if terminal == ReserveConsumed {
			// Consuming ships the unit: it must already have left
			// reserved (issue first; the issue path auto-consumes).
			// The status stays issued or on rent.
			var status string
			err := tx.QueryRowContext(ctx, `SELECT status FROM athena_serials WHERE company_name = ? AND book = ? AND item_id = ? AND serial_no = ? FOR UPDATE`,
				held.Bin.Company, held.Bin.Book, held.Bin.ItemID, held.Bin.SerialNo).Scan(&status)
			if err != nil {
				return err
			}
			if status == SerialReserved {
				return fmt.Errorf("%w: issue serial %q before consuming its hold", ErrSerialState, held.Bin.SerialNo)
			}
		} else if err := unholdSerial(ctx, tx, held.Bin); err != nil {
			return err
		}
	}
	return nil
}

// lockReservation reads one reservation, locking it, and refuses anything
// but an active hold.
func lockReservation(ctx context.Context, tx *sql.Tx, id string) (HeldReservation, error) {
	var held HeldReservation
	var company, book, owner, itemID, location, lot, serial, uom, qty, status string
	var expiresAt sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT company_name, book, owner, item_id, location_name, lot_no, serial_no, base_uom, qty, status, holder_type, holder_id, expires_at FROM athena_reservations WHERE id = ? FOR UPDATE`,
		id).Scan(&company, &book, &owner, &itemID, &location, &lot, &serial, &uom, &qty, &status, &held.HolderType, &held.HolderID, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HeldReservation{}, fmt.Errorf("%w: reservation %q not found", ErrInvalid, id)
		}
		return HeldReservation{}, err
	}
	if status != ReserveActive {
		return HeldReservation{}, fmt.Errorf("%w: reservation %q is %s", ErrReservationState, id, status)
	}
	held.ID = id
	held.Bin = binKey{Company: company, Book: book, Owner: owner, ItemID: itemID, Location: location, LotNo: lot, SerialNo: serial}
	held.UoM = uom
	if held.Qty, err = scanDecimal(qty); err != nil {
		return HeldReservation{}, err
	}
	if expiresAt.Valid {
		held.ExpiresAt = expiresAt.Time.Format("2006-01-02 15:04:05")
	}
	return held, nil
}

// ExpireReservations flips every active hold past its expiry to expired,
// freeing bins and serial units. It runs in the caller's transaction and
// returns the expired count.
func (s *Service) ExpireReservations(ctx context.Context, tx *sql.Tx, company, book, now string) (int, error) {
	if _, err := time.Parse("2006-01-02 15:04:05", now); err != nil {
		return 0, fmt.Errorf("%w: timestamp must be YYYY-MM-DD HH:MM:SS", ErrInvalid)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM athena_reservations WHERE company_name = ? AND book = ? AND status = 'active' AND expires_at IS NOT NULL AND expires_at < ? FOR UPDATE`,
		company, book, now)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	flipped := 0
	for _, id := range ids {
		held, err := lockReservation(ctx, tx, id)
		if err != nil {
			if errors.Is(err, ErrReservationState) {
				continue
			}
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE athena_reservations SET status = 'expired', revision = revision + 1 WHERE id = ?`, id); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE athena_stock_bins SET reserved_qty = reserved_qty - ?, revision = revision + 1 WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND lot_no = ? AND serial_no = ?`,
			held.Qty.String(), held.Bin.Company, held.Bin.Book, held.Bin.Owner, held.Bin.ItemID, held.Bin.Location, held.Bin.LotNo, held.Bin.SerialNo); err != nil {
			return 0, err
		}
		if held.Bin.SerialNo != "" {
			if err := unholdSerial(ctx, tx, held.Bin); err != nil {
				return 0, err
			}
		}
		flipped++
	}
	return flipped, nil
}
