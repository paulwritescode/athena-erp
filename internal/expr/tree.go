// Condition trees for the F4 no-code builder (ADR-0034): a JSON model the
// Desk edits, compiled to the canonical expr subset and parsed back.
// Slice 1 is literals-only: every right side is literal text (or a day
// count); field-to-field compare arrives in the next slice.
//
// The model stores variable names ("doc.<field>"); the Desk shows field
// labels. Compile validates operators against variable kinds and renders
// deterministic source; Parse accepts exactly that canonical shape, so a
// hand-written expression that merely means the same thing is refused with
// "not a builder expression" instead of guessing. Groups nest freely; the
// root holds one or more children, nested groups two or more (a nested
// single would be invisible after parentheses drop, breaking round-trip).
package expr

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/operators"
	celtypes "cel.dev/cel-go/common/types"
)

// Operator is one builder comparison.
type Operator string

const (
	OpIs           Operator = "is"
	OpIsNot        Operator = "is_not"
	OpContains     Operator = "contains"
	OpNotContains  Operator = "not_contains"
	OpStartsWith   Operator = "starts_with"
	OpIsEmpty      Operator = "is_empty"
	OpIsNotEmpty   Operator = "is_not_empty"
	OpGT           Operator = "more_than"
	OpGTE          Operator = "at_least"
	OpLT           Operator = "less_than"
	OpLTE          Operator = "at_most"
	OpIsAnyOf      Operator = "is_any_of"
	OpIsNoneOf     Operator = "is_none_of"
	OpBefore       Operator = "before"
	OpAfter        Operator = "after"
	OpOnOrBefore   Operator = "on_or_before"
	OpOnOrAfter    Operator = "on_or_after"
	OpLastNDays    Operator = "in_the_last_n_days"
	OpNextNDays    Operator = "in_the_next_n_days"
	OpIsChecked    Operator = "is_checked"
	OpIsNotChecked Operator = "is_not_checked"
)

// Condition is one builder row: a variable, an operator, literal values.
// Values holds one literal per choice (is_any_of), the day count N for the
// relative-day operators, and nothing for empty/checked operators.
type Condition struct {
	Field  string   `json:"field"`
	Op     Operator `json:"op"`
	Values []string `json:"values,omitempty"`
}

// Group joins conditions and groups with one connector: All is AND,
// !All is OR.
type Group struct {
	All        bool        `json:"all"`
	Conditions []Condition `json:"conditions,omitempty"`
	Groups     []Group     `json:"groups,omitempty"`
}

// Tree is one builder state: a root group.
type Tree struct {
	Root Group `json:"root"`
}

func (t Tree) validate() error {
	return validateGroup(t.Root, true)
}

func validateGroup(g Group, root bool) error {
	n := len(g.Conditions) + len(g.Groups)
	if root && n < 1 {
		return fmt.Errorf("the builder needs at least one condition")
	}
	if !root && n < 2 {
		return fmt.Errorf("a nested group needs at least two conditions")
	}
	for _, child := range g.Groups {
		if err := validateGroup(child, false); err != nil {
			return err
		}
	}
	return nil
}

// CompileTree renders the canonical expression for a tree. vars carries the
// variable kinds (Money needs Currency and Scale); unknown fields, operators
// a kind cannot use, and malformed values are errors naming the field.
func CompileTree(t Tree, vars map[string]Variable) (string, error) {
	if err := t.validate(); err != nil {
		return "", err
	}
	return compileGroup(t.Root, vars, true)
}

func compileGroup(g Group, vars map[string]Variable, root bool) (string, error) {
	parts := make([]string, 0, len(g.Conditions)+len(g.Groups))
	for _, c := range g.Conditions {
		part, err := compileCondition(c, vars)
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	for _, child := range g.Groups {
		part, err := compileGroup(child, vars, false)
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	joiner := " || "
	if g.All {
		joiner = " && "
	}
	out := strings.Join(parts, joiner)
	if root && len(parts) == 1 {
		return out, nil
	}
	return "(" + out + ")", nil
}

func quoteCEL(s string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), "'", "\\'") + "'"
}

