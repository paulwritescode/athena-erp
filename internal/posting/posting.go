// Package posting is the posting engine of the kernel (batch-2 G6,
// ADR-0023, Proposed): it validates a Posting_Plan and writes it as balanced
// ledger rows exactly once. A Plan names one voucher revision, one event and
// one book; Post writes the ledger rows, balance deltas, audit event and
// outbox row in a single transaction, with one posting-group row per voucher
// revision, event and book and one operation row per attempt, so a voucher
// posts once and an operation id replays its first result.
//
// Two policies stay open for the sponsor (Z2): the backdating/correction
// window (O-011) and the book policy (O-012). Anywhere posting needs them,
// the engine returns ErrBookPolicyOpen or ErrCorrectionWindowOpen, which the
// server maps to failed_precondition; it never invents a window or a default
// book.
package posting

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"athena-erp/internal/events"
	"athena-erp/internal/ledger"
	"athena-erp/internal/store"
	"athena-erp/internal/types"
)

// Events the engine stores on ledger rows.
const (
	// EventSubmit posts a voucher's effect.
	EventSubmit = "submit"
	// EventCancel mirrors a posted voucher (6.6).
	EventCancel = "cancel"
)

// OutboxEventType is the event type of the row Post writes to athena_outbox.
// The event bus (G8) owns the envelope and may extend the payload.
const OutboxEventType = events.TypePostingPosted

// OutboxEventTypeReversed is the event type of the row Reverse writes. Its
// payload carries reversal_of_group_id beside the standard fields.
const OutboxEventTypeReversed = "posting.reversed"

// AuditAction is the Audit Log action of the row Post writes. The lifecycle
// transition writes its own submitted/cancelled row beside it.
const AuditAction = "posted"

// AuditActionReverse is the Audit Log action of the row Reverse writes,
// with a reversal_of change naming the unwound submit group.
const AuditActionReverse = "reversed"

var (
	// ErrInvalid reports a plan that fails mechanical validation: unbalanced
	// totals, a group or inactive account, a missing or soft-closed period, a
	// dimension breach, or a control account without its source.
	ErrInvalid = errors.New("posting: invalid plan")
	// ErrBookPolicyOpen reports a plan that needs the undecided book policy
	// (Z2, O-012): the engine will not default or invent a book.
	ErrBookPolicyOpen = errors.New("posting: book policy is not decided yet")
	// ErrCorrectionWindowOpen reports a posting date in a hard-closed period:
	// the correction window (Z2, O-011) that would name an allowed period is
	// undecided, so the engine refuses rather than guessing one.
	ErrCorrectionWindowOpen = errors.New("posting: correction window is not decided yet")
	// ErrConflict reports an operation id reused with another payload. The
	// ledgers keep the first payload; the caller must send a fresh id.
	ErrConflict = errors.New("posting: operation id was reused with a different payload")
)

// Line is one leg of a plan. Exactly one of DebitBase and CreditBase is
// above zero; the side gives the direction, so amounts never go negative.
// TxnCurrency with its amounts is the original currency when it differs from
// the company's base; empty means base only. Dimensions maps a defined
// dimension to one of its members. SourceType and SourceID link the leg to
// the source both endpoints of a control-account posting share, such as a
// party item; the existence check arrives with the party ledger (G7).
type Line struct {
	Company     string
	Account     string
	DebitBase   types.Decimal
	CreditBase  types.Decimal
	TxnCurrency string
	TxnDebit    types.Decimal
	TxnCredit   types.Decimal
	Quantity    *types.Decimal
	QuantityUOM string
	Dimensions  map[string]string
	SourceType  string
	SourceID    string
}

// Plan is one voucher revision's posting for one event and one book. Every
// line posts on PostingDate; the period covering it must be open in each
// (company, book) the lines name.
type Plan struct {
	VoucherType string
	VoucherID   string
	Revision    int64
	Event       string
	Book        string
	PostingDate string // YYYY-MM-DD
	Lines       []Line
}

