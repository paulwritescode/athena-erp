package expr

import (
	"errors"
	"math"
	"regexp"
	"strings"
	"time"

	"athena-erp/internal/types"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/operators"
	"cel.dev/cel-go/common/types/ref"

	celtypes "cel.dev/cel-go/common/types"
)

// The language is a closed set of functions. CEL's standard library is not
// loaded except for the logical operators and equality, so there are no
// loops (macros are off), no floating point and no string-to-number casts.
// Null rules (spec 23.6): arithmetic with null is null, an ordering
// comparison with null is false, == and != treat null as a value that equals
// only null.

var (
	boolT      = cel.BoolType
	intT       = cel.IntType
	stringT    = cel.StringType
	dateT      = cel.TimestampType
	decimalT   = cel.OpaqueType("decimal")
	moneyT     = cel.OpaqueType("money")
	nullT      = cel.NullType
	valueKinds = []*cel.Type{intT, stringT, boolT, decimalT, moneyT, dateT}
)

const (
	maxDigits   = types.MaxDigits
	maxRoundTo  = 30
	maxMonths   = 12 * 1000
	maxDays     = 366 * 1000
	todayVar    = "athena.today"
	maxExprSize = 2000
)

// errFn builds a CEL error value.
func errFn(msg string) ref.Val { return celtypes.NewErr("%s", msg) }

func isNull(v ref.Val) bool { return v == celtypes.NullValue || v.Type() == celtypes.NullType }

// number reads an int or decimal as a decimal.
func number(v ref.Val) (types.Decimal, bool) {
	switch x := v.(type) {
	case decimalVal:
		return x.d, true
	case celtypes.Int:
		return intDecimal(int64(x)), true
	}
	return types.Decimal{}, false
}

// fit rejects a decimal wider than the 65 digits every engine stores.
func fit(d types.Decimal) ref.Val {
	if len(strings.TrimLeft(d.String(), "-")) > maxDigits+1 {
		return errFn("the decimal result has more than 65 digits")
	}
	return decimalVal{d}
}

func arith(op string) func(a, b ref.Val) ref.Val {
	return func(a, b ref.Val) ref.Val {
		if isNull(a) || isNull(b) {
			return celtypes.NullValue
		}
		if x, ok := a.(celtypes.Int); ok {
			if y, ok := b.(celtypes.Int); ok {
				return intArith(op, int64(x), int64(y))
			}
		}
		if x, ok := a.(celtypes.String); ok {
			if y, ok := b.(celtypes.String); ok && op == "+" {
				return celtypes.String(string(x) + string(y))
			}
		}
		if x, ok := a.(moneyVal); ok {
			if y, ok := b.(moneyVal); ok {
				var m types.Money
				var err error
				if op == "+" {
					m, err = x.m.Add(y.m)
				} else if op == "-" {
					m, err = x.m.Sub(y.m)
				} else {
					return errFn("money cannot be multiplied by money")
				}
				if err != nil {
					return errFn(err.Error())
				}
				return moneyVal{m}
			}
			if y, ok := number(b); ok && op == "*" {
				return scaleMoney(x.m, y)
			}
		}
		if y, ok := b.(moneyVal); ok && op == "*" {
			if x, ok := number(a); ok {
				return scaleMoney(y.m, x)
			}
		}
		x, okx := number(a)
		y, oky := number(b)
		if !okx || !oky {
			return errFn("no such operation for these values")
		}
		switch op {
		case "+":
			return fit(x.Add(y))
		case "-":
			return fit(x.Sub(y))
		}
		return fit(x.Mul(y))
	}
}

// scaleMoney multiplies once and rounds to the currency's own exponent with
// its rounding mode; the rounding difference is dropped, as in any total
// computed by an expression (spec 3.4 records it only in postings).
func scaleMoney(m types.Money, factor types.Decimal) ref.Val {
	out, _, err := m.Mul(factor)
	if err != nil {
		return errFn(err.Error())
	}
	return moneyVal{out}
}

