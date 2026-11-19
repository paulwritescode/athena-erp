package types

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
)

var (
	ErrCurrency         = errors.New("types: invalid currency")
	ErrCurrencyMismatch = errors.New("types: currency mismatch")
	ErrOverflow         = errors.New("types: amount overflows 64-bit minor units")
)

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

// MaxExponent is the largest minor-unit exponent a Currency may declare.
const MaxExponent = 9

// Currency declares how amounts in one currency are stored and rounded:
// Exponent is the number of minor-unit digits (2 for USD, 0 for JPY, 3 for
// KWD) and Mode is the rounding mode applied when an amount loses digits.
type Currency struct {
	Code     string
	Exponent int
	Mode     RoundingMode
}

// NewCurrency validates and returns a Currency. Code is three upper-case ASCII
// letters (ISO 4217 style).
func NewCurrency(code string, exponent int, mode RoundingMode) (Currency, error) {
	if !currencyCode.MatchString(code) || exponent < 0 || exponent > MaxExponent || !mode.Valid() {
		return Currency{}, ErrCurrency
	}
	return Currency{Code: code, Exponent: exponent, Mode: mode}, nil
}

// Money is an amount in integer minor units of one currency.
type Money struct {
	Minor    int64
	Currency Currency
}

// NewMoney returns minor units of cur.
func NewMoney(minor int64, cur Currency) Money { return Money{Minor: minor, Currency: cur} }

func (m Money) sameCurrency(o Money) error {
	if m.Currency.Code != o.Currency.Code {
		return fmt.Errorf("%w: %s and %s", ErrCurrencyMismatch, m.Currency.Code, o.Currency.Code)
	}
	return nil
}

// Add returns m + o. The currencies must match and the sum must fit in int64.
func (m Money) Add(o Money) (Money, error) {
	if err := m.sameCurrency(o); err != nil {
		return Money{}, err
	}
	if (o.Minor > 0 && m.Minor > math.MaxInt64-o.Minor) || (o.Minor < 0 && m.Minor < math.MinInt64-o.Minor) {
		return Money{}, ErrOverflow
	}
	return Money{Minor: m.Minor + o.Minor, Currency: m.Currency}, nil
}

// Sub returns m - o. The currencies must match and the result must fit in int64.
func (m Money) Sub(o Money) (Money, error) {
	if o.Minor == math.MinInt64 {
		return Money{}, ErrOverflow
	}
	return m.Add(Money{Minor: -o.Minor, Currency: o.Currency})
}

// Decimal returns the amount in major units, e.g. 1234 USD minor -> 12.34.
func (m Money) Decimal() Decimal { return newDecimal(big.NewInt(m.Minor), m.Currency.Exponent) }

// String formats the amount as "12.34 USD".
func (m Money) String() string { return m.Decimal().String() + " " + m.Currency.Code }

// MoneyFromDecimal rounds d (major units) to cur's exponent with cur's mode. It
// also returns the rounding difference (d minus the rounded amount, in major
// units) so the caller can post it as an explicit rounding line.
func MoneyFromDecimal(d Decimal, cur Currency) (Money, Decimal, error) {
	r, err := d.Round(cur.Exponent, cur.Mode)
	if err != nil {
		return Money{}, Decimal{}, err
	}
	if !r.unscaled.IsInt64() {
		return Money{}, Decimal{}, ErrOverflow
	}
	return Money{Minor: r.unscaled.Int64(), Currency: cur}, d.Sub(r), nil
}

// Mul multiplies by a decimal factor (a quantity or a rate) and rounds once,
// returning the product and the rounding difference.
func (m Money) Mul(factor Decimal) (Money, Decimal, error) {
	return MoneyFromDecimal(m.Decimal().Mul(factor), m.Currency)
}

// Convert applies an exchange rate (units of `to` per one unit of m's
// currency) and rounds once into `to`, returning the converted amount and the
// rounding difference in major units of `to`.
func (m Money) Convert(rate Decimal, to Currency) (Money, Decimal, error) {
	if rate.Sign() <= 0 {
		return Money{}, Decimal{}, errors.New("types: exchange rate must be positive")
	}
	return MoneyFromDecimal(m.Decimal().Mul(rate), to)
}

// Allocate splits m across weights so the parts add up to m exactly (see
// AllocateUnits for the remainder rule).
func (m Money) Allocate(weights []int64) ([]Money, error) {
	units, err := AllocateUnits(big.NewInt(m.Minor), weights)
	if err != nil {
		return nil, err
	}
	out := make([]Money, len(units))
	for i, u := range units {
		out[i] = Money{Minor: u.Int64(), Currency: m.Currency} // each |share| <= |m.Minor|
	}
	return out, nil
}
