package stock

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"athena-erp/internal/ledger"

	"github.com/google/uuid"
)

// StockToGLCheck names the integrity check comparing stock value to its
// general-ledger control account.
const StockToGLCheck = "stock_to_gl"

// AccountCheck compares one control account: Expected is the GL control
// balance posted on or before the cutoff, Observed is the stock ledger
// value (l_stock rows plus on-hand cost deltas; sold deltas post to cost
// of sales, not the control account). Match reports equality. LogID names
// the integrity-log row written for a break, or the reused identical open
// row; "" when the account matched.
type AccountCheck struct {
	Control    string
	Expected   string
	Observed   string
	Difference string
	Match      bool
	LogID      string
}

// CheckReport is one stock-to-GL check run over every control account the
// stock ledger names.
type CheckReport struct {
	Company  string
	Book     string
	Cutoff   string
	Accounts []AccountCheck
}

// Matched reports whether every control account matched.
func (r CheckReport) Matched() bool {
	for _, account := range r.Accounts {
		if !account.Match {
			return false
		}
	}
	return true
}

// CheckStockToGL compares, per control account named in the stock ledger,
// the stock value posted on or before the cutoff against the GL control
// balance at the same cutoff. Both sides cut on posting date: the books'
// date. Every break writes an integrity-log row with expected, observed
// and owner, unless an identical open row already exists; matches write
// nothing. It joins the caller's transaction or owns one.
func (s *Service) CheckStockToGL(ctx context.Context, db ledger.DB, company, book, cutoff, owner string) (CheckReport, error) {
	report := CheckReport{Company: company, Book: book, Cutoff: cutoff}
	if company == "" || book == "" || owner == "" {
		return report, fmt.Errorf("%w: company, book and owner are required", ErrInvalid)
	}
	if !validDate(cutoff) {
		return report, fmt.Errorf("%w: cutoff must be YYYY-MM-DD", ErrInvalid)
	}
	if tx, ok := db.(*sql.Tx); ok {
		return s.checkInTx(ctx, tx, report, owner)
	}
	beginner, ok := db.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return report, errors.New("stock: database does not support transactions")
	}
	tx, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return report, err
	}
	defer tx.Rollback()
	report, err = s.checkInTx(ctx, tx, report, owner)
	if err != nil {
		return report, err
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	return report, nil
}

func (s *Service) checkInTx(ctx context.Context, tx *sql.Tx, report CheckReport, owner string) (CheckReport, error) {
	masters := &ledger.Service{DB: tx, Dialect: s.Dialect}
	company, err := masters.GetCompany(ctx, report.Company)
	if err != nil {
		if errors.Is(err, ledger.ErrNotFound) {
			return report, fmt.Errorf("%w: company %q does not exist", ErrInvalid, report.Company)
		}
		return report, err
	}
	if company.BaseCurrency == "" {
		return report, fmt.Errorf("%w: company %q has no base currency", ErrInvalid, report.Company)
	}
	controls, err := stockControls(ctx, tx, report.Company, report.Book)
	if err != nil {
		return report, err
	}
	for _, control := range controls {
		check, err := s.checkAccount(ctx, tx, report, control, company.BaseCurrency, owner)
		if err != nil {
			return report, err
		}
		report.Accounts = append(report.Accounts, check)
	}
	return report, nil
}

// stockControls lists the control accounts the stock ledger names for a
// company and book.
func stockControls(ctx context.Context, tx *sql.Tx, company, book string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT control_account_name FROM l_stock WHERE company_name = ? AND book = ? ORDER BY control_account_name`,
		company, book)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var control string
		if err := rows.Scan(&control); err != nil {
			return nil, err
		}
		out = append(out, control)
	}
	return out, rows.Err()
}

func (s *Service) checkAccount(ctx context.Context, tx *sql.Tx, report CheckReport, control, currency, owner string) (AccountCheck, error) {
	check := AccountCheck{Control: control}
	var glBalance string
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(debit_base - credit_base), 0) FROM l_gl WHERE company_name = ? AND book = ? AND account_name = ? AND posting_date <= ?`,
		report.Company, report.Book, control, report.Cutoff).Scan(&glBalance)
	if err != nil {
		return check, err
	}
	expected, err := scanDecimal(glBalance)
	if err != nil {
		return check, err
	}
	var stockValue, stockRows string
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(value_base), 0), COUNT(*) FROM l_stock WHERE company_name = ? AND book = ? AND control_account_name = ? AND posting_date <= ?`,
		report.Company, report.Book, control, report.Cutoff).Scan(&stockValue, &stockRows)
	if err != nil {
		return check, err
	}
	stockTotal, err := scanDecimal(stockValue)
	if err != nil {
		return check, err
	}
	var onHand, sold, deltaRows string
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(on_hand_delta), 0), COALESCE(SUM(sold_delta), 0), COUNT(*) FROM l_stock_value WHERE company_name = ? AND book = ? AND control_account_name = ? AND posting_date <= ?`,
		report.Company, report.Book, control, report.Cutoff).Scan(&onHand, &sold, &deltaRows)
	if err != nil {
		return check, err
	}
	onHandTotal, err := scanDecimal(onHand)
	if err != nil {
		return check, err
	}
	observed := stockTotal.Add(onHandTotal)
	check.Expected = expected.String()
	check.Observed = observed.String()
	check.Difference = observed.Sub(expected).String()
	check.Match = observed.Equal(expected)
	if check.Match {
		return check, nil
	}
	var glRows string
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM l_gl WHERE company_name = ? AND book = ? AND account_name = ? AND posting_date <= ?`,
		report.Company, report.Book, control, report.Cutoff).Scan(&glRows); err != nil {
		return check, err
	}
	var openID, openExpected, openObserved string
	err = tx.QueryRowContext(ctx, `SELECT id, expected_value, observed_value FROM athena_integrity_log WHERE company_name = ? AND book = ? AND check_name = ? AND control_account_name = ? AND cutoff_date = ? AND status = 'open' ORDER BY created_at DESC LIMIT 1 FOR UPDATE`,
		report.Company, report.Book, StockToGLCheck, control, report.Cutoff).Scan(&openID, &openExpected, &openObserved)
	if err == nil {
		sameExpected, parseErr := scanDecimal(openExpected)
		if parseErr != nil {
			return check, parseErr
		}
		sameObserved, parseErr := scanDecimal(openObserved)
		if parseErr != nil {
			return check, parseErr
		}
		if sameExpected.Equal(expected) && sameObserved.Equal(observed) {
			check.LogID = openID
			return check, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return check, err
	}
	detail, err := json.Marshal(map[string]string{
		"difference":          check.Difference,
		"stock_row_count":     stockRows,
		"gl_row_count":        glRows,
		"delta_row_count":     deltaRows,
		"on_hand_delta":       onHand,
		"sold_delta_excluded": sold,
	})
	if err != nil {
		return check, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return check, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO athena_integrity_log (id, company_name, book, check_name, control_account_name, cutoff_date, expected_value, observed_value, base_currency, owner, status, detail) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', ?)`,
		id.String(), report.Company, report.Book, StockToGLCheck, control, report.Cutoff,
		expected.String(), observed.String(), currency, owner, string(detail)); err != nil {
		return check, err
	}
	check.LogID = id.String()
	return check, nil
}
