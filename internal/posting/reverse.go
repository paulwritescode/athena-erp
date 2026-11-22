package posting

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"athena-erp/internal/ledger"
	"athena-erp/internal/types"
)

// ReverseOptions carries the audit identity of a reversal with its posting
// date. PostingDate defaults to the submit group's date; naming another
// date books the correction in that date's period.
type ReverseOptions struct {
	Actor       string
	Source      string // api, cli, rule, import or job
	RequestID   string
	OperationID string
	PostingDate string // YYYY-MM-DD, empty for the original date
}

func (o ReverseOptions) postOptions() PostOptions {
	return PostOptions{Actor: o.Actor, Source: o.Source, RequestID: o.RequestID, OperationID: o.OperationID}
}

// Reverse unwinds one posted submit group: it mirrors the stored rows with
// swapped sides under the cancel event, so submit followed by cancel nets
// every ledger to zero. Anything failing rolls everything back and writes
// nothing. Callers inside a larger unit of work, such as the cancel path,
// use ReverseInTx instead.
//
// The mirror inherits the submit's validity: company status, accounts,
// dimensions and control sources are not rechecked, because an unwind must
// succeed even when a master changed since the post. Only the reversal
// date's period is checked, and it must be open: a hard-closed period needs
// a correction in an allowed period, which the undecided correction window
// (Z2, O-011) cannot name, so the attempt fails naming it instead of
// guessing one.
func (e *Engine) Reverse(ctx context.Context, voucherType, voucherID string, revision int64, book string, opts ReverseOptions) (Result, error) {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()
	result, err := e.ReverseInTx(ctx, tx, voucherType, voucherID, revision, book, opts)
	if err != nil {
		return Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return Result{}, err
	}
	return result, nil
}

// ReverseInTx is Reverse inside the caller's transaction.
func (e *Engine) ReverseInTx(ctx context.Context, tx *sql.Tx, voucherType, voucherID string, revision int64, book string, opts ReverseOptions) (Result, error) {
	shell := Plan{VoucherType: voucherType, VoucherID: voucherID, Revision: revision, Event: EventCancel, Book: book}
	operationID, err := effectiveOperationID(opts.OperationID)
	if err != nil {
		return Result{}, err
	}
	if err := checkIdentity(opts.Actor, opts.Source, opts.OperationID); err != nil {
		e.logFailure(ctx, shell, operationID, opts.postOptions(), OutcomeRejected, CodeInvalid, "", err)
		return Result{}, err
	}
	stored, submitGroup, err := loadSubmitGroup(ctx, tx, voucherType, voucherID, revision, book)
	if err != nil {
		e.logFailure(ctx, shell, operationID, opts.postOptions(), OutcomeRejected, CodeInvalid, "", err)
		return Result{}, err
	}
	date := opts.PostingDate
	if date == "" {
		date = stored[0].postingDate
	}
	if !validDate(date) {
		err := fmt.Errorf("%w: %q is not a date as YYYY-MM-DD", ErrInvalid, date)
		e.logFailure(ctx, shell, operationID, opts.postOptions(), OutcomeRejected, CodeInvalid, "", err)
		return Result{}, err
	}
	plan, reversed, err := mirrorPlan(voucherType, voucherID, revision, book, date, stored)
	if err != nil {
		e.logFailure(ctx, shell, operationID, opts.postOptions(), OutcomeError, CodeInternal, "", err)
		return Result{}, err
	}
	hash, err := hashPlan(plan)
	if err != nil {
		e.logFailure(ctx, shell, operationID, opts.postOptions(), OutcomeError, CodeInternal, "", err)
		return Result{}, err
	}
	if opts.OperationID != "" {
		if result, done, err := e.checkReplay(ctx, tx, plan, opts.postOptions(), operationID, hash); done || err != nil {
			return result, err
		}
	}
	periods, err := reversalPeriods(ctx, tx, stored, book, date)
	if err != nil {
		outcome, code := codeFor(err)
		e.logFailure(ctx, shell, operationID, opts.postOptions(), outcome, code, hash, err)
		return Result{}, err
	}
	return e.commitPost(ctx, tx, plan, opts.postOptions(), operationID, hash, func(result Result) error {
		return e.writeReversal(ctx, tx, plan, opts.postOptions(), result, rowFlags{periods: periods, reversed: reversed}, submitGroup)
	})
}

// storedRow is one l_gl row of the submit group being unwound.
type storedRow struct {
	id          string
	company     string
	account     string
	postingDate string
	debitBase   types.Decimal
	creditBase  types.Decimal
	txnCurrency string
	txnDebit    types.Decimal
	txnCredit   types.Decimal
	quantity    *types.Decimal
	quantityUOM string
	dimensions  map[string]string
}

