package expr

import (
	"strings"
	"testing"
	"time"

	"athena-erp/internal/types"
)

func dec(s string) types.Decimal { return types.MustDecimal(s) }

func usd(minor int64) types.Money {
	cur, _ := types.NewCurrency("USD", 2, types.HalfUp)
	return types.NewMoney(minor, cur)
}

func testEnv(t *testing.T) *Env {
	t.Helper()
	e, err := NewEnv(
		Variable{Name: "doc.qty", Kind: Decimal}, Variable{Name: "doc.rate", Kind: Decimal},
		Variable{Name: "doc.count", Kind: Int}, Variable{Name: "doc.name", Kind: String},
		Variable{Name: "doc.paid", Kind: Bool}, Variable{Name: "doc.due", Kind: Date},
		Variable{Name: "doc.total", Kind: Money, Currency: "USD", Scale: 2}, Variable{Name: "doc.fee", Kind: Money, Currency: "USD", Scale: 2},
		Variable{Name: "doc.items.amount", Kind: Decimal, List: true}, Variable{Name: "doc.items.cost", Kind: Money, List: true, Currency: "USD", Scale: 2},
	)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

var today = time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)

func TestEvaluate(t *testing.T) {
	e := testEnv(t)
	scope := Scope{Today: today, Values: map[string]any{
		"doc.qty": dec("2.5"), "doc.rate": dec("0.10"), "doc.count": int64(3), "doc.name": "Acme Ltd", "doc.paid": true,
		"doc.due": time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), "doc.total": usd(1050), "doc.fee": usd(250),
		"doc.items.amount": []any{dec("1.10"), nil, dec("2.20")}, "doc.items.cost": []any{usd(100), usd(250)},
	}}
	tests := []struct {
		src  string
		want any
	}{
		{"doc.qty * doc.rate", dec("0.25")},
		{"0.1 + 0.2 == 0.3", nil}, // rejected: no floats; see TestRejected
		{"doc.qty + doc.rate", dec("2.6")},
		{"doc.qty * 2", dec("5")},
		{"doc.count * 2 + 1", int64(7)},
		{"doc.qty > 2", true},
		{"doc.qty == dec('2.50')", true},
		{"doc.qty == 2", false},
		{"doc.total + doc.fee", usd(1300)},
		{"doc.total - doc.fee > doc.fee", true},
		{"doc.total * dec('0.5')", usd(525)},
		{"doc.total * dec('0.333')", usd(350)},
		{"3 * doc.fee", usd(750)},
		{"amount(doc.total)", dec("10.50")},
		{"currency(doc.total)", "USD"},
		{"money(dec('12.30'), 'USD', 2) < doc.total", false},
		{"sum(doc.items.amount)", dec("3.30")},
		{"min(doc.items.amount)", dec("1.10")},
		{"max(doc.items.amount)", dec("2.20")},
		{"count(doc.items.amount)", int64(2)},
		{"sum(doc.items.cost)", usd(350)},
		{"round(doc.qty * dec('1.234'), 2)", dec("3.09")},
		{"div(dec('10'), dec('3'), 4)", dec("3.3333")},
		{"coalesce(doc.due, today())", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
		{"add_days(today(), 30)", time.Date(2026, 12, 19, 0, 0, 0, 0, time.UTC)},
		{"add_months(dec('1') == dec('1') ? add_days(today(), 72) : today(), 1)", time.Date(2027, 2, 28, 0, 0, 0, 0, time.UTC)},
		{"doc.due > today() && !doc.paid", false},
		{"doc.paid ? 'yes' : 'no'", "yes"},
		{"lower(doc.name)", "acme ltd"},
		{"doc.name.startsWith('Acme')", true},
		{"doc.name.matches('^A.*d$')", true},
		{"doc.name + '!'", "Acme Ltd!"},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			p, err := e.Compile(tc.src)
			if tc.want == nil {
				if err == nil {
					t.Fatal("expected a compile error")
				}
				return
			}
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			got, err := p.Eval(scope)
			if err != nil {
				t.Fatalf("eval: %v", err)
			}
			if !same(got, tc.want) {
				t.Fatalf("got %v (%T), want %v", got, got, tc.want)
			}
		})
	}
}

func same(a, b any) bool {
	switch x := a.(type) {
	case types.Decimal:
		y, ok := b.(types.Decimal)
		return ok && x.Cmp(y) == 0
	case types.Money:
		y, ok := b.(types.Money)
		return ok && x.Minor == y.Minor && x.Currency.Code == y.Currency.Code
	case time.Time:
		y, ok := b.(time.Time)
		return ok && x.Equal(y)
	}
	return a == b
}

