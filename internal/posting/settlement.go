// Settlement keeps the party subledger behind control accounts (batch-2 G7,
// ADR-0031, Proposed): open items per instalment, line-level settlement
// applications, both-residual locks with an aggregate ceiling, and unapplied
// cash kept separate from open items. The SettlementService is the only
// writer of l_party, athena_party_items and athena_settlement_applications.
//
// One Apply covers one company, book, party, control account and side in one
// currency: the payment, every item and the company base must match. The
// amounts on an application and a party row are therefore equal in 7.2; the
// columns already exist for the divergent currencies of 7.4, which converts
// through exchange rates and posts realised gain or loss through the G6
// engine. Reversal mirrors and the control-account source existence check
// also arrive in 7.4. This file posts nothing to l_gl: the payment voucher's
// own posting Rule owns its general-ledger legs, and Apply takes the G6
// posting group id from its caller to link the subledger rows to them.
package posting

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"athena-erp/internal/ledger"
	"athena-erp/internal/store"
	"athena-erp/internal/types"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

// Item sides mirror the control leg: a debit-side item is a receivable, a
// credit-side item a payable. Amounts are magnitudes; the side gives the
// direction, as in l_gl.
const (
	ItemSideDebit  = "debit"
	ItemSideCredit = "credit"
)

// Item statuses. An item settles when its residual reaches zero; settled
// rows stay for the audit trail and refuse further application.
const (
	ItemOpen    = "open"
	ItemSettled = "settled"
)

var (
	// ErrItemExists reports an instalment already recorded for its source
	// voucher revision: the unique source key refuses the second insert.
	ErrItemExists = errors.New("posting: the instalment is already recorded")
	// ErrAlreadyApplied reports a settlement line already applied to its
	// item: the unique application key refuses the replay.
	ErrAlreadyApplied = errors.New("posting: the settlement line is already applied to the item")
)

// SettlementService applies payments to open items. DB is the workspace
// database; Dialect classifies unique-key collisions into ErrItemExists and
// ErrAlreadyApplied, so always set it: a nil Dialect degrades collisions
// to raw errors.
type SettlementService struct {
	DB      *sql.DB
	Dialect store.Dialect
}

// NewItem is one instalment row to record for a source voucher revision.
// Original is the full instalment amount; the residual starts equal to it.
type NewItem struct {
	Company        string
	Book           string
	PartyType      string
	PartyID        string
	ControlAccount string
	SourceType     string
	SourceID       string
	SourceRevision int64
	InstalmentNo   int
	DueDate        string // YYYY-MM-DD, or "" when the instalment has none
	Currency       string
	Side           string // ItemSideDebit or ItemSideCredit
	Amount         types.Decimal
}

// Item is one stored open-item row.
type Item struct {
	ID             string
	Company        string
	Book           string
	PartyType      string
	PartyID        string
	ControlAccount string
	SourceType     string
	SourceID       string
	SourceRevision int64
	InstalmentNo   int
	DueDate        string
	Currency       string
	Side           string
	Original       types.Decimal
	Residual       types.Decimal
	Status         string
	Revision       int64
}

// LineItem names one application target of a settlement line. A zero Amount
// takes an automatic proportional share of the line remainder; a positive
// Amount applies exactly that much and must not exceed the item residual.
type LineItem struct {
	ItemID string
	Amount types.Decimal
}

// SettleLine is one payment line: LineNo with a payment-currency magnitude
// and its target items in any order. Items settle in stable id order, so the
// result never depends on the order the caller lists them.
type SettleLine struct {
	LineNo int
	Amount types.Decimal
	Items  []LineItem
}

// SettleRequest applies one settlement voucher to open items. The event is
// always submit here; cancel mirrors arrive in 7.4. PostingGroupID is the
// G6 group the caller posted the settlement under, linking the subledger
// rows to the general-ledger legs.
type SettleRequest struct {
	Company         string
	Book            string
	PartyType       string
	PartyID         string
	ControlAccount  string
	Side            string
	VoucherType     string
	VoucherID       string
	Revision        int64
	Event           string
	PostingDate     string // YYYY-MM-DD
	PostingGroupID  string
	PaymentCurrency string
	Lines           []SettleLine
}

