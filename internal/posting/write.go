package posting

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"athena-erp/internal/audit"
	"athena-erp/internal/ledger"
	"athena-erp/internal/types"

	"github.com/google/uuid"
)

// Post validates the plan and writes it in one transaction: the ledger rows,
// the balance deltas, the audit event and the outbox row. Anything failing
// rolls everything back and writes nothing. Callers inside a larger unit of
// work, such as the submit path, use PostInTx instead.
func (e *Engine) Post(ctx context.Context, plan Plan, opts PostOptions) (Result, error) {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()
	result, err := e.PostInTx(ctx, tx, plan, opts)
	if err != nil {
		return Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return Result{}, err
	}
	return result, nil
}

// PostInTx is Post inside the caller's transaction. It replays a known
// operation id before validating, so a retry returns the first result even
// when the plan would no longer validate; writes one posting group per
// voucher revision, event and book; and records the attempt in the posting
// log, through a separate connection when the attempt fails.
func (e *Engine) PostInTx(ctx context.Context, tx *sql.Tx, plan Plan, opts PostOptions) (Result, error) {
	operationID := opts.OperationID
	if operationID == "" {
		generated, err := uuid.NewV7()
		if err != nil {
			return Result{}, err
		}
		operationID = generated.String()
	}
	if err := checkOptions(opts); err != nil {
		e.logFailure(ctx, plan, operationID, opts, OutcomeRejected, CodeInvalid, "", err)
		return Result{}, err
	}
	hash, err := hashPlan(plan)
	if err != nil {
		e.logFailure(ctx, plan, operationID, opts, OutcomeError, CodeInternal, "", err)
		return Result{}, err
	}
	if opts.OperationID != "" {
		found, stored, err := lookupOperation(ctx, tx, opts.Actor, CommandPost, operationID)
		if err != nil {
			e.logFailure(ctx, plan, operationID, opts, OutcomeError, CodeInternal, hash, err)
			return Result{}, err
		}
		if found {
			if stored.PayloadHash != hash {
				e.logFailure(ctx, plan, operationID, opts, OutcomeConflict, CodeConflict, hash, ErrConflict)
				return Result{}, fmt.Errorf("%w: operation %q already posted another payload", ErrConflict, operationID)
			}
			stored.Result.Replayed = true
			if err := insertLogRow(ctx, tx, plan, operationID, opts, OutcomeReplayed, CodeOK, "", hash); err != nil {
				return Result{}, err
			}
			return stored.Result, nil
		}
	}
	if err := ValidatePlan(ctx, tx, plan); err != nil {
		outcome, code := codeFor(err)
		e.logFailure(ctx, plan, operationID, opts, outcome, code, hash, err)
		return Result{}, err
	}
	groupID, err := uuid.NewV7()
	if err != nil {
		e.logFailure(ctx, plan, operationID, opts, OutcomeError, CodeInternal, hash, err)
		return Result{}, err
	}
	if err := insertGroup(ctx, tx, groupID.String(), plan, operationID); err != nil {
		if e.isDuplicate(err) {
			return e.replayGroupResult(ctx, tx, plan, operationID, opts, hash)
		}
		e.logFailure(ctx, plan, operationID, opts, OutcomeError, CodeInternal, hash, err)
		return Result{}, err
	}
	rowIDs := make([]string, len(plan.Lines))
	for i := range plan.Lines {
		id, err := uuid.NewV7()
		if err != nil {
			e.logFailure(ctx, plan, operationID, opts, OutcomeError, CodeInternal, hash, err)
			return Result{}, err
		}
		rowIDs[i] = id.String()
	}
	result := Result{PostingGroupID: groupID.String(), RowIDs: rowIDs, PlanHash: hash, OperationID: operationID}
	if err := insertOperation(ctx, tx, opts.Actor, operationID, hash, result); err != nil {
		if e.isDuplicate(err) {
			return e.replayOperationResult(ctx, tx, plan, operationID, opts, hash)
		}
		e.logFailure(ctx, plan, operationID, opts, OutcomeError, CodeInternal, hash, err)
		return Result{}, err
	}
	if err := e.writePosting(ctx, tx, plan, opts, result); err != nil {
		e.logFailure(ctx, plan, operationID, opts, OutcomeError, CodeInternal, hash, err)
		return Result{}, err
	}
	if err := insertLogRow(ctx, tx, plan, operationID, opts, OutcomePosted, CodeOK, "", hash); err != nil {
		return Result{}, err
	}
	return result, nil
}

// writePosting writes the ledger rows, balance deltas, audit event and
// outbox row of one validated post.
func (e *Engine) writePosting(ctx context.Context, tx *sql.Tx, plan Plan, opts PostOptions, result Result) error {
	if err := insertRows(ctx, tx, result.PostingGroupID, result.RowIDs, plan); err != nil {
		return err
	}
	if err := e.applyBalances(ctx, tx, plan); err != nil {
		return err
	}
	if _, err := audit.Append(ctx, tx, e.Dialect, audit.Entry{
		RecordType: plan.VoucherType, RecordID: plan.VoucherID, Revision: plan.Revision,
		Action: AuditAction,
		Changes: []audit.Change{
			{Field: "ledger_rows", New: ptr(strconv.Itoa(len(plan.Lines)))},
			{Field: "posting_group", New: ptr(result.PostingGroupID)},
		},
		Actor: opts.Actor, Source: opts.Source, RequestID: opts.RequestID,
	}); err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	if err := insertOutbox(ctx, tx, result.PostingGroupID, result.PlanHash, plan); err != nil {
		return err
	}
	return nil
}