func compileCondition(c Condition, vars map[string]Variable) (string, error) {
	v, ok := vars[c.Field]
	if !ok {
		return "", fmt.Errorf("unknown field %q", c.Field)
	}
	f := c.Field
	one := func() (string, error) {
		if len(c.Values) != 1 {
			return "", fmt.Errorf("%s on %q needs exactly one value", c.Op, c.Field)
		}
		return c.Values[0], nil
	}
	none := func() error {
		if len(c.Values) != 0 {
			return fmt.Errorf("%s on %q takes no value", c.Op, c.Field)
		}
		return nil
	}
	switch v.Kind {
	case String:
		return compileString(c, f, one, none)
	case Decimal:
		return compileDecimal(c, f, one, none)
	case Money:
		return compileMoney(c, f, v, one, none)
	case Date:
		return compileDate(c, f, one, none)
	case Bool:
		if err := none(); err != nil {
			return "", err
		}
		switch c.Op {
		case OpIsChecked:
			return f + " == true", nil
		case OpIsNotChecked:
			return f + " == false", nil
		}
	case Int:
		return compileInt(c, f, one, none)
	}
	return "", fmt.Errorf("%s cannot be used on %q", c.Op, c.Field)
}

func compileString(c Condition, f string, one func() (string, error), none func() error) (string, error) {
	switch c.Op {
	case OpIs, OpIsNot:
		v, err := one()
		if err != nil {
			return "", err
		}
		op := "=="
		if c.Op == OpIsNot {
			op = "!="
		}
		return f + " " + op + " " + quoteCEL(v), nil
	case OpContains, OpNotContains:
		v, err := one()
		if err != nil {
			return "", err
		}
		// Case-sensitive substring through the RE2 matcher; QuoteMeta keeps
		// user text literal. Not SQL-translatable: lists evaluate it in
		// memory and say so.
		call := f + ".matches(" + quoteCEL("(?s).*"+regexp.QuoteMeta(v)+".*") + ")"
		if c.Op == OpNotContains {
			return "!(" + call + ")", nil
		}
		return call, nil
	case OpStartsWith:
		v, err := one()
		if err != nil {
			return "", err
		}
		return f + ".startsWith(" + quoteCEL(v) + ")", nil
	case OpIsEmpty:
		if err := none(); err != nil {
			return "", err
		}
		return f + " == null", nil
	case OpIsNotEmpty:
		if err := none(); err != nil {
			return "", err
		}
		return f + " != null", nil
	case OpIsAnyOf, OpIsNoneOf:
		if len(c.Values) < 1 {
			return "", fmt.Errorf("%s on %q needs at least one value", c.Op, c.Field)
		}
		if c.Op == OpIsAnyOf {
			parts := make([]string, len(c.Values))
			for i, v := range c.Values {
				parts[i] = f + " == " + quoteCEL(v)
			}
			return "(" + strings.Join(parts, " || ") + ")", nil
		}
		parts := make([]string, len(c.Values))
		for i, v := range c.Values {
			parts[i] = f + " != " + quoteCEL(v)
		}
		return "(" + strings.Join(parts, " && ") + ")", nil
	}
	return "", fmt.Errorf("%s cannot be used on %q", c.Op, c.Field)
}