// PostOptions carries the audit identity of a post. OperationID is the
// caller's idempotency key: repeating a post with the same id and payload
// replays the first result, while another payload under the id conflicts.
// Empty lets the engine generate one, which posts without replay cover.
type PostOptions struct {
	Actor       string
	Source      string // api, cli, rule, import or job
	RequestID   string
	OperationID string
}

// Result describes a committed post. Replayed is true when the result came
// from an earlier post instead of the ledgers being written again.
type Result struct {
	PostingGroupID string   `json:"posting_group_id"`
	RowIDs         []string `json:"row_ids"`
	PlanHash       string   `json:"plan_hash"`
	OperationID    string   `json:"operation_id"`
	Replayed       bool     `json:"replayed"`
}

// Engine posts plans. DB is the workspace database; Dialect classifies its
// errors and is required wherever a duplicate key decides the replay path,
// so always set it: a nil Dialect degrades collisions to raw errors.
// Workspace names the event envelope's Workspace; empty leaves it blank.
type Engine struct {
	DB        *sql.DB
	Dialect   store.Dialect
	Workspace string
}

// ValidatePlan checks the mechanical rules of Requirement 6: the plan names
// its voucher, event, book and date; every line names an enabled company and
// a leaf, enabled, in-scope account; amounts fit the ledger columns with one
// side above zero; transaction amounts sit on the same side as base; each
// (company, book) has an open period covering the date; dimensions exist,
// hold members and cover every mandatory axis; control accounts carry their
// source; and base totals balance per company and book with transaction
// totals consistent per currency. Q is usually the posting transaction, so
// validation sees the same rows the writer writes.
func ValidatePlan(ctx context.Context, q ledger.DB, plan Plan) error {
	if plan.VoucherType == "" || plan.VoucherID == "" {
		return fmt.Errorf("%w: the plan must name its voucher type and id", ErrInvalid)
	}
	if plan.Revision < 1 {
		return fmt.Errorf("%w: voucher revision %d is below 1", ErrInvalid, plan.Revision)
	}
	if plan.Event == "" {
		return fmt.Errorf("%w: the plan must name its event", ErrInvalid)
	}
	if plan.Book == "" {
		return fmt.Errorf("%w: a plan must name its book", ErrBookPolicyOpen)
	}
	if !validDate(plan.PostingDate) {
		return fmt.Errorf("%w: %q is not a date as YYYY-MM-DD", ErrInvalid, plan.PostingDate)
	}
	if len(plan.Lines) == 0 {
		return fmt.Errorf("%w: the plan has no lines", ErrInvalid)
	}
	masters := &ledger.Service{DB: q}
	dimensions, err := masters.ListDimensions(ctx)
	if err != nil {
		return err
	}
	byName := make(map[string]ledger.Dimension, len(dimensions))
	for _, dimension := range dimensions {
		byName[dimension.Name] = dimension
	}
	companies := map[string]ledger.Company{}
	accounts := map[string]ledger.Account{}
	for i := range plan.Lines {
		line := &plan.Lines[i]
		company, err := lookupCompany(ctx, masters, companies, line.Company)
		if err != nil {
			return err
		}
		if err := checkBaseCurrency(ctx, masters, company); err != nil {
			return err
		}
		account, err := lookupAccount(ctx, masters, accounts, line.Account)
		if err != nil {
			return err
		}
		if err := checkAccountScope(company, account); err != nil {
			return err
		}
		if err := checkAmounts(line); err != nil {
			return err
		}
		if err := checkTxn(ctx, masters, line); err != nil {
			return err
		}
		if err := checkQuantity(line); err != nil {
			return err
		}
		if err := checkDimensions(byName, plan.PostingDate, line); err != nil {
			return err
		}
		if err := checkControl(account, line); err != nil {
			return err
		}
	}
	if err := checkPeriods(ctx, masters, plan); err != nil {
		return err
	}
	return checkBalance(plan)
}

