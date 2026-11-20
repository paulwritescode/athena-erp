package rules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"athena-erp/internal/types"

	"go.starlark.net/starlark"
	"pgregory.net/rapid"
)

type fakeHost struct {
	sets    map[string]any
	records map[string]Row
	setErr  error
}

func (h *fakeHost) Get(recordType, id string) (Row, bool, error) {
	row, ok := h.records[recordType+"/"+id]
	return row, ok, nil
}

func (h *fakeHost) Find(recordType string, filters map[string]any, limit int) ([]Row, error) {
	var out []Row
	for key, row := range h.records {
		if strings.HasPrefix(key, recordType+"/") {
			out = append(out, row)
		}
	}
	return out, nil
}

func (h *fakeHost) Set(field string, value any) error {
	if h.setErr != nil {
		return h.setErr
	}
	if h.sets == nil {
		h.sets = map[string]any{}
	}
	h.sets[field] = value
	return nil
}

func (h *fakeHost) Currency(code string) (types.Currency, bool) {
	if code == "USD" {
		c, _ := types.NewCurrency("USD", 2, types.HalfUp)
		return c, true
	}
	return types.Currency{}, false
}

func usd(minor int64) types.Money {
	c, _ := types.NewCurrency("USD", 2, types.HalfUp)
	return types.NewMoney(minor, c)
}

func input(values map[string]any) Input {
	return Input{Rule: "r", Version: 1, Event: "validate", RecordType: "invoice", RecordID: "id1", Doc: Row{Values: values}, Now: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC), User: "ana", RequestID: "req"}
}

