package ledger

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"athena-erp/internal/types"
)

// Currency is an ISO-style code with its minor-unit exponent: amounts store
// value * 10^exponent as an integer.
type Currency struct {
	Code     string
	Label    string
	Exponent int
	Disabled bool
	Revision int64
}

// ExchangeRate converts one unit of From to Rate units of To. Rates are
// immutable facts: a correction is a new row at a new instant, never an
// overwrite.
type ExchangeRate struct {
	From        string
	To          string
	RateType    string
	Source      string
	EffectiveAt time.Time
	Rate        types.Decimal
}

func (s *Service) SaveCurrency(ctx context.Context, currency Currency, expected int64) (Currency, error) {
	if _, err := types.NewCurrency(currency.Code, 0, types.HalfUp); err != nil {
		return Currency{}, inputErrorf("%v", err)
	}
	if err := checkLabel(currency.Label); err != nil {
		return Currency{}, err
	}
	if currency.Exponent < 0 || currency.Exponent > 12 {
		return Currency{}, inputErrorf("the minor exponent is 0 to 12")
	}
	if expected == 0 {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_currencies (code, label, minor_exponent, disabled) VALUES (?, ?, ?, ?)`,
			currency.Code, currency.Label, currency.Exponent, currency.Disabled)
		if err != nil {
			return Currency{}, s.conflict(err, "currency", currency.Code)
		}
		currency.Revision = 1
		return currency, nil
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_currencies SET label = ?, minor_exponent = ?, disabled = ?, revision = revision + 1 WHERE code = ? AND revision = ?`,
		currency.Label, currency.Exponent, currency.Disabled, currency.Code, expected)
	if err != nil {
		return Currency{}, err
	}
	return currency, s.cas(ctx, result, "athena_currencies", "code", currency.Code, expected, &currency.Revision)
}

func (s *Service) GetCurrency(ctx context.Context, code string) (Currency, error) {
	var currency Currency
	err := s.DB.QueryRowContext(ctx, `SELECT code, label, minor_exponent, disabled, revision FROM athena_currencies WHERE code = ?`, code).
		Scan(&currency.Code, &currency.Label, &currency.Exponent, &currency.Disabled, &currency.Revision)
	if err != nil {
		return Currency{}, notFound(err, "currency", code)
	}
	return currency, nil
}

func (s *Service) ListCurrencies(ctx context.Context) ([]Currency, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT code, label, minor_exponent, disabled, revision FROM athena_currencies ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Currency
	for rows.Next() {
		var currency Currency
		if err := rows.Scan(&currency.Code, &currency.Label, &currency.Exponent, &currency.Disabled, &currency.Revision); err != nil {
			return nil, err
		}
		out = append(out, currency)
	}
	return out, rows.Err()
}

// SaveRate records one rate. The key is the pair, rate type, source and
// effective instant; saving the same key twice is an error, never an update.
func (s *Service) SaveRate(ctx context.Context, rate ExchangeRate) (ExchangeRate, error) {
	if _, err := s.GetCurrency(ctx, rate.From); err != nil {
		return ExchangeRate{}, err
	}
	if _, err := s.GetCurrency(ctx, rate.To); err != nil {
		return ExchangeRate{}, err
	}
	if err := checkName("rate type", rate.RateType); err != nil {
		return ExchangeRate{}, err
	}
	if err := checkName("rate source", rate.Source); err != nil {
		return ExchangeRate{}, err
	}
	if rate.EffectiveAt.IsZero() {
		return ExchangeRate{}, inputErrorf("a rate needs its effective instant")
	}
	if rate.Rate.Sign() <= 0 {
		return ExchangeRate{}, inputErrorf("a rate must be above zero")
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_exchange_rates (from_code, to_code, rate_type, source, effective_at, rate) VALUES (?, ?, ?, ?, ?, ?)`,
		rate.From, rate.To, rate.RateType, rate.Source, rate.EffectiveAt.UTC(), rate.Rate.String())
	if err != nil {
		return ExchangeRate{}, s.conflict(err, "exchange rate", rate.From+"->"+rate.To)
	}
	return rate, nil
}

// GetRate resolves the rate in force at an instant: the latest row at or
// before it. Same codes convert at exactly one with no row. An empty source
// accepts every source, and fails naming each one when they disagree.
func (s *Service) GetRate(ctx context.Context, from, to, rateType, source string, at time.Time) (types.Decimal, error) {
	if from == to {
		one, err := types.ParseDecimal("1")
		if err != nil {
			return types.Decimal{}, err
		}
		return one, nil
	}
	if source != "" {
		return s.latestForSource(ctx, from, to, rateType, source, at)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT source FROM athena_exchange_rates WHERE from_code = ? AND to_code = ? AND rate_type = ? AND effective_at <= ?`,
		from, to, rateType, at.UTC())
	if err != nil {
		return types.Decimal{}, err
	}
	var sources []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return types.Decimal{}, err
		}
		sources = append(sources, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return types.Decimal{}, err
	}
	if len(sources) == 0 {
		return types.Decimal{}, fmt.Errorf("%w: no %s rate for %s->%s", ErrNotFound, rateType, from, to)
	}
	sort.Strings(sources)
	rates := make([]types.Decimal, len(sources))
	for i, name := range sources {
		rate, err := s.latestForSource(ctx, from, to, rateType, name, at)
		if err != nil {
			return types.Decimal{}, err
		}
		rates[i] = rate
	}
	for _, rate := range rates[1:] {
		if !rate.Equal(rates[0]) {
			parts := make([]string, len(sources))
			for i := range sources {
				parts[i] = fmt.Sprintf("%s=%s", sources[i], rates[i])
			}
			return types.Decimal{}, fmt.Errorf("%w: sources disagree for %s->%s: %s", ErrAmbiguous, from, to, strings.Join(parts, ", "))
		}
	}
	return rates[0], nil
}

func (s *Service) latestForSource(ctx context.Context, from, to, rateType, source string, at time.Time) (types.Decimal, error) {
	var text string
	err := s.DB.QueryRowContext(ctx, `SELECT rate FROM athena_exchange_rates WHERE from_code = ? AND to_code = ? AND rate_type = ? AND source = ? AND effective_at <= ? ORDER BY effective_at DESC LIMIT 1`,
		from, to, rateType, source, at.UTC()).Scan(&text)
	if err != nil {
		return types.Decimal{}, notFound(err, "exchange rate", from+"->"+to)
	}
	return types.ParseDecimal(text)
}
