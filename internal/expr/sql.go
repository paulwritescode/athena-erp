package expr

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"athena-erp/internal/types"

	"cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/operators"
	celtypes "cel.dev/cel-go/common/types"
)

// Dialect supplies the engine-specific pieces of a predicate. store.Dialect
// implements it. Everything else the translator writes is plain SQL that
// MariaDB and PostgreSQL share (PostgreSQL itself is untested, ADR-0005).
type Dialect interface {
	// NullSafeEqual compares two values so that null equals only null.
	NullSafeEqual(a, b string) string
	// BinaryString makes a string expression compare exactly: case, accents
	// and trailing spaces all count, like Go's string comparison.
	BinaryString(expr string) string
	// DecimalParam and DateParam are placeholders that bind a text argument
	// as an exact decimal or a date, so no comparison goes through floating
	// point or a collation.
	DecimalParam() string
	DateParam() string
}

// SQL is a translated predicate with its positional arguments.
type SQL struct {
	Where string
	Args  []any
}

// NotTranslatableError says which part of an expression has no SQL form.
type NotTranslatableError struct{ Reason string }

func (e NotTranslatableError) Error() string {
	return "this expression cannot run in the database: " + e.Reason
}

func notTranslatable(format string, args ...any) error {
	return NotTranslatableError{Reason: fmt.Sprintf(format, args...)}
}

// IsNotTranslatable reports whether err means the expression has no SQL form.
func IsNotTranslatable(err error) bool {
	var target NotTranslatableError
	return errors.As(err, &target)
}

type frag struct {
	sql  string
	args []any
	kind Kind // empty for the null literal
	cur  string
	exp  int
}

type constant struct {
	value any
	set   bool
}

type translator struct {
	p       *Program
	d       Dialect
	columns map[string]string
	today   time.Time
}

// Translate turns a boolean expression into a SQL predicate. columns maps
// each variable name to its already-quoted column. The predicate selects the
// rows for which Eval returns true: comparisons with null are false and
// equality treats null as a value, in both worlds. Expressions that read
// lists, use text functions other than startsWith, divide, or build values
// from columns with date or money arithmetic return a NotTranslatableError;
// the caller then evaluates in memory.
func (p *Program) Translate(d Dialect, columns map[string]string, today time.Time) (SQL, error) {
	if p.Result != Bool {
		return SQL{}, errors.New("a filter must return true or false")
	}
	t := translator{p: p, d: d, columns: columns, today: today.UTC().Truncate(24 * time.Hour)}
	f, err := t.expr(p.ast.NativeRep().Expr())
	if err != nil {
		return SQL{}, err
	}
	return SQL{Where: f.sql, Args: f.args}, nil
}

func (t *translator) expr(e ast.Expr) (frag, error) {
	c, err := t.constant(e)
	if err != nil {
		return frag{}, err
	}
	if c.set {
		return t.param(c.value)
	}
	switch e.Kind() {
	case ast.IdentKind:
		return t.ident(e.AsIdent())
	case ast.CallKind:
		return t.call(e.AsCall())
	}
	return frag{}, notTranslatable("an unsupported form")
}

// constant folds the parts of an expression that read no variable, with the
// same helpers the evaluator uses, so both worlds agree.
func (t *translator) constant(e ast.Expr) (constant, error) {
	switch e.Kind() {
	case ast.LiteralKind:
		switch v := e.AsLiteral().(type) {
		case celtypes.Int:
			return constant{int64(v), true}, nil
		case celtypes.String:
			return constant{string(v), true}, nil
		case celtypes.Bool:
			return constant{bool(v), true}, nil
		case celtypes.Null:
			return constant{nil, true}, nil
		}
	case ast.IdentKind:
		if e.AsIdent() == todayVar {
			return constant{t.today, true}, nil
		}
	case ast.CallKind:
		call := e.AsCall()
		args := call.Args()
		values := make([]any, len(args))
		for i, arg := range args {
			c, err := t.constant(arg)
			if err != nil || !c.set {
				return constant{}, err
			}
			values[i] = c.value
		}
		switch call.FunctionName() {
		case "dec":
			switch v := values[0].(type) {
			case int64:
				return constant{intDecimal(v), true}, nil
			case string:
				d, err := parseDecimal(v)
				return constant{d, err == nil}, err
			}
		case "money":
			d, okd := values[0].(types.Decimal)
			code, okc := values[1].(string)
			exp, oke := values[2].(int64)
			if okd && okc && oke {
				m, err := makeMoney(d, code, exp)
				return constant{m, err == nil}, err
			}
		case "add_days", "add_months":
			day, okd := values[0].(time.Time)
			n, okn := values[1].(int64)
			if okd && okn {
				add := addDays
				if call.FunctionName() == "add_months" {
					add = addMonths
				}
				out, err := add(day, n)
				return constant{out, err == nil}, err
			}
		case operators.Negate:
			switch v := values[0].(type) {
			case int64:
				return constant{-v, v != -v || v == 0}, nil
			case types.Decimal:
				return constant{v.Neg(), true}, nil
			}
		}
	}
	return constant{}, nil
}