// Application is one written settlement edge between a line and an item.
// The three amounts agree in 7.2 (one currency); 7.4 fills divergent
// payment, item and base amounts through exchange rates.
type Application struct {
	ID            string
	LineNo        int
	ItemID        string
	PaymentAmount types.Decimal
	ItemAmount    types.Decimal
	BaseAmount    types.Decimal
}

// UnappliedCash is one line's remainder that matched no residual, recorded
// as an l_party row with no item rather than forced onto an item.
type UnappliedCash struct {
	LineNo     int
	Amount     types.Decimal
	PartyRowID string
}

// ApplyResult is one committed Apply. Per line, the applications plus the
// unapplied remainder sum exactly to the line amount (R7.3); Items holds
// every touched item at its post-state.
type ApplyResult struct {
	Applications []Application
	Unapplied    []UnappliedCash
	Items        []Item
}

// CreateItemsInTx records open items inside the caller's transaction. Each
// instalment of a source voucher revision records once; a repeat reports
// ErrItemExists.
func (s *SettlementService) CreateItemsInTx(ctx context.Context, tx *sql.Tx, items []NewItem) ([]Item, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: no items to record", ErrInvalid)
	}
	for i := range items {
		if err := checkNewItem(&items[i]); err != nil {
			return nil, err
		}
	}
	const columns = `id, company_name, book, party_type, party_id, control_account_name, ` +
		`source_voucher_type, source_voucher_id, source_voucher_revision, instalment_no, due_date, ` +
		`currency, side, original_amount, residual_amount, status, revision`
	var statement strings.Builder
	statement.WriteString("INSERT INTO athena_party_items (" + columns + ") VALUES ")
	args := make([]any, 0, len(items)*17)
	created := make([]Item, len(items))
	for i := range items {
		item := &items[i]
		id, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("new item id: %w", err)
		}
		if i > 0 {
			statement.WriteString(", ")
		}
		statement.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			id.String(), item.Company, item.Book, item.PartyType, item.PartyID, item.ControlAccount,
			item.SourceType, item.SourceID, item.SourceRevision, item.InstalmentNo, nullText(item.DueDate),
			item.Currency, item.Side, item.Amount.String(), item.Amount.String(), ItemOpen, 1,
		)
		created[i] = Item{
			ID: id.String(), Company: item.Company, Book: item.Book,
			PartyType: item.PartyType, PartyID: item.PartyID, ControlAccount: item.ControlAccount,
			SourceType: item.SourceType, SourceID: item.SourceID,
			SourceRevision: item.SourceRevision, InstalmentNo: item.InstalmentNo,
			DueDate: item.DueDate, Currency: item.Currency, Side: item.Side,
			Original: item.Amount, Residual: item.Amount, Status: ItemOpen, Revision: 1,
		}
	}
	if _, err := tx.ExecContext(ctx, statement.String(), args...); err != nil {
		if s.isDuplicate(err) {
			return nil, fmt.Errorf("%w: one of these instalments is already recorded", ErrItemExists)
		}
		return nil, err
	}
	return created, nil
}

func checkNewItem(item *NewItem) error {
	for name, value := range map[string]string{
		"company": item.Company, "book": item.Book, "party type": item.PartyType,
		"party id": item.PartyID, "control account": item.ControlAccount,
		"source voucher type": item.SourceType, "source voucher id": item.SourceID,
	} {
		if value == "" {
			return fmt.Errorf("%w: an item must name its %s", ErrInvalid, name)
		}
	}
	if item.SourceRevision < 1 {
		return fmt.Errorf("%w: source revision %d is below 1", ErrInvalid, item.SourceRevision)
	}
	if item.InstalmentNo < 1 {
		return fmt.Errorf("%w: instalment number %d is below 1", ErrInvalid, item.InstalmentNo)
	}
	if item.DueDate != "" && !validDate(item.DueDate) {
		return fmt.Errorf("%w: %q is not a date as YYYY-MM-DD", ErrInvalid, item.DueDate)
	}
	if !validCurrency(item.Currency) {
		return fmt.Errorf("%w: %q is not a currency code", ErrInvalid, item.Currency)
	}
	if item.Side != ItemSideDebit && item.Side != ItemSideCredit {
		return fmt.Errorf("%w: side %q must be debit or credit", ErrInvalid, item.Side)
	}
	if item.Amount.Sign() <= 0 {
		return fmt.Errorf("%w: an item amount must be above zero", ErrInvalid)
	}
	if !fitsLedger(item.Amount) {
		return fmt.Errorf("%w: an item amount must fit the ledger columns", ErrInvalid)
	}
	return nil
}