// loadSubmitGroup reads the submit group and its rows in stable id order.
// The group carries the voucher's posted truth; reversing anything else is
// refused.
func loadSubmitGroup(ctx context.Context, tx *sql.Tx, voucherType, voucherID string, revision int64, book string) ([]storedRow, string, error) {
	var groupID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM athena_posting_groups WHERE voucher_type = ? AND voucher_id = ? AND voucher_revision = ? AND event = ? AND book = ?`,
		voucherType, voucherID, revision, EventSubmit, book).Scan(&groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", fmt.Errorf("%w: voucher %q %s revision %d in book %q has no posted submit group to reverse", ErrInvalid, voucherType, voucherID, revision, book)
	}
	if err != nil {
		return nil, "", err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, company_name, account_name, posting_date, debit_base, credit_base, txn_currency, txn_debit, txn_credit, quantity, quantity_uom, dimensions FROM l_gl WHERE posting_group_id = ? ORDER BY id`, groupID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var stored []storedRow
	for rows.Next() {
		var row storedRow
		var date time.Time
		var debit, credit, txnDebit, txnCredit string
		var currency, quantity, uom sql.NullString
		var dimensions []byte
		if err := rows.Scan(&row.id, &row.company, &row.account, &date, &debit, &credit, &currency, &txnDebit, &txnCredit, &quantity, &uom, &dimensions); err != nil {
			return nil, "", err
		}
		row.postingDate = date.Format("2006-01-02")
		for target, text := range map[*types.Decimal]string{&row.debitBase: debit, &row.creditBase: credit, &row.txnDebit: txnDebit, &row.txnCredit: txnCredit} {
			amount, err := types.ParseDecimal(text)
			if err != nil {
				return nil, "", fmt.Errorf("stored amount %q is not a decimal: %w", text, err)
			}
			*target = amount
		}
		row.txnCurrency = currency.String
		if quantity.Valid {
			amount, err := types.ParseDecimal(quantity.String)
			if err != nil {
				return nil, "", fmt.Errorf("stored quantity %q is not a decimal: %w", quantity.String, err)
			}
			row.quantity = &amount
		}
		row.quantityUOM = uom.String
		row.dimensions = map[string]string{}
		if err := json.Unmarshal(dimensions, &row.dimensions); err != nil {
			return nil, "", fmt.Errorf("stored dimensions are not an object: %w", err)
		}
		stored = append(stored, row)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(stored) == 0 {
		return nil, "", fmt.Errorf("posted group %q holds no rows", groupID)
	}
	return stored, groupID, nil
}

// reversalPeriods resolves the open period covering the reversal date in
// every company the submit touched. A soft close restricts normal posting;
// a hard close needs a correction in an allowed period, which the undecided
// correction window cannot name (Z2).
func reversalPeriods(ctx context.Context, tx *sql.Tx, stored []storedRow, book, date string) (map[string]string, error) {
	masters := &ledger.Service{DB: tx}
	periods := map[string]string{}
	for _, row := range stored {
		if _, seen := periods[row.company]; seen {
			continue
		}
		period, err := masters.PeriodFor(ctx, row.company, book, date)
		if err != nil {
			if errors.Is(err, ledger.ErrNotFound) {
				return nil, fmt.Errorf("%w: no period covers %s for company %q in book %q", ErrInvalid, date, row.company, book)
			}
			return nil, err
		}
		switch period.Status {
		case ledger.PeriodOpen:
		case ledger.PeriodSoftClosed:
			return nil, fmt.Errorf("%w: period %q is soft-closed and takes no normal posting", ErrInvalid, period.Name)
		case ledger.PeriodHardClosed:
			return nil, fmt.Errorf("%w: period %q is hard-closed", ErrCorrectionWindowOpen, period.Name)
		default:
			return nil, fmt.Errorf("%w: period %q has status %q", ErrInvalid, period.Name, period.Status)
		}
		periods[row.company] = period.Name
	}
	return periods, nil
}

// mirrorPlan builds the cancel plan from the stored rows: every amount and
// rate swaps sides, quantities, currencies, units and dimensions carry over
// untouched, and each line remembers the row it reverses.
func mirrorPlan(voucherType, voucherID string, revision int64, book, date string, stored []storedRow) (Plan, []string, error) {
	plan := Plan{
		VoucherType: voucherType, VoucherID: voucherID, Revision: revision,
		Event: EventCancel, Book: book, PostingDate: date,
		Lines: make([]Line, len(stored)),
	}
	reversed := make([]string, len(stored))
	for i := range stored {
		row := &stored[i]
		plan.Lines[i] = Line{
			Company: row.company, Account: row.account,
			DebitBase: row.creditBase, CreditBase: row.debitBase,
			TxnCurrency: row.txnCurrency,
			TxnDebit:    row.txnCredit, TxnCredit: row.txnDebit,
			Quantity: row.quantity, QuantityUOM: row.quantityUOM,
			Dimensions: row.dimensions,
		}
		reversed[i] = row.id
	}
	return plan, reversed, nil
}