func (t *translator) param(v any) (frag, error) {
	switch x := v.(type) {
	case nil:
		return frag{sql: "NULL"}, nil
	case bool:
		if x {
			return frag{sql: "TRUE", kind: Bool}, nil
		}
		return frag{sql: "FALSE", kind: Bool}, nil
	case int64:
		return frag{sql: "?", args: []any{x}, kind: Int}, nil
	case string:
		return frag{sql: t.d.BinaryString("?"), args: []any{x}, kind: String}, nil
	case types.Decimal:
		text := x.String()
		if x.Scale() > 30 || len(strings.TrimLeft(text, "-")) > 36 {
			return frag{}, notTranslatable("a decimal with more than 30 decimal places or 35 digits")
		}
		return frag{sql: t.d.DecimalParam(), args: []any{text}, kind: Decimal}, nil
	case types.Money:
		return frag{sql: "?", args: []any{x.Minor}, kind: Money, cur: x.Currency.Code, exp: x.Currency.Exponent}, nil
	case time.Time:
		return frag{sql: t.d.DateParam(), args: []any{x.UTC().Format(time.DateOnly)}, kind: Date}, nil
	}
	return frag{}, notTranslatable("a value of an unsupported kind")
}

func (t *translator) ident(name string) (frag, error) {
	variable, ok := t.p.env.vars[name]
	column, mapped := t.columns[name]
	switch {
	case !ok || variable.List:
		return frag{}, notTranslatable("%s is a list", name)
	case !mapped:
		return frag{}, notTranslatable("%s has no column", name)
	}
	f := frag{sql: column, kind: variable.Kind}
	switch variable.Kind {
	case String:
		f.sql = t.d.BinaryString(column)
	case Bool:
		f.sql = "COALESCE(" + column + ", FALSE)"
	case Money:
		f.cur, f.exp = variable.Currency, variable.Scale
	}
	return f, nil
}

func join(parts ...frag) []any {
	var args []any
	for _, part := range parts {
		args = append(args, part.args...)
	}
	return args
}

// sameDomain refuses to compare amounts that may be in different currencies
// or minor units; the evaluator would raise an error for them.
func sameDomain(a, b frag) error {
	if a.kind == Money && b.kind == Money && (a.cur != b.cur || a.exp != b.exp) {
		return notTranslatable("amounts in different currencies")
	}
	return nil
}

func (t *translator) operands(args ...ast.Expr) ([]frag, error) {
	out := make([]frag, len(args))
	for i, arg := range args {
		f, err := t.expr(arg)
		if err != nil {
			return nil, err
		}
		out[i] = f
	}
	return out, nil
}