func intArith(op string, x, y int64) ref.Val {
	switch op {
	case "+":
		if (y > 0 && x > math.MaxInt64-y) || (y < 0 && x < math.MinInt64-y) {
			return errFn("integer overflow")
		}
		return celtypes.Int(x + y)
	case "-":
		if (y < 0 && x > math.MaxInt64+y) || (y > 0 && x < math.MinInt64+y) {
			return errFn("integer overflow")
		}
		return celtypes.Int(x - y)
	}
	if x != 0 && y != 0 {
		p := x * y
		if p/y != x || (x == -1 && y == math.MinInt64) || (y == -1 && x == math.MinInt64) {
			return errFn("integer overflow")
		}
		return celtypes.Int(p)
	}
	return celtypes.Int(0)
}

func negate(v ref.Val) ref.Val {
	switch x := v.(type) {
	case celtypes.Int:
		if int64(x) == math.MinInt64 {
			return errFn("integer overflow")
		}
		return celtypes.Int(-int64(x))
	case decimalVal:
		return decimalVal{x.d.Neg()}
	}
	if isNull(v) {
		return celtypes.NullValue
	}
	return errFn("no such operation for this value")
}

// compare returns -1, 0 or 1, or ok false when either side is null.
func compare(a, b ref.Val) (c int, ok bool, err error) {
	if isNull(a) || isNull(b) {
		return 0, false, nil
	}
	if x, okx := a.(celtypes.Int); okx {
		if y, oky := b.(celtypes.Int); oky {
			switch {
			case x < y:
				return -1, true, nil
			case x > y:
				return 1, true, nil
			}
			return 0, true, nil
		}
	}
	if x, okx := number(a); okx {
		if y, oky := number(b); oky {
			return x.Cmp(y), true, nil
		}
	}
	switch x := a.(type) {
	case moneyVal:
		if y, ok := b.(moneyVal); ok {
			if x.m.Currency.Code != y.m.Currency.Code || x.m.Currency.Exponent != y.m.Currency.Exponent {
				return 0, false, errors.New("money in different currencies cannot be compared")
			}
			switch {
			case x.m.Minor < y.m.Minor:
				return -1, true, nil
			case x.m.Minor > y.m.Minor:
				return 1, true, nil
			}
			return 0, true, nil
		}
	case celtypes.String:
		if y, ok := b.(celtypes.String); ok {
			return strings.Compare(string(x), string(y)), true, nil
		}
	case celtypes.Timestamp:
		if y, ok := b.(celtypes.Timestamp); ok {
			return x.Time.Compare(y.Time), true, nil
		}
	}
	return 0, false, errors.New("no such comparison for these values")
}

func ordering(test func(int) bool) func(a, b ref.Val) ref.Val {
	return func(a, b ref.Val) ref.Val {
		c, ok, err := compare(a, b)
		if err != nil {
			return errFn(err.Error())
		}
		return celtypes.Bool(ok && test(c))
	}
}

// signatures lists the operand pairs an operator accepts.
func pairs(sig ...[2]*cel.Type) [][2]*cel.Type { return sig }

func overloads(prefix string, result func(a, b *cel.Type) *cel.Type, sig [][2]*cel.Type) []cel.FunctionOpt {
	var opts []cel.FunctionOpt
	for _, p := range sig {
		opts = append(opts, cel.Overload(prefix+"_"+p[0].String()+"_"+p[1].String(), []*cel.Type{p[0], p[1]}, result(p[0], p[1])))
	}
	return opts
}

var (
	numericPairs = pairs([2]*cel.Type{intT, intT}, [2]*cel.Type{decimalT, decimalT}, [2]*cel.Type{intT, decimalT}, [2]*cel.Type{decimalT, intT})
	orderPairs   = append(append(pairs(), numericPairs...), [2]*cel.Type{moneyT, moneyT}, [2]*cel.Type{stringT, stringT}, [2]*cel.Type{dateT, dateT})
)

