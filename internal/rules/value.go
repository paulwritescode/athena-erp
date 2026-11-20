package rules

import (
	"errors"
	"fmt"
	"sort"

	"athena-erp/internal/types"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// ErrFloat is returned for any float a Rule produces or passes to the host.
var ErrFloat = errors.New("floats are not allowed; use decimal(\"1.5\") or an integer")

// maxIntBits bounds the integers converted to text or decimals. 256 bits hold
// 78 digits, past the 65 decimals accept, so nothing convertible is refused
// and a giant integer fails before its text is ever built.
const maxIntBits = 256

// intTooLarge reports an integer no decimal can hold.
func intTooLarge(v starlark.Int) bool { return v.BigInt().BitLen() > maxIntBits }

// maxDecimalDigits bounds the digits of a multiplied decimal. Products stay
// exact; repeated squaring stops instead of doubling past memory.
const maxDecimalDigits = 2 * types.MaxDigits

// decimalTooBig reports a decimal with more digits than intermediates need.
func decimalTooBig(d types.Decimal) bool {
	text := d.String()
	n := 0
	for i := 0; i < len(text); i++ {
		if c := text[i]; c >= '0' && c <= '9' {
			n++
			if n > maxDecimalDigits {
				return true
			}
		}
	}
	return false
}

// decimalValue is an exact decimal in Starlark. Only +, -, * and comparisons
// with another decimal or an integer exist; division is the div builtin, so no
// binary fraction can appear.
type decimalValue struct{ d types.Decimal }

func (v decimalValue) String() string       { return v.d.String() }
func (decimalValue) Type() string           { return "decimal" }
func (decimalValue) Freeze()                {}
func (v decimalValue) Truth() starlark.Bool { return v.d.Sign() != 0 }
func (decimalValue) Hash() (uint32, error)  { return 0, errors.New("unhashable type: decimal") }

// asDecimal reads a decimal or an integer. Integers past maxIntBits fail
// without building their text; callers report the size with intTooLarge.
func asDecimal(v starlark.Value) (types.Decimal, bool) {
	switch v := v.(type) {
	case decimalValue:
		return v.d, true
	case starlark.Int:
		if intTooLarge(v) {
			return types.Decimal{}, false
		}
		d, err := types.ParseDecimal(v.String())
		return d, err == nil
	}
	return types.Decimal{}, false
}

func (v decimalValue) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	if m, ok := y.(moneyValue); ok && op == syntax.STAR {
		return m.times(v.d)
	}
	if i, ok := y.(starlark.Int); ok && intTooLarge(i) {
		return nil, &HostError{Code: CodeMemoryLimit, Err: errors.New("the integer is too large to use as a decimal")}
	}
	other, ok := asDecimal(y)
	if !ok {
		return nil, nil
	}
	l, r := v.d, other
	if side == starlark.Right {
		l, r = other, v.d
	}
	switch op {
	case syntax.PLUS:
		return decimalValue{l.Add(r)}, nil
	case syntax.MINUS:
		return decimalValue{l.Sub(r)}, nil
	case syntax.STAR:
		product := l.Mul(r)
		if decimalTooBig(product) {
			return nil, &HostError{Code: CodeMemoryLimit, Err: fmt.Errorf("the product has more than %d digits", maxDecimalDigits)}
		}
		return decimalValue{product}, nil
	}
	return nil, nil
}

func (v decimalValue) Unary(op syntax.Token) (starlark.Value, error) {
	switch op {
	case syntax.MINUS:
		return decimalValue{v.d.Neg()}, nil
	case syntax.PLUS:
		return v, nil
	}
	return nil, nil
}

func (v decimalValue) CompareSameType(op syntax.Token, y starlark.Value, _ int) (bool, error) {
	return compareResult(op, v.d.Cmp(y.(decimalValue).d))
}

func compareResult(op syntax.Token, c int) (bool, error) {
	switch op {
	case syntax.EQL:
		return c == 0, nil
	case syntax.NEQ:
		return c != 0, nil
	case syntax.LT:
		return c < 0, nil
	case syntax.LE:
		return c <= 0, nil
	case syntax.GT:
		return c > 0, nil
	case syntax.GE:
		return c >= 0, nil
	}
	return false, fmt.Errorf("unsupported comparison %s", op)
}

// moneyValue is an amount in integer minor units of one currency.
type moneyValue struct{ m types.Money }

func (v moneyValue) String() string       { return v.m.String() }
func (moneyValue) Type() string           { return "money" }
func (moneyValue) Freeze()                {}
func (v moneyValue) Truth() starlark.Bool { return v.m.Minor != 0 }
func (moneyValue) Hash() (uint32, error)  { return 0, errors.New("unhashable type: money") }

func (v moneyValue) AttrNames() []string { return []string{"amount", "currency", "minor"} }

