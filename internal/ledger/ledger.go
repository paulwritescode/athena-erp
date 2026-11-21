// Package ledger owns the kernel's ledger masters: companies, branches,
// departments, cost centres, dimensions, fiscal years, periods, currencies,
// exchange rates, account types, accounts, determination mappings and
// Settings. The tables arrive in migration 13 with no rows: jurisdiction,
// chart and tax stay open for the sponsor (ADR-0022). Masters are validated
// here and written by LedgerService, never through RecordService.
package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"athena-erp/internal/store"
)

// DB is the database surface the ledger needs. *sql.DB, *sql.Conn and *sql.Tx
// all satisfy it, so the posting engine can reuse these helpers inside its
// own transaction. Connections must parse temporal columns into time.Time;
// every Athena open path does (utcConfig, testdb).
type DB interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// scanDate formats a DATE column as YYYY-MM-DD.
func scanDate(value time.Time) string { return value.Format("2006-01-02") }

func scanNullDate(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return scanDate(value.Time)
}

var (
	// ErrNotFound reports a master that does not exist.
	ErrNotFound = errors.New("ledger: not found")
	// ErrExists reports a master that already exists.
	ErrExists = errors.New("ledger: already exists")
	// ErrConflict reports a save against a stale revision.
	ErrConflict = errors.New("ledger: saved somewhere else; reload it and try again")
	// ErrAmbiguous reports a lookup that matches more than one row.
	ErrAmbiguous = errors.New("ledger: ambiguous match")
)

// InputError is invalid input: bad names, broken references, bad dates and
// refused transitions. Callers map it to an invalid-argument error; anything
// else from this package is a lookup miss above or an internal failure.
type InputError struct {
	Msg string
}

func (e *InputError) Error() string { return e.Msg }

func inputErrorf(format string, args ...any) error {
	return &InputError{Msg: "ledger: " + fmt.Sprintf(format, args...)}
}

// namePattern keeps master names safe as keys and quoted SQL identifiers.
// Leading digits are allowed for account codes, and hyphens for dated names
// such as 2026-01.
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,139}$`)

func checkName(what, name string) error {
	if !namePattern.MatchString(name) {
		return inputErrorf("%s %q must start with a lowercase letter or digit and use up to 140 lowercase letters, digits, underscores or hyphens", what, name)
	}
	return nil
}

func checkLabel(label string) error {
	if label == "" || len(label) > 255 {
		return inputErrorf("the label is required and must be at most 255 characters")
	}
	return nil
}

// Service reads and writes ledger masters. Dialect classifies database
// errors; DB is usually *sql.DB, or a transaction for multi-step work.
type Service struct {
	DB      DB
	Dialect store.Dialect
}

// conflict maps a duplicate key to ErrExists.
func (s *Service) conflict(err error, what, name string) error {
	if err == nil {
		return nil
	}
	if s.Dialect.Classify(err) == store.ErrDuplicateKey {
		return fmt.Errorf("%w: %s %q", ErrExists, what, name)
	}
	return err
}

// withTransaction joins a caller's transaction or owns one around the change.
func (s *Service) withTransaction(ctx context.Context, write func(*sql.Tx) error) error {
	if tx, ok := s.DB.(*sql.Tx); ok {
		return write(tx)
	}
	db, ok := s.DB.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return errors.New("ledger: database does not support transactions")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := write(tx); err != nil {
		return err
	}
	return tx.Commit()
}
