// Package stock is the kernel's stock ledger (batch-2 G11, ADR-0032,
// Proposed): every stock movement becomes one append-only l_stock row with
// signed base units and signed base-currency value, receipt layers keep
// residuals with item application edges, and lockable bin balances carry
// on-hand, reserved and value totals that the negative-stock refusal
// checks.
//
// Valuation is perpetual weighted (moving) average for all stock (Z3, one
// formula for items of similar nature per IAS 2.25), behind the CostMethod
// interface. Perishables pick first-expiry-first-out over batches, rental
// units move with zero value and are never consumed, and negative stock is
// refused for valued items. Historical rows are never rewritten: returns
// mirror issues and cancellations mirror submits as flagged reversals,
// while cost corrections arrive as appended l_stock_value deltas (slice 3).
//
// A movement posts in the caller's transaction in three steps, because the
// posting group id only exists after the G6 engine posts: PlanMovement
// locks the bin, converts the entered quantity to base units through the
// UoM masters, computes the cost and returns balanced value legs; the
// caller adds those legs to the voucher plan and posts it with
// posting.Engine.PostInTx; ApplyMovement writes the stock rows under the
// resulting group id. Quantity-only moves (zero value) post no legs, so
// Apply claims a posting group of its own for them. Plan and Apply must
// run in the same transaction; Apply rechecks the refusal under the lock,
// and both replay when the voucher key already has rows.
//
// The submit-path wiring that calls this seam from the document lifecycle
// is not scheduled in any C7 slice yet; until it lands, the seam is driven
// by tests and, from slice 9, by StockService.
package stock

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"athena-erp/internal/ledger"
	"athena-erp/internal/posting"
	"athena-erp/internal/store"
	"athena-erp/internal/types"

	"github.com/google/uuid"
)

var (
	// ErrInvalid reports a movement that fails mechanical validation.
	ErrInvalid = errors.New("stock: invalid movement")
	// ErrNegativeStock reports an issue that would drive a valued bin below
	// zero. Z3 refuses negative stock for valued items.
	ErrNegativeStock = errors.New("stock: negative stock is refused for valued items")
	// ErrOverReturn reports a return larger than the unreturned part of the
	// issue it reverses.
	ErrOverReturn = errors.New("stock: return exceeds the unreturned quantity")
	// ErrNoSubmit reports a cancel for a voucher key with no submit rows.
	ErrNoSubmit = errors.New("stock: nothing posted to cancel")
	// ErrCancelBlocked reports a cancel whose stock was consumed downstream:
	// the receipt layer no longer holds the received quantity.
	ErrCancelBlocked = errors.New("stock: cancel is blocked by downstream consumption")
	// ErrIntegrity reports stock state that cannot be true when every write
	// went through this service, such as layers short of the bin quantity.
	ErrIntegrity = errors.New("stock: inconsistent layers or bins")
)

// LedgerScale is the fraction scale of every stored quantity and value,
// matching the DECIMAL(28,6) ledger columns.
const LedgerScale = 6

// LedgerRounding rounds computed costs to LedgerScale. Ties go away from
// zero, the same HalfUp the UoM conversions default to.
const LedgerRounding = types.HalfUp

// Service writes and reads the stock ledger. Dialect classifies duplicate
// keys on the bin insert race; Method values issues, defaulting to
// WeightedAverage when nil.
type Service struct {
	DB      *sql.DB
	Dialect store.Dialect
	Method  CostMethod
}

func (s *Service) method() CostMethod {
	if s.Method == nil {
		return WeightedAverage{}
	}
	return s.Method
}

// Movement is one stock receipt, issue or return on a voucher line. The
// sign of Quantity gives the direction: positive receives, negative
// issues. A positive quantity with ReversesRowID returns part of the named
// issue row at that issue's unit cost. Cancel is not a Movement: callers
// mirror submits through PlanCancel and ApplyCancel.
type Movement struct {
	Company string
	Book    string
	Owner   string
	// ItemType names the item's Record Type and ItemID its Record id.
	ItemType string
	ItemID   string
	// ControlAccount is the stock control account the value legs use;
	// OffsetAccount is the other leg (GRN clearing, cost of sales). Both
	// must be enabled leaves. Zero-value moves post no legs.
	ControlAccount string
	OffsetAccount  string
	Location       string
	// LotNo and SerialNo name the batch and unit for tracked items; empty
	// means untracked.
	LotNo    string
	SerialNo string
	// Quantity is the entered quantity in any unit of its UoM family; the
	// service converts it to base units. It must not be zero.
	Quantity types.Quantity
	// UnitCost is the base-currency cost per base unit of a receipt. Issues
	// and returns must leave it zero: issues take the moving average and
	// returns take the reversed issue's unit cost.
	UnitCost types.Decimal
	// Valued marks stock with a cost. Only valued moves post value legs,
	// and only valued bins refuse negative stock (Z3).
	Valued        bool
	MovementDate  string // YYYY-MM-DD, the economic date
	PostingDate   string // YYYY-MM-DD, the allowed posting date
	Dimensions    map[string]string
	VoucherType   string
	VoucherID     string
	Revision      int64
	SourceLineNo  int
	ReversesRowID string
}