func ptr(value string) *string { return &value }

func checkOptions(opts PostOptions) error {
	if opts.Actor == "" {
		return fmt.Errorf("%w: a post needs its actor", ErrInvalid)
	}
	if opts.Source == "" {
		return fmt.Errorf("%w: a post needs its source", ErrInvalid)
	}
	if utf8.RuneCountInString(opts.OperationID) > 140 {
		return fmt.Errorf("%w: the operation id is longer than 140 characters", ErrInvalid)
	}
	return nil
}

func insertRows(ctx context.Context, tx *sql.Tx, groupID string, rowIDs []string, plan Plan) error {
	const columns = `id, company_name, book, account_name, period_name, posting_date, ` +
		`debit_base, credit_base, txn_currency, txn_debit, txn_credit, ` +
		`quantity, quantity_uom, dimensions, voucher_type, voucher_id, ` +
		`voucher_revision, event, posting_group_id, is_reversal, reversed_row_id`
	masters := &ledger.Service{DB: tx}
	periods := map[string]string{}
	var statement strings.Builder
	statement.WriteString("INSERT INTO l_gl (" + columns + ") VALUES ")
	args := make([]any, 0, len(plan.Lines)*21)
	for i := range plan.Lines {
		line := &plan.Lines[i]
		period, err := periodName(ctx, masters, periods, line.Company, plan.Book, plan.PostingDate)
		if err != nil {
			return err
		}
		dimensions, err := json.Marshal(nonNilMap(line.Dimensions))
		if err != nil {
			return err
		}
		if i > 0 {
			statement.WriteString(", ")
		}
		statement.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			rowIDs[i], line.Company, plan.Book, line.Account, period, plan.PostingDate,
			line.DebitBase.String(), line.CreditBase.String(),
			nullText(line.TxnCurrency), line.TxnDebit.String(), line.TxnCredit.String(),
			nullDecimal(line.Quantity), nullText(line.QuantityUOM), string(dimensions),
			plan.VoucherType, plan.VoucherID, plan.Revision, plan.Event, groupID, false, nil,
		)
	}
	_, err := tx.ExecContext(ctx, statement.String(), args...)
	return err
}

// periodName resolves the open period ValidatePlan already checked, keyed by
// company; validation runs first, so this lookup cannot miss.
func periodName(ctx context.Context, masters *ledger.Service, cache map[string]string, company, book, date string) (string, error) {
	if name, seen := cache[company]; seen {
		return name, nil
	}
	period, err := masters.PeriodFor(ctx, company, book, date)
	if err != nil {
		return "", err
	}
	cache[company] = period.Name
	return period.Name, nil
}

func nonNilMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	return values
}

func nullText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullDecimal(amount *types.Decimal) any {
	if amount == nil {
		return nil
	}
	return amount.String()
}

// balanceDelta is one account's signed movement in base currency.
type balanceDelta struct {
	company string
	book    string
	account string
	debit   string
	credit  string
}

// applyBalances adds every line to athena_balance_account. The update runs
// first so the common case holds the row lock; a missing row inserts, and a
// lost insert race falls back to the update. The statements use portable SQL
// with one placeholder style, like the ledger package.
func (e *Engine) applyBalances(ctx context.Context, tx *sql.Tx, plan Plan) error {
	deltas := map[balanceKey]*balanceDelta{}
	order := []balanceKey{}
	for i := range plan.Lines {
		line := &plan.Lines[i]
		key := balanceKey{line.Company, plan.Book, line.Account}
		delta, seen := deltas[key]
		if !seen {
			delta = &balanceDelta{company: line.Company, book: plan.Book, account: line.Account,
				debit: "0", credit: "0"}
			deltas[key] = delta
			order = append(order, key)
		}
		debit, err := addDecimalText(delta.debit, line.DebitBase.String())
		if err != nil {
			return err
		}
		credit, err := addDecimalText(delta.credit, line.CreditBase.String())
		if err != nil {
			return err
		}
		delta.debit, delta.credit = debit, credit
	}
	masters := &ledger.Service{DB: tx}
	companies := map[string]ledger.Company{}
	// One global lock order for every post: concurrent posts take the same
	// balance rows in the same order and never deadlock each other.
	sort.Slice(order, func(i, j int) bool {
		if order[i].company != order[j].company {
			return order[i].company < order[j].company
		}
		if order[i].book != order[j].book {
			return order[i].book < order[j].book
		}
		return order[i].account < order[j].account
	})
	for _, key := range order {
		delta := deltas[key]
		result, err := tx.ExecContext(ctx, `UPDATE athena_balance_account SET debit_total = debit_total + ?, credit_total = credit_total + ? WHERE company_name = ? AND book = ? AND account_name = ?`,
			delta.debit, delta.credit, delta.company, delta.book, delta.account)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected > 0 {
			continue
		}
		company, err := lookupBalanceCompany(ctx, masters, companies, delta.company)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO athena_balance_account (company_name, book, account_name, base_currency, debit_total, credit_total) VALUES (?, ?, ?, ?, ?, ?)`,
			delta.company, delta.book, delta.account, company.BaseCurrency, delta.debit, delta.credit)
		if err == nil {
			continue
		}
		if !e.isDuplicate(err) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE athena_balance_account SET debit_total = debit_total + ?, credit_total = credit_total + ? WHERE company_name = ? AND book = ? AND account_name = ?`,
			delta.debit, delta.credit, delta.company, delta.book, delta.account); err != nil {
			return err
		}
	}
	return nil
}