func compileDecimal(c Condition, f string, one func() (string, error), none func() error) (string, error) {
	lit := func() (string, error) {
		v, err := one()
		if err != nil {
			return "", err
		}
		d, err := parseDecimal(v)
		if err != nil {
			return "", fmt.Errorf("%s: %q is not a number", c.Field, v)
		}
		return "dec('" + d.String() + "')", nil
	}
	switch c.Op {
	case OpIs, OpIsNot, OpGT, OpGTE, OpLT, OpLTE:
		v, err := lit()
		if err != nil {
			return "", err
		}
		op := map[Operator]string{OpIs: "==", OpIsNot: "!=", OpGT: ">", OpGTE: ">=", OpLT: "<", OpLTE: "<="}[c.Op]
		return f + " " + op + " " + v, nil
	case OpIsEmpty:
		if err := none(); err != nil {
			return "", err
		}
		return f + " == null", nil
	case OpIsNotEmpty:
		if err := none(); err != nil {
			return "", err
		}
		return f + " != null", nil
	}
	return "", fmt.Errorf("%s cannot be used on %q", c.Op, c.Field)
}

func compileMoney(c Condition, f string, v Variable, one func() (string, error), none func() error) (string, error) {
	if v.Currency == "" {
		return "", fmt.Errorf("%s has no currency", c.Field)
	}
	lit := func() (string, error) {
		text, err := one()
		if err != nil {
			return "", err
		}
		d, err := parseDecimal(text)
		if err != nil {
			return "", fmt.Errorf("%s: %q is not a number", c.Field, text)
		}
		if _, err := makeMoney(d, v.Currency, int64(v.Scale)); err != nil {
			return "", fmt.Errorf("%s: %q does not fit %s", c.Field, text, v.Currency)
		}
		return "money(dec('" + d.String() + "'), '" + v.Currency + "', " + strconv.FormatInt(int64(v.Scale), 10) + ")", nil
	}
	switch c.Op {
	case OpIs, OpIsNot, OpGT, OpGTE, OpLT, OpLTE:
		value, err := lit()
		if err != nil {
			return "", err
		}
		op := map[Operator]string{OpIs: "==", OpIsNot: "!=", OpGT: ">", OpGTE: ">=", OpLT: "<", OpLTE: "<="}[c.Op]
		return f + " " + op + " " + value, nil
	case OpIsEmpty:
		if err := none(); err != nil {
			return "", err
		}
		return f + " == null", nil
	case OpIsNotEmpty:
		if err := none(); err != nil {
			return "", err
		}
		return f + " != null", nil
	}
	return "", fmt.Errorf("%s cannot be used on %q", c.Op, c.Field)
}

func compileDate(c Condition, f string, one func() (string, error), none func() error) (string, error) {
	day := func() (string, error) {
		text, err := one()
		if err != nil {
			return "", err
		}
		if _, err := time.Parse(time.DateOnly, text); err != nil {
			return "", fmt.Errorf("%s: %q is not a YYYY-MM-DD date", c.Field, text)
		}
		return "date('" + text + "')", nil
	}
	days := func() (int64, error) {
		text, err := one()
		if err != nil {
			return 0, err
		}
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("%s: %q is not a day count of 0 or more", c.Field, text)
		}
		return n, nil
	}
	switch c.Op {
	case OpIs, OpIsNot, OpBefore, OpAfter, OpOnOrBefore, OpOnOrAfter:
		value, err := day()
		if err != nil {
			return "", err
		}
		op := map[Operator]string{OpIs: "==", OpIsNot: "!=", OpBefore: "<", OpAfter: ">", OpOnOrBefore: "<=", OpOnOrAfter: ">="}[c.Op]
		return f + " " + op + " " + value, nil
	case OpLastNDays:
		n, err := days()
		if err != nil {
			return "", err
		}
		low := "add_days(today(), " + strconv.FormatInt(-n, 10) + ")"
		return "(" + f + " >= " + low + " && " + f + " <= today())", nil
	case OpNextNDays:
		n, err := days()
		if err != nil {
			return "", err
		}
		high := "add_days(today(), " + strconv.FormatInt(n, 10) + ")"
		return "(" + f + " >= today() && " + f + " <= " + high + ")", nil
	case OpIsEmpty:
		if err := none(); err != nil {
			return "", err
		}
		return f + " == null", nil
	case OpIsNotEmpty:
		if err := none(); err != nil {
			return "", err
		}
		return f + " != null", nil
	}
	return "", fmt.Errorf("%s cannot be used on %q", c.Op, c.Field)
}