// PlannedMovement is a validated movement with its computed cost, balanced
// value legs, and the bin, layer and application writes Apply performs.
// Replayed is true when the voucher key already has rows: Apply returns
// the first write without writing again.
type PlannedMovement struct {
	Movement     Movement
	BaseQty      types.Decimal // signed base units
	BaseUoM      string
	UnitCost     types.Decimal // applied unit cost (receipt cost, average, or issue cost)
	Value        types.Decimal // signed base-currency value
	Average      types.Decimal // moving average after the movement
	Legs         []posting.Line
	Bin          binKey
	BinFound     bool
	BinBefore    binState
	BaseCurrency string
	consume      []layerOp
	restore      []layerOp
	create       bool
	Reverses     *stockRow
	Replayed     bool
	ReplayRow    string
}

// AppliedMovement describes a committed stock write.
type AppliedMovement struct {
	RowID          string `json:"row_id"`
	PostingGroupID string `json:"posting_group_id"`
	Value          string `json:"value"`
	Replayed       bool   `json:"replayed"`
}

// PlanMovement validates a movement, locks its bin, converts the entered
// quantity to base units and computes the cost and value legs. It runs in
// the caller's transaction and must be followed by ApplyMovement in the
// same transaction once the caller has posted the legs. When the voucher
// key already has submit rows the plan replays: it validates the shape of
// the movement but skips costing and locking.
func (s *Service) PlanMovement(ctx context.Context, tx *sql.Tx, m Movement) (PlannedMovement, error) {
	if err := checkMovement(m); err != nil {
		return PlannedMovement{}, err
	}
	if rowID, found, err := findSubmitRow(ctx, tx, m); err != nil {
		return PlannedMovement{}, err
	} else if found {
		return PlannedMovement{Movement: m, Replayed: true, ReplayRow: rowID}, nil
	}
	masters := &ledger.Service{DB: tx, Dialect: s.Dialect}
	company, err := checkMasters(ctx, masters, m)
	if err != nil {
		return PlannedMovement{}, err
	}
	base, _, err := masters.ToBaseQuantity(ctx, m.Quantity, strings.ToLower(m.ItemID), LedgerRounding)
	if err != nil {
		return PlannedMovement{}, fmt.Errorf("%w: quantity: %v", ErrInvalid, err)
	}
	baseQty, err := base.Value.Rescale(LedgerScale)
	if err != nil {
		return PlannedMovement{}, fmt.Errorf("%w: base quantity %s needs rounding", ErrInvalid, base.Value.String())
	}
	if !fitsAmount(baseQty) {
		return PlannedMovement{}, fmt.Errorf("%w: base quantity exceeds DECIMAL(28,6)", ErrInvalid)
	}
	planned := PlannedMovement{Movement: m, BaseQty: baseQty, BaseUoM: base.UoM, Bin: binKeyOf(m), BaseCurrency: company.BaseCurrency}
	bin, found, err := lockBin(ctx, tx, planned.Bin)
	if err != nil {
		return PlannedMovement{}, err
	}
	planned.BinFound, planned.BinBefore = found, bin
	if found {
		if err := checkBinMatch(planned.Bin, bin, base.UoM); err != nil {
			return PlannedMovement{}, err
		}
	}
	switch {
	case m.ReversesRowID != "":
		if err := s.planReturn(ctx, tx, &planned, bin, found); err != nil {
			return PlannedMovement{}, err
		}
	case baseQty.Sign() < 0:
		if err := s.planIssue(ctx, tx, &planned, bin, found); err != nil {
			return PlannedMovement{}, err
		}
	default:
		if err := s.planReceipt(&planned, bin, found); err != nil {
			return PlannedMovement{}, err
		}
	}
	if !fitsAmount(planned.Value) {
		return PlannedMovement{}, fmt.Errorf("%w: value exceeds DECIMAL(28,6)", ErrInvalid)
	}
	if planned.Value.Sign() != 0 {
		legs := valueLegs(m, base.UoM, baseQty, planned.Value)
		planned.Legs = legs
		fragment := posting.Plan{
			VoucherType: m.VoucherType,
			VoucherID:   m.VoucherID,
			Revision:    m.Revision,
			Event:       posting.EventSubmit,
			Book:        m.Book,
			PostingDate: m.PostingDate,
			Lines:       legs,
		}
		if err := posting.ValidatePlan(ctx, tx, fragment); err != nil {
			return PlannedMovement{}, fmt.Errorf("stock: value legs: %w", err)
		}
	}
	return planned, nil
}