func TestNullSemantics(t *testing.T) {
	e := testEnv(t)
	scope := Scope{Today: today, Values: map[string]any{"doc.rate": dec("2")}}
	tests := []struct {
		src  string
		want any
	}{
		{"doc.qty == null", true},
		{"doc.rate == null", false},
		{"doc.qty != null", false},
		{"doc.qty < 5", false},
		{"doc.qty >= 5", false},
		{"!(doc.qty < 5)", true},
		{"doc.qty * doc.rate", nil},
		{"doc.qty + 1 == null", true},
		{"coalesce(doc.qty, dec(0)) + doc.rate", dec("2")},
		{"doc.name.startsWith('a')", false},
		{"lower(doc.name) == null", true},
		{"doc.total > money(dec('1'), 'USD', 2)", false},
		{"doc.total == doc.fee", true},
		{"sum(doc.items.amount)", dec("0")},
		{"min(doc.items.amount) == null", true},
		{"count(doc.items.cost)", int64(0)},
		{"sum(doc.items.cost) == null", true},
		{"doc.due > today()", false},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			p, err := e.Compile(tc.src)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			got, err := p.Eval(scope)
			if err != nil {
				t.Fatalf("eval: %v", err)
			}
			if !same(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRejectedAtCompile(t *testing.T) {
	e := testEnv(t)
	for _, src := range []string{
		"1.5 > 1", "doc.qty / doc.rate", "doc.qty % 2", "doc.items.amount.map(a, a)", "[1, 2]", "{'a': 1}", "doc.other", "doc.qty + 'a'",
		"doc.total * doc.total", "doc.qty > doc.name", "sum(doc.qty)", "double(1)", "int('1')", "string(1)", "timestamp('2026-01-01T00:00:00Z')",
		"doc.qty", // fine as a formula but see TestKinds; compiled here only to prove a bare name is allowed
		"exists_one(x)", "size('a')", "'a' in ['a']", "doc.paid && 1", "",
	} {
		_, err := e.Compile(src)
		if src == "doc.qty" {
			if err != nil {
				t.Errorf("%q should compile: %v", src, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%q should not compile", src)
		}
	}
}

func TestResultKindAndDeps(t *testing.T) {
	e := testEnv(t)
	p, err := e.Compile("sum(doc.items.amount) + doc.qty * doc.rate > dec(0) && doc.due > today()")
	if err != nil {
		t.Fatal(err)
	}
	if p.Result != Bool || !p.UsesToday {
		t.Fatalf("result %v, today %v", p.Result, p.UsesToday)
	}
	if got := strings.Join(p.Deps, ","); got != "doc.due,doc.items.amount,doc.qty,doc.rate" {
		t.Fatalf("deps = %s", got)
	}
	if _, err := e.Compile("doc.items.amount"); err == nil {
		t.Fatal("a list is not a value")
	}
}

func TestEvaluationErrors(t *testing.T) {
	e := testEnv(t)
	scope := Scope{Today: today, Values: map[string]any{"doc.qty": dec("1"), "doc.rate": dec("0"), "doc.total": usd(1), "doc.count": int64(9223372036854775807), "doc.name": "x"}}
	eur, _ := types.NewCurrency("EUR", 2, types.HalfUp)
	scope.Values["doc.fee"] = types.NewMoney(1, eur)
	for _, src := range []string{
		"div(doc.qty, doc.rate, 2) > dec(0)", "doc.total + doc.fee > doc.total", "doc.total > doc.fee", "doc.count + 1 > 0",
		"dec('abc') > dec(0)", "round(doc.qty, 99) > dec(0)", "money(dec('1.234'), 'USD', 2) == doc.total", "doc.name.matches('(') ",
	} {
		p, err := e.Compile(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if _, err := p.Eval(scope); err == nil {
			t.Errorf("%q should fail at run time", src)
		}
	}
}

func TestDeterministic(t *testing.T) {
	e := testEnv(t)
	p, err := e.Compile("round(doc.qty * doc.rate + sum(doc.items.amount), 2)")
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{Values: map[string]any{"doc.qty": dec("3.3333"), "doc.rate": dec("7.77"), "doc.items.amount": []any{dec("0.005"), dec("0.005")}}}
	first, _ := p.Eval(scope)
	for i := 0; i < 50; i++ {
		got, err := p.Eval(scope)
		if err != nil || !same(got, first) {
			t.Fatalf("run %d: %v %v", i, got, err)
		}
	}
	// A second compile of the same text gives the same answer.
	q, _ := e.Compile(p.Source)
	if got, _ := q.Eval(scope); !same(got, first) {
		t.Fatal("recompiled expression differs")
	}
}

func TestScopeTypeConfusion(t *testing.T) {
	e := testEnv(t)
	p, _ := e.Compile("doc.qty > 1")
	if _, err := p.Eval(Scope{Values: map[string]any{"doc.qty": "x"}}); err == nil {
		t.Fatal("a string in a decimal variable must be refused")
	}
}

type fakeDialect struct{}

func (fakeDialect) NullSafeEqual(a, b string) string { return "(" + a + " <=> " + b + ")" }
func (fakeDialect) BinaryString(e string) string     { return e + " BIN" }
func (fakeDialect) DecimalParam() string             { return "DEC(?)" }
func (fakeDialect) DateParam() string                { return "DATE(?)" }

func TestTranslate(t *testing.T) {
	e := testEnv(t)
	columns := map[string]string{"doc.qty": "`qty`", "doc.rate": "`rate`", "doc.name": "`name`", "doc.total": "`total`", "doc.fee": "`fee`", "doc.paid": "`paid`", "doc.due": "`due`"}
	got, err := mustCompile(t, e, `doc.qty * doc.rate > dec("2") && doc.name.startsWith("Ac") && doc.due < add_days(today(), 3)`).Translate(fakeDialect{}, columns, today)
	if err != nil {
		t.Fatal(err)
	}
	want := "((COALESCE(((`qty` * `rate`) > DEC(?)), FALSE) AND COALESCE((SUBSTRING(`name` BIN, 1, 2) = ? BIN), FALSE)) AND COALESCE((`due` < DATE(?)), FALSE))"
	if got.Where != want || len(got.Args) != 3 || got.Args[0] != "2" || got.Args[2] != "2026-11-22" {
		t.Fatalf("where %s\nargs %v", got.Where, got.Args)
	}
	for _, src := range []string{"sum(doc.items.amount) > dec(1)", "lower(doc.name) == 'a'", "doc.name.matches('a')", "doc.total * dec('2') > doc.fee", "doc.name + 'x' == 'ax'", "div(doc.qty, doc.rate, 2) > dec(1)", "doc.total > money(dec('1'), 'USD', 3)", "add_days(doc.due, 1) > today()"} {
		if _, err := mustCompile(t, e, src).Translate(fakeDialect{}, columns, today); !IsNotTranslatable(err) {
			t.Errorf("%q: err = %v, want not translatable", src, err)
		}
	}
	if _, err := mustCompile(t, e, "doc.qty").Translate(fakeDialect{}, columns, today); err == nil {
		t.Error("a non-boolean expression cannot be a predicate")
	}
}

func mustCompile(t *testing.T, e *Env, src string) *Program {
	t.Helper()
	p, err := e.Compile(src)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	return p
}

// TestInputLimits pins the limits that guard against hostile expressions: a
// source over the size limit and one nested past the recursion limit are
// refused at compile, and the message never repeats the source.
func TestInputLimits(t *testing.T) {
	e := testEnv(t)
	long := func(terms int) string { return "doc.count" + strings.Repeat(" + 1", terms) }
	// Four characters per term: 490 terms stay under the limit, 600 go over.
	if src := long(490); len(src) > maxExprSize {
		t.Fatalf("test source is %d long, over the limit", len(src))
	}
	over := long(600)
	if len(over) <= maxExprSize {
		t.Fatalf("test source is %d long, not over the limit", len(over))
	}
	_, err := e.Compile(over)
	if err == nil {
		t.Fatal("a source over the size limit compiled")
	}
	if !strings.Contains(err.Error(), "size") || strings.Contains(err.Error(), "doc.count") {
		t.Errorf("size error should name the limit and not the source: %.120s", err)
	}
	nest := func(depth int) string { return strings.Repeat("(", depth) + "doc.paid" + strings.Repeat(")", depth) }
	if _, err := e.Compile(nest(10)); err != nil {
		t.Errorf("a shallow expression should compile: %v", err)
	}
	_, err = e.Compile(nest(40))
	if err == nil {
		t.Fatal("an expression nested past the recursion limit compiled")
	}
	if !strings.Contains(err.Error(), "recursion limit") || strings.Contains(err.Error(), "doc.paid") {
		t.Errorf("recursion error should name the limit and not the source: %.120s", err)
	}
}