func compileInt(c Condition, f string, one func() (string, error), none func() error) (string, error) {
	lit := func() (string, error) {
		text, err := one()
		if err != nil {
			return "", err
		}
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return "", fmt.Errorf("%s: %q is not a whole number", c.Field, text)
		}
		return strconv.FormatInt(n, 10), nil
	}
	switch c.Op {
	case OpIs, OpIsNot, OpGT, OpGTE, OpLT, OpLTE:
		value, err := lit()
		if err != nil {
			return "", err
		}
		op := map[Operator]string{OpIs: "==", OpIsNot: "!=", OpGT: ">", OpGTE: ">=", OpLT: "<", OpLTE: "<="}[c.Op]
		return f + " " + op + " " + value, nil
	case OpIsEmpty:
		if err := none(); err != nil {
			return "", err
		}
		return f + " == null", nil
	case OpIsNotEmpty:
		if err := none(); err != nil {
			return "", err
		}
		return f + " != null", nil
	}
	return "", fmt.Errorf("%s cannot be used on %q", c.Op, c.Field)
}

// ParseTree reads a canonical builder expression back into a tree. Only the
// shapes CompileTree emits are accepted; anything else fails with "not a
// builder expression" instead of guessing. Two normalizations apply:
// single-choice any_of/none_of become is/is_not, and a lone condition
// parses under an AND root (their compiled forms are identical).
func ParseTree(src string, vars map[string]Variable) (Tree, error) {
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)
	decls := make([]Variable, 0, len(names))
	for _, name := range names {
		decls = append(decls, vars[name])
	}
	env, err := NewEnv(decls...)
	if err != nil {
		return Tree{}, err
	}
	program, err := env.Compile(src)
	if err != nil {
		return Tree{}, fmt.Errorf("not a builder expression: %v", err)
	}
	if program.Result != Bool {
		return Tree{}, fmt.Errorf("not a builder expression: it must return yes or no")
	}
	parser := treeParser{vars: vars}
	root, err := parser.group(program.ast.NativeRep().Expr(), true)
	if err != nil {
		return Tree{}, err
	}
	return Tree{Root: root}, nil
}

func notBuilder() error {
	return fmt.Errorf("not a builder expression")
}

type treeParser struct {
	vars map[string]Variable
}

func callParts(e ast.Expr) (string, []ast.Expr, bool) {
	if e.Kind() != ast.CallKind {
		return "", nil, false
	}
	call := e.AsCall()
	args := append([]ast.Expr{}, call.Args()...)
	if call.IsMemberFunction() {
		args = append([]ast.Expr{call.Target()}, args...)
	}
	return call.FunctionName(), args, true
}

func identName(e ast.Expr) (string, bool) {
	if e.Kind() != ast.IdentKind {
		return "", false
	}
	return e.AsIdent(), true
}

// group reads one && / || level: chains split into conjuncts, relative-day
// pairs and same-field equality runs fuse into one condition each, and a
// nested level with the other connector becomes a subgroup.
func (p treeParser) group(root ast.Expr, top bool) (Group, error) {
	name, _, isCall := callParts(root)
	all := true
	chain := []ast.Expr{root}
	if isCall && (name == operators.LogicalOr || name == operators.LogicalAnd) {
		all = name == operators.LogicalAnd
		chain = flattenChain(root, name)
	} else if !top {
		return Group{}, notBuilder()
	}
	var conditions []Condition
	var groups []Group
	rest := chain
	for len(rest) > 0 {
		if all {
			if rel, n, ok := p.relativePair(rest); ok {
				conditions = append(conditions, rel)
				rest = rest[n:]
				continue
			}
			if fused, n, ok := p.fuseNoneOf(rest); ok {
				conditions = append(conditions, fused)
				rest = rest[n:]
				continue
			}
		} else if fused, n, ok := p.fuseAnyOf(rest); ok {
			conditions = append(conditions, fused)
			rest = rest[n:]
			continue
		}
		head := rest[0]
		rest = rest[1:]
		if sub, ok := p.subgroup(head, all); ok {
			child, err := p.group(sub, false)
			if err != nil {
				return Group{}, err
			}
			groups = append(groups, child)
			continue
		}
		cond, err := p.condition(head)
		if err != nil {
			return Group{}, err
		}
		conditions = append(conditions, cond)
	}
	g := Group{All: all, Conditions: conditions, Groups: groups}
	if err := validateGroup(g, top); err != nil {
		return Group{}, err
	}
	return g, nil
}

