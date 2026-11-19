// Package types holds Athena's exact value types: decimals, money in minor
// units, quantities with a unit of measure, dates and deterministic
// allocation. The package is pure Go with no database or network access and
// never converts to binary fractions, so every result is exact.
package types

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// RoundingMode selects how a value is rounded when it loses digits.
type RoundingMode string

const (
	// HalfUp rounds a tie away from zero (1.5 -> 2, -1.5 -> -2).
	HalfUp RoundingMode = "half_up"
	// HalfEven rounds a tie to the even neighbour (bankers rounding).
	HalfEven RoundingMode = "half_even"
)

// Valid reports whether m is a known rounding mode.
func (m RoundingMode) Valid() bool { return m == HalfUp || m == HalfEven }

// MaxDigits is the most significant digits Parse accepts, the same bound as
// MariaDB DECIMAL.
const MaxDigits = 65

var (
	ErrSyntax       = errors.New("types: invalid decimal syntax")
	ErrTooLong      = errors.New("types: decimal has too many digits")
	ErrRounding     = errors.New("types: value needs rounding")
	ErrDivideByZero = errors.New("types: division by zero")
	ErrRoundingMode = errors.New("types: unknown rounding mode")
)

// Decimal is an exact base-10 number: unscaled / 10^scale with scale >= 0.
// The zero value is 0. Decimals are immutable; every method returns a new one.
type Decimal struct {
	unscaled *big.Int
	scale    int
}

// NewDecimal returns unscaled / 10^scale. A negative scale is an error.
func NewDecimal(unscaled int64, scale int) (Decimal, error) {
	if scale < 0 || scale > MaxDigits {
		return Decimal{}, fmt.Errorf("types: scale %d out of range", scale)
	}
	return Decimal{unscaled: big.NewInt(unscaled), scale: scale}, nil
}

func newDecimal(u *big.Int, scale int) Decimal { return Decimal{unscaled: u, scale: scale} }

func (d Decimal) value() *big.Int {
	if d.unscaled == nil {
		return new(big.Int)
	}
	return d.unscaled
}