func compile(t *testing.T, kind Type, source string) *Program {
	t.Helper()
	p, err := Compile(kind, source, Refs{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return p
}

func TestRejectionAndWarning(t *testing.T) {
	p := compile(t, Validation, `
def validate(doc, ctx):
    warn("low", "quantity is low")
    require(doc.qty > decimal("0"), "Quantity must be above zero")
`)
	out := p.Run(context.Background(), input(map[string]any{"qty": types.MustDecimal("0")}), &fakeHost{}, DefaultLimits)
	if out.Status != Rejected || out.Code != "required" || out.Message != "Quantity must be above zero" || len(out.Warnings) != 1 {
		t.Fatalf("outcome = %+v", out)
	}
	out = p.Run(context.Background(), input(map[string]any{"qty": types.MustDecimal("2.5")}), &fakeHost{}, DefaultLimits)
	if out.Status != OK {
		t.Fatalf("valid input: %+v", out)
	}
	fail := compile(t, Validation, "def validate(doc, ctx):\n    fail(\"credit\", \"Over the limit\")\n")
	if out := fail.Run(context.Background(), input(nil), &fakeHost{}, DefaultLimits); out.Status != Rejected || out.Code != "credit" {
		t.Fatalf("fail outcome = %+v", out)
	}
}

func TestReplayIsDeterministic(t *testing.T) {
	p := compile(t, Derivation, `
def derive(doc, ctx):
    log.info("today", ctx.today, ctx.user)
    set("total", doc.qty * doc.rate)
`)
	in := input(map[string]any{"qty": types.MustDecimal("3"), "rate": types.MustDecimal("1.10")})
	first := p.Run(context.Background(), in, &fakeHost{}, DefaultLimits)
	for i := 0; i < 3; i++ {
		again := p.Run(context.Background(), in, &fakeHost{}, DefaultLimits)
		if again.InputHash != first.InputHash || again.Steps != first.Steps || len(again.Writes) != 1 || again.Writes[0].Value.(types.Decimal).String() != "3.30" || again.Logs[0].Text != first.Logs[0].Text {
			t.Fatalf("replay differs: %+v vs %+v", again, first)
		}
	}
	in.Doc.Values["qty"] = types.MustDecimal("4")
	if p.Run(context.Background(), in, &fakeHost{}, DefaultLimits).InputHash == first.InputHash {
		t.Fatal("a different input gave the same hash")
	}
}

func TestStepAndTimeLimits(t *testing.T) {
	loop := compile(t, Validation, "def validate(doc, ctx):\n    n = 0\n    for i in range(100000000):\n        n += i\n")
	out := loop.Run(context.Background(), input(nil), &fakeHost{}, DefaultLimits)
	if out.Status != Errored || out.Code != CodeStepLimit {
		t.Fatalf("runaway rule: %+v", out)
	}
	slow := DefaultLimits
	slow.MaxSteps, slow.Timeout = 1<<40, 20*time.Millisecond
	out = loop.Run(context.Background(), input(nil), &fakeHost{}, slow)
	if out.Status != Errored || out.Code != CodeTimeLimit {
		t.Fatalf("slow rule: %+v", out)
	}
}

func TestFloatsAreRejected(t *testing.T) {
	for name, source := range map[string]string{
		"literal":  "def validate(doc, ctx):\n    x = 1.5\n",
		"exponent": "def validate(doc, ctx):\n    x = 1e3\n",
		"builtin":  "def validate(doc, ctx):\n    x = float(1)\n",
		"divide":   "def validate(doc, ctx):\n    x = 3 / 2\n",
		"augment":  "def validate(doc, ctx):\n    x = 3\n    x /= 2\n",
	} {
		_, err := Compile(Validation, source, Refs{})
		var se *SourceError
		if !errors.As(err, &se) || se.Line == 0 {
			t.Errorf("%s: want a source error with a line, got %v", name, err)
		}
	}
	// A float cannot enter through the host either.
	p := compile(t, Validation, "def validate(doc, ctx):\n    decimal(1)\n")
	if out := p.Run(context.Background(), input(nil), &fakeHost{}, DefaultLimits); out.Status != OK {
		t.Fatalf("integer decimal: %+v", out)
	}
	if _, err := (&state{limits: DefaultLimits, outcome: &Outcome{}}).toGo(floatValue(1.5)); !errors.Is(err, ErrFloat) {
		t.Fatalf("toGo float = %v", err)
	}
}

func TestSandboxHasNoIOOrWrites(t *testing.T) {
	for name, source := range map[string]string{
		"set in validation": "def validate(doc, ctx):\n    set(\"a\", 1)\n",
		"load":              "load(\"x.star\", \"y\")\ndef validate(doc, ctx):\n    pass\n",
		"open":              "def validate(doc, ctx):\n    open(\"/etc/passwd\")\n",
		"while":             "def validate(doc, ctx):\n    while True:\n        pass\n",
		"set literal":       "def validate(doc, ctx):\n    x = set([1])\n",
		"no entry":          "x = 1\n",
		"wrong params":      "def validate(doc):\n    pass\n",
	} {
		if _, err := Compile(Validation, source, Refs{}); err == nil {
			t.Errorf("%s: compiled", name)
		}
	}
	if _, err := Compile(Transform, "def transform(row, ctx):\n    get(\"a\", \"b\")\n", Refs{}); err == nil {
		t.Error("a transform may not read")
	}
}

func TestRecursionStopsAtRuntime(t *testing.T) {
	p := compile(t, Validation, "def f(n):\n    return f(n)\ndef validate(doc, ctx):\n    f(1)\n")
	if out := p.Run(context.Background(), input(nil), &fakeHost{}, DefaultLimits); out.Status != Errored {
		t.Fatalf("%+v", out)
	}
}

func TestReferencesAreCheckedAtCompile(t *testing.T) {
	refs := Refs{Doc: map[string]bool{"qty": true}, Writable: map[string]bool{"total": true}}
	if _, err := Compile(Derivation, "def derive(doc, ctx):\n    set(\"total\", doc.qty)\n", refs); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		"def derive(doc, ctx):\n    x = doc.qtty\n",
		"def derive(doc, ctx):\n    set(\"docstate\", 1)\n",
	} {
		_, err := Compile(Derivation, source, refs)
		var se *SourceError
		if !errors.As(err, &se) || se.Line != 2 {
			t.Errorf("%q: %v", source, err)
		}
	}
}

func TestMoneyAndDecimalMath(t *testing.T) {
	p := compile(t, Derivation, `
def derive(doc, ctx):
    price = doc.price
    set("a", price + money(250, "USD"))
    set("b", price * decimal("1.5"))
    set("c", allocate(money(100, "USD"), [1, 1, 1])[0])
    set("d", div(1, 3, 4))
    set("e", round(decimal("2.345"), 2, "half_up"))
    set("f", price > money(50, "USD"))
`)
	host := &fakeHost{}
	out := p.Run(context.Background(), input(map[string]any{"price": usd(101)}), host, DefaultLimits)
	if out.Status != OK {
		t.Fatalf("%+v", out)
	}
	want := map[string]string{"a": "3.51 USD", "b": "1.52 USD", "c": "0.34 USD", "d": "0.3333", "e": "2.35"}
	for key, text := range want {
		if got := host.sets[key].(interface{ String() string }).String(); got != text {
			t.Errorf("%s = %s, want %s", key, got, text)
		}
	}
	if host.sets["f"] != true {
		t.Errorf("f = %v", host.sets["f"])
	}
	mismatch := compile(t, Validation, "def validate(doc, ctx):\n    x = doc.price + money(1, \"EUR\")\n")
	if out := mismatch.Run(context.Background(), input(map[string]any{"price": usd(1)}), &fakeHost{}, DefaultLimits); out.Status != Errored {
		t.Fatalf("EUR is not a known currency here: %+v", out)
	}
}

func TestHiddenFieldsAndReads(t *testing.T) {
	p := compile(t, Validation, "def validate(doc, ctx):\n    x = doc.secret\n")
	in := input(map[string]any{"secret": "x"})
	in.Doc.Hidden = map[string]bool{"secret": true}
	out := p.Run(context.Background(), in, &fakeHost{}, DefaultLimits)
	if out.Status != Errored || !strings.Contains(out.Message, "permission level") {
		t.Fatalf("hidden read: %+v", out)
	}
	read := compile(t, Validation, `
def validate(doc, ctx):
    c = get("customer", "c1")
    require(c != None, "no customer")
    require(c.name == "Acme", "wrong customer")
    require(exists("customer", {"name": "Acme"}), "none")
    require(len(find("customer")) == 1, "one")
`)
	host := &fakeHost{records: map[string]Row{"customer/c1": {Values: map[string]any{"name": "Acme"}}}}
	if out := read.Run(context.Background(), input(nil), host, DefaultLimits); out.Status != OK || out.Reads != 3 {
		t.Fatalf("reads: %+v", out)
	}
	tight := DefaultLimits
	tight.MaxReads = 2
	if out := read.Run(context.Background(), input(nil), host, tight); out.Code != CodeRowLimit {
		t.Fatalf("read limit: %+v", out)
	}
}

func TestPrintAndRecordNeverLeakValues(t *testing.T) {
	p := compile(t, Validation, "def validate(doc, ctx):\n    print(doc)\n    log.info(doc)\n")
	out := p.Run(context.Background(), input(map[string]any{"secret": "hunter2"}), &fakeHost{}, DefaultLimits)
	for _, m := range out.Logs {
		if strings.Contains(m.Text, "hunter2") {
			t.Fatalf("log leaked a value: %q", m.Text)
		}
	}
}

func TestTransformReturnsRows(t *testing.T) {
	p := compile(t, Transform, "def transform(row, ctx):\n    return {\"name\": row.name.upper(), \"qty\": decimal(row.qty) + 1}\n")
	out := p.Run(context.Background(), input(map[string]any{"name": "ana", "qty": "2"}), nil, DefaultLimits)
	if out.Status != OK || len(out.Result) != 2 || out.Result[0].Value != "ANA" || out.Result[1].Value.(types.Decimal).String() != "3" {
		t.Fatalf("%+v", out)
	}
}

func TestHostSetErrorStopsTheRule(t *testing.T) {
	p := compile(t, Derivation, "def derive(doc, ctx):\n    set(\"x\", 1)\n")
	host := &fakeHost{setErr: &HostError{Code: "invalid", Err: errors.New("x is read only")}}
	out := p.Run(context.Background(), input(nil), host, DefaultLimits)
	if out.Status != Errored || out.Code != "invalid" {
		t.Fatalf("%+v", out)
	}
}

func TestCascadeStack(t *testing.T) {
	var s Stack
	var err error
	for _, name := range []string{"a", "b", "c"} {
		if s, err = s.Enter(name, MaxCascadeDepth); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Enter("a", MaxCascadeDepth); !errors.Is(err, ErrCycle) {
		t.Fatalf("cycle = %v", err)
	}
	if _, err := s.Enter("d", MaxCascadeDepth); !errors.Is(err, ErrDepth) {
		t.Fatalf("depth = %v", err)
	}
	if _, err := (Stack{}).Enter("a", 0); !errors.Is(err, ErrDepth) {
		t.Fatalf("zero depth = %v", err)
	}
}

func floatValue(f float64) starlark.Value { return starlark.Float(f) }

func TestRepeatIsRejectedAtSaveTime(t *testing.T) {
	sources := []string{
		"def validate(doc, ctx):\n    x = [0] * 1000000000\n",
		"def validate(doc, ctx):\n    x = \"ab\" * 1000000\n",
		"def validate(doc, ctx):\n    x = (1,) * 1000000\n",
		"def validate(doc, ctx):\n    x = ([0]) * 1000000\n",
	}
	for _, source := range sources {
		_, err := Compile(Validation, source, Refs{})
		var srcErr *SourceError
		if !errors.As(err, &srcErr) || srcErr.Line != 2 || !strings.Contains(srcErr.Msg, "repeating") {
			t.Fatalf("source %q: err = %v", source, err)
		}
	}
	// Numeric multiplication is unaffected.
	p := compile(t, Validation, "def validate(doc, ctx):\n    require(doc.qty * 2 == decimal(\"6\"), \"no\")\n")
	out := p.Run(context.Background(), input(map[string]any{"qty": types.MustDecimal("3")}), &fakeHost{}, DefaultLimits)
	if out.Status != OK {
		t.Fatalf("numeric multiply: %+v", out)
	}
}

func TestRepeatThroughAVariableStopsBeforeAllocation(t *testing.T) {
	// 10000000000 is past the interpreter's pre-allocation repeat guard, so
	// this runs without allocating; 1000000000 would allocate 16 GB, which
	// is why literal repeats never reach a run.
	p := compile(t, Validation, "def validate(doc, ctx):\n    x = [0]\n    y = x * 10000000000\n")
	out := p.Run(context.Background(), input(nil), &fakeHost{}, DefaultLimits)
	if out.Status != Errored || out.Code != CodeMemoryLimit {
		t.Fatalf("variable repeat: %+v", out)
	}
}

func TestDecimalSquaringStops(t *testing.T) {
	seed := strings.Repeat("9", 65)
	p := compile(t, Derivation, "def derive(doc, ctx):\n    d = decimal(\""+seed+"\")\n    d = d * d\n    set(\"total\", d * d)\n")
	out := p.Run(context.Background(), input(nil), &fakeHost{}, DefaultLimits)
	if out.Status != Errored || out.Code != CodeMemoryLimit {
		t.Fatalf("squaring: %+v", out)
	}
}

func TestOversizedTextStopsAtTheBoundary(t *testing.T) {
	big := strings.Repeat("y", DefaultLimits.MaxValueLen+1)
	p := compile(t, Derivation, "def derive(doc, ctx):\n    set(\"note\", doc.big)\n")
	out := p.Run(context.Background(), input(map[string]any{"big": big}), &fakeHost{}, DefaultLimits)
	if out.Status != Errored || out.Code != CodeMemoryLimit {
		t.Fatalf("oversized set: %+v", out)
	}
}

func TestFindPastTheRowCapStops(t *testing.T) {
	host := &fakeHost{records: map[string]Row{}}
	// fakeHost.Find ignores limit, like a misbehaving host would.
	for i := 0; i < DefaultLimits.MaxRows+5; i++ {
		host.records["item/"+strings.Repeat("x", i+1)] = Row{Values: map[string]any{"n": int64(i)}}
	}
	p := compile(t, Validation, "def validate(doc, ctx):\n    rows = find(\"item\")\n")
	out := p.Run(context.Background(), input(nil), host, DefaultLimits)
	if out.Status != Errored || out.Code != CodeRowLimit {
		t.Fatalf("row cap: %+v", out)
	}
}

func TestValuesCapStops(t *testing.T) {
	tight := DefaultLimits
	tight.MaxValues = 3
	p := compile(t, Validation, "def validate(doc, ctx):\n    decimal(\"1\")\n    decimal(\"2\")\n    decimal(\"3\")\n")
	out := p.Run(context.Background(), input(nil), &fakeHost{}, tight)
	if out.Status != Errored || out.Code != CodeMemoryLimit {
		t.Fatalf("values cap: %+v", out)
	}
	out = p.Run(context.Background(), input(nil), &fakeHost{}, DefaultLimits)
	if out.Status != OK || out.Values != 4 {
		t.Fatalf("values count: %+v", out)
	}
}

func TestGiantIntegerStops(t *testing.T) {
	p := compile(t, Validation, "def validate(doc, ctx):\n    decimal("+strings.Repeat("9", 100)+")\n")
	out := p.Run(context.Background(), input(nil), &fakeHost{}, DefaultLimits)
	if out.Status != Errored || out.Code != CodeMemoryLimit {
		t.Fatalf("giant integer: %+v", out)
	}
}

// TestResourceCapsProperty checks Property 5 over drawn caps and workloads: a
// value past MaxValueLen or MaxValues stops with memory_limit, rows past
// MaxRows stop with row_limit, anything within caps runs, the Values count is
// exact, and Stack.Enter admits exactly maxDepth distinct names.
//
// Feature: athena-batch-2, Property 5: Resource caps.
func TestResourceCapsProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		switch rapid.IntRange(0, 2).Draw(t, "case") {
		case 0: // one value length against MaxValueLen
			maxLen := rapid.IntRange(1, 50).Draw(t, "maxLen")
			length := rapid.IntRange(0, maxLen+5).Draw(t, "length")
			limits := DefaultLimits
			limits.MaxValueLen = maxLen
			p, err := Compile(Derivation, "def derive(doc, ctx):\n    set(\"f\", \""+strings.Repeat("a", length)+"\")\n", Refs{})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			out := p.Run(context.Background(), input(nil), &fakeHost{}, limits)
			if length > maxLen {
				if out.Status != Errored || out.Code != CodeMemoryLimit {
					t.Fatalf("len %d over %d: %+v", length, maxLen, out)
				}
				return
			}
			if out.Status != OK || out.Values != 2 {
				t.Fatalf("len %d within %d: %+v", length, maxLen, out)
			}
		case 1: // set calls against MaxValues
			maxVals := rapid.IntRange(1, 30).Draw(t, "maxVals")
			n := rapid.IntRange(1, 8).Draw(t, "sets")
			limits := DefaultLimits
			limits.MaxValues = maxVals
			var body strings.Builder
			body.WriteString("def derive(doc, ctx):\n")
			for i := 0; i < n; i++ {
				fmt.Fprintf(&body, "    set(\"f%d\", \"v\")\n", i)
			}
			p, err := Compile(Derivation, body.String(), Refs{})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			out := p.Run(context.Background(), input(nil), &fakeHost{}, limits)
			if 1+n > maxVals {
				if out.Status != Errored || out.Code != CodeMemoryLimit {
					t.Fatalf("%d sets over %d: %+v", n, maxVals, out)
				}
				return
			}
			if out.Status != OK || out.Values != 1+n {
				t.Fatalf("%d sets within %d: %+v", n, maxVals, out)
			}
		default: // host rows against MaxRows
			maxRows := rapid.IntRange(1, 20).Draw(t, "maxRows")
			r := rapid.IntRange(0, maxRows+3).Draw(t, "rows")
			limits := DefaultLimits
			limits.MaxRows, limits.MaxReads = maxRows, 1000
			host := &fakeHost{records: map[string]Row{}}
			for i := 0; i < r; i++ {
				host.records["item/"+strings.Repeat("y", i+1)] = Row{Values: map[string]any{"n": int64(i)}}
			}
			p, err := Compile(Derivation, "def derive(doc, ctx):\n    find(\"item\")\n", Refs{})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			out := p.Run(context.Background(), input(nil), host, limits)
			if r > maxRows {
				if out.Status != Errored || out.Code != CodeRowLimit {
					t.Fatalf("%d rows over %d: %+v", r, maxRows, out)
				}
				return
			}
			if out.Status != OK || out.Values != 1+1+r*2 {
				t.Fatalf("%d rows within %d: %+v", r, maxRows, out)
			}
		}
	})
}

// TestCascadeDepthProperty checks the cascade half of Property 5: Enter admits
// exactly maxDepth distinct names and always refuses a running one.
func TestCascadeDepthProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxDepth := rapid.IntRange(0, 6).Draw(t, "maxDepth")
		depth := rapid.IntRange(0, 6).Draw(t, "depth")
		var s Stack
		var err error
		for i := 0; i < depth && err == nil; i++ {
			s, err = s.Enter(fmt.Sprintf("rule%d", i), maxDepth)
		}
		if depth <= maxDepth && err != nil {
			t.Fatalf("depth %d within %d: %v", depth, maxDepth, err)
		}
		if depth > maxDepth && !errors.Is(err, ErrDepth) {
			t.Fatalf("depth %d over %d: %v", depth, maxDepth, err)
		}
		if _, err := s.Enter("rule0", maxDepth); depth > 0 && maxDepth > 0 && !errors.Is(err, ErrCycle) {
			t.Fatalf("re-enter running rule: %v", err)
		}
	})
}