func lookupCompany(ctx context.Context, masters *ledger.Service, cache map[string]ledger.Company, name string) (ledger.Company, error) {
	if company, seen := cache[name]; seen {
		return company, nil
	}
	company, err := masters.GetCompany(ctx, name)
	if err != nil {
		if errors.Is(err, ledger.ErrNotFound) {
			return ledger.Company{}, fmt.Errorf("%w: company %q does not exist", ErrInvalid, name)
		}
		return ledger.Company{}, err
	}
	if company.Disabled {
		return ledger.Company{}, fmt.Errorf("%w: company %q is disabled", ErrInvalid, name)
	}
	cache[name] = company
	return company, nil
}

func checkBaseCurrency(ctx context.Context, masters *ledger.Service, company ledger.Company) error {
	if company.BaseCurrency == "" {
		return fmt.Errorf("%w: company %q has no base currency", ErrInvalid, company.Name)
	}
	currency, err := masters.GetCurrency(ctx, company.BaseCurrency)
	if err != nil {
		if errors.Is(err, ledger.ErrNotFound) {
			return fmt.Errorf("%w: base currency %q does not exist", ErrInvalid, company.BaseCurrency)
		}
		return err
	}
	if currency.Disabled {
		return fmt.Errorf("%w: base currency %q is disabled", ErrInvalid, company.BaseCurrency)
	}
	return nil
}

func lookupAccount(ctx context.Context, masters *ledger.Service, cache map[string]ledger.Account, name string) (ledger.Account, error) {
	if account, seen := cache[name]; seen {
		return account, nil
	}
	account, err := masters.GetAccount(ctx, name)
	if err != nil {
		if errors.Is(err, ledger.ErrNotFound) {
			return ledger.Account{}, fmt.Errorf("%w: account %q does not exist", ErrInvalid, name)
		}
		return ledger.Account{}, err
	}
	if account.IsGroup {
		return ledger.Account{}, fmt.Errorf("%w: account %q is a group and takes no postings", ErrInvalid, name)
	}
	if account.Disabled {
		return ledger.Account{}, fmt.Errorf("%w: account %q is disabled", ErrInvalid, name)
	}
	cache[name] = account
	return account, nil
}

func checkAccountScope(company ledger.Company, account ledger.Account) error {
	if account.Company != "" && account.Company != company.Name {
		return fmt.Errorf("%w: account %q belongs to company %q, not %q", ErrInvalid, account.Name, account.Company, company.Name)
	}
	return nil
}

// fitsLedger reports whether an amount fits DECIMAL(28,6): at most 6 fraction
// digits and 28 significant digits in total.
func fitsLedger(amount types.Decimal) bool {
	if _, err := amount.Rescale(6); err != nil {
		return false
	}
	digits := amount.String()
	digits = strings.TrimPrefix(digits, "-")
	digits = strings.TrimPrefix(digits, "+")
	digits = strings.Replace(digits, ".", "", 1)
	digits = strings.TrimLeft(digits, "0")
	return len(digits) <= 28
}

func checkAmounts(line *Line) error {
	if line.DebitBase.Sign() < 0 || line.CreditBase.Sign() < 0 {
		return fmt.Errorf("%w: line to %q carries a negative base amount", ErrInvalid, line.Account)
	}
	if !fitsLedger(line.DebitBase) || !fitsLedger(line.CreditBase) {
		return fmt.Errorf("%w: line to %q exceeds DECIMAL(28,6)", ErrInvalid, line.Account)
	}
	switch {
	case line.DebitBase.Sign() > 0 && line.CreditBase.Sign() > 0:
		return fmt.Errorf("%w: line to %q is both debit and credit; split it into two lines", ErrInvalid, line.Account)
	case line.DebitBase.Sign() == 0 && line.CreditBase.Sign() == 0:
		return fmt.Errorf("%w: line to %q has no amount", ErrInvalid, line.Account)
	}
	return nil
}