// ParseDecimal parses an optional sign, digits and an optional fraction
// ("-12.340"). Exponents, spaces, separators and special values are rejected.
// The scale of the result is the number of fraction digits written.
func ParseDecimal(s string) (Decimal, error) {
	body := s
	neg := false
	if strings.HasPrefix(body, "-") {
		neg, body = true, body[1:]
	} else if strings.HasPrefix(body, "+") {
		body = body[1:]
	}
	whole, frac, hasDot := strings.Cut(body, ".")
	if whole == "" && frac == "" {
		return Decimal{}, ErrSyntax
	}
	if hasDot && frac == "" {
		return Decimal{}, ErrSyntax
	}
	if !allDigits(whole) || !allDigits(frac) {
		return Decimal{}, ErrSyntax
	}
	digits := strings.TrimLeft(whole+frac, "0")
	if len(digits) > MaxDigits || len(frac) > MaxDigits {
		return Decimal{}, ErrTooLong
	}
	u, ok := new(big.Int).SetString(whole+frac, 10)
	if !ok {
		return Decimal{}, ErrSyntax
	}
	if neg {
		u.Neg(u)
	}
	return newDecimal(u, len(frac)), nil
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// MustDecimal parses s and panics on error. Use it for constants in tests.
func MustDecimal(s string) Decimal {
	d, err := ParseDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

// Scale returns the number of fraction digits the value carries.
func (d Decimal) Scale() int { return d.scale }

// Sign returns -1, 0 or +1.
func (d Decimal) Sign() int { return d.value().Sign() }

// String formats the value with exactly Scale fraction digits.
func (d Decimal) String() string {
	u := d.value()
	neg := u.Sign() < 0
	digits := new(big.Int).Abs(u).String()
	if d.scale > 0 {
		if len(digits) <= d.scale {
			digits = strings.Repeat("0", d.scale-len(digits)+1) + digits
		}
		digits = digits[:len(digits)-d.scale] + "." + digits[len(digits)-d.scale:]
	}
	if neg {
		return "-" + digits
	}
	return digits
}

// Neg returns -d.
func (d Decimal) Neg() Decimal { return newDecimal(new(big.Int).Neg(d.value()), d.scale) }

// Abs returns |d|.
func (d Decimal) Abs() Decimal { return newDecimal(new(big.Int).Abs(d.value()), d.scale) }

// align brings two decimals to the larger scale without changing either value.
func align(a, b Decimal) (*big.Int, *big.Int, int) {
	switch {
	case a.scale == b.scale:
		return a.value(), b.value(), a.scale
	case a.scale < b.scale:
		return new(big.Int).Mul(a.value(), pow10(b.scale-a.scale)), b.value(), b.scale
	default:
		return a.value(), new(big.Int).Mul(b.value(), pow10(a.scale-b.scale)), a.scale
	}
}

// Add returns a + b at the larger of the two scales.
func (a Decimal) Add(b Decimal) Decimal {
	x, y, s := align(a, b)
	return newDecimal(new(big.Int).Add(x, y), s)
}

// Sub returns a - b at the larger of the two scales.
func (a Decimal) Sub(b Decimal) Decimal {
	x, y, s := align(a, b)
	return newDecimal(new(big.Int).Sub(x, y), s)
}

// Mul returns a * b exactly; the scale is the sum of the two scales.
func (a Decimal) Mul(b Decimal) Decimal {
	return newDecimal(new(big.Int).Mul(a.value(), b.value()), a.scale+b.scale)
}

// Cmp compares by value (1.50 equals 1.5) and returns -1, 0 or +1.
func (a Decimal) Cmp(b Decimal) int {
	x, y, _ := align(a, b)
	return x.Cmp(y)
}

// Equal reports whether a and b have the same value, whatever their scales.
func (a Decimal) Equal(b Decimal) bool { return a.Cmp(b) == 0 }

// Div returns a / b rounded to scale fraction digits with mode.
func (a Decimal) Div(b Decimal, scale int, mode RoundingMode) (Decimal, error) {
	if scale < 0 || scale > MaxDigits {
		return Decimal{}, fmt.Errorf("types: scale %d out of range", scale)
	}
	if b.Sign() == 0 {
		return Decimal{}, ErrDivideByZero
	}
	// a/b at scale s = au * 10^(bs+s) / (bu * 10^as)
	num := new(big.Int).Mul(a.value(), pow10(b.scale+scale))
	den := new(big.Int).Mul(b.value(), pow10(a.scale))
	q, err := divRound(num, den, mode)
	if err != nil {
		return Decimal{}, err
	}
	return newDecimal(q, scale), nil
}

// Round returns d at the given scale. Raising the scale is exact; lowering it
// rounds with mode.
func (d Decimal) Round(scale int, mode RoundingMode) (Decimal, error) {
	if scale < 0 || scale > MaxDigits {
		return Decimal{}, fmt.Errorf("types: scale %d out of range", scale)
	}
	if scale >= d.scale {
		return newDecimal(new(big.Int).Mul(d.value(), pow10(scale-d.scale)), scale), nil
	}
	q, err := divRound(d.value(), pow10(d.scale-scale), mode)
	if err != nil {
		return Decimal{}, err
	}
	return newDecimal(q, scale), nil
}

// Rescale returns d at the given scale and fails with ErrRounding when that
// would drop non-zero digits.
func (d Decimal) Rescale(scale int) (Decimal, error) {
	r, err := d.Round(scale, HalfUp)
	if err != nil {
		return Decimal{}, err
	}
	if !r.Equal(d) {
		return Decimal{}, ErrRounding
	}
	return r, nil
}

func pow10(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

// divRound returns num/den rounded to an integer with mode. den must not be 0.
func divRound(num, den *big.Int, mode RoundingMode) (*big.Int, error) {
	if !mode.Valid() {
		return nil, ErrRoundingMode
	}
	neg := (num.Sign() < 0) != (den.Sign() < 0)
	n, d := new(big.Int).Abs(num), new(big.Int).Abs(den)
	q, r := new(big.Int).QuoRem(n, d, new(big.Int))
	twice := r.Lsh(r, 1)
	switch c := twice.Cmp(d); {
	case c > 0:
		q.Add(q, big.NewInt(1))
	case c == 0 && (mode == HalfUp || q.Bit(0) == 1):
		q.Add(q, big.NewInt(1))
	}
	if neg {
		q.Neg(q)
	}
	return q, nil
}