// checkMovement validates the shape of a movement without touching the
// database.
func checkMovement(m Movement) error {
	for name, value := range map[string]string{
		"company": m.Company, "book": m.Book, "owner": m.Owner,
		"item type": m.ItemType, "item id": m.ItemID,
		"control account": m.ControlAccount, "offset account": m.OffsetAccount,
		"location": m.Location, "voucher type": m.VoucherType, "voucher id": m.VoucherID,
	} {
		if value == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalid, name)
		}
	}
	if m.ControlAccount == m.OffsetAccount {
		return fmt.Errorf("%w: control and offset accounts must differ", ErrInvalid)
	}
	if m.Revision < 1 {
		return fmt.Errorf("%w: voucher revision %d is below 1", ErrInvalid, m.Revision)
	}
	if m.SourceLineNo < 1 {
		return fmt.Errorf("%w: source line %d is below 1", ErrInvalid, m.SourceLineNo)
	}
	if !validDate(m.MovementDate) || !validDate(m.PostingDate) {
		return fmt.Errorf("%w: dates must be YYYY-MM-DD", ErrInvalid)
	}
	if !types.ValidUoM(m.Quantity.UoM) {
		return fmt.Errorf("%w: invalid unit %q", ErrInvalid, m.Quantity.UoM)
	}
	if m.Quantity.Value.Sign() == 0 {
		return fmt.Errorf("%w: quantity must not be zero", ErrInvalid)
	}
	if m.ReversesRowID != "" {
		if m.Quantity.Value.Sign() < 0 {
			return fmt.Errorf("%w: a return must carry a positive quantity", ErrInvalid)
		}
		if m.UnitCost.Sign() != 0 {
			return fmt.Errorf("%w: a return takes the reversed issue's unit cost, not a new one", ErrInvalid)
		}
		return nil
	}
	if m.Quantity.Value.Sign() < 0 && m.UnitCost.Sign() != 0 {
		return fmt.Errorf("%w: an issue takes the moving average, not a unit cost", ErrInvalid)
	}
	if m.Quantity.Value.Sign() > 0 && m.UnitCost.Sign() < 0 {
		return fmt.Errorf("%w: receipt unit cost must not be negative", ErrInvalid)
	}
	return nil
}

