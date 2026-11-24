package stock

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"athena-erp/internal/ledger"
	"athena-erp/internal/types"
)

// Count posts a stock count as the delta between counted and expected base
// units. Expected backs movements newer than the cutoff out of the current
// bin, so a count taken earlier posts only what changed since: gains come
// in at the current average (zero into an empty bin) and shrinkage goes
// out at the average. A zero delta posts nothing.
type Count struct {
	Company  string
	Book     string
	Owner    string
	ItemType string
	ItemID   string
	Location string
	LotNo    string
	SerialNo string
	// Counted is the counted quantity in any unit of its UoM family.
	Counted types.Quantity
	// CountDate is the YYYY-MM-DD cutoff the count was taken at;
	// PostingDate is the allowed YYYY-MM-DD posting date.
	CountDate      string
	PostingDate    string
	ControlAccount string
	OffsetAccount  string
	Valued         bool
	Dimensions     map[string]string
	VoucherType    string
	VoucherID      string
	Revision       int64
	SourceLineNo   int
}

// PlannedCount is a validated count with its expected base units, the
// counted base units, their delta, and the inner movement plan (nil when
// the delta is zero and nothing posts).
type PlannedCount struct {
	Count       Count
	Expected    types.Decimal
	CountedBase types.Decimal
	Delta       types.Decimal
	Movement    *PlannedMovement
}

// PlanCount validates a count, locks its bin, and plans the gain receipt
// or shrinkage issue for the delta in the caller's transaction. The caller
// posts the inner plan's legs and applies with ApplyCount.
func (s *Service) PlanCount(ctx context.Context, tx *sql.Tx, c Count) (PlannedCount, error) {
	for name, value := range map[string]string{
		"company": c.Company, "book": c.Book, "owner": c.Owner,
		"item type": c.ItemType, "item id": c.ItemID, "location": c.Location,
		"control account": c.ControlAccount, "offset account": c.OffsetAccount,
		"voucher type": c.VoucherType, "voucher id": c.VoucherID,
	} {
		if value == "" {
			return PlannedCount{}, fmt.Errorf("%w: %s is required", ErrInvalid, name)
		}
	}
	if c.ControlAccount == c.OffsetAccount {
		return PlannedCount{}, fmt.Errorf("%w: control and offset accounts must differ", ErrInvalid)
	}
	if c.Revision < 1 {
		return PlannedCount{}, fmt.Errorf("%w: voucher revision %d is below 1", ErrInvalid, c.Revision)
	}
	if c.SourceLineNo < 1 {
		return PlannedCount{}, fmt.Errorf("%w: source line %d is below 1", ErrInvalid, c.SourceLineNo)
	}
	if !validDate(c.CountDate) || !validDate(c.PostingDate) {
		return PlannedCount{}, fmt.Errorf("%w: dates must be YYYY-MM-DD", ErrInvalid)
	}
	if !types.ValidUoM(c.Counted.UoM) {
		return PlannedCount{}, fmt.Errorf("%w: invalid unit %q", ErrInvalid, c.Counted.UoM)
	}
	masters := &ledger.Service{DB: tx, Dialect: s.Dialect}
	if _, err := checkMasters(ctx, masters, c.Company, c.ControlAccount, c.OffsetAccount); err != nil {
		return PlannedCount{}, err
	}
	base, _, err := masters.ToBaseQuantity(ctx, c.Counted, strings.ToLower(c.ItemID), LedgerRounding)
	if err != nil {
		return PlannedCount{}, fmt.Errorf("%w: quantity: %v", ErrInvalid, err)
	}
	counted, err := base.Value.Rescale(LedgerScale)
	if err != nil {
		return PlannedCount{}, fmt.Errorf("%w: counted quantity %s needs rounding", ErrInvalid, base.Value.String())
	}
	if !fitsAmount(counted) {
		return PlannedCount{}, fmt.Errorf("%w: counted quantity exceeds DECIMAL(28,6)", ErrInvalid)
	}
	key := binKey{Company: c.Company, Book: c.Book, Owner: c.Owner, ItemID: c.ItemID, Location: c.Location, LotNo: c.LotNo, SerialNo: c.SerialNo}
	bin, found, err := lockBin(ctx, tx, key)
	if err != nil {
		return PlannedCount{}, err
	}
	if found {
		if err := checkBinMatch(key, bin, c.ItemType, base.UoM); err != nil {
			return PlannedCount{}, err
		}
	}
	var newer string
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(qty_base), 0) FROM l_stock WHERE company_name = ? AND book = ? AND owner = ? AND item_id = ? AND location_name = ? AND COALESCE(lot_no, '') = ? AND COALESCE(serial_no, '') = ? AND movement_date > ?`,
		key.Company, key.Book, key.Owner, key.ItemID, key.Location, key.LotNo, key.SerialNo, c.CountDate).Scan(&newer)
	if err != nil {
		return PlannedCount{}, err
	}
	since, err := scanDecimal(newer)
	if err != nil {
		return PlannedCount{}, err
	}
	planned := PlannedCount{Count: c, CountedBase: counted}
	planned.Expected = bin.Qty.Sub(since)
	planned.Delta = counted.Sub(planned.Expected)
	if planned.Delta.Sign() == 0 {
		return planned, nil
	}
	movement := Movement{
		Company: c.Company, Book: c.Book, Owner: c.Owner,
		ItemType: c.ItemType, ItemID: c.ItemID,
		ControlAccount: c.ControlAccount, OffsetAccount: c.OffsetAccount,
		Location: c.Location, LotNo: c.LotNo, SerialNo: c.SerialNo,
		Quantity:     types.Quantity{Value: planned.Delta, UoM: base.UoM},
		Valued:       c.Valued,
		MovementDate: c.CountDate, PostingDate: c.PostingDate, Dimensions: c.Dimensions,
		VoucherType: c.VoucherType, VoucherID: c.VoucherID, Revision: c.Revision, SourceLineNo: c.SourceLineNo,
	}
	if planned.Delta.Sign() > 0 {
		if bin.Qty.Sign() > 0 {
			average, err := s.method().IssueUnitCost(bin.Qty, bin.Value)
			if err != nil {
				return PlannedCount{}, err
			}
			movement.UnitCost = average
		} else {
			movement.UnitCost = types.MustDecimal("0.000000")
		}
	}
	inner, err := s.PlanMovement(ctx, tx, movement)
	if err != nil {
		return PlannedCount{}, err
	}
	planned.Movement = &inner
	return planned, nil
}

// ApplyCount writes a planned count's inner movement under the caller's
// posting group. A zero delta posts nothing and reports posted false.
func (s *Service) ApplyCount(ctx context.Context, tx *sql.Tx, p PlannedCount, groupID string) (AppliedMovement, bool, error) {
	if p.Movement == nil {
		return AppliedMovement{}, false, nil
	}
	applied, err := s.ApplyMovement(ctx, tx, *p.Movement, groupID)
	if err != nil {
		return AppliedMovement{}, false, err
	}
	return applied, true, nil
}