func (t *translator) call(c ast.CallExpr) (frag, error) {
	name := c.FunctionName()
	args := c.Args()
	if c.IsMemberFunction() {
		args = append([]ast.Expr{c.Target()}, args...)
	}
	switch name {
	case "startsWith":
		return t.startsWith(args)
	case "round":
		return t.round(args)
	}
	operand, err := t.operands(args...)
	if err != nil {
		return frag{}, err
	}
	switch name {
	case operators.LogicalAnd, operators.LogicalOr:
		word := map[string]string{operators.LogicalAnd: "AND", operators.LogicalOr: "OR"}[name]
		return frag{sql: "(" + operand[0].sql + " " + word + " " + operand[1].sql + ")", args: join(operand...), kind: Bool}, nil
	case operators.LogicalNot:
		return frag{sql: "(NOT " + operand[0].sql + ")", args: operand[0].args, kind: Bool}, nil
	case operators.Conditional:
		if err := sameDomain(operand[1], operand[2]); err != nil {
			return frag{}, err
		}
		kind := operand[1].kind
		if kind == "" {
			kind = operand[2].kind
		}
		return frag{sql: "CASE WHEN " + operand[0].sql + " THEN " + operand[1].sql + " ELSE " + operand[2].sql + " END", args: join(operand...), kind: kind, cur: operand[1].cur, exp: operand[1].exp}, nil
	case operators.Equals, operators.NotEquals:
		return t.equality(name == operators.NotEquals, operand)
	case operators.Less, operators.LessEquals, operators.Greater, operators.GreaterEquals:
		if err := sameDomain(operand[0], operand[1]); err != nil {
			return frag{}, err
		}
		symbol := map[string]string{operators.Less: "<", operators.LessEquals: "<=", operators.Greater: ">", operators.GreaterEquals: ">="}[name]
		return frag{sql: "COALESCE((" + operand[0].sql + " " + symbol + " " + operand[1].sql + "), FALSE)", args: join(operand...), kind: Bool}, nil
	case operators.Add, operators.Subtract, operators.Multiply:
		return t.arithmetic(name, operand)
	case operators.Negate:
		return frag{sql: "(-" + operand[0].sql + ")", args: operand[0].args, kind: operand[0].kind}, nil
	case "coalesce":
		if err := sameDomain(operand[0], operand[1]); err != nil {
			return frag{}, err
		}
		return frag{sql: "COALESCE(" + operand[0].sql + ", " + operand[1].sql + ")", args: join(operand...), kind: operand[0].kind, cur: operand[0].cur, exp: operand[0].exp}, nil
	}
	return frag{}, notTranslatable("%s has no SQL form", name)
}

func (t *translator) equality(negate bool, operand []frag) (frag, error) {
	a, b := operand[0], operand[1]
	if err := sameDomain(a, b); err != nil {
		return frag{}, err
	}
	var sql string
	var args []any
	switch {
	case a.kind == "" && b.kind == "":
		sql = "TRUE"
	case b.kind == "":
		sql, args = "("+a.sql+" IS NULL)", a.args
	case a.kind == "":
		sql, args = "("+b.sql+" IS NULL)", b.args
	default:
		sql, args = t.d.NullSafeEqual(a.sql, b.sql), join(a, b)
	}
	if negate {
		sql = "(NOT " + sql + ")"
	}
	return frag{sql: sql, args: args, kind: Bool}, nil
}

func (t *translator) arithmetic(name string, operand []frag) (frag, error) {
	a, b := operand[0], operand[1]
	symbol := map[string]string{operators.Add: "+", operators.Subtract: "-", operators.Multiply: "*"}[name]
	switch {
	case a.kind == Money || b.kind == Money:
		if name == operators.Multiply {
			return frag{}, notTranslatable("multiplying an amount rounds it in the evaluator")
		}
		if err := sameDomain(a, b); err != nil {
			return frag{}, err
		}
		return frag{sql: "(" + a.sql + " " + symbol + " " + b.sql + ")", args: join(a, b), kind: Money, cur: a.cur, exp: a.exp}, nil
	case a.kind == String || b.kind == String:
		return frag{}, notTranslatable("joining text")
	}
	kind := Decimal
	if a.kind == Int && b.kind == Int {
		kind = Int
	}
	return frag{sql: "(" + a.sql + " " + symbol + " " + b.sql + ")", args: join(a, b), kind: kind}, nil
}

func (t *translator) round(args []ast.Expr) (frag, error) {
	places, err := t.constant(args[1])
	n, ok := places.value.(int64)
	if err != nil || !ok || n < 0 || n > maxRoundTo {
		return frag{}, notTranslatable("round needs a fixed number of places")
	}
	x, err := t.expr(args[0])
	if err != nil {
		return frag{}, err
	}
	return frag{sql: "ROUND(" + x.sql + ", " + strconv.FormatInt(n, 10) + ")", args: x.args, kind: Decimal}, nil
}

func (t *translator) startsWith(args []ast.Expr) (frag, error) {
	prefix, err := t.constant(args[1])
	text, ok := prefix.value.(string)
	if err != nil || !ok {
		return frag{}, notTranslatable("startsWith needs a fixed prefix")
	}
	s, err := t.expr(args[0])
	if err != nil {
		return frag{}, err
	}
	p, _ := t.param(text)
	sql := "COALESCE((SUBSTRING(" + s.sql + ", 1, " + strconv.Itoa(utf8.RuneCountInString(text)) + ") = " + p.sql + "), FALSE)"
	return frag{sql: sql, args: join(s, p), kind: Bool}, nil
}