func flattenChain(root ast.Expr, name string) []ast.Expr {
	var out []ast.Expr
	var walk func(e ast.Expr)
	walk = func(e ast.Expr) {
		if n, args, ok := callParts(e); ok && n == name {
			for _, arg := range args {
				walk(arg)
			}
			return
		}
		out = append(out, e)
	}
	walk(root)
	return out
}

// subgroup recognizes a nested level: the other connector's call.
func (p treeParser) subgroup(e ast.Expr, all bool) (ast.Expr, bool) {
	name, _, ok := callParts(e)
	if !ok {
		return nil, false
	}
	if (all && name == operators.LogicalOr) || (!all && name == operators.LogicalAnd) {
		return e, true
	}
	return nil, false
}

// relativePair fuses the two conjuncts the day-range operators compile to:
// last_n_days is [f >= add_days(today(), -N), f <= today()], next_n_days is
// [f >= today(), f <= add_days(today(), N)].
func (p treeParser) relativePair(rest []ast.Expr) (Condition, int, bool) {
	if len(rest) < 2 {
		return Condition{}, 0, false
	}
	lo, hi := rest[0], rest[1]
	loField, loOp, loArg, ok := comparisonParts(lo)
	if !ok {
		return Condition{}, 0, false
	}
	hiField, hiOp, hiArg, ok := comparisonParts(hi)
	if !ok || hiField != loField {
		return Condition{}, 0, false
	}
	if _, ok := p.vars[loField]; !ok {
		return Condition{}, 0, false
	}
	if loOp == operators.GreaterEquals && hiOp == operators.LessEquals {
		if n, ok := relativeBound(loArg, true); ok && isToday(hiArg) {
			return Condition{Field: loField, Op: OpLastNDays, Values: []string{strconv.FormatInt(n, 10)}}, 2, true
		}
		if isToday(loArg) {
			if n, ok := relativeBound(hiArg, false); ok {
				return Condition{Field: loField, Op: OpNextNDays, Values: []string{strconv.FormatInt(n, 10)}}, 2, true
			}
		}
	}
	return Condition{}, 0, false
}

func comparisonParts(e ast.Expr) (field, op string, arg ast.Expr, ok bool) {
	name, args, isCall := callParts(e)
	if !isCall || len(args) != 2 {
		return "", "", nil, false
	}
	switch name {
	case operators.Equals, operators.NotEquals, operators.Greater, operators.GreaterEquals, operators.Less, operators.LessEquals:
		field, ok := identName(args[0])
		if !ok {
			return "", "", nil, false
		}
		return field, name, args[1], true
	}
	return "", "", nil, false
}

func isToday(e ast.Expr) bool {
	name, ok := identName(e)
	return ok && name == todayVar
}

// relativeBound reads add_days(today(), ±N): negative for last_n_days,
// positive for next_n_days.
func relativeBound(e ast.Expr, negative bool) (int64, bool) {
	name, args, ok := callParts(e)
	if !ok || name != "add_days" || len(args) != 2 || !isToday(args[0]) {
		return 0, false
	}
	n, ok := intOrNegated(args[1])
	if !ok {
		return 0, false
	}
	if negative {
		if n > 0 {
			return 0, false
		}
		return -n, true
	}
	if n < 0 {
		return 0, false
	}
	return n, true
}

