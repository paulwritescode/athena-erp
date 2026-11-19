// Package expr is Athena's expression language for conditions and formulas
// (spec 23.6, ADR-0013). It is a restricted subset of CEL, built on cel-go:
// side-effect free, always terminating (no macros, so no loops, and a size
// limit), type-checked when a Record Type is saved, and exact (decimal and
// money values wrap internal/types, there is no floating point). A subset of
// the language translates to a SQL predicate through a Dialect, so a filter
// can run in the database and return the rows the in-memory evaluation would.
//
// The package reads no clock: today() is the date the caller injects. It
// never touches the database.
package expr

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"athena-erp/internal/types"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/env"
	"cel.dev/cel-go/common/operators"
	celtypes "cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
)

// Kind is the type of a value in the language.
type Kind string

const (
	Bool    Kind = "bool"
	Int     Kind = "int"
	String  Kind = "string"
	Decimal Kind = "decimal"
	Money   Kind = "money"
	Date    Kind = "date"
)

// Variable declares one name an expression may read, such as "doc.qty". A
// list variable holds one value per child Record (a rollup source). Currency
// and Scale describe a Money variable, so the SQL translator can keep amounts
// in one currency.
type Variable struct {
	Name     string
	Kind     Kind
	List     bool
	Currency string
	Scale    int
}

// Env type-checks and runs expressions over a fixed set of variables. It is
// safe for concurrent use.
type Env struct {
	cel  *cel.Env
	vars map[string]Variable
}

func celType(v Variable) (*cel.Type, error) {
	var t *cel.Type
	switch v.Kind {
	case Bool:
		t = boolT
	case Int:
		t = intT
	case String:
		t = stringT
	case Decimal:
		t = decimalT
	case Money:
		t = moneyT
	case Date:
		t = dateT
	default:
		return nil, fmt.Errorf("variable %q has an unknown kind %q", v.Name, v.Kind)
	}
	if v.List {
		if v.Kind != Decimal && v.Kind != Money {
			return nil, fmt.Errorf("variable %q: only decimal and money lists exist", v.Name)
		}
		return cel.ListType(t), nil
	}
	return t, nil
}

var todayMacro = cel.GlobalMacro("today", 0, func(eh cel.MacroExprFactory, _ ast.Expr, _ []ast.Expr) (ast.Expr, *cel.Error) {
	return eh.NewIdent(todayVar), nil
})

// NewEnv builds an environment over vars.
func NewEnv(vars ...Variable) (*Env, error) {
	stdlib := &env.LibrarySubset{DisableMacros: true}
	for _, name := range []string{operators.LogicalAnd, operators.LogicalOr, operators.LogicalNot, operators.Conditional} {
		stdlib.IncludeFunctions = append(stdlib.IncludeFunctions, &env.Function{Name: name})
	}
	options := []cel.EnvOption{
		cel.StdLib(cel.StdLibSubset(stdlib)),
		cel.Macros(todayMacro),
		cel.ParserRecursionLimit(32),
		cel.ParserExpressionSizeLimit(maxExprSize),
		cel.Variable(todayVar, dateT),
	}
	options = append(options, operatorOptions()...)
	options = append(options, functionOptions()...)
	byName := make(map[string]Variable, len(vars))
	for _, v := range vars {
		t, err := celType(v)
		if err != nil {
			return nil, err
		}
		if _, dup := byName[v.Name]; dup {
			return nil, fmt.Errorf("variable %q is declared twice", v.Name)
		}
		byName[v.Name] = v
		options = append(options, cel.Variable(v.Name, t))
	}
	e, err := cel.NewCustomEnv(options...)
	if err != nil {
		return nil, err
	}
	return &Env{cel: e, vars: byName}, nil
}

// Program is a compiled expression.
type Program struct {
	Source string
	// Result is the kind of the value the expression returns.
	Result Kind
	// Deps lists the variables the expression reads, sorted.
	Deps []string
	// UsesToday is true when the expression calls today().
	UsesToday bool

	env *Env
	ast *cel.Ast
	prg cel.Program
}