func (v moneyValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "minor":
		return starlark.MakeInt64(v.m.Minor), nil
	case "currency":
		return starlark.String(v.m.Currency.Code), nil
	case "amount":
		return decimalValue{v.m.Decimal()}, nil
	}
	return nil, nil
}

// times multiplies by a factor and rounds once, with the currency's mode.
func (v moneyValue) times(factor types.Decimal) (starlark.Value, error) {
	product, _, err := v.m.Mul(factor)
	if err != nil {
		return nil, err
	}
	return moneyValue{product}, nil
}

func (v moneyValue) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	if other, ok := y.(moneyValue); ok {
		l, r := v.m, other.m
		if side == starlark.Right {
			l, r = other.m, v.m
		}
		var sum types.Money
		var err error
		switch op {
		case syntax.PLUS:
			sum, err = l.Add(r)
		case syntax.MINUS:
			sum, err = l.Sub(r)
		default:
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return moneyValue{sum}, nil
	}
	if op == syntax.STAR {
		if i, ok := y.(starlark.Int); ok && intTooLarge(i) {
			return nil, &HostError{Code: CodeMemoryLimit, Err: errors.New("the integer is too large to use as a decimal")}
		}
		if factor, ok := asDecimal(y); ok {
			return v.times(factor)
		}
	}
	return nil, nil
}

func (v moneyValue) Unary(op syntax.Token) (starlark.Value, error) {
	switch op {
	case syntax.MINUS:
		return moneyValue{types.NewMoney(-v.m.Minor, v.m.Currency)}, nil
	case syntax.PLUS:
		return v, nil
	}
	return nil, nil
}

func (v moneyValue) CompareSameType(op syntax.Token, y starlark.Value, _ int) (bool, error) {
	other := y.(moneyValue).m
	if other.Currency.Code != v.m.Currency.Code {
		return false, fmt.Errorf("cannot compare %s with %s", v.m.Currency.Code, other.Currency.Code)
	}
	c := 0
	switch {
	case v.m.Minor < other.Minor:
		c = -1
	case v.m.Minor > other.Minor:
		c = 1
	}
	return compareResult(op, c)
}

// recordValue is a document or a stored Record: read-only attributes, one per
// field. Its text form never prints values, so a stray str() or print cannot
// leak one into a log.
type recordValue struct {
	name string
	row  Row
}

func newRecord(name string, row Row) (starlark.Value, error) {
	for key, value := range row.Values {
		if _, err := toStarlark(value); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
	}
	return recordValue{name: name, row: row}, nil
}

func (v recordValue) String() string      { return "record(" + v.name + ")" }
func (recordValue) Type() string          { return "record" }
func (recordValue) Freeze()               {}
func (recordValue) Truth() starlark.Bool  { return true }
func (recordValue) Hash() (uint32, error) { return 0, errors.New("unhashable type: record") }

func (v recordValue) AttrNames() []string {
	names := make([]string, 0, len(v.row.Values))
	for key := range v.row.Values {
		names = append(names, key)
	}
	sort.Strings(names)
	return names
}

func (v recordValue) Attr(name string) (starlark.Value, error) {
	if v.row.Hidden[name] {
		return nil, fmt.Errorf("%s is above the acting user's permission level", name)
	}
	value, ok := v.row.Values[name]
	if !ok {
		return nil, nil
	}
	return toStarlark(value)
}

// toStarlark converts a host value. Anything else is a programming error in
// the host and is refused rather than passed through.
func toStarlark(value any) (starlark.Value, error) {
	switch v := value.(type) {
	case nil:
		return starlark.None, nil
	case bool:
		return starlark.Bool(v), nil
	case string:
		return starlark.String(v), nil
	case int64:
		return starlark.MakeInt64(v), nil
	case types.Decimal:
		return decimalValue{v}, nil
	case types.Money:
		return moneyValue{v}, nil
	}
	return nil, fmt.Errorf("a value of type %T cannot be given to a Rule", value)
}

// toGo converts what a Rule passes to the host. Integers become decimals, so
// the host sees one numeric type; floats are refused. Text past the length
// cap and integers past maxIntBits stop the run instead of reaching the host.
func (s *state) toGo(value starlark.Value) (any, error) {
	switch v := value.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(v), nil
	case starlark.String:
		if err := s.checkLen("the text", len(v)); err != nil {
			return nil, err
		}
		return string(v), nil
	case starlark.Int:
		if intTooLarge(v) {
			return nil, &HostError{Code: CodeMemoryLimit, Err: errors.New("the integer is too large to store")}
		}
		d, err := types.ParseDecimal(v.String())
		if err != nil {
			return nil, fmt.Errorf("the integer %s is too large", v)
		}
		return d, nil
	case decimalValue:
		return v.d, nil
	case moneyValue:
		return v.m, nil
	case starlark.Float:
		return nil, ErrFloat
	}
	return nil, fmt.Errorf("a %s cannot be stored in a field", value.Type())
}