// intOrNegated reads an integer literal, which arrives as a Negate call
// when negative.
func intOrNegated(e ast.Expr) (int64, bool) {
	if e.Kind() == ast.LiteralKind {
		if lit, ok := e.AsLiteral().(celtypes.Int); ok {
			return int64(lit), true
		}
		return 0, false
	}
	if name, args, ok := callParts(e); ok && name == operators.Negate && len(args) == 1 &&
		args[0].Kind() == ast.LiteralKind {
		if lit, ok := args[0].AsLiteral().(celtypes.Int); ok {
			return -int64(lit), true
		}
	}
	return 0, false
}

// fuseAnyOf fuses runs of two or more same-field == 'literal' under OR.
// A lone equality stays a plain is (see the normalization note on ParseTree).
func (p treeParser) fuseAnyOf(rest []ast.Expr) (Condition, int, bool) {
	return p.fuseEqualityRun(rest, operators.Equals, OpIsAnyOf)
}

// fuseNoneOf fuses runs of two or more same-field != 'literal' under AND.
func (p treeParser) fuseNoneOf(rest []ast.Expr) (Condition, int, bool) {
	return p.fuseEqualityRun(rest, operators.NotEquals, OpIsNoneOf)
}

func (p treeParser) fuseEqualityRun(rest []ast.Expr, wantOp string, op Operator) (Condition, int, bool) {
	field := ""
	var values []string
	for _, e := range rest {
		f, name, arg, ok := comparisonParts(e)
		if !ok || name != wantOp {
			break
		}
		v, isString := p.vars[f]
		if !isString || v.Kind != String {
			break
		}
		lit, ok := arg.AsLiteral().(celtypes.String)
		if !ok || arg.Kind() != ast.LiteralKind {
			break
		}
		if field == "" {
			field = f
		} else if f != field {
			break
		}
		values = append(values, string(lit))
	}
	if len(values) < 2 {
		return Condition{}, 0, false
	}
	return Condition{Field: field, Op: op, Values: values}, len(values), true
}

// condition reads one canonical comparison or call.
func (p treeParser) condition(e ast.Expr) (Condition, error) {
	if name, args, ok := callParts(e); ok && name == operators.LogicalNot && len(args) == 1 {
		field, pattern, ok := matchesCall(args[0])
		if !ok {
			return Condition{}, notBuilder()
		}
		v, known := p.vars[field]
		if !known || v.Kind != String {
			return Condition{}, notBuilder()
		}
		text, ok := unquoteContains(pattern)
		if !ok {
			return Condition{}, notBuilder()
		}
		return Condition{Field: field, Op: OpNotContains, Values: []string{text}}, nil
	}
	if field, pattern, ok := matchesCall(e); ok {
		v, known := p.vars[field]
		if !known || v.Kind != String {
			return Condition{}, notBuilder()
		}
		text, ok := unquoteContains(pattern)
		if !ok {
			return Condition{}, notBuilder()
		}
		return Condition{Field: field, Op: OpContains, Values: []string{text}}, nil
	}
	if field, value, ok := startsWithCall(e); ok {
		v, known := p.vars[field]
		if !known || v.Kind != String {
			return Condition{}, notBuilder()
		}
		return Condition{Field: field, Op: OpStartsWith, Values: []string{value}}, nil
	}
	field, op, arg, ok := comparisonParts(e)
	if !ok {
		return Condition{}, notBuilder()
	}
	v, known := p.vars[field]
	if !known {
		return Condition{}, notBuilder()
	}
	return p.compared(field, v, op, arg)
}

func matchesCall(e ast.Expr) (field, pattern string, ok bool) {
	name, args, isCall := callParts(e)
	if !isCall || name != "matches" || len(args) != 2 {
		return "", "", false
	}
	field, ok = identName(args[0])
	if !ok {
		return "", "", false
	}
	lit, ok := args[1].AsLiteral().(celtypes.String)
	if !ok || args[1].Kind() != ast.LiteralKind {
		return "", "", false
	}
	return field, string(lit), true
}

