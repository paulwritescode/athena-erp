// Settlement keeps the party subledger behind control accounts (batch-2 G7,
// ADR-0031, Proposed): open items per instalment, line-level settlement
// applications, both-residual locks with an aggregate ceiling, unapplied
// cash kept separate from open items, realised foreign-exchange gains and
// losses posted as plans through the G6 engine, and mirror reversals that
// land in the current period. The SettlementService is the only writer of
// l_party, athena_party_items and athena_settlement_applications.
//
// One Apply covers one company, book, party, control account and side, with
// the payment in the items' currency; items may be foreign to the company
// base, and the base amounts convert through exchange rates. The payment
// voucher's own posting Rule owns its general-ledger legs; the service
// posts only the realised FX top-up, under its own voucher identity so the
// group keys never collide with the payment's own post.
//
// Periods follow the decided Z2 policy (ADR-0023, O-011/O-012): subledger
// rows land in open periods, or in soft-closed periods with close authority;
// hard-closed periods refuse everything; corrections land in the current
// period as reversals, never edits and never backdated.
package posting

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

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
	// ErrAlreadyReversed reports a settlement with no unreversed
	// application left: every line already has its cancel mirror.
	ErrAlreadyReversed = errors.New("posting: the settlement is already reversed")
)

// FXVoucherType is the voucher identity realised-FX plans post under. The
// gain or loss belongs to the settlement economically, but posting it under
// the payment's own (voucher, revision, submit, book) key would collide
// with the payment's own general-ledger group and silently replay instead
// of posting. The fx_settlement identity (with the settlement's voucher id
// and revision) keeps its own submit and cancel groups, so the engine's
// replay and reverse paths work on it unchanged.
const FXVoucherType = "fx_settlement"

// FXConfig carries what a foreign-currency settlement needs beyond the
// request: the exchange-rate type (and optional source, empty for the
// all-sources-agree lookup) and the resolved gain and loss accounts. A nil
// config means a base-currency settlement: any foreign item refuses. The
// caller resolves the accounts (determinations live workspace-side); the
// service only checks they are named and lets the engine validate them.
type FXConfig struct {
	RateType    string
	RateSource  string
	GainAccount string
	LossAccount string
}

// CloseAuthority asserts the caller may post an adjustment into a
// soft-closed period: the service layer verified the ledger:close grant and
// passes the reason through. Settlement records nothing itself; the server
// writes the audit row with its full identity beside the call.
type CloseAuthority struct {
	Allowed bool
	Reason  string
}

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

// Item is one stored open-item row. CreatedAt is the booking instant items
// are recorded at invoice submit; realised FX reads the invoice rate in
// force then.
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
	CreatedAt      time.Time
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
// always submit here; Reverse owns cancel. PostingGroupID is the G6 group
// the caller posted the settlement under, linking the subledger rows to the
// general-ledger legs. Actor and Source identify the engine call behind a
// realised-FX post. FX carries the rate type and gain/loss accounts when
// items are foreign to the base; nil means a base-currency settlement.
// Authority asserts close authority for a soft-closed posting date.
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
	Actor           string
	Source          string // api, cli, rule, import or job
	RequestID       string
	FX              *FXConfig
	Authority       *CloseAuthority
	Lines           []SettleLine
}

// Application is one written settlement edge between a line and an item:
// the payment-currency share, the item-currency share (equal in 7.4: one
// settlement covers one transaction currency) and the base value at the
// settlement rate.
type Application struct {
	ID            string
	LineNo        int
	ItemID        string
	PaymentAmount types.Decimal
	ItemAmount    types.Decimal
	BaseAmount    types.Decimal
}

// UnappliedCash is one line's remainder that matched no residual, recorded
// as an l_party row with no item rather than forced onto an item. Amount is
// the payment-currency remainder; BaseAmount its value at the settlement
// rate.
type UnappliedCash struct {
	LineNo     int
	Amount     types.Decimal
	BaseAmount types.Decimal
	PartyRowID string
}