type balanceKey struct {
	company string
	book    string
	account string
}

// addDecimalText adds two exact decimal texts, for balance deltas summed
// outside the database.
func addDecimalText(left, right string) (string, error) {
	a, err := types.ParseDecimal(left)
	if err != nil {
		return "", err
	}
	b, err := types.ParseDecimal(right)
	if err != nil {
		return "", err
	}
	return a.Add(b).String(), nil
}

func lookupBalanceCompany(ctx context.Context, masters *ledger.Service, cache map[string]ledger.Company, name string) (ledger.Company, error) {
	if company, seen := cache[name]; seen {
		return company, nil
	}
	company, err := masters.GetCompany(ctx, name)
	if err != nil {
		return ledger.Company{}, err
	}
	cache[name] = company
	return company, nil
}

func insertOutbox(ctx context.Context, tx *sql.Tx, groupID, hash string, plan Plan) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"voucher_type":     plan.VoucherType,
		"voucher_id":       plan.VoucherID,
		"voucher_revision": plan.Revision,
		"event":            plan.Event,
		"book":             plan.Book,
		"posting_date":     plan.PostingDate,
		"posting_group_id": groupID,
		"row_count":        len(plan.Lines),
		"plan_hash":        hash,
	})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO athena_outbox (id, event_type, payload) VALUES (?, ?, ?)`,
		id.String(), OutboxEventType, string(payload))
	return err
}

// hashPlan is the SHA-256 of the plan's canonical JSON: lines in order with
// dimensions sorted by encoding/json, amounts in normalized decimal text so
// that 100.00 and 100 hash alike and never conflict with each other.
func hashPlan(plan Plan) (string, error) {
	type line struct {
		Company     string            `json:"company"`
		Account     string            `json:"account"`
		DebitBase   string            `json:"debit_base"`
		CreditBase  string            `json:"credit_base"`
		TxnCurrency string            `json:"txn_currency"`
		TxnDebit    string            `json:"txn_debit"`
		TxnCredit   string            `json:"txn_credit"`
		Quantity    string            `json:"quantity"`
		QuantityUOM string            `json:"quantity_uom"`
		Dimensions  map[string]string `json:"dimensions"`
		SourceType  string            `json:"source_type"`
		SourceID    string            `json:"source_id"`
	}
	type canonical struct {
		VoucherType string `json:"voucher_type"`
		VoucherID   string `json:"voucher_id"`
		Revision    int64  `json:"revision"`
		Event       string `json:"event"`
		Book        string `json:"book"`
		PostingDate string `json:"posting_date"`
		Lines       []line `json:"lines"`
	}
	out := canonical{
		VoucherType: plan.VoucherType, VoucherID: plan.VoucherID, Revision: plan.Revision,
		Event: plan.Event, Book: plan.Book, PostingDate: plan.PostingDate,
		Lines: make([]line, len(plan.Lines)),
	}
	for i := range plan.Lines {
		source := &plan.Lines[i]
		quantity := ""
		if source.Quantity != nil {
			quantity = canonicalDecimalText(source.Quantity.String())
		}
		out.Lines[i] = line{
			Company: source.Company, Account: source.Account,
			DebitBase:   canonicalDecimalText(source.DebitBase.String()),
			CreditBase:  canonicalDecimalText(source.CreditBase.String()),
			TxnCurrency: source.TxnCurrency,
			TxnDebit:    canonicalDecimalText(source.TxnDebit.String()),
			TxnCredit:   canonicalDecimalText(source.TxnCredit.String()),
			Quantity:    quantity, QuantityUOM: source.QuantityUOM,
			Dimensions: nonNilMap(source.Dimensions),
			SourceType: source.SourceType, SourceID: source.SourceID,
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// canonicalDecimalText normalizes a Decimal's text so equal values hash
// alike whatever scale they carry: trailing fraction zeros go, and a bare
// point with them, so 100.00 and 100 both read 100.
func canonicalDecimalText(text string) string {
	whole, frac, hasDot := strings.Cut(text, ".")
	if !hasDot {
		return text
	}
	frac = strings.TrimRight(frac, "0")
	if frac == "" {
		return whole
	}
	return whole + "." + frac
}