func resultOf(t *cel.Type) func(a, b *cel.Type) *cel.Type {
	return func(_, _ *cel.Type) *cel.Type { return t }
}

// numericResult is int for two ints and decimal otherwise.
func numericResult(a, b *cel.Type) *cel.Type {
	if a == intT && b == intT {
		return intT
	}
	return decimalT
}

func operatorOptions() []cel.EnvOption {
	var opts []cel.EnvOption
	add := append(overloads("add", numericResult, numericPairs), cel.Overload("add_money_money", []*cel.Type{moneyT, moneyT}, moneyT), cel.Overload("add_string_string", []*cel.Type{stringT, stringT}, stringT))
	opts = append(opts, cel.Function(operators.Add, append(add, cel.SingletonBinaryBinding(arith("+")))...))
	sub := append(overloads("sub", numericResult, numericPairs), cel.Overload("sub_money_money", []*cel.Type{moneyT, moneyT}, moneyT))
	opts = append(opts, cel.Function(operators.Subtract, append(sub, cel.SingletonBinaryBinding(arith("-")))...))
	mul := append(overloads("mul", numericResult, numericPairs),
		cel.Overload("mul_money_decimal", []*cel.Type{moneyT, decimalT}, moneyT), cel.Overload("mul_decimal_money", []*cel.Type{decimalT, moneyT}, moneyT),
		cel.Overload("mul_money_int", []*cel.Type{moneyT, intT}, moneyT), cel.Overload("mul_int_money", []*cel.Type{intT, moneyT}, moneyT))
	opts = append(opts, cel.Function(operators.Multiply, append(mul, cel.SingletonBinaryBinding(arith("*")))...))
	opts = append(opts, cel.Function(operators.Negate,
		cel.Overload("neg_int", []*cel.Type{intT}, intT), cel.Overload("neg_decimal", []*cel.Type{decimalT}, decimalT), cel.SingletonUnaryBinding(negate)))
	for name, test := range map[string]func(int) bool{
		operators.Less:          func(c int) bool { return c < 0 },
		operators.LessEquals:    func(c int) bool { return c <= 0 },
		operators.Greater:       func(c int) bool { return c > 0 },
		operators.GreaterEquals: func(c int) bool { return c >= 0 },
	} {
		opts = append(opts, cel.Function(name, append(overloads("cmp"+name, resultOf(boolT), orderPairs), cel.SingletonBinaryBinding(ordering(test)))...))
	}
	// == and != are planned by CEL itself and call Equal on the values, so the
	// declarations only tell the checker what may be compared: a value with
	// its own kind, a value with null, and a decimal with a whole number.
	for _, name := range []string{operators.Equals, operators.NotEquals} {
		var eq []cel.FunctionOpt
		for _, t := range valueKinds {
			eq = append(eq, cel.Overload(name+"_"+t.String(), []*cel.Type{t, t}, boolT),
				cel.Overload(name+"_"+t.String()+"_null", []*cel.Type{t, nullT}, boolT), cel.Overload(name+"_null_"+t.String(), []*cel.Type{nullT, t}, boolT))
		}
		eq = append(eq, cel.Overload(name+"_decimal_int", []*cel.Type{decimalT, intT}, boolT), cel.Overload(name+"_null_null", []*cel.Type{nullT, nullT}, boolT))
		opts = append(opts, cel.Function(name, eq...))
	}
	return opts
}

// ---- functions -----------------------------------------------------------

func listOf(t *cel.Type) *cel.Type { return cel.ListType(t) }

func elements(v ref.Val) ([]ref.Val, bool) {
	list, ok := v.(interface {
		Size() ref.Val
		Get(ref.Val) ref.Val
	})
	if !ok {
		return nil, false
	}
	n, _ := list.Size().Value().(int64)
	out := make([]ref.Val, 0, n)
	for i := int64(0); i < n; i++ {
		out = append(out, list.Get(celtypes.Int(i)))
	}
	return out, true
}

