// Package numbering builds document names from series patterns and hands out
// the counter values behind them (spec 10.4, 24.3).
//
// A pattern mixes literal text with the tokens {yyyy}, {yy}, {mm} and one
// counter token of 1 to 9 "#" characters, for example SINV-{yyyy}-{#####}.
// Each distinct value of the pattern with its date tokens filled in (SINV-2026-)
// is its own counter, so a series restarts every year when the pattern holds
// {yyyy}. Company and branch scopes arrive with the company master.
package numbering

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"athena-erp/internal/store"
)

// MaxPatternLength bounds a pattern, so a generated name fits the name column.
const MaxPatternLength = 100

// Pattern is a parsed series pattern.
type Pattern struct {
	parts []part
	width int
}

type part struct {
	kind    byte // 'l' literal, 'Y' year4, 'y' year2, 'm' month, '#' counter
	literal string
}

// Parse validates a pattern. It needs exactly one counter token.
func Parse(pattern string) (Pattern, error) {
	if pattern == "" || len(pattern) > MaxPatternLength {
		return Pattern{}, fmt.Errorf("a series pattern has 1 to %d characters", MaxPatternLength)
	}
	var p Pattern
	rest := pattern
	counters := 0
	for rest != "" {
		open := strings.IndexAny(rest, "{}")
		if open < 0 {
			p.parts = append(p.parts, part{kind: 'l', literal: rest})
			break
		}
		if rest[open] == '}' {
			return Pattern{}, errors.New(`a series pattern has an unmatched "}"`)
		}
		if open > 0 {
			p.parts = append(p.parts, part{kind: 'l', literal: rest[:open]})
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			return Pattern{}, errors.New(`a series pattern has an unmatched "{"`)
		}
		token := rest[open+1 : open+end]
		rest = rest[open+end+1:]
		switch {
		case token == "yyyy":
			p.parts = append(p.parts, part{kind: 'Y'})
		case token == "yy":
			p.parts = append(p.parts, part{kind: 'y'})
		case token == "mm":
			p.parts = append(p.parts, part{kind: 'm'})
		case token != "" && strings.Trim(token, "#") == "" && len(token) <= 9:
			counters++
			p.width = len(token)
			p.parts = append(p.parts, part{kind: '#'})
		default:
			return Pattern{}, fmt.Errorf("{%s} is not a series token; use {yyyy}, {yy}, {mm} or {#####}", token)
		}
	}
	if counters != 1 {
		return Pattern{}, errors.New("a series pattern needs exactly one counter token such as {#####}")
	}
	return p, nil
}

// Scope is the pattern with its date tokens filled in and the counter left
// open. Documents numbered in the same scope share one counter.
func (p Pattern) Scope(at time.Time) string { return p.render(at, 0, false) }

// Format returns the name for counter value n at the given time. The counter
// is zero-padded to the token width and grows past it.
func (p Pattern) Format(at time.Time, n int64) string { return p.render(at, n, true) }

func (p Pattern) render(at time.Time, n int64, withCounter bool) string {
	at = at.UTC()
	var b strings.Builder
	for _, x := range p.parts {
		switch x.kind {
		case 'l':
			b.WriteString(x.literal)
		case 'Y':
			fmt.Fprintf(&b, "%04d", at.Year())
		case 'y':
			fmt.Fprintf(&b, "%02d", at.Year()%100)
		case 'm':
			fmt.Fprintf(&b, "%02d", int(at.Month()))
		case '#':
			if withCounter {
				fmt.Fprintf(&b, "%0*d", p.width, n)
			} else {
				b.WriteString("{#}")
			}
		}
	}
	return b.String()
}

// Tx is the part of *sql.Tx the counter needs.
type Tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Ensure creates the counter row of a scope if it is missing, in its own
// statement. Calling it before a gap-free Next keeps two first-time callers
// from racing to insert the same row inside their transactions.
func Ensure(ctx context.Context, db *sql.DB, d store.Dialect, recordType, scope string) error {
	_, err := db.ExecContext(ctx, d.Upsert("athena_number_counters", []string{"counter_key", "record_type_name", "last_number"}, "counter_key", nil), recordType+":"+scope, recordType, 0)
	return err
}

// Next increments the counter of one scope and returns the new value. It
// runs in tx and holds the counter row lock until tx ends, so a rollback
// gives the number back: use it for gap-free series.
func Next(ctx context.Context, tx Tx, d store.Dialect, recordType, scope string) (int64, error) {
	key := recordType + ":" + scope
	if _, err := tx.ExecContext(ctx, d.Upsert("athena_number_counters", []string{"counter_key", "record_type_name", "last_number"}, "counter_key", nil), key, recordType, 0); err != nil {
		return 0, fmt.Errorf("create counter: %w", err)
	}
	var last int64
	if err := tx.QueryRowContext(ctx, "SELECT `last_number` FROM athena_number_counters WHERE `counter_key` = ? "+d.ForUpdate(), key).Scan(&last); err != nil {
		return 0, fmt.Errorf("lock counter: %w", err)
	}
	last++
	if _, err := tx.ExecContext(ctx, "UPDATE athena_number_counters SET `last_number` = ? WHERE `counter_key` = ?", last, key); err != nil {
		return 0, fmt.Errorf("advance counter: %w", err)
	}
	return last, nil
}

// NextIndependent increments the counter in its own short transaction. A
// number taken this way stays used if the caller's transaction rolls back, so
// the series may skip numbers but never makes writers wait on each other.
func NextIndependent(ctx context.Context, db *sql.DB, d store.Dialect, recordType, scope string) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	n, err := Next(ctx, tx, d, recordType, scope)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}