func validCurrency(code string) bool {
	if len(code) != 3 {
		return false
	}
	for _, c := range code {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

func (s *SettlementService) isDuplicate(err error) bool {
	return s.Dialect != nil && s.Dialect.Classify(err) == store.ErrDuplicateKey
}

// ListOpenItems returns a party's open items in stable id order. Q is the
// database or the caller's transaction.
func (s *SettlementService) ListOpenItems(ctx context.Context, q ledger.DB, company, book, partyType, partyID string) ([]Item, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, company_name, book, party_type, party_id, control_account_name,
		source_voucher_type, source_voucher_id, source_voucher_revision, instalment_no, due_date,
		currency, side, original_amount, residual_amount, status, revision
		FROM athena_party_items
		WHERE company_name = ? AND book = ? AND party_type = ? AND party_id = ? AND status = ?
		ORDER BY id`, company, book, partyType, partyID, ItemOpen)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type itemRows interface {
	Scan(...any) error
}

func scanItem(rows itemRows) (Item, error) {
	var item Item
	var due sql.NullTime
	var original, residual string
	if err := rows.Scan(&item.ID, &item.Company, &item.Book, &item.PartyType, &item.PartyID,
		&item.ControlAccount, &item.SourceType, &item.SourceID, &item.SourceRevision,
		&item.InstalmentNo, &due, &item.Currency, &item.Side, &original, &residual,
		&item.Status, &item.Revision); err != nil {
		return Item{}, err
	}
	if due.Valid {
		item.DueDate = due.Time.Format("2006-01-02")
	}
	for target, text := range map[*types.Decimal]string{&item.Original: original, &item.Residual: residual} {
		amount, err := types.ParseDecimal(text)
		if err != nil {
			return Item{}, fmt.Errorf("stored amount %q is not a decimal: %w", text, err)
		}
		*target = amount
	}
	return item, nil
}

// applyAttempts bounds Apply's retries after transient lock failures.
const applyAttempts = 3

// Apply settles a payment inside its own transaction at READ COMMITTED, so
// locking reads see the latest committed residuals, and retries transient
// locking failures (deadlock, or a row changed under a locking read after a
// concurrent commit): both clear when the transaction runs again.
func (s *SettlementService) Apply(ctx context.Context, req SettleRequest) (ApplyResult, error) {
	var result ApplyResult
	var err error
	for attempt := 0; attempt < applyAttempts; attempt++ {
		if result, err = s.applyOnce(ctx, req); err == nil || !isLockRetryable(s.Dialect, err) {
			return result, err
		}
	}
	return ApplyResult{}, err
}

func (s *SettlementService) applyOnce(ctx context.Context, req SettleRequest) (ApplyResult, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return ApplyResult{}, err
	}
	defer tx.Rollback()
	result, err := s.ApplyInTx(ctx, tx, req)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ApplyResult{}, err
	}
	return result, nil
}

// isLockRetryable reports a transient locking failure: a deadlock, or
// InnoDB error 1020 (a row changed under a locking read after a concurrent
// commit). Callers that own their transaction (ApplyInTx) see the raw error
// and retry the same way.
func isLockRetryable(dialect store.Dialect, err error) bool {
	if dialect != nil && dialect.Classify(err) == store.ErrDeadlock {
		return true
	}
	var me *mysql.MySQLError
	return errors.As(err, &me) && (me.Number == 1213 || me.Number == 1020)
}

// ApplyInTx settles a payment inside the caller's transaction: it locks
// every touched item, checks the aggregate ceiling, allocates each line and
// writes the applications, party rows and residual updates together, so a
// failure applies nothing. Lines run in request order against running
// residuals, while the items of one line settle in stable id order.
//
// The both-residual lock of R7.4 is the item rows themselves: every
// application touches at least one item, so locking all touched rows orders
// every conflicting settlement. The settlement side needs no second lock:
// each line remainder is request state computed under the item locks, and
// the ceiling (every share within its residual, every line within its
// amount) holds before anything writes.
func (s *SettlementService) ApplyInTx(ctx context.Context, tx *sql.Tx, req SettleRequest) (ApplyResult, error) {
	if err := checkSettleRequest(req); err != nil {
		return ApplyResult{}, err
	}
	masters := &ledger.Service{DB: tx}
	company, err := masters.GetCompany(ctx, req.Company)
	if err != nil {
		return ApplyResult{}, err
	}
	if req.PaymentCurrency != company.BaseCurrency {
		return ApplyResult{}, fmt.Errorf("%w: payment currency %s is not the company base %s; one settlement covers the base currency", ErrInvalid, req.PaymentCurrency, company.BaseCurrency)
	}
	locked, err := lockItems(ctx, tx, req)
	if err != nil {
		return ApplyResult{}, err
	}
	var result ApplyResult
	running := locked
	touched := map[string]bool{}
	for i := range req.Lines {
		line := &req.Lines[i]
		applied, unapplied, err := allocateLine(line, running)
		if err != nil {
			return ApplyResult{}, err
		}
		for _, share := range applied {
			item := running[share.itemID]
			item.Residual = item.Residual.Sub(share.amount)
			item.Status = ItemOpen
			if item.Residual.Sign() == 0 {
				item.Status = ItemSettled
			}
			running[share.itemID] = item
			touched[share.itemID] = true
		}
		applications, err := sharesToApplications(line.LineNo, applied)
		if err != nil {
			return ApplyResult{}, err
		}
		result.Applications = append(result.Applications, applications...)
		if unapplied.Sign() > 0 {
			result.Unapplied = append(result.Unapplied, UnappliedCash{LineNo: line.LineNo, Amount: unapplied})
		}
	}
	if err := insertApplications(ctx, tx, s, req, result.Applications); err != nil {
		return ApplyResult{}, err
	}
	if err := insertPartyRows(ctx, tx, req, &result); err != nil {
		return ApplyResult{}, err
	}
	if err := updateResiduals(ctx, tx, running, touched); err != nil {
		return ApplyResult{}, err
	}
	for id := range touched {
		result.Items = append(result.Items, running[id])
	}
	sort.Slice(result.Items, func(a, b int) bool { return result.Items[a].ID < result.Items[b].ID })
	return result, nil
}

func checkSettleRequest(req SettleRequest) error {
	for name, value := range map[string]string{
		"company": req.Company, "book": req.Book, "party type": req.PartyType,
		"party id": req.PartyID, "control account": req.ControlAccount,
		"voucher type": req.VoucherType, "voucher id": req.VoucherID,
		"posting group id": req.PostingGroupID,
	} {
		if value == "" {
			return fmt.Errorf("%w: a settlement must name its %s", ErrInvalid, name)
		}
	}
	if req.Side != ItemSideDebit && req.Side != ItemSideCredit {
		return fmt.Errorf("%w: side %q must be debit or credit", ErrInvalid, req.Side)
	}
	if req.Revision < 1 {
		return fmt.Errorf("%w: voucher revision %d is below 1", ErrInvalid, req.Revision)
	}
	if req.Event != EventSubmit {
		return fmt.Errorf("%w: a settlement applies on submit, not %q", ErrInvalid, req.Event)
	}
	if !validDate(req.PostingDate) {
		return fmt.Errorf("%w: %q is not a date as YYYY-MM-DD", ErrInvalid, req.PostingDate)
	}
	if !validCurrency(req.PaymentCurrency) {
		return fmt.Errorf("%w: %q is not a currency code", ErrInvalid, req.PaymentCurrency)
	}
	if len(req.Lines) == 0 {
		return fmt.Errorf("%w: a settlement needs at least one line", ErrInvalid)
	}
	seenLines := map[int]bool{}
	for i := range req.Lines {
		line := &req.Lines[i]
		if line.LineNo < 1 {
			return fmt.Errorf("%w: line number %d is below 1", ErrInvalid, line.LineNo)
		}
		if seenLines[line.LineNo] {
			return fmt.Errorf("%w: line number %d repeats", ErrInvalid, line.LineNo)
		}
		seenLines[line.LineNo] = true
		if line.Amount.Sign() <= 0 {
			return fmt.Errorf("%w: line %d needs an amount above zero", ErrInvalid, line.LineNo)
		}
		if !fitsLedger(line.Amount) {
			return fmt.Errorf("%w: line %d must fit the ledger columns", ErrInvalid, line.LineNo)
		}
		if len(line.Items) == 0 {
			return fmt.Errorf("%w: line %d names no item", ErrInvalid, line.LineNo)
		}
		seenItems := map[string]bool{}
		for _, target := range line.Items {
			if target.ItemID == "" {
				return fmt.Errorf("%w: line %d names an empty item", ErrInvalid, line.LineNo)
			}
			if seenItems[target.ItemID] {
				return fmt.Errorf("%w: line %d names item %s twice", ErrInvalid, line.LineNo, target.ItemID)
			}
			seenItems[target.ItemID] = true
			if target.Amount.Sign() < 0 || !fitsLedger(target.Amount) {
				return fmt.Errorf("%w: line %d names an invalid explicit amount", ErrInvalid, line.LineNo)
			}
		}
	}
	return nil
}

// lockItems reads every item the request names with a write lock, in id
// order so concurrent settlements cannot deadlock. Each item must belong to
// the request's company, book, party, control account, side and currency,
// and must still be open.
func lockItems(ctx context.Context, tx *sql.Tx, req SettleRequest) (map[string]Item, error) {
	ids := map[string]bool{}
	for i := range req.Lines {
		for _, target := range req.Lines[i].Items {
			ids[target.ItemID] = true
		}
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	locked := make(map[string]Item, len(ordered))
	for _, id := range ordered {
		var item Item
		var due sql.NullTime
		var original, residual string
		err := tx.QueryRowContext(ctx, `SELECT id, company_name, book, party_type, party_id, control_account_name,
			source_voucher_type, source_voucher_id, source_voucher_revision, instalment_no, due_date,
			currency, side, original_amount, residual_amount, status, revision
			FROM athena_party_items WHERE id = ? FOR UPDATE`, id).Scan(
			&item.ID, &item.Company, &item.Book, &item.PartyType, &item.PartyID,
			&item.ControlAccount, &item.SourceType, &item.SourceID, &item.SourceRevision,
			&item.InstalmentNo, &due, &item.Currency, &item.Side, &original, &residual,
			&item.Status, &item.Revision)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: item %s does not exist", ErrInvalid, id)
		}
		if err != nil {
			return nil, err
		}
		if due.Valid {
			item.DueDate = due.Time.Format("2006-01-02")
		}
		for target, text := range map[*types.Decimal]string{&item.Original: original, &item.Residual: residual} {
			amount, err := types.ParseDecimal(text)
			if err != nil {
				return nil, fmt.Errorf("stored amount %q is not a decimal: %w", text, err)
			}
			*target = amount
		}
		if item.Company != req.Company || item.Book != req.Book || item.PartyType != req.PartyType ||
			item.PartyID != req.PartyID || item.ControlAccount != req.ControlAccount {
			return nil, fmt.Errorf("%w: item %s belongs to another party or account", ErrInvalid, id)
		}
		if item.Side != req.Side {
			return nil, fmt.Errorf("%w: item %s is on the %s side, the settlement is %s", ErrInvalid, id, item.Side, req.Side)
		}
		if item.Currency != req.PaymentCurrency {
			return nil, fmt.Errorf("%w: item %s is in %s, the payment is in %s: one settlement covers one currency", ErrInvalid, id, item.Currency, req.PaymentCurrency)
		}
		if item.Status != ItemOpen {
			return nil, fmt.Errorf("%w: item %s is already settled", ErrInvalid, id)
		}
		locked[id] = item
	}
	return locked, nil
}

// share is one line's application to one item, decided before anything
// writes.
type share struct {
	itemID string
	amount types.Decimal
}

// allocateLine splits one line across its items against running residuals.
// Explicit amounts apply first and must fit; the remainder splits
// proportionally to residuals through types.Allocate (R7.3), each share
// capped at its residual, and whatever matches no residual stays unapplied.
// The applied shares plus the unapplied remainder always sum exactly to the
// line amount.
func allocateLine(line *SettleLine, running map[string]Item) ([]share, types.Decimal, error) {
	targets := append([]LineItem(nil), line.Items...)
	sort.Slice(targets, func(a, b int) bool { return targets[a].ItemID < targets[b].ItemID })
	var applied []share
	explicit := types.Decimal{}
	var auto []LineItem
	for _, target := range targets {
		residual := running[target.ItemID].Residual
		if target.Amount.Sign() == 0 {
			auto = append(auto, target)
			continue
		}
		if target.Amount.Cmp(residual) > 0 {
			return nil, types.Decimal{}, fmt.Errorf("%w: line %d applies %s to item %s with %s left",
				ErrInvalid, line.LineNo, target.Amount.String(), target.ItemID, residual.String())
		}
		explicit = explicit.Add(target.Amount)
		applied = append(applied, share{itemID: target.ItemID, amount: target.Amount})
	}
	if explicit.Cmp(line.Amount) > 0 {
		return nil, types.Decimal{}, fmt.Errorf("%w: line %d names more than its amount", ErrInvalid, line.LineNo)
	}
	remainder := line.Amount.Sub(explicit)
	if remainder.Sign() == 0 || len(auto) == 0 {
		return applied, remainder, nil
	}
	weights := make([]int64, len(auto))
	anyOpen := false
	for i, target := range auto {
		residual := running[target.ItemID].Residual
		if residual.Sign() > 0 {
			anyOpen = true
		}
		weight, err := weightUnits(residual)
		if err != nil {
			return nil, types.Decimal{}, fmt.Errorf("%w: residual of item %s is too large to allocate", ErrInvalid, target.ItemID)
		}
		weights[i] = weight
	}
	if !anyOpen {
		return applied, remainder, nil
	}
	parts, err := remainder.Allocate(weights, 6)
	if err != nil {
		return nil, types.Decimal{}, err
	}
	left := remainder
	for i, target := range auto {
		part := parts[i]
		if residual := running[target.ItemID].Residual; part.Cmp(residual) > 0 {
			part = residual
		}
		if part.Sign() == 0 {
			continue
		}
		left = left.Sub(part)
		applied = append(applied, share{itemID: target.ItemID, amount: part})
	}
	if left.Sign() < 0 || explicit.Add(remainder.Sub(left)).Cmp(line.Amount) > 0 {
		return nil, types.Decimal{}, fmt.Errorf("settlement ceiling slipped on line %d", line.LineNo)
	}
	return applied, left, nil
}

// weightUnits reads a residual as integer minor units at ledger scale for
// Allocate's int64 weights. Only proportionality matters, so the scale is
// fixed; a residual past int64 range (absurd money) refuses instead of
// overflowing.
func weightUnits(residual types.Decimal) (int64, error) {
	scaled, err := residual.Rescale(6)
	if err != nil {
		return 0, err
	}
	digits := strings.Replace(scaled.String(), ".", "", 1)
	units, ok := new(big.Int).SetString(digits, 10)
	if !ok || !units.IsInt64() {
		return 0, fmt.Errorf("residual %q is not an int64 weight", scaled.String())
	}
	return units.Int64(), nil
}

// sharesToApplications numbers the decided shares as application rows with
// fresh ids. The three amounts agree in 7.2; see ApplyResult.
func sharesToApplications(lineNo int, applied []share) ([]Application, error) {
	out := make([]Application, 0, len(applied))
	for _, share := range applied {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("new application id: %w", err)
		}
		out = append(out, Application{
			ID: id.String(), LineNo: lineNo, ItemID: share.itemID,
			PaymentAmount: share.amount, ItemAmount: share.amount, BaseAmount: share.amount,
		})
	}
	return out, nil
}

// insertApplications writes the decided application edges. A replay of the
// same settlement line against the same item hits the unique line key and
// reports ErrAlreadyApplied.
func insertApplications(ctx context.Context, tx *sql.Tx, s *SettlementService, req SettleRequest, applications []Application) error {
	if len(applications) == 0 {
		return nil
	}
	const columns = `id, company_name, book, event, settlement_voucher_type, settlement_voucher_id, ` +
		`settlement_voucher_revision, settlement_line_no, item_id, payment_currency, payment_amount, ` +
		`item_currency, item_amount, base_amount, is_reversal, reverses_application_id, posting_group_id`
	var statement strings.Builder
	statement.WriteString("INSERT INTO athena_settlement_applications (" + columns + ") VALUES ")
	args := make([]any, 0, len(applications)*17)
	for i := range applications {
		application := &applications[i]
		if i > 0 {
			statement.WriteString(", ")
		}
		statement.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			application.ID, req.Company, req.Book, req.Event,
			req.VoucherType, req.VoucherID, req.Revision, application.LineNo, application.ItemID,
			req.PaymentCurrency, application.PaymentAmount.String(),
			req.PaymentCurrency, application.ItemAmount.String(), application.BaseAmount.String(),
			false, nil, req.PostingGroupID,
		)
	}
	if _, err := tx.ExecContext(ctx, statement.String(), args...); err != nil {
		if s.isDuplicate(err) {
			return fmt.Errorf("%w: this settlement already applied these lines", ErrAlreadyApplied)
		}
		return err
	}
	return nil
}

// insertPartyRows writes one l_party row per application plus one unapplied
// row per line remainder, and stamps the unapplied row ids back on the
// result. Applications reduce item balances, so they take the opposite side
// of the item: a debit-side (receivable) item settles by credit, a
// credit-side (payable) item by debit. Unapplied cash takes the opposite of
// the request side for the same reason. Reversal mirrors arrive in 7.4.
func insertPartyRows(ctx context.Context, tx *sql.Tx, req SettleRequest, result *ApplyResult) error {
	type partyRow struct {
		itemID        any
		applicationID any
		debit         string
		credit        string
	}
	rows := make([]partyRow, 0, len(result.Applications)+len(result.Unapplied))
	byIndex := map[int]int{}
	for i := range result.Applications {
		application := &result.Applications[i]
		amount := application.ItemAmount.String()
		row := partyRow{itemID: application.ItemID, applicationID: application.ID}
		if req.Side == ItemSideDebit {
			row.credit = amount
		} else {
			row.debit = amount
		}
		byIndex[len(rows)] = -1
		rows = append(rows, row)
	}
	for i := range result.Unapplied {
		amount := result.Unapplied[i].Amount.String()
		row := partyRow{}
		if req.Side == ItemSideDebit {
			row.credit = amount
		} else {
			row.debit = amount
		}
		byIndex[len(rows)] = i
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}
	const columns = `id, company_name, book, party_type, party_id, control_account_name, ` +
		`item_id, application_id, posting_date, debit_base, credit_base, txn_currency, ` +
		`txn_debit, txn_credit, voucher_type, voucher_id, voucher_revision, event, ` +
		`posting_group_id, is_reversal, reversed_row_id`
	var statement strings.Builder
	statement.WriteString("INSERT INTO l_party (" + columns + ") VALUES ")
	args := make([]any, 0, len(rows)*21)
	zero := "0"
	for i := range rows {
		row := &rows[i]
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("new party row id: %w", err)
		}
		if unapplied := byIndex[i]; unapplied >= 0 {
			result.Unapplied[unapplied].PartyRowID = id.String()
		}
		if i > 0 {
			statement.WriteString(", ")
		}
		statement.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		debit, credit := zero, zero
		if row.debit != "" {
			debit = row.debit
		}
		if row.credit != "" {
			credit = row.credit
		}
		args = append(args,
			id.String(), req.Company, req.Book, req.PartyType, req.PartyID, req.ControlAccount,
			row.itemID, row.applicationID, req.PostingDate, debit, credit,
			req.PaymentCurrency, debit, credit,
			req.VoucherType, req.VoucherID, req.Revision, req.Event,
			req.PostingGroupID, false, nil,
		)
	}
	_, err := tx.ExecContext(ctx, statement.String(), args...)
	return err
}

// updateResiduals stores the decided post-state of every touched item. The
// rows are already write-locked, so the update cannot lose a concurrent
// settlement; the revision still bumps for readers.
func updateResiduals(ctx context.Context, tx *sql.Tx, running map[string]Item, touched map[string]bool) error {
	for id := range touched {
		item := running[id]
		if _, err := tx.ExecContext(ctx, `UPDATE athena_party_items
			SET residual_amount = ?, status = ?, revision = revision + 1 WHERE id = ?`,
			item.Residual.String(), item.Status, id); err != nil {
			return err
		}
	}
	return nil
}