func validDate(date string) bool {
	if len(date) != len("2006-01-02") {
		return false
	}
	for i, c := range date {
		switch i {
		case 4, 7:
			if c != '-' {
				return false
			}
		default:
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// checkMasters rejects unknown or disabled companies, companies without an
// enabled base currency, and accounts that are not enabled leaves. The
// fragment validation in PlanMovement rechecks periods and dimensions for
// moves that post value.
func checkMasters(ctx context.Context, masters *ledger.Service, m Movement) (ledger.Company, error) {
	company, err := masters.GetCompany(ctx, m.Company)
	if err != nil {
		if errors.Is(err, ledger.ErrNotFound) {
			return ledger.Company{}, fmt.Errorf("%w: company %q does not exist", ErrInvalid, m.Company)
		}
		return ledger.Company{}, err
	}
	if company.Disabled {
		return ledger.Company{}, fmt.Errorf("%w: company %q is disabled", ErrInvalid, m.Company)
	}
	if company.BaseCurrency == "" {
		return ledger.Company{}, fmt.Errorf("%w: company %q has no base currency", ErrInvalid, m.Company)
	}
	currency, err := masters.GetCurrency(ctx, company.BaseCurrency)
	if err != nil {
		if errors.Is(err, ledger.ErrNotFound) {
			return ledger.Company{}, fmt.Errorf("%w: base currency %q does not exist", ErrInvalid, company.BaseCurrency)
		}
		return ledger.Company{}, err
	}
	if currency.Disabled {
		return ledger.Company{}, fmt.Errorf("%w: base currency %q is disabled", ErrInvalid, company.BaseCurrency)
	}
	for name, account := range map[string]string{"control": m.ControlAccount, "offset": m.OffsetAccount} {
		stored, err := masters.GetAccount(ctx, account)
		if err != nil {
			if errors.Is(err, ledger.ErrNotFound) {
				return ledger.Company{}, fmt.Errorf("%w: %s account %q does not exist", ErrInvalid, name, account)
			}
			return ledger.Company{}, err
		}
		if stored.IsGroup {
			return ledger.Company{}, fmt.Errorf("%w: %s account %q is a group", ErrInvalid, name, account)
		}
		if stored.Disabled {
			return ledger.Company{}, fmt.Errorf("%w: %s account %q is disabled", ErrInvalid, name, account)
		}
	}
	return company, nil
}

// fitsAmount reports whether d fits DECIMAL(28,6): at most 28 digits.
func fitsAmount(d types.Decimal) bool {
	digits := d.String()
	digits = strings.TrimPrefix(digits, "-")
	digits = strings.ReplaceAll(digits, ".", "")
	digits = strings.TrimLeft(digits, "0")
	return len(digits) <= 28
}

// planReceipt costs a positive quantity without a reversed row: value is
// quantity times unit cost and a receipt layer is created. Unvalued
// receipts carry no cost and create no layer.
func (s *Service) planReceipt(p *PlannedMovement, bin binState, found bool) error {
	m := p.Movement
	if !m.Valued {
		return nil
	}
	cost, err := m.UnitCost.Rescale(LedgerScale)
	if err != nil {
		return fmt.Errorf("%w: receipt unit cost needs rounding to 6 digits", ErrInvalid)
	}
	value, err := round6(p.BaseQty.Mul(cost))
	if err != nil {
		return err
	}
	var binQty, binValue types.Decimal
	if found {
		binQty, binValue = bin.Qty, bin.Value
	}
	average, err := s.method().ReceiptAverage(binQty, binValue, p.BaseQty, cost)
	if err != nil {
		return err
	}
	p.UnitCost, p.Value, p.Average, p.create = cost, value, average, true
	return nil
}

// planIssue costs a negative quantity at the moving average, refuses it
// when the valued bin is short, and selects the layers whose residuals the
// issue consumes oldest first. Unvalued issues skip costing, refusal and
// layers.
func (s *Service) planIssue(ctx context.Context, tx *sql.Tx, p *PlannedMovement, bin binState, found bool) error {
	m := p.Movement
	if !m.Valued {
		return nil
	}
	magnitude := p.BaseQty.Neg()
	if !found || bin.Qty.Cmp(magnitude) < 0 {
		have := "none"
		if found {
			have = bin.Qty.String()
		}
		return fmt.Errorf("%w: bin holds %s, issue needs %s base %s", ErrNegativeStock, have, magnitude.String(), p.BaseUoM)
	}
	average, err := s.method().IssueUnitCost(bin.Qty, bin.Value)
	if err != nil {
		return err
	}
	total, err := round6(magnitude.Mul(average))
	if err != nil {
		return err
	}
	layers, err := openLayers(ctx, tx, p.Bin)
	if err != nil {
		return err
	}
	consume, err := coverLayers(layers, magnitude, average, total)
	if err != nil {
		return err
	}
	p.UnitCost, p.Value, p.Average, p.consume = average, total.Neg(), average, consume
	return nil
}

// planReturn values a positive quantity against the reversed issue row: the
// return takes the issue's unit cost and restores the layers the issue
// consumed, in proportion for a partial return. Returns beyond the
// unreturned part are refused. The return must land on the issue's own
// bin; anything else is a transfer, not a return.
func (s *Service) planReturn(ctx context.Context, tx *sql.Tx, p *PlannedMovement, bin binState, found bool) error {
	m := p.Movement
	orig, err := loadStockRow(ctx, tx, m.ReversesRowID)
	if err != nil {
		return err
	}
	if orig.Event != posting.EventSubmit || orig.Qty.Sign() >= 0 || orig.Reversal {
		return fmt.Errorf("%w: row %s is not a submitted issue", ErrInvalid, m.ReversesRowID)
	}
	if orig.Bin != p.Bin || orig.ItemType != m.ItemType {
		return fmt.Errorf("%w: return bin differs from the issue's bin", ErrInvalid)
	}
	prior, err := sumPriorReturns(ctx, tx, m.ReversesRowID)
	if err != nil {
		return err
	}
	if unreturned := orig.Qty.Neg().Sub(prior); p.BaseQty.Cmp(unreturned) > 0 {
		return fmt.Errorf("%w: issue has %s unreturned, return wants %s", ErrOverReturn, unreturned.String(), p.BaseQty.String())
	}
	p.Reverses = orig
	if !m.Valued {
		return nil
	}
	unitCost, err := orig.Value.Neg().Div(orig.Qty.Neg(), LedgerScale, LedgerRounding)
	if err != nil {
		return err
	}
	value, err := round6(p.BaseQty.Mul(unitCost))
	if err != nil {
		return err
	}
	var binQty, binValue types.Decimal
	if found {
		binQty, binValue = bin.Qty, bin.Value
	}
	average, err := s.method().ReceiptAverage(binQty, binValue, p.BaseQty, unitCost)
	if err != nil {
		return err
	}
	edges, err := applicationEdges(ctx, tx, m.ReversesRowID, "issue")
	if err != nil {
		return err
	}
	restore, err := prorateRestore(edges, orig.Qty.Neg(), p.BaseQty, value)
	if err != nil {
		return err
	}
	p.UnitCost, p.Value, p.Average, p.restore = unitCost, value, average, restore
	return nil
}

// valueLegs builds the balanced control and offset legs of a movement with
// a non-zero value. Receipts and returns debit the stock control account;
// issues credit it. The control leg carries the base quantity and the item
// source the control-role check requires.
func valueLegs(m Movement, baseUoM string, baseQty, value types.Decimal) []posting.Line {
	magnitude := value.Abs()
	quantity := baseQty.Abs()
	control := posting.Line{
		Company:     m.Company,
		Account:     m.ControlAccount,
		Quantity:    &quantity,
		QuantityUOM: baseUoM,
		Dimensions:  m.Dimensions,
		SourceType:  m.ItemType,
		SourceID:    m.ItemID,
	}
	offset := posting.Line{
		Company:    m.Company,
		Account:    m.OffsetAccount,
		Dimensions: m.Dimensions,
	}
	if baseQty.Sign() > 0 {
		control.DebitBase = magnitude
		offset.CreditBase = magnitude
	} else {
		control.CreditBase = magnitude
		offset.DebitBase = magnitude
	}
	return []posting.Line{control, offset}
}

func round6(d types.Decimal) (types.Decimal, error) {
	return d.Round(LedgerScale, LedgerRounding)
}

// ApplyMovement writes a planned movement in the caller's transaction: the
// l_stock row, the layer create, consumes and restores with their
// application edges, and the bin update. groupID is the posting group the
// caller created with posting.Engine.PostInTx; when the plan posts no legs
// the caller passes "" and Apply claims a posting group of its own so the
// post-once slot is still taken. The bin is relocked and the refusal
// rechecked, so Apply is safe even without a preceding Plan. When the
// voucher line already has submit rows Apply returns the first write.
func (s *Service) ApplyMovement(ctx context.Context, tx *sql.Tx, p PlannedMovement, groupID string) (AppliedMovement, error) {
	m := p.Movement
	if p.Replayed {
		return replayApplied(ctx, tx, p.ReplayRow)
	}
	if len(p.Legs) > 0 && groupID == "" {
		return AppliedMovement{}, fmt.Errorf("%w: valued movement needs its posting group id", ErrInvalid)
	}
	// Claim the group before the replay check: the group row serializes
	// concurrent applies, and the locking check then observes the winner.
	if groupID == "" {
		claimed, err := s.claimGroup(ctx, tx, m.VoucherType, m.VoucherID, m.Revision, m.Book, posting.EventSubmit)
		if err != nil {
			return AppliedMovement{}, err
		}
		groupID = claimed
	} else if err := lockGroup(ctx, tx, groupID); err != nil {
		return AppliedMovement{}, err
	}
	if rowID, found, err := findSubmitRow(ctx, tx, m); err != nil {
		return AppliedMovement{}, err
	} else if found {
		return replayApplied(ctx, tx, rowID)
	}
	bin, found, err := lockBin(ctx, tx, p.Bin)
	if err != nil {
		return AppliedMovement{}, err
	}
	if m.Valued && bin.Qty.Add(p.BaseQty).Sign() < 0 {
		return AppliedMovement{}, fmt.Errorf("%w: bin holds %s, movement needs %s", ErrNegativeStock, bin.Qty.String(), p.BaseQty.Neg().String())
	}
	rowID, err := uuid.NewV7()
	if err != nil {
		return AppliedMovement{}, err
	}
	if err := insertStockRow(ctx, tx, m, p, rowID.String(), groupID, false, m.ReversesRowID); err != nil {
		return AppliedMovement{}, err
	}
	if err := applyLayerOps(ctx, tx, rowID.String(), groupID, p); err != nil {
		return AppliedMovement{}, err
	}
	if err := s.applyBinDelta(ctx, tx, p.Bin, m.ItemType, p.BaseUoM, p.BaseCurrency, p.BaseQty, p.Value, bin, found); err != nil {
		return AppliedMovement{}, err
	}
	return AppliedMovement{RowID: rowID.String(), PostingGroupID: groupID, Value: p.Value.String()}, nil
}

// claimGroup takes the post-once slot of a quantity-only movement: it
// returns the posting group row for the voucher key, inserting the row the
// G6 engine would have written for a valued post, so concurrent applies
// serialize on it instead of writing twice.
func (s *Service) claimGroup(ctx context.Context, tx *sql.Tx, voucherType, voucherID string, revision int64, book, event string) (string, error) {
	selectGroup := func() (string, error) {
		var id string
		err := tx.QueryRowContext(ctx, `SELECT id FROM athena_posting_groups WHERE voucher_type = ? AND voucher_id = ? AND voucher_revision = ? AND event = ? AND book = ? FOR UPDATE`,
			voucherType, voucherID, revision, event, book).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return id, err
	}
	if id, err := selectGroup(); err != nil || id != "" {
		return id, err
	}
	groupID, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	operationID, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO athena_posting_groups (id, voucher_type, voucher_id, voucher_revision, event, book, operation_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		groupID.String(), voucherType, voucherID, revision, event, book, operationID.String()); err != nil {
		if s.Dialect.Classify(err) == store.ErrDuplicateKey {
			return selectGroup()
		}
		return "", err
	}
	return groupID.String(), nil
}

// stockRow is a stored l_stock row with the fields costing and cancel need.
type stockRow struct {
	ID         string
	Event      string
	Bin        binKey
	ItemType   string
	Control    string
	UoM        string
	Currency   string
	Qty        types.Decimal
	Value      types.Decimal
	Dimensions string
	SourceLine int
	Reversal   bool
	GroupID    string
	Reversed   string
}

func scanDecimal(value string) (types.Decimal, error) {
	return types.ParseDecimal(value)
}

// loadStockRow reads one l_stock row by id.
func loadStockRow(ctx context.Context, q ledger.DB, id string) (*stockRow, error) {
	var row stockRow
	var company, book, owner, itemID, location, lot, serial, qty, value string
	var reversed sql.NullString
	var reversal int
	err := q.QueryRowContext(ctx, `SELECT id, event, company_name, book, owner, item_type, item_id, control_account_name, location_name, COALESCE(lot_no, ''), COALESCE(serial_no, ''), base_uom, qty_base, value_base, base_currency, dimensions, source_line_no, is_reversal, posting_group_id, reversed_row_id FROM l_stock WHERE id = ?`,
		id).Scan(&row.ID, &row.Event, &company, &book, &owner, &row.ItemType, &itemID, &row.Control, &location, &lot, &serial, &row.UoM, &qty, &value, &row.Currency, &row.Dimensions, &row.SourceLine, &reversal, &row.GroupID, &reversed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: stock row %q not found", ErrInvalid, id)
		}
		return nil, err
	}
	if row.Qty, err = scanDecimal(qty); err != nil {
		return nil, err
	}
	if row.Value, err = scanDecimal(value); err != nil {
		return nil, err
	}
	row.Bin = binKey{Company: company, Book: book, Owner: owner, ItemID: itemID, Location: location, LotNo: lot, SerialNo: serial}
	row.Reversal = reversal != 0
	if reversed.Valid {
		row.Reversed = reversed.String
	}
	return &row, nil
}

// sumPriorReturns totals the quantities already returned against an issue
// row: submitted, non-reversal returns minus their cancel mirrors, so a
// cancelled return frees its quantity for a fresh return.
func sumPriorReturns(ctx context.Context, q ledger.DB, reversesID string) (types.Decimal, error) {
	var total string
	err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(qty_base), 0) FROM l_stock WHERE reversed_row_id = ? AND event = ? AND is_reversal = 0`,
		reversesID, posting.EventSubmit).Scan(&total)
	if err != nil {
		return types.Decimal{}, err
	}
	returned, err := scanDecimal(total)
	if err != nil {
		return types.Decimal{}, err
	}
	var mirrors string
	err = q.QueryRowContext(ctx, `SELECT COALESCE(SUM(-c.qty_base), 0) FROM l_stock c JOIN l_stock r ON r.id = c.reversed_row_id WHERE r.reversed_row_id = ? AND r.event = ? AND r.is_reversal = 0 AND c.event = ? AND c.is_reversal = 1`,
		reversesID, posting.EventSubmit, posting.EventCancel).Scan(&mirrors)
	if err != nil {
		return types.Decimal{}, err
	}
	unwound, err := scanDecimal(mirrors)
	if err != nil {
		return types.Decimal{}, err
	}
	return returned.Sub(unwound), nil
}

// findSubmitRow returns the submitted row id of a voucher line, locking it.
// The locking read sees the latest committed state, so a retry observes
// the first write.
func findSubmitRow(ctx context.Context, q ledger.DB, m Movement) (string, bool, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM l_stock WHERE voucher_type = ? AND voucher_id = ? AND voucher_revision = ? AND source_line_no = ? AND event = ? AND book = ? LIMIT 1 FOR UPDATE`,
		m.VoucherType, m.VoucherID, m.Revision, m.SourceLineNo, posting.EventSubmit, m.Book).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return id, true, nil
}