func startsWithCall(e ast.Expr) (field, value string, ok bool) {
	name, args, isCall := callParts(e)
	if !isCall || name != "startsWith" || len(args) != 2 {
		return "", "", false
	}
	field, ok = identName(args[0])
	if !ok {
		return "", "", false
	}
	lit, ok := args[1].AsLiteral().(celtypes.String)
	if !ok || args[1].Kind() != ast.LiteralKind {
		return "", "", false
	}
	return field, string(lit), true
}

// unquoteContains reverses the contains compilation: "(?s).*Q.*"
// where Q is QuoteMeta-escaped user text.
func unquoteContains(pattern string) (string, bool) {
	inner, ok := strings.CutPrefix(pattern, "(?s).*")
	if !ok {
		return "", false
	}
	inner, ok = strings.CutSuffix(inner, ".*")
	if !ok {
		return "", false
	}
	var out strings.Builder
	escaped := false
	for _, r := range inner {
		if escaped {
			out.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		out.WriteRune(r)
	}
	if escaped {
		return "", false
	}
	return out.String(), true
}

func (p treeParser) compared(field string, v Variable, op string, arg ast.Expr) (Condition, error) {
	literal := func() (any, bool) {
		if arg.Kind() != ast.LiteralKind {
			return nil, false
		}
		return arg.AsLiteral(), true
	}
	switch v.Kind {
	case String:
		raw, ok := literal()
		if !ok {
			return Condition{}, notBuilder()
		}
		if _, isNull := raw.(celtypes.Null); isNull {
			return p.nullOp(field, op)
		}
		text, ok := raw.(celtypes.String)
		if !ok {
			return Condition{}, notBuilder()
		}
		switch op {
		case operators.Equals:
			return Condition{Field: field, Op: OpIs, Values: []string{string(text)}}, nil
		case operators.NotEquals:
			return Condition{Field: field, Op: OpIsNot, Values: []string{string(text)}}, nil
		}
	case Bool:
		raw, ok := literal()
		if !ok {
			return Condition{}, notBuilder()
		}
		if _, isNull := raw.(celtypes.Null); isNull {
			return Condition{}, notBuilder()
		}
		yes, ok := raw.(celtypes.Bool)
		if !ok || op != operators.Equals {
			return Condition{}, notBuilder()
		}
		if bool(yes) {
			return Condition{Field: field, Op: OpIsChecked}, nil
		}
		return Condition{Field: field, Op: OpIsNotChecked}, nil
	case Int:
		raw, ok := literal()
		if !ok {
			if n, ok := intOrNegated(arg); ok {
				mapped, ok := intOp(op)
				if !ok {
					return Condition{}, notBuilder()
				}
				return Condition{Field: field, Op: mapped, Values: []string{strconv.FormatInt(n, 10)}}, nil
			}
			return Condition{}, notBuilder()
		}
		if _, isNull := raw.(celtypes.Null); isNull {
			return p.nullOp(field, op)
		}
		n, ok := raw.(celtypes.Int)
		if !ok {
			return Condition{}, notBuilder()
		}
		mapped, ok := intOp(op)
		if !ok {
			return Condition{}, notBuilder()
		}
		return Condition{Field: field, Op: mapped, Values: []string{strconv.FormatInt(int64(n), 10)}}, nil
	case Decimal:
		dec, ok := decimalCall(arg)
		if !ok {
			if _, isNull := literalNull(arg); isNull {
				return p.nullOp(field, op)
			}
			return Condition{}, notBuilder()
		}
		mapped, ok := orderedOp(op)
		if !ok {
			return Condition{}, notBuilder()
		}
		return Condition{Field: field, Op: mapped, Values: []string{dec}}, nil
	case Money:
		amount, code, exp, ok := moneyCall(arg)
		if !ok {
			if _, isNull := literalNull(arg); isNull {
				return p.nullOp(field, op)
			}
			return Condition{}, notBuilder()
		}
		if code != v.Currency || exp != int64(v.Scale) {
			return Condition{}, notBuilder()
		}
		mapped, ok := orderedOp(op)
		if !ok {
			return Condition{}, notBuilder()
		}
		return Condition{Field: field, Op: mapped, Values: []string{amount}}, nil
	case Date:
		day, ok := dateCall(arg)
		if !ok {
			if _, isNull := literalNull(arg); isNull {
				return p.nullOp(field, op)
			}
			return Condition{}, notBuilder()
		}
		mapped, ok := dateOp(op)
		if !ok {
			return Condition{}, notBuilder()
		}
		return Condition{Field: field, Op: mapped, Values: []string{day}}, nil
	}
	return Condition{}, notBuilder()
}

func (p treeParser) nullOp(field, op string) (Condition, error) {
	switch op {
	case operators.Equals:
		return Condition{Field: field, Op: OpIsEmpty}, nil
	case operators.NotEquals:
		return Condition{Field: field, Op: OpIsNotEmpty}, nil
	}
	return Condition{}, notBuilder()
}

func literalNull(e ast.Expr) (any, bool) {
	if e.Kind() != ast.LiteralKind {
		return nil, false
	}
	_, ok := e.AsLiteral().(celtypes.Null)
	return nil, ok
}

func intOp(op string) (Operator, bool) {
	return orderedOp(op)
}

func orderedOp(op string) (Operator, bool) {
	switch op {
	case operators.Equals:
		return OpIs, true
	case operators.NotEquals:
		return OpIsNot, true
	case operators.Greater:
		return OpGT, true
	case operators.GreaterEquals:
		return OpGTE, true
	case operators.Less:
		return OpLT, true
	case operators.LessEquals:
		return OpLTE, true
	}
	return "", false
}

func dateOp(op string) (Operator, bool) {
	switch op {
	case operators.Equals:
		return OpIs, true
	case operators.NotEquals:
		return OpIsNot, true
	case operators.Less:
		return OpBefore, true
	case operators.Greater:
		return OpAfter, true
	case operators.LessEquals:
		return OpOnOrBefore, true
	case operators.GreaterEquals:
		return OpOnOrAfter, true
	}
	return "", false
}

func decimalCall(e ast.Expr) (string, bool) {
	name, args, ok := callParts(e)
	if !ok || name != "dec" {
		return "", false
	}
	if len(args) == 1 {
		if lit, ok := args[0].AsLiteral().(celtypes.String); ok && args[0].Kind() == ast.LiteralKind {
			return string(lit), true
		}
	}
	return "", false
}

func moneyCall(e ast.Expr) (amount, code string, exp int64, ok bool) {
	name, args, ok := callParts(e)
	if !ok || name != "money" || len(args) != 3 {
		return "", "", 0, false
	}
	dec, ok := decimalCall(args[0])
	if !ok {
		return "", "", 0, false
	}
	cur, ok := args[1].AsLiteral().(celtypes.String)
	if !ok || args[1].Kind() != ast.LiteralKind {
		return "", "", 0, false
	}
	scale, ok := args[2].AsLiteral().(celtypes.Int)
	if !ok || args[2].Kind() != ast.LiteralKind {
		return "", "", 0, false
	}
	return dec, string(cur), int64(scale), true
}

func dateCall(e ast.Expr) (string, bool) {
	name, args, ok := callParts(e)
	if !ok || name != "date" || len(args) != 1 {
		return "", false
	}
	lit, ok := args[0].AsLiteral().(celtypes.String)
	if !ok || args[0].Kind() != ast.LiteralKind {
		return "", false
	}
	if _, err := time.Parse(time.DateOnly, string(lit)); err != nil {
		return "", false
	}
	return string(lit), true
}