func nonNull(v ref.Val) ([]ref.Val, ref.Val) {
	items, ok := elements(v)
	if !ok {
		return nil, errFn("expected a list")
	}
	out := items[:0:0]
	for _, item := range items {
		if !isNull(item) {
			out = append(out, item)
		}
	}
	return out, nil
}

func sumDecimals(v ref.Val) ref.Val {
	items, bad := nonNull(v)
	if bad != nil {
		return bad
	}
	total := intDecimal(0)
	for _, item := range items {
		d, ok := item.(decimalVal)
		if !ok {
			return errFn("expected decimals")
		}
		total = total.Add(d.d)
	}
	return fit(total)
}

// sumMoney is null for an empty list: with no amounts there is no currency to
// give a zero.
func sumMoney(v ref.Val) ref.Val {
	items, bad := nonNull(v)
	if bad != nil {
		return bad
	}
	if len(items) == 0 {
		return celtypes.NullValue
	}
	total, ok := items[0].(moneyVal)
	if !ok {
		return errFn("expected money")
	}
	for _, item := range items[1:] {
		next, ok := item.(moneyVal)
		if !ok {
			return errFn("expected money")
		}
		sum, err := total.m.Add(next.m)
		if err != nil {
			return errFn(err.Error())
		}
		total = moneyVal{sum}
	}
	return total
}

// extreme returns the smallest (sign -1) or largest (sign 1) element, or
// null for an empty list.
func extreme(sign int) func(args ...ref.Val) ref.Val {
	return func(args ...ref.Val) ref.Val {
		items, bad := nonNull(args[0])
		if bad != nil {
			return bad
		}
		if len(items) == 0 {
			return celtypes.NullValue
		}
		best := items[0]
		for _, item := range items[1:] {
			c, ok, err := compare(item, best)
			if err != nil {
				return errFn(err.Error())
			}
			if ok && c*sign > 0 {
				best = item
			}
		}
		return best
	}
}

func count(args ...ref.Val) ref.Val {
	items, bad := nonNull(args[0])
	if bad != nil {
		return bad
	}
	return celtypes.Int(len(items))
}

func coalesce(a, b ref.Val) ref.Val {
	if isNull(a) {
		return b
	}
	return a
}

func roundTo(a, b ref.Val) ref.Val {
	if isNull(a) {
		return celtypes.NullValue
	}
	d, _ := a.(decimalVal)
	places, ok := b.(celtypes.Int)
	if !ok || places < 0 || places > maxRoundTo {
		return errFn("round needs 0 to 30 decimal places")
	}
	out, err := d.d.Round(int(places), types.HalfUp)
	if err != nil {
		return errFn(err.Error())
	}
	return fit(out)
}

func divide(args ...ref.Val) ref.Val {
	if isNull(args[0]) || isNull(args[1]) {
		return celtypes.NullValue
	}
	x, okx := number(args[0])
	y, oky := number(args[1])
	places, okp := args[2].(celtypes.Int)
	if !okx || !oky || !okp || places < 0 || places > maxRoundTo {
		return errFn("div needs two numbers and 0 to 30 decimal places")
	}
	out, err := x.Div(y, int(places), types.HalfUp)
	if err != nil {
		if errors.Is(err, types.ErrDivideByZero) {
			return errFn("division by zero")
		}
		return errFn(err.Error())
	}
	return fit(out)
}

// ParseDecimalLiteral is dec("1.50"), shared with the SQL translator.
func parseDecimal(text string) (types.Decimal, error) { return types.ParseDecimal(text) }