// ApplyResult is one committed Apply. Per line, the applications plus the
// unapplied remainder sum exactly to the line amount (R7.3); Items holds
// every touched item at its post-state; FXGroupID is the realised-FX engine
// group, empty when the settlement carried no gain or loss.
type ApplyResult struct {
	Applications []Application
	Unapplied    []UnappliedCash
	Items        []Item
	FXGroupID    string
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
		currency, side, original_amount, residual_amount, status, revision, created_at
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
		&item.Status, &item.Revision, &item.CreatedAt); err != nil {
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

// txAttempts bounds settlement retries after transient lock failures.
const txAttempts = 3

// Apply settles a payment inside its own transaction at READ COMMITTED, so
// locking reads see the latest committed residuals, and retries transient
// locking failures (deadlock, or a row changed under a locking read after a
// concurrent commit): both clear when the transaction runs again.
func (s *SettlementService) Apply(ctx context.Context, req SettleRequest) (ApplyResult, error) {
	return RunTx(ctx, s.DB, s.Dialect, func(tx *sql.Tx) (ApplyResult, error) {
		return s.ApplyInTx(ctx, tx, req)
	})
}

// RunTx runs fn in one READ COMMITTED transaction, retrying transient
// locking failures. A failed attempt rolls back whole before the next run.
// Server handlers compose settlement calls with audit rows through it.
func RunTx[T any](ctx context.Context, db *sql.DB, dialect store.Dialect, fn func(tx *sql.Tx) (T, error)) (T, error) {
	var zero T
	var err error
	for attempt := 0; attempt < txAttempts; attempt++ {
		var tx *sql.Tx
		tx, err = db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			return zero, err
		}
		var result T
		result, err = fn(tx)
		if err != nil {
			tx.Rollback()
			if !isLockRetryable(dialect, err) {
				return zero, err
			}
			continue
		}
		if err = tx.Commit(); err != nil {
			tx.Rollback()
			if !isLockRetryable(dialect, err) {
				return zero, err
			}
			continue
		}
		return result, nil
	}
	return zero, err
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
	if err := checkControlAccount(ctx, masters, req.ControlAccount); err != nil {
		return ApplyResult{}, err
	}
	period, err := checkPostingPeriod(ctx, masters, req.Company, req.Book, req.PostingDate, req.Authority, false)
	if err != nil {
		return ApplyResult{}, err
	}
	locked, err := lockItems(ctx, tx, req.Company, req.Book, req.PartyType, req.PartyID, req.ControlAccount, req.Side, req.PaymentCurrency, lockRequestItems(req), true)
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
	gains, err := priceFX(ctx, masters, req, &result, running, company.BaseCurrency)
	if err != nil {
		return ApplyResult{}, err
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
	if result.FXGroupID, err = s.postFX(ctx, tx, req, result.Applications, gains, period.Status); err != nil {
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
		"posting group id": req.PostingGroupID, "actor": req.Actor, "source": req.Source,
	} {
		if value == "" {
			return fmt.Errorf("%w: a settlement must name its %s", ErrInvalid, name)
		}
	}
	if req.FX != nil {
		for name, value := range map[string]string{
			"rate type": req.FX.RateType, "gain account": req.FX.GainAccount, "loss account": req.FX.LossAccount,
		} {
			if value == "" {
				return fmt.Errorf("%w: foreign-currency settlement must name its %s", ErrInvalid, name)
			}
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

// lockItems reads every named item with a write lock, in id order so
// concurrent settlements cannot deadlock. Each item must belong to the
// given company, book, party, control account, side and currency. requireOpen
// refuses settled rows; reversals pass false because restoring settles in
// reverse.
func lockItems(ctx context.Context, tx *sql.Tx, company, book, partyType, partyID, controlAccount, side, currency string, ids []string, requireOpen bool) (map[string]Item, error) {
	ordered := append([]string(nil), ids...)
	sort.Strings(ordered)
	locked := make(map[string]Item, len(ordered))
	for _, id := range ordered {
		var item Item
		var due sql.NullTime
		var original, residual string
		err := tx.QueryRowContext(ctx, `SELECT id, company_name, book, party_type, party_id, control_account_name,
			source_voucher_type, source_voucher_id, source_voucher_revision, instalment_no, due_date,
			currency, side, original_amount, residual_amount, status, revision, created_at
			FROM athena_party_items WHERE id = ? FOR UPDATE`, id).Scan(
			&item.ID, &item.Company, &item.Book, &item.PartyType, &item.PartyID,
			&item.ControlAccount, &item.SourceType, &item.SourceID, &item.SourceRevision,
			&item.InstalmentNo, &due, &item.Currency, &item.Side, &original, &residual,
			&item.Status, &item.Revision, &item.CreatedAt)
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
		if item.Company != company || item.Book != book || item.PartyType != partyType ||
			item.PartyID != partyID || item.ControlAccount != controlAccount {
			return nil, fmt.Errorf("%w: item %s belongs to another party or account", ErrInvalid, id)
		}
		if item.Side != side {
			return nil, fmt.Errorf("%w: item %s is on the %s side, the settlement is %s", ErrInvalid, id, item.Side, side)
		}
		if item.Currency != currency {
			return nil, fmt.Errorf("%w: item %s is in %s, the payment is in %s: one settlement covers one currency", ErrInvalid, id, item.Currency, currency)
		}
		if requireOpen && item.Status != ItemOpen {
			return nil, fmt.Errorf("%w: item %s is already settled", ErrInvalid, id)
		}
		locked[id] = item
	}
	return locked, nil
}

// checkControlAccount requires the named control account to exist and carry
// a control role: only control accounts may anchor items.
func checkControlAccount(ctx context.Context, masters *ledger.Service, name string) error {
	account, err := masters.GetAccount(ctx, name)
	if err != nil {
		return err
	}
	if account.ControlRole == "" {
		return fmt.Errorf("%w: account %q carries no control role", ErrInvalid, name)
	}
	return nil
}

// checkPostingPeriod enforces the decided book and period policy (Z2): the
// book must be defined for the company (periods exist under it), a period
// must cover the posting date, open periods proceed, soft-closed periods
// need close authority, and hard-closed periods refuse everything. Reversals
// pass strictOpen: corrections land in the current period, which is open,
// never in a closed one.
func checkPostingPeriod(ctx context.Context, masters *ledger.Service, company, book, date string, authority *CloseAuthority, strictOpen bool) (ledger.Period, error) {
	periods, err := masters.ListPeriods(ctx, company, book)
	if err != nil {
		return ledger.Period{}, err
	}
	if len(periods) == 0 {
		return ledger.Period{}, fmt.Errorf("%w: book %q is not defined for company %q", ErrInvalid, book, company)
	}
	period, err := masters.PeriodFor(ctx, company, book, date)
	if err != nil {
		if errors.Is(err, ledger.ErrNotFound) {
			return ledger.Period{}, fmt.Errorf("%w: no period covers %s for company %q in book %q", ErrInvalid, date, company, book)
		}
		return ledger.Period{}, err
	}
	switch period.Status {
	case ledger.PeriodOpen:
		return period, nil
	case ledger.PeriodSoftClosed:
		if strictOpen {
			return ledger.Period{}, fmt.Errorf("%w: reversals land in the current open period, not in soft-closed %q", ErrInvalid, period.Name)
		}
		if authority == nil || !authority.Allowed || authority.Reason == "" {
			return ledger.Period{}, fmt.Errorf("%w: period %q is soft-closed: adjustments need the ledger:close grant and a reason", ErrInvalid, period.Name)
		}
		return period, nil
	case ledger.PeriodHardClosed:
		return ledger.Period{}, fmt.Errorf("%w: period %q is hard-closed and refuses everything; post into the current open period", ErrInvalid, period.Name)
	default:
		return ledger.Period{}, fmt.Errorf("%w: period %q has status %q", ErrInvalid, period.Name, period.Status)
	}
}

// lockRequestItems collects the distinct item ids of a settle request for
// locking.
func lockRequestItems(req SettleRequest) []string {
	seen := map[string]bool{}
	var ids []string
	for i := range req.Lines {
		for _, target := range req.Lines[i].Items {
			if !seen[target.ItemID] {
				seen[target.ItemID] = true
				ids = append(ids, target.ItemID)
			}
		}
	}
	return ids
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

// priceFX fills the base amounts of decided applications and unapplied
// remainders and returns the signed realised gain per application id (R7.5):
// the settled base value minus the booked base value, with the sign flipped
// for credit-side items (owing less in base terms is the gain there). The
// invoice rate is the rate in force when the item was recorded; the
// settlement rate is the rate in force at the end of the posting date.
// Base-currency items price at par with no rate reads; foreign items need
// the request's FX config. Products round half up to ledger scale, the
// codebase default.
func priceFX(ctx context.Context, masters *ledger.Service, req SettleRequest, result *ApplyResult, items map[string]Item, base string) (map[string]types.Decimal, error) {
	settleAt, err := endOfDate(req.PostingDate)
	if err != nil {
		return nil, err
	}
	gains := map[string]types.Decimal{}
	convert := func(amount types.Decimal, item Item) (types.Decimal, error) {
		if item.Currency == base {
			return amount, nil
		}
		if req.FX == nil {
			return types.Decimal{}, fmt.Errorf("%w: item %s is in %s with base %s but the settlement names no FX config", ErrInvalid, item.ID, item.Currency, base)
		}
		settleRate, err := masters.GetRate(ctx, item.Currency, base, req.FX.RateType, req.FX.RateSource, settleAt)
		if err != nil {
			return types.Decimal{}, err
		}
		return roundLedger(amount.Mul(settleRate))
	}
	for i := range result.Applications {
		application := &result.Applications[i]
		item := items[application.ItemID]
		settled, err := convert(application.ItemAmount, item)
		if err != nil {
			return nil, err
		}
		application.BaseAmount = settled
		if item.Currency == base {
			continue
		}
		invoiceRate, err := masters.GetRate(ctx, item.Currency, base, req.FX.RateType, req.FX.RateSource, item.CreatedAt)
		if err != nil {
			return nil, err
		}
		booked, err := roundLedger(application.ItemAmount.Mul(invoiceRate))
		if err != nil {
			return nil, err
		}
		share := settled.Sub(booked)
		if req.Side == ItemSideCredit {
			share = share.Neg()
		}
		if share.Sign() != 0 {
			gains[application.ID] = share
		}
	}
	for i := range result.Unapplied {
		unapplied := &result.Unapplied[i]
		// Unapplied cash has no booked base to differ from: it records at
		// the settlement rate directly. The item currency is the payment
		// currency here (one settlement covers one transaction currency).
		probe := Item{Currency: req.PaymentCurrency}
		valued, err := convert(unapplied.Amount, probe)
		if err != nil {
			return nil, err
		}
		unapplied.BaseAmount = valued
	}
	return gains, nil
}

func endOfDate(date string) (time.Time, error) {
	day, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q is not a date as YYYY-MM-DD", ErrInvalid, date)
	}
	return day.Add(23*time.Hour + 59*time.Minute + 59*time.Second), nil
}

func roundLedger(amount types.Decimal) (types.Decimal, error) {
	rounded, err := amount.Round(6, types.HalfUp)
	if err != nil {
		return types.Decimal{}, err
	}
	if !fitsLedger(rounded) {
		return types.Decimal{}, fmt.Errorf("%w: converted amount %s overflows the ledger columns", ErrInvalid, rounded.String())
	}
	return rounded, nil
}

// postFX posts the realised gains and losses as one balanced plan through
// the G6 engine under the fx_settlement identity, and returns its group id
// (empty when nothing posted). Every nonzero per-application gain becomes
// its own leg pair, so offsetting gains and losses all post instead of
// netting away: a gain debits the control account and credits the gain
// account, a loss debits the loss account and credits the control account.
// Each control leg sources its locked item, satisfying the engine's control
// check with rows known to exist. One FX post per voucher revision: a repeat
// refuses upfront, and a concurrent winner surfaces as a replay, which also
// refuses instead of returning stale gain. Realised FX posts only into open
// periods: the engine does not take adjustments yet, so a soft-closed date
// refuses here with the decided wording instead of surfacing the engine's
// stale one.
func (s *SettlementService) postFX(ctx context.Context, tx *sql.Tx, req SettleRequest, applications []Application, gains map[string]types.Decimal, status string) (string, error) {
	var lines []Line
	for i := range applications {
		application := &applications[i]
		gain, ok := gains[application.ID]
		if !ok || gain.Sign() == 0 {
			continue
		}
		if status != ledger.PeriodOpen {
			return "", fmt.Errorf("%w: realised FX of %s posts only into an open period; the posting period is %s",
				ErrInvalid, gain.String(), status)
		}
		magnitude := gain.Abs()
		if gain.Sign() < 0 {
			lines = append(lines,
				Line{Company: req.Company, Account: req.FX.LossAccount, DebitBase: magnitude},
				Line{Company: req.Company, Account: req.ControlAccount, CreditBase: magnitude, SourceType: "party_item", SourceID: application.ItemID},
			)
		} else {
			lines = append(lines,
				Line{Company: req.Company, Account: req.ControlAccount, DebitBase: magnitude, SourceType: "party_item", SourceID: application.ItemID},
				Line{Company: req.Company, Account: req.FX.GainAccount, CreditBase: magnitude},
			)
		}
	}
	if len(lines) == 0 {
		return "", nil
	}
	var posted string
	err := tx.QueryRowContext(ctx, `SELECT id FROM athena_posting_groups
		WHERE voucher_type = ? AND voucher_id = ? AND voucher_revision = ? AND event = ? AND book = ?`,
		FXVoucherType, req.VoucherID, req.Revision, EventSubmit, req.Book).Scan(&posted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if posted != "" {
		return "", fmt.Errorf("%w: realised FX for this voucher revision is already posted; amend the voucher to apply again", ErrInvalid)
	}
	plan := Plan{
		VoucherType: FXVoucherType, VoucherID: req.VoucherID, Revision: req.Revision,
		Event: EventSubmit, Book: req.Book, PostingDate: req.PostingDate,
		Lines: lines,
	}
	engine := &Engine{DB: s.DB, Dialect: s.Dialect}
	result, err := engine.PostInTx(ctx, tx, plan, PostOptions{Actor: req.Actor, Source: req.Source, RequestID: req.RequestID})
	if err != nil {
		return "", err
	}
	if result.Replayed {
		return "", fmt.Errorf("%w: realised FX for this revision posted concurrently; amend the voucher to apply again", ErrInvalid)
	}
	return result.PostingGroupID, nil
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
		txnDebit      string
		txnCredit     string
	}
	rows := make([]partyRow, 0, len(result.Applications)+len(result.Unapplied))
	byIndex := map[int]int{}
	for i := range result.Applications {
		application := &result.Applications[i]
		row := partyRow{itemID: application.ItemID, applicationID: application.ID}
		if req.Side == ItemSideDebit {
			row.credit = application.BaseAmount.String()
			row.txnCredit = application.ItemAmount.String()
		} else {
			row.debit = application.BaseAmount.String()
			row.txnDebit = application.ItemAmount.String()
		}
		byIndex[len(rows)] = -1
		rows = append(rows, row)
	}
	for i := range result.Unapplied {
		unapplied := &result.Unapplied[i]
		row := partyRow{}
		if req.Side == ItemSideDebit {
			row.credit = unapplied.BaseAmount.String()
			row.txnCredit = unapplied.Amount.String()
		} else {
			row.debit = unapplied.BaseAmount.String()
			row.txnDebit = unapplied.Amount.String()
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
		txnDebit, txnCredit := zero, zero
		if row.txnDebit != "" {
			txnDebit = row.txnDebit
		}
		if row.txnCredit != "" {
			txnCredit = row.txnCredit
		}
		args = append(args,
			id.String(), req.Company, req.Book, req.PartyType, req.PartyID, req.ControlAccount,
			row.itemID, row.applicationID, req.PostingDate, debit, credit,
			req.PaymentCurrency, txnDebit, txnCredit,
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

// ReverseRequest unwinds one settlement voucher: every unreversed
// submit-event application gets its cancel mirror, the items' residuals are
// restored, unapplied cash mirrors back, and any realised-FX post unwinds
// through the engine. The applications of every submit revision unwind
// together, so a settlement re-revisioned after submit still unwinds fully;
// each mirror keeps its submit revision for exact pairing while the mirror
// party rows record Revision, the cancel revision the mirrors land under.
// PostingDate is the current date: corrections land in the current period,
// which is open, never backdated into a closed one.
type ReverseRequest struct {
	Company        string
	Book           string
	PartyType      string
	PartyID        string
	ControlAccount string
	Side           string
	VoucherType    string
	VoucherID      string
	Revision       int64
	PostingDate    string // YYYY-MM-DD, the current date
	Actor          string
	Source         string // api, cli, rule, import or job
	RequestID      string
}

// ReverseResult is one committed Reverse. Applications holds the cancel
// mirrors, Items the restored items, and FXCancelGroups the engine cancel
// group per unwound FX post (empty when the settlement posted no FX).
type ReverseResult struct {
	Applications   []Application
	Items          []Item
	FXCancelGroups []string
}

// Reverse unwinds a settlement inside its own transaction, with the same
// isolation and lock-failure retries as Apply.
func (s *SettlementService) Reverse(ctx context.Context, req ReverseRequest) (ReverseResult, error) {
	return RunTx(ctx, s.DB, s.Dialect, func(tx *sql.Tx) (ReverseResult, error) {
		return s.ReverseInTx(ctx, tx, req)
	})
}

// ReverseInTx unwinds a settlement inside the caller's transaction. Stored
// rows are mirrored, never edited: applications gain cancel mirrors,
// residuals restore, unapplied cash mirrors back, and each realised-FX post
// unwinds through the engine. Anything failing rolls everything back and
// writes nothing.
func (s *SettlementService) ReverseInTx(ctx context.Context, tx *sql.Tx, req ReverseRequest) (ReverseResult, error) {
	if err := checkReverseRequest(req); err != nil {
		return ReverseResult{}, err
	}
	masters := &ledger.Service{DB: tx}
	if err := checkControlAccount(ctx, masters, req.ControlAccount); err != nil {
		return ReverseResult{}, err
	}
	if _, err := checkPostingPeriod(ctx, masters, req.Company, req.Book, req.PostingDate, nil, true); err != nil {
		return ReverseResult{}, err
	}
	originals, err := unreversedApplications(ctx, tx, req)
	if err != nil {
		return ReverseResult{}, err
	}
	ids := make([]string, 0, len(originals))
	for _, original := range originals {
		ids = append(ids, original.ItemID)
	}
	currency := ""
	if len(originals) > 0 {
		currency = originals[0].PaymentCurrency
	}
	locked, err := lockItems(ctx, tx, req.Company, req.Book, req.PartyType, req.PartyID, req.ControlAccount, req.Side, currency, ids, false)
	if err != nil {
		return ReverseResult{}, err
	}
	batch, err := uuid.NewV7()
	if err != nil {
		return ReverseResult{}, fmt.Errorf("new mirror batch id: %w", err)
	}
	var result ReverseResult
	for _, original := range originals {
		id, err := uuid.NewV7()
		if err != nil {
			return ReverseResult{}, fmt.Errorf("new mirror id: %w", err)
		}
		mirror := Application{
			ID: id.String(), LineNo: original.LineNo, ItemID: original.ItemID,
			PaymentAmount: original.PaymentAmount, ItemAmount: original.ItemAmount, BaseAmount: original.BaseAmount,
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO athena_settlement_applications
			(id, company_name, book, event, settlement_voucher_type, settlement_voucher_id,
			settlement_voucher_revision, settlement_line_no, item_id, payment_currency, payment_amount,
			item_currency, item_amount, base_amount, is_reversal, reverses_application_id, posting_group_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			mirror.ID, req.Company, req.Book, EventCancel, req.VoucherType, req.VoucherID,
			original.Revision, mirror.LineNo, mirror.ItemID, original.PaymentCurrency, mirror.PaymentAmount.String(),
			original.ItemCurrency, mirror.ItemAmount.String(), mirror.BaseAmount.String(),
			true, original.ApplicationID, batch.String()); err != nil {
			if s.isDuplicate(err) {
				return ReverseResult{}, fmt.Errorf("%w: this settlement already reversed these lines", ErrAlreadyReversed)
			}
			return ReverseResult{}, err
		}
		if err := mirrorPartyRow(ctx, tx, req, batch.String(), original.ApplicationID, mirror.ID); err != nil {
			return ReverseResult{}, err
		}
		item := locked[original.ItemID]
		item.Residual = item.Residual.Add(original.ItemAmount)
		if item.Residual.Cmp(item.Original) > 0 {
			return ReverseResult{}, fmt.Errorf("restored residual %s of item %s exceeds its original %s",
				item.Residual.String(), item.ID, item.Original.String())
		}
		item.Status = ItemOpen
		locked[original.ItemID] = item
		result.Applications = append(result.Applications, mirror)
	}
	if err := mirrorUnappliedCash(ctx, tx, req, batch.String()); err != nil {
		return ReverseResult{}, err
	}
	touched := map[string]bool{}
	for _, original := range originals {
		touched[original.ItemID] = true
	}
	if err := updateResiduals(ctx, tx, locked, touched); err != nil {
		return ReverseResult{}, err
	}
	for id := range touched {
		result.Items = append(result.Items, locked[id])
	}
	sort.Slice(result.Items, func(a, b int) bool { return result.Items[a].ID < result.Items[b].ID })
	groups, err := s.reverseFX(ctx, tx, req, originals)
	if err != nil {
		return ReverseResult{}, err
	}
	result.FXCancelGroups = groups
	return result, nil
}

func checkReverseRequest(req ReverseRequest) error {
	for name, value := range map[string]string{
		"company": req.Company, "book": req.Book, "party type": req.PartyType,
		"party id": req.PartyID, "control account": req.ControlAccount,
		"voucher type": req.VoucherType, "voucher id": req.VoucherID,
		"actor": req.Actor, "source": req.Source,
	} {
		if value == "" {
			return fmt.Errorf("%w: a reversal must name its %s", ErrInvalid, name)
		}
	}
	if req.Side != ItemSideDebit && req.Side != ItemSideCredit {
		return fmt.Errorf("%w: side %q must be debit or credit", ErrInvalid, req.Side)
	}
	if req.Revision < 1 {
		return fmt.Errorf("%w: voucher revision %d is below 1", ErrInvalid, req.Revision)
	}
	if !validDate(req.PostingDate) {
		return fmt.Errorf("%w: %q is not a date as YYYY-MM-DD", ErrInvalid, req.PostingDate)
	}
	return nil
}

// storedApplication is one submit-event application row with its currencies.
type storedApplication struct {
	ApplicationID   string
	LineNo          int
	ItemID          string
	PaymentCurrency string
	PaymentAmount   types.Decimal
	ItemCurrency    string
	ItemAmount      types.Decimal
	BaseAmount      types.Decimal
	Revision        int64
}

// unreversedApplications reads every submit-event application of the voucher
// without a cancel mirror yet, in id order. A voucher whose lines all
// reversed reports ErrAlreadyReversed; one that never applied reports
// ErrInvalid.
func unreversedApplications(ctx context.Context, tx *sql.Tx, req ReverseRequest) ([]storedApplication, error) {
	rows, err := tx.QueryContext(ctx, `SELECT a.id, a.settlement_line_no, a.item_id,
		a.payment_currency, a.payment_amount, a.item_currency, a.item_amount, a.base_amount,
		a.settlement_voucher_revision
		FROM athena_settlement_applications a
		WHERE a.settlement_voucher_type = ? AND a.settlement_voucher_id = ? AND a.event = ?
		AND NOT EXISTS (SELECT 1 FROM athena_settlement_applications m WHERE m.reverses_application_id = a.id)
		ORDER BY a.id`, req.VoucherType, req.VoucherID, EventSubmit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []storedApplication
	for rows.Next() {
		var application storedApplication
		var payment, item, base string
		if err := rows.Scan(&application.ApplicationID, &application.LineNo, &application.ItemID,
			&application.PaymentCurrency, &payment, &application.ItemCurrency, &item, &base,
			&application.Revision); err != nil {
			return nil, err
		}
		for target, text := range map[*types.Decimal]string{
			&application.PaymentAmount: payment, &application.ItemAmount: item, &application.BaseAmount: base,
		} {
			amount, err := types.ParseDecimal(text)
			if err != nil {
				return nil, fmt.Errorf("stored amount %q is not a decimal: %w", text, err)
			}
			*target = amount
		}
		out = append(out, application)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 0 {
		return out, nil
	}
	var submitCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_settlement_applications
		WHERE settlement_voucher_type = ? AND settlement_voucher_id = ? AND event = ?`,
		req.VoucherType, req.VoucherID, EventSubmit).Scan(&submitCount); err != nil {
		return nil, err
	}
	if submitCount > 0 {
		return nil, fmt.Errorf("%w: every line of this settlement already has its mirror", ErrAlreadyReversed)
	}
	return nil, fmt.Errorf("%w: this settlement has no applications to reverse", ErrInvalid)
}

// mirrorPartyRow writes the cancel mirror of one application party row: the
// base and transaction amounts swap sides, the row flags its reversal link,
// and the voucher revision and date move to the cancel.
func mirrorPartyRow(ctx context.Context, tx *sql.Tx, req ReverseRequest, batch, applicationID, mirrorID string) error {
	var row struct {
		id          string
		company     string
		book        string
		partyType   string
		partyID     string
		control     string
		itemID      sql.NullString
		debitBase   string
		creditBase  string
		txnCurrency sql.NullString
		txnDebit    string
		txnCredit   string
	}
	err := tx.QueryRowContext(ctx, `SELECT id, company_name, book, party_type, party_id, control_account_name,
		item_id, debit_base, credit_base, txn_currency, txn_debit, txn_credit
		FROM l_party WHERE application_id = ?`, applicationID).Scan(
		&row.id, &row.company, &row.book, &row.partyType, &row.partyID, &row.control,
		&row.itemID, &row.debitBase, &row.creditBase, &row.txnCurrency, &row.txnDebit, &row.txnCredit)
	if err != nil {
		return err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("new mirror row id: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO l_party
		(id, company_name, book, party_type, party_id, control_account_name,
		item_id, application_id, posting_date, debit_base, credit_base, txn_currency,
		txn_debit, txn_credit, voucher_type, voucher_id, voucher_revision, event,
		posting_group_id, is_reversal, reversed_row_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), row.company, row.book, row.partyType, row.partyID, row.control,
		row.itemID, mirrorID, req.PostingDate, row.creditBase, row.debitBase, row.txnCurrency,
		row.txnCredit, row.txnDebit, req.VoucherType, req.VoucherID, req.Revision, EventCancel,
		batch, true, row.id)
	return err
}

// mirrorUnappliedCash writes cancel mirrors for the voucher's unapplied cash
// rows without mirrors yet: cancelled cash leaves the party with the
// payment, so the subledger must not keep showing it held.
func mirrorUnappliedCash(ctx context.Context, tx *sql.Tx, req ReverseRequest, batch string) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, company_name, book, party_type, party_id, control_account_name,
		debit_base, credit_base, txn_currency, txn_debit, txn_credit
		FROM l_party p
		WHERE p.voucher_type = ? AND p.voucher_id = ? AND p.event = ? AND p.item_id IS NULL
		AND NOT EXISTS (SELECT 1 FROM l_party m WHERE m.reversed_row_id = p.id)
		ORDER BY p.id`, req.VoucherType, req.VoucherID, EventSubmit)
	if err != nil {
		return err
	}
	defer rows.Close()
	type cashRow struct {
		id          string
		company     string
		book        string
		partyType   string
		partyID     string
		control     string
		debitBase   string
		creditBase  string
		txnCurrency sql.NullString
		txnDebit    string
		txnCredit   string
	}
	var cash []cashRow
	for rows.Next() {
		var row cashRow
		if err := rows.Scan(&row.id, &row.company, &row.book, &row.partyType, &row.partyID,
			&row.control, &row.debitBase, &row.creditBase, &row.txnCurrency, &row.txnDebit, &row.txnCredit); err != nil {
			return err
		}
		cash = append(cash, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, row := range cash {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("new mirror row id: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO l_party
			(id, company_name, book, party_type, party_id, control_account_name,
			item_id, application_id, posting_date, debit_base, credit_base, txn_currency,
			txn_debit, txn_credit, voucher_type, voucher_id, voucher_revision, event,
			posting_group_id, is_reversal, reversed_row_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id.String(), row.company, row.book, row.partyType, row.partyID, row.control,
			nil, nil, req.PostingDate, row.creditBase, row.debitBase, row.txnCurrency,
			row.txnCredit, row.txnDebit, req.VoucherType, req.VoucherID, req.Revision, EventCancel,
			batch, true, row.id); err != nil {
			return err
		}
	}
	return nil
}

// reverseFX unwinds one realised-FX post per submit revision the reversal
// covers, through the engine: each fx_settlement submit group gains its
// cancel mirror in the reversal date's open period. Revisions that posted no
// FX (base-currency settlements) are skipped: there is nothing to unwind.
func (s *SettlementService) reverseFX(ctx context.Context, tx *sql.Tx, req ReverseRequest, originals []storedApplication) ([]string, error) {
	revisions := map[int64]bool{}
	for _, original := range originals {
		revisions[original.Revision] = true
	}
	ordered := make([]int64, 0, len(revisions))
	for revision := range revisions {
		ordered = append(ordered, revision)
	}
	sort.Slice(ordered, func(a, b int) bool { return ordered[a] < ordered[b] })
	engine := &Engine{DB: s.DB, Dialect: s.Dialect}
	var groups []string
	for _, revision := range ordered {
		var groupID string
		err := tx.QueryRowContext(ctx, `SELECT id FROM athena_posting_groups
			WHERE voucher_type = ? AND voucher_id = ? AND voucher_revision = ? AND event = ? AND book = ?`,
			FXVoucherType, req.VoucherID, revision, EventSubmit, req.Book).Scan(&groupID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result, err := engine.ReverseInTx(ctx, tx, FXVoucherType, req.VoucherID, revision, req.Book,
			ReverseOptions{Actor: req.Actor, Source: req.Source, RequestID: req.RequestID, PostingDate: req.PostingDate})
		if err != nil {
			return nil, err
		}
		groups = append(groups, result.PostingGroupID)
	}
	return groups, nil
}