func checkTxn(ctx context.Context, masters *ledger.Service, line *Line) error {
	if line.TxnCurrency == "" {
		if line.TxnDebit.Sign() != 0 || line.TxnCredit.Sign() != 0 {
			return fmt.Errorf("%w: line to %q has transaction amounts without a currency", ErrInvalid, line.Account)
		}
		return nil
	}
	currency, err := masters.GetCurrency(ctx, line.TxnCurrency)
	if err != nil {
		if errors.Is(err, ledger.ErrNotFound) {
			return fmt.Errorf("%w: currency %q does not exist", ErrInvalid, line.TxnCurrency)
		}
		return err
	}
	if currency.Disabled {
		return fmt.Errorf("%w: currency %q is disabled", ErrInvalid, line.TxnCurrency)
	}
	if line.TxnDebit.Sign() < 0 || line.TxnCredit.Sign() < 0 {
		return fmt.Errorf("%w: line to %q carries a negative transaction amount", ErrInvalid, line.Account)
	}
	if !fitsLedger(line.TxnDebit) || !fitsLedger(line.TxnCredit) {
		return fmt.Errorf("%w: line to %q exceeds DECIMAL(28,6) in transaction currency", ErrInvalid, line.Account)
	}
	debit := line.TxnDebit.Sign() > 0
	credit := line.TxnCredit.Sign() > 0
	if debit == credit {
		return fmt.Errorf("%w: line to %q in %s needs a transaction amount on one side", ErrInvalid, line.Account, line.TxnCurrency)
	}
	if debit != (line.DebitBase.Sign() > 0) {
		return fmt.Errorf("%w: line to %q mixes a base debit with a transaction credit or the reverse", ErrInvalid, line.Account)
	}
	return nil
}

func checkQuantity(line *Line) error {
	if line.Quantity == nil || line.Quantity.Sign() == 0 {
		if line.QuantityUOM != "" {
			return fmt.Errorf("%w: line to %q has a unit without a quantity", ErrInvalid, line.Account)
		}
		return nil
	}
	if line.Quantity.Sign() < 0 {
		return fmt.Errorf("%w: line to %q carries a negative quantity", ErrInvalid, line.Account)
	}
	if line.QuantityUOM == "" {
		return fmt.Errorf("%w: line to %q has a quantity without a unit", ErrInvalid, line.Account)
	}
	if !fitsLedger(*line.Quantity) {
		return fmt.Errorf("%w: line to %q exceeds DECIMAL(28,6) in quantity", ErrInvalid, line.Account)
	}
	return nil
}

func checkDimensions(byName map[string]ledger.Dimension, postingDate string, line *Line) error {
	for name, value := range line.Dimensions {
		dimension, known := byName[name]
		if !known {
			return fmt.Errorf("%w: dimension %q does not exist", ErrInvalid, name)
		}
		if dimension.Disabled {
			return fmt.Errorf("%w: dimension %q is disabled", ErrInvalid, name)
		}
		if !dimensionApplies(dimension, postingDate) {
			return fmt.Errorf("%w: dimension %q is not in force on %s", ErrInvalid, name, postingDate)
		}
		if value == "" {
			return fmt.Errorf("%w: dimension %q has an empty value", ErrInvalid, name)
		}
		if len(dimension.Members) > 0 && !contains(dimension.Members, value) {
			return fmt.Errorf("%w: %q is not a member of dimension %q", ErrInvalid, value, name)
		}
	}
	for _, dimension := range byName {
		if dimension.Disabled || !dimensionApplies(dimension, postingDate) {
			continue
		}
		if contains(dimension.MandatoryFor, line.Account) {
			if _, present := line.Dimensions[dimension.Name]; !present {
				return fmt.Errorf("%w: account %q needs dimension %q", ErrInvalid, line.Account, dimension.Name)
			}
		}
	}
	return nil
}