// lockGroup locks a caller-provided posting group, rejecting unknown ids.
func lockGroup(ctx context.Context, tx *sql.Tx, groupID string) error {
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM athena_posting_groups WHERE id = ? FOR UPDATE`, groupID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: posting group %q not found", ErrInvalid, groupID)
		}
		return err
	}
	return nil
}

func replayApplied(ctx context.Context, q ledger.DB, rowID string) (AppliedMovement, error) {
	row, err := loadStockRow(ctx, q, rowID)
	if err != nil {
		return AppliedMovement{}, err
	}
	return AppliedMovement{RowID: row.ID, PostingGroupID: row.GroupID, Value: row.Value.String(), Replayed: true}, nil
}

func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

// insertStockRow writes one l_stock row. Dimensions marshal from the
// movement; an empty map stores {}.
func insertStockRow(ctx context.Context, tx *sql.Tx, m Movement, p PlannedMovement, rowID, groupID string, reversal bool, reverses string) error {
	dimensions := "{}"
	if len(m.Dimensions) > 0 {
		encoded, err := json.Marshal(m.Dimensions)
		if err != nil {
			return err
		}
		dimensions = string(encoded)
	}
	isReversal := 0
	if reversal {
		isReversal = 1
	}
	event := posting.EventSubmit
	if reversal {
		event = posting.EventCancel
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO l_stock (id, company_name, book, owner, item_type, item_id, control_account_name, location_name, lot_no, serial_no, base_uom, qty_base, value_base, base_currency, movement_date, posting_date, dimensions, voucher_type, voucher_id, voucher_revision, source_line_no, event, posting_group_id, is_reversal, reversed_row_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rowID, m.Company, m.Book, m.Owner, m.ItemType, m.ItemID, m.ControlAccount, m.Location,
		nullString(m.LotNo), nullString(m.SerialNo), p.BaseUoM, p.BaseQty.String(), p.Value.String(), p.BaseCurrency,
		m.MovementDate, m.PostingDate, dimensions,
		m.VoucherType, m.VoucherID, m.Revision, m.SourceLineNo, event, groupID, isReversal, nullString(reverses))
	return err
}

// PlannedCancel mirrors every submit row of a voucher revision as flagged
// reversals, unwinding layers and bins in reverse application order.
type PlannedCancel struct {
	VoucherType  string
	VoucherID    string
	Revision     int64
	Book         string
	MovementDate string
	PostingDate  string
	Mirrors      []cancelMirror
	Replayed     bool
	ReplayRows   []string
}

// cancelMirror unwinds one submit row. Qty and Value are the signed mirror
// amounts (the negated originals).
type cancelMirror struct {
	Orig    stockRow
	Qty     types.Decimal
	Value   types.Decimal
	consume []layerOp
	restore []layerOp
	valued  bool
}

// AppliedCancel describes a committed cancel.
type AppliedCancel struct {
	RowIDs         []string `json:"row_ids"`
	PostingGroupID string   `json:"posting_group_id"`
	Replayed       bool     `json:"replayed"`
}

// PlanCancel plans the cancel of a voucher revision: each submit row is
// mirrored with negated quantity and value, receipt layers are consumed
// back, issue consumes are restored, and return restores are consumed
// again. A receipt whose layer no longer holds the received quantity was
// consumed downstream and blocks the cancel. Cancellations post no value
// legs: the caller reverses the general ledger with
// posting.Engine.ReverseInTx.
func (s *Service) PlanCancel(ctx context.Context, tx *sql.Tx, voucherType, voucherID string, revision int64, book, movementDate, postingDate string) (PlannedCancel, error) {
	if voucherType == "" || voucherID == "" || book == "" {
		return PlannedCancel{}, fmt.Errorf("%w: voucher type, id and book are required", ErrInvalid)
	}
	if revision < 1 {
		return PlannedCancel{}, fmt.Errorf("%w: voucher revision %d is below 1", ErrInvalid, revision)
	}
	if !validDate(movementDate) || !validDate(postingDate) {
		return PlannedCancel{}, fmt.Errorf("%w: dates must be YYYY-MM-DD", ErrInvalid)
	}
	if rows, found, err := findCancelRows(ctx, tx, voucherType, voucherID, revision, book); err != nil {
		return PlannedCancel{}, err
	} else if found {
		return PlannedCancel{Replayed: true, ReplayRows: rows}, nil
	}
	submits, err := submitRowsOf(ctx, tx, voucherType, voucherID, revision, book)
	if err != nil {
		return PlannedCancel{}, err
	}
	if len(submits) == 0 {
		return PlannedCancel{}, fmt.Errorf("%w: voucher %s %s revision %d", ErrNoSubmit, voucherType, voucherID, revision)
	}
	planned := PlannedCancel{VoucherType: voucherType, VoucherID: voucherID, Revision: revision, Book: book, MovementDate: movementDate, PostingDate: postingDate}
	bins := map[binKey]binState{}
	lock := func(bin binKey) (binState, error) {
		if locked, seen := bins[bin]; seen {
			return locked, nil
		}
		state, _, err := lockBin(ctx, tx, bin)
		if err != nil {
			return binState{}, err
		}
		bins[bin] = state
		return state, nil
	}
	for _, orig := range submits {
		mirror := cancelMirror{Orig: *orig, Qty: orig.Qty.Neg(), Value: orig.Value.Neg(), valued: orig.Value.Sign() != 0}
		if _, err := lock(orig.Bin); err != nil {
			return PlannedCancel{}, err
		}
		switch {
		case orig.Qty.Sign() > 0 && orig.Reversed == "":
			if err := planReceiptCancel(ctx, tx, &mirror); err != nil {
				return PlannedCancel{}, err
			}
		case orig.Qty.Sign() < 0:
			if err := planIssueCancel(ctx, tx, &mirror); err != nil {
				return PlannedCancel{}, err
			}
		default:
			if err := planReturnCancel(ctx, tx, &mirror); err != nil {
				return PlannedCancel{}, err
			}
		}
		if mirror.valued {
			running := bins[orig.Bin].Qty.Add(mirror.Qty)
			if running.Sign() < 0 {
				return PlannedCancel{}, fmt.Errorf("%w: cancel leaves bin at %s", ErrCancelBlocked, running.String())
			}
			state := bins[orig.Bin]
			state.Qty = running
			state.Value = state.Value.Add(mirror.Value)
			bins[orig.Bin] = state
		}
		planned.Mirrors = append(planned.Mirrors, mirror)
	}
	return planned, nil
}

// planReceiptCancel consumes the receipt's own layer back. A residual
// short of the received quantity means downstream issues consumed it.
func planReceiptCancel(ctx context.Context, tx *sql.Tx, mirror *cancelMirror) error {
	layer, found, err := layerByReceipt(ctx, tx, mirror.Orig.ID)
	if err != nil {
		return err
	}
	if !found {
		if mirror.valued {
			return fmt.Errorf("%w: receipt row %s has no layer", ErrIntegrity, mirror.Orig.ID)
		}
		return nil
	}
	if magnitude := mirror.Orig.Qty; layer.Residual.Cmp(magnitude) < 0 {
		return fmt.Errorf("%w: layer holds %s of received %s", ErrCancelBlocked, layer.Residual.String(), magnitude.String())
	}
	mirror.consume = []layerOp{{LayerID: layer.ID, Qty: mirror.Orig.Qty, Value: mirror.Orig.Value.Abs()}}
	return nil
}

// planIssueCancel restores exactly what the issue consumed along its
// application edges.
func planIssueCancel(ctx context.Context, tx *sql.Tx, mirror *cancelMirror) error {
	edges, err := applicationEdges(ctx, tx, mirror.Orig.ID, "issue")
	if err != nil {
		return err
	}
	if len(edges) == 0 {
		if mirror.valued {
			return fmt.Errorf("%w: issue row %s has no applications", ErrIntegrity, mirror.Orig.ID)
		}
		return nil
	}
	for _, edge := range edges {
		mirror.restore = append(mirror.restore, layerOp{LayerID: edge.LayerID, Qty: edge.Qty, Value: edge.Value, ReversesEdge: edge.ID})
	}
	return nil
}

// planReturnCancel consumes again what the return restored along its
// application edges.
func planReturnCancel(ctx context.Context, tx *sql.Tx, mirror *cancelMirror) error {
	edges, err := applicationEdges(ctx, tx, mirror.Orig.ID, "return")
	if err != nil {
		return err
	}
	if len(edges) == 0 {
		if mirror.valued {
			return fmt.Errorf("%w: return row %s has no applications", ErrIntegrity, mirror.Orig.ID)
		}
		return nil
	}
	for _, edge := range edges {
		layer, found, err := layerByID(ctx, tx, edge.LayerID)
		if err != nil {
			return err
		}
		if !found || layer.Residual.Cmp(edge.Qty) < 0 {
			return fmt.Errorf("%w: layer %s is short for the return cancel", ErrCancelBlocked, edge.LayerID)
		}
		mirror.consume = append(mirror.consume, layerOp{LayerID: edge.LayerID, Qty: edge.Qty, Value: edge.Value, ReversesEdge: edge.ID})
	}
	return nil
}

// ApplyCancel writes planned mirrors in the caller's transaction with the
// cancel group id the caller created by reversing the general ledger, or
// claims a group of its own for a quantity-only cancel. When the voucher
// key already has cancel rows ApplyCancel returns the first write.
func (s *Service) ApplyCancel(ctx context.Context, tx *sql.Tx, p PlannedCancel, cancelGroupID string) (AppliedCancel, error) {
	if p.Replayed {
		groupID := ""
		if len(p.ReplayRows) > 0 {
			if row, err := loadStockRow(ctx, tx, p.ReplayRows[0]); err == nil {
				groupID = row.GroupID
			}
		}
		return AppliedCancel{RowIDs: p.ReplayRows, PostingGroupID: groupID, Replayed: true}, nil
	}
	// Claim the group before the replay check, as in ApplyMovement.
	if cancelGroupID == "" {
		claimed, err := s.claimGroup(ctx, tx, p.VoucherType, p.VoucherID, p.Revision, p.Book, posting.EventCancel)
		if err != nil {
			return AppliedCancel{}, err
		}
		cancelGroupID = claimed
	} else if err := lockGroup(ctx, tx, cancelGroupID); err != nil {
		return AppliedCancel{}, err
	}
	if rows, found, err := findCancelRows(ctx, tx, p.VoucherType, p.VoucherID, p.Revision, p.Book); err != nil {
		return AppliedCancel{}, err
	} else if found {
		groupID := ""
		if row, err := loadStockRow(ctx, tx, rows[0]); err == nil {
			groupID = row.GroupID
		}
		return AppliedCancel{RowIDs: rows, PostingGroupID: groupID, Replayed: true}, nil
	}
	applied := AppliedCancel{PostingGroupID: cancelGroupID}
	for _, mirror := range p.Mirrors {
		rowID, err := uuid.NewV7()
		if err != nil {
			return AppliedCancel{}, err
		}
		if err := insertMirrorRow(ctx, tx, p, mirror, rowID.String(), cancelGroupID); err != nil {
			return AppliedCancel{}, err
		}
		if err := applyCancelLayers(ctx, tx, p, rowID.String(), cancelGroupID, mirror); err != nil {
			return AppliedCancel{}, err
		}
		bin, found, err := lockBin(ctx, tx, mirror.Orig.Bin)
		if err != nil {
			return AppliedCancel{}, err
		}
		if err := s.applyBinDelta(ctx, tx, mirror.Orig.Bin, mirror.Orig.ItemType, mirror.Orig.UoM, mirror.Orig.Currency, mirror.Qty, mirror.Value, bin, found); err != nil {
			return AppliedCancel{}, err
		}
		applied.RowIDs = append(applied.RowIDs, rowID.String())
	}
	return applied, nil
}

// submitRowsOf loads the submitted, non-reversal rows of a voucher revision
// newest first, locking them.
func submitRowsOf(ctx context.Context, tx *sql.Tx, voucherType, voucherID string, revision int64, book string) ([]*stockRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM l_stock WHERE voucher_type = ? AND voucher_id = ? AND voucher_revision = ? AND event = ? AND book = ? AND is_reversal = 0 ORDER BY id DESC FOR UPDATE`,
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
	var out []*stockRow
	for _, id := range ids {
		row, err := loadStockRow(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// findCancelRows returns the cancel row ids of a voucher revision, locking
// them.
func findCancelRows(ctx context.Context, q ledger.DB, voucherType, voucherID string, revision int64, book string) ([]string, bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM l_stock WHERE voucher_type = ? AND voucher_id = ? AND voucher_revision = ? AND event = ? AND book = ? ORDER BY id FOR UPDATE`,
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

// insertMirrorRow writes one flagged reversal of a submit row, keeping the
// original's grain, accounts, units, currency, dimensions and source line
// with negated quantity and value under the cancel dates.
func insertMirrorRow(ctx context.Context, tx *sql.Tx, p PlannedCancel, mirror cancelMirror, rowID, groupID string) error {
	orig := mirror.Orig
	bin := orig.Bin
	_, err := tx.ExecContext(ctx, `INSERT INTO l_stock (id, company_name, book, owner, item_type, item_id, control_account_name, location_name, lot_no, serial_no, base_uom, qty_base, value_base, base_currency, movement_date, posting_date, dimensions, voucher_type, voucher_id, voucher_revision, source_line_no, event, posting_group_id, is_reversal, reversed_row_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
		rowID, bin.Company, bin.Book, bin.Owner, orig.ItemType, bin.ItemID, orig.Control, bin.Location,
		nullString(bin.LotNo), nullString(bin.SerialNo), orig.UoM, mirror.Qty.String(), mirror.Value.String(), orig.Currency,
		p.MovementDate, p.PostingDate, orig.Dimensions,
		p.VoucherType, p.VoucherID, p.Revision, orig.SourceLine, posting.EventCancel, groupID, orig.ID)
	return err
}