func toDecimal(args ...ref.Val) ref.Val {
	switch x := args[0].(type) {
	case celtypes.Int:
		return decimalVal{intDecimal(int64(x))}
	case celtypes.String:
		d, err := parseDecimal(string(x))
		if err != nil {
			return errFn("not a decimal number")
		}
		return decimalVal{d}
	}
	if isNull(args[0]) {
		return celtypes.NullValue
	}
	return errFn("dec needs a string or an integer")
}

// makeMoney builds an exact amount; digits beyond the exponent are an error.
func makeMoney(d types.Decimal, code string, exponent int64) (types.Money, error) {
	cur, err := types.NewCurrency(code, int(exponent), types.HalfUp)
	if err != nil {
		return types.Money{}, err
	}
	m, rounding, err := types.MoneyFromDecimal(d, cur)
	if err != nil {
		return types.Money{}, err
	}
	if rounding.Sign() != 0 {
		return types.Money{}, types.ErrRounding
	}
	return m, nil
}

func toMoney(args ...ref.Val) ref.Val {
	if isNull(args[0]) {
		return celtypes.NullValue
	}
	d, okd := args[0].(decimalVal)
	code, okc := args[1].(celtypes.String)
	exp, oke := args[2].(celtypes.Int)
	if !okd || !okc || !oke {
		return errFn("money needs a decimal, a currency code and the minor-unit digits")
	}
	m, err := makeMoney(d.d, string(code), int64(exp))
	if err != nil {
		return errFn(err.Error())
	}
	return moneyVal{m}
}

func amount(v ref.Val) ref.Val {
	if m, ok := v.(moneyVal); ok {
		return decimalVal{m.m.Decimal()}
	}
	return celtypes.NullValue
}

func currency(v ref.Val) ref.Val {
	if m, ok := v.(moneyVal); ok {
		return celtypes.String(m.m.Currency.Code)
	}
	return celtypes.NullValue
}

func lower(v ref.Val) ref.Val {
	if s, ok := v.(celtypes.String); ok {
		return celtypes.String(strings.ToLower(string(s)))
	}
	return celtypes.NullValue
}

func startsWith(a, b ref.Val) ref.Val {
	x, okx := a.(celtypes.String)
	y, oky := b.(celtypes.String)
	return celtypes.Bool(okx && oky && strings.HasPrefix(string(x), string(y)))
}

func matches(a, b ref.Val) ref.Val {
	x, okx := a.(celtypes.String)
	y, oky := b.(celtypes.String)
	if !okx || !oky {
		return celtypes.False
	}
	re, err := regexp.Compile(string(y))
	if err != nil {
		return errFn("the pattern is not a valid regular expression")
	}
	return celtypes.Bool(re.MatchString(string(x)))
}

// addDays and addMonths are shared with the SQL translator's constant folding.
func addDays(t time.Time, n int64) (time.Time, error) {
	if n < -maxDays || n > maxDays {
		return time.Time{}, errors.New("add_days is limited to 366000 days")
	}
	return t.AddDate(0, 0, int(n)), nil
}

// addMonths clamps to the last day of the target month, so Jan 31 plus one
// month is Feb 28 or 29.
func addMonths(t time.Time, n int64) (time.Time, error) {
	if n < -maxMonths || n > maxMonths {
		return time.Time{}, errors.New("add_months is limited to 12000 months")
	}
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, int(n), 0)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(t.Day(), last), 0, 0, 0, 0, time.UTC), nil
}

func shiftDate(add func(time.Time, int64) (time.Time, error)) func(a, b ref.Val) ref.Val {
	return func(a, b ref.Val) ref.Val {
		if isNull(a) || isNull(b) {
			return celtypes.NullValue
		}
		t, okt := a.(celtypes.Timestamp)
		n, okn := b.(celtypes.Int)
		if !okt || !okn {
			return errFn("expected a date and a number of days or months")
		}
		out, err := add(t.Time, int64(n))
		if err != nil {
			return errFn(err.Error())
		}
		return celtypes.Timestamp{Time: out}
	}
}