// Compile parses and type-checks source. Errors read for a person: they name
// the column and the problem and never include a value.
func (e *Env) Compile(source string) (*Program, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, errors.New("the expression is empty")
	}
	parsed, issues := e.cel.Parse(source)
	if err := issueError(issues); err != nil {
		return nil, err
	}
	if err := rejectForms(parsed.NativeRep().Expr(), false); err != nil {
		return nil, err
	}
	checked, issues := e.cel.Check(parsed)
	if err := issueError(issues); err != nil {
		return nil, err
	}
	// A declared name such as doc.qty is a field selection before the check
	// and a plain name after it; anything still selecting is not declared.
	if err := rejectForms(checked.NativeRep().Expr(), true); err != nil {
		return nil, err
	}
	kind, err := kindOf(checked.OutputType())
	if err != nil {
		return nil, err
	}
	prg, err := e.cel.Program(checked)
	if err != nil {
		return nil, err
	}
	p := &Program{Source: source, Result: kind, env: e, ast: checked, prg: prg}
	seen := map[string]bool{}
	for _, reference := range checked.NativeRep().ReferenceMap() {
		switch {
		case reference.Name == todayVar:
			p.UsesToday = true
		case reference.Name != "" && !seen[reference.Name]:
			seen[reference.Name] = true
			p.Deps = append(p.Deps, reference.Name)
		}
	}
	sort.Strings(p.Deps)
	return p, nil
}

func issueError(issues *cel.Issues) error {
	if issues == nil || issues.Err() == nil {
		return nil
	}
	var parts []string
	for _, e := range issues.Errors() {
		parts = append(parts, fmt.Sprintf("column %d: %s", e.Location.Column()+1, firstLine(e.Message)))
	}
	return errors.New(strings.Join(parts, "; "))
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// rejectForms refuses what the language leaves out: floating point, unsigned
// and byte literals, lists, maps and message construction.
func rejectForms(root ast.Expr, checked bool) error {
	var err error
	ast.PreOrderVisit(root, ast.NewExprVisitor(func(x ast.Expr) {
		if err != nil {
			return
		}
		switch x.Kind() {
		case ast.LiteralKind:
			switch x.AsLiteral().(type) {
			case celtypes.Int, celtypes.String, celtypes.Bool, celtypes.Null:
			default:
				err = errors.New("only whole numbers, text, true, false and null can be written; use dec(\"1.5\") for a decimal")
			}
		case ast.ListKind, ast.MapKind, ast.StructKind, ast.ComprehensionKind:
			err = errors.New("lists, maps and loops are not part of the language")
		case ast.SelectKind:
			if checked {
				err = errors.New("only declared names such as doc.field can be read")
			}
		}
	}))
	return err
}

func kindOf(t *cel.Type) (Kind, error) {
	switch t.String() {
	case "bool":
		return Bool, nil
	case "int":
		return Int, nil
	case "string":
		return String, nil
	case "decimal":
		return Decimal, nil
	case "money":
		return Money, nil
	case "google.protobuf.Timestamp", "timestamp":
		return Date, nil
	}
	return "", fmt.Errorf("an expression must return one value, not %s", t)
}

// Scope holds the values of an evaluation. Values uses the Go model: nil,
// bool, int64, string, types.Decimal, types.Money, time.Time (a date at UTC
// midnight) and []any for list variables. A missing scalar is null and a
// missing list is empty. Today is the date today() returns.
type Scope struct {
	Values map[string]any
	Today  time.Time
}

// Eval runs the program. The same program and scope always give the same
// result. The error never contains a Record value.
func (p *Program) Eval(scope Scope) (any, error) {
	activation := make(map[string]any, len(p.Deps)+1)
	for _, name := range p.Deps {
		variable := p.env.vars[name]
		value, present := scope.Values[name]
		if !present && variable.List {
			value = []any{}
		}
		if err := checkValue(variable, value); err != nil {
			return nil, err
		}
		activation[name] = toRef(value)
	}
	if p.UsesToday {
		if scope.Today.IsZero() {
			return nil, errors.New("today is not set")
		}
		activation[todayVar] = toRef(scope.Today.UTC().Truncate(24 * time.Hour))
	}
	out, _, err := p.prg.Eval(activation)
	if err != nil {
		return nil, errors.New(evalMessage(err))
	}
	return fromRef(out)
}

func evalMessage(err error) string {
	message := err.Error()
	if strings.Contains(message, "no such overload") {
		return "the expression cannot be applied to these values"
	}
	return message
}

// checkValue guards the type confusion a wrong scope would cause.
func checkValue(v Variable, value any) error {
	if v.List {
		list, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be a list", v.Name)
		}
		for _, item := range list {
			if err := checkScalar(v, item); err != nil {
				return err
			}
		}
		return nil
	}
	return checkScalar(v, value)
}

func checkScalar(v Variable, value any) error {
	ok := value == nil
	switch value.(type) {
	case bool:
		ok = v.Kind == Bool
	case int64:
		ok = v.Kind == Int
	case string:
		ok = v.Kind == String
	case types.Decimal:
		ok = v.Kind == Decimal
	case types.Money:
		ok = v.Kind == Money
	case time.Time:
		ok = v.Kind == Date
	}
	if !ok {
		return fmt.Errorf("the value of %s is not a %s", v.Name, v.Kind)
	}
	return nil
}

var _ ref.Val = decimalVal{}