func dimensionApplies(dimension ledger.Dimension, postingDate string) bool {
	if dimension.ValidFrom != "" && postingDate < dimension.ValidFrom {
		return false
	}
	if dimension.ValidTo != "" && postingDate > dimension.ValidTo {
		return false
	}
	return true
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func checkControl(account ledger.Account, line *Line) error {
	if account.ControlRole == "" {
		return nil
	}
	if line.SourceType == "" || line.SourceID == "" {
		return fmt.Errorf("%w: a manual journal to control account %q lacks the matching party item", ErrInvalid, account.Name)
	}
	return nil
}

// checkPeriods resolves the period covering the posting date in every
// (company, book) the plan touches. A soft close restricts normal posting;
// a hard close needs a correction in an allowed period, which the undecided
// correction window cannot name (Z2).
func checkPeriods(ctx context.Context, masters *ledger.Service, plan Plan) error {
	seen := map[string]bool{}
	for i := range plan.Lines {
		company := plan.Lines[i].Company
		if seen[company] {
			continue
		}
		seen[company] = true
		period, err := masters.PeriodFor(ctx, company, plan.Book, plan.PostingDate)
		if err != nil {
			if errors.Is(err, ledger.ErrNotFound) {
				return fmt.Errorf("%w: no period covers %s for company %q in book %q", ErrInvalid, plan.PostingDate, company, plan.Book)
			}
			return err
		}
		switch period.Status {
		case ledger.PeriodOpen:
		case ledger.PeriodSoftClosed:
			return fmt.Errorf("%w: period %q is soft-closed and takes no normal posting", ErrInvalid, period.Name)
		case ledger.PeriodHardClosed:
			return fmt.Errorf("%w: period %q is hard-closed", ErrCorrectionWindowOpen, period.Name)
		default:
			return fmt.Errorf("%w: period %q has status %q", ErrInvalid, period.Name, period.Status)
		}
	}
	return nil
}

// totals groups signed base amounts by company and book, and signed
// transaction amounts by company, book and currency.
type totals struct {
	base map[totalsKey]types.Decimal
	txn  map[txnKey]types.Decimal
}

type totalsKey struct {
	company string
	book    string
}

type txnKey struct {
	company  string
	book     string
	currency string
}

func sumPlan(plan Plan) totals {
	// The zero Decimal is a valid additive identity, so missing keys start
	// at zero with no special case.
	out := totals{base: map[totalsKey]types.Decimal{}, txn: map[txnKey]types.Decimal{}}
	for i := range plan.Lines {
		line := &plan.Lines[i]
		baseKey := totalsKey{line.Company, plan.Book}
		out.base[baseKey] = out.base[baseKey].Add(line.DebitBase).Sub(line.CreditBase)
		if line.TxnCurrency == "" {
			continue
		}
		txnKey := txnKey{line.Company, plan.Book, line.TxnCurrency}
		out.txn[txnKey] = out.txn[txnKey].Add(line.TxnDebit).Sub(line.TxnCredit)
	}
	return out
}

func checkBalance(plan Plan) error {
	sums := sumPlan(plan)
	for key, net := range sums.base {
		if net.Sign() != 0 {
			return fmt.Errorf("%w: company %q in book %q nets to %s", ErrInvalid, key.company, key.book, net.String())
		}
	}
	for key, net := range sums.txn {
		if net.Sign() != 0 {
			return fmt.Errorf("%w: company %q in book %q nets to %s in %s", ErrInvalid, key.company, key.book, net.String(), key.currency)
		}
	}
	return nil
}

func validDate(value string) bool {
	if len(value) != 10 || value[4] != '-' || value[7] != '-' {
		return false
	}
	for i, c := range value {
		if i == 4 || i == 7 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return value[5:7] >= "01" && value[5:7] <= "12" && value[8:10] >= "01" && value[8:10] <= "31"
}