func functionOptions() []cel.EnvOption {
	var opts []cel.EnvOption
	fn := func(name string, binding cel.FunctionOpt, overloadOpts ...cel.FunctionOpt) {
		if binding != nil {
			overloadOpts = append(overloadOpts, binding)
		}
		opts = append(opts, cel.Function(name, overloadOpts...))
	}
	fn("sum", nil,
		cel.Overload("sum_list_decimal", []*cel.Type{listOf(decimalT)}, decimalT, cel.UnaryBinding(sumDecimals)),
		cel.Overload("sum_list_money", []*cel.Type{listOf(moneyT)}, moneyT, cel.UnaryBinding(sumMoney)))
	fn("min", cel.SingletonFunctionBinding(extreme(-1)),
		cel.Overload("min_list_decimal", []*cel.Type{listOf(decimalT)}, decimalT), cel.Overload("min_list_money", []*cel.Type{listOf(moneyT)}, moneyT))
	fn("max", cel.SingletonFunctionBinding(extreme(1)),
		cel.Overload("max_list_decimal", []*cel.Type{listOf(decimalT)}, decimalT), cel.Overload("max_list_money", []*cel.Type{listOf(moneyT)}, moneyT))
	fn("count", cel.SingletonFunctionBinding(count),
		cel.Overload("count_list_decimal", []*cel.Type{listOf(decimalT)}, intT), cel.Overload("count_list_money", []*cel.Type{listOf(moneyT)}, intT))
	var coalesceOverloads []cel.FunctionOpt
	for _, t := range valueKinds {
		coalesceOverloads = append(coalesceOverloads, cel.Overload("coalesce_"+t.String(), []*cel.Type{t, t}, t))
	}
	fn("coalesce", cel.SingletonBinaryBinding(coalesce), coalesceOverloads...)
	fn("round", cel.SingletonBinaryBinding(roundTo), cel.Overload("round_decimal_int", []*cel.Type{decimalT, intT}, decimalT))
	fn("div", cel.SingletonFunctionBinding(divide), cel.Overload("div_numeric", []*cel.Type{decimalT, decimalT, intT}, decimalT),
		cel.Overload("div_decimal_int", []*cel.Type{decimalT, intT, intT}, decimalT), cel.Overload("div_int_decimal", []*cel.Type{intT, decimalT, intT}, decimalT),
		cel.Overload("div_int_int", []*cel.Type{intT, intT, intT}, decimalT))
	fn("dec", cel.SingletonFunctionBinding(toDecimal), cel.Overload("dec_string", []*cel.Type{stringT}, decimalT), cel.Overload("dec_int", []*cel.Type{intT}, decimalT))
	fn("money", cel.SingletonFunctionBinding(toMoney), cel.Overload("money_decimal", []*cel.Type{decimalT, stringT, intT}, moneyT))
	fn("amount", cel.SingletonUnaryBinding(amount), cel.Overload("amount_money", []*cel.Type{moneyT}, decimalT))
	fn("currency", cel.SingletonUnaryBinding(currency), cel.Overload("currency_money", []*cel.Type{moneyT}, stringT))
	fn("lower", cel.SingletonUnaryBinding(lower), cel.Overload("lower_string", []*cel.Type{stringT}, stringT))
	fn("startsWith", cel.SingletonBinaryBinding(startsWith), cel.MemberOverload("starts_with_string", []*cel.Type{stringT, stringT}, boolT))
	fn("matches", cel.SingletonBinaryBinding(matches), cel.MemberOverload("matches_string", []*cel.Type{stringT, stringT}, boolT))
	fn("add_days", cel.SingletonBinaryBinding(shiftDate(addDays)), cel.Overload("add_days_date", []*cel.Type{dateT, intT}, dateT))
	fn("add_months", cel.SingletonBinaryBinding(shiftDate(addMonths)), cel.Overload("add_months_date", []*cel.Type{dateT, intT}, dateT))
	return opts
}
