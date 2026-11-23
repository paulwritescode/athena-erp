package types

import (
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"
	"time"
)

var (
	usd = Currency{Code: "USD", Exponent: 2, Mode: HalfUp}
	jpy = Currency{Code: "JPY", Exponent: 0, Mode: HalfEven}
)

func TestParseDecimalRoundTrip(t *testing.T) {
	for _, s := range []string{"0", "12", "-12.340", "0.005", "-0.5", "+7.1"} {
		d, err := ParseDecimal(s)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		want := strings.TrimPrefix(s, "+")
		if d.String() != want {
			t.Errorf("%q round-trips to %q", s, d.String())
		}
	}
	for _, s := range []string{"", ".", "1.", "1e3", "NaN", "Inf", "1,000", " 1", "--1", "1.2.3", strings.Repeat("9", 66)} {
		if _, err := ParseDecimal(s); err == nil {
			t.Errorf("%q should be rejected", s)
		}
	}
}

func TestRoundingModes(t *testing.T) {
	cases := []struct {
		in    string
		mode  RoundingMode
		scale int
		want  string
	}{
		{"2.5", HalfUp, 0, "3"}, {"2.5", HalfEven, 0, "2"},
		{"3.5", HalfEven, 0, "4"}, {"-2.5", HalfUp, 0, "-3"},
		{"-2.5", HalfEven, 0, "-2"}, {"1.005", HalfUp, 2, "1.01"},
		{"1.005", HalfEven, 2, "1.00"}, {"1.2349", HalfUp, 2, "1.23"},
		{"1.5", HalfUp, 3, "1.500"},
	}
	for _, c := range cases {
		got, err := MustDecimal(c.in).Round(c.scale, c.mode)
		if err != nil || got.String() != c.want {
			t.Errorf("%s %s@%d = %v, %v; want %s", c.in, c.mode, c.scale, got, err, c.want)
		}
	}
	if _, err := MustDecimal("1.5").Round(0, "floor"); !errors.Is(err, ErrRoundingMode) {
		t.Errorf("unknown mode: %v", err)
	}
}

func TestDecimalArithmetic(t *testing.T) {
	a, b := MustDecimal("0.1"), MustDecimal("0.2")
	if got := a.Add(b); got.String() != "0.3" || !got.Equal(MustDecimal("0.30")) {
		t.Errorf("0.1+0.2 = %s", got)
	}
	if got := MustDecimal("1.5").Mul(MustDecimal("-2.25")); got.String() != "-3.375" {
		t.Errorf("mul = %s", got)
	}
	q, err := MustDecimal("1").Div(MustDecimal("3"), 4, HalfUp)
	if err != nil || q.String() != "0.3333" {
		t.Errorf("1/3 = %v, %v", q, err)
	}
	q, _ = MustDecimal("-2").Div(MustDecimal("3"), 2, HalfUp)
	if q.String() != "-0.67" {
		t.Errorf("-2/3 = %s", q)
	}
	if _, err := a.Div(Decimal{}, 2, HalfUp); !errors.Is(err, ErrDivideByZero) {
		t.Errorf("div by zero: %v", err)
	}
	if _, err := MustDecimal("1.234").Rescale(2); !errors.Is(err, ErrRounding) {
		t.Errorf("rescale loses digits: %v", err)
	}
	var zero Decimal
	if zero.String() != "0" || zero.Sign() != 0 {
		t.Errorf("zero value = %q", zero.String())
	}
}

func TestMoney(t *testing.T) {
	m, diff, err := MoneyFromDecimal(MustDecimal("10.005"), usd)
	if err != nil || m.Minor != 1001 || diff.String() != "-0.005" {
		t.Fatalf("got %v diff %v err %v", m, diff, err)
	}
	if m.String() != "10.01 USD" {
		t.Errorf("string = %s", m)
	}
	if _, err := m.Add(NewMoney(1, jpy)); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("mismatch: %v", err)
	}
	if _, err := NewMoney(1<<62, usd).Add(NewMoney(1<<62, usd)); !errors.Is(err, ErrOverflow) {
		t.Errorf("overflow: %v", err)
	}
	conv, diff, err := NewMoney(1000, usd).Convert(MustDecimal("110.555"), jpy)
	if err != nil || conv.Minor != 1106 || conv.Currency.Code != "JPY" {
		t.Fatalf("convert = %v %v %v", conv, diff, err)
	}
	// The difference plus the rounded amount rebuilds the exact product.
	if !conv.Decimal().Add(diff).Equal(MustDecimal("1105.55")) {
		t.Errorf("rounded + diff != exact: %s + %s", conv.Decimal(), diff)
	}
	if _, err := NewCurrency("usd", 2, HalfUp); err == nil {
		t.Error("lower-case code accepted")
	}
}

func TestQuantity(t *testing.T) {
	kg, _ := NewQuantity(MustDecimal("1.25"), "kg")
	if _, err := NewQuantity(MustDecimal("1"), "Kg!"); !errors.Is(err, ErrUoM) {
		t.Errorf("bad uom: %v", err)
	}
	box, _ := NewQuantity(MustDecimal("1"), "box")
	if _, err := kg.Add(box); !errors.Is(err, ErrUoMMismatch) {
		t.Errorf("mismatch: %v", err)
	}
	sum, _ := kg.Add(kg)
	if sum.String() != "2.50 kg" {
		t.Errorf("sum = %s", sum)
	}
	g, err := kg.Convert(MustDecimal("1000"), "g", 0, HalfEven)
	if err != nil || g.String() != "1250 g" {
		t.Errorf("convert = %v %v", g, err)
	}
	back, err := g.FromBase(MustDecimal("1000"), "kg", 2, HalfEven)
	if err != nil || back.String() != "1.25 kg" {
		t.Errorf("from base = %v %v", back, err)
	}
	if _, err := g.FromBase(MustDecimal("0"), "kg", 2, HalfUp); err == nil {
		t.Error("zero factor accepted")
	}
	if _, err := g.FromBase(MustDecimal("1000"), "KG", 2, HalfUp); !errors.Is(err, ErrUoM) {
		t.Errorf("bad uom: %v", err)
	}
	for _, uom := range []string{"kg", "box_2", "a"} {
		if !ValidUoM(uom) {
			t.Errorf("ValidUoM(%q) = false", uom)
		}
	}
	for _, uom := range []string{"", "Kg", "has space", "0box"} {
		if ValidUoM(uom) {
			t.Errorf("ValidUoM(%q) = true", uom)
		}
	}
}

func TestDates(t *testing.T) {
	d, err := ParseDate("2026-02-28")
	if err != nil || FormatDate(d) != "2026-02-28" {
		t.Fatalf("date: %v %v", d, err)
	}
	for _, s := range []string{"2026-02-30", "2026-2-3", "2026-02-28T00:00:00Z", ""} {
		if _, err := ParseDate(s); err == nil {
			t.Errorf("date %q accepted", s)
		}
	}
	dt, err := ParseDateTime("2026-03-01T03:30:00+03:00")
	if err != nil || dt.Location() != time.UTC || FormatDateTime(dt) != "2026-03-01T00:30:00Z" {
		t.Errorf("datetime: %v %v", dt, err)
	}
	if _, err := ParseDateTime("2026-03-01T03:30:00"); err == nil {
		t.Error("datetime without offset accepted")
	}
}

func TestAllocateExamples(t *testing.T) {
	got, err := NewMoney(100, usd).Allocate([]int64{1, 1, 1})
	if err != nil || got[0].Minor != 34 || got[1].Minor != 33 || got[2].Minor != 33 {
		t.Errorf("100/3 = %v %v", got, err)
	}
	got, _ = NewMoney(-100, usd).Allocate([]int64{1, 1, 1})
	if got[0].Minor != -34 || got[1].Minor != -33 {
		t.Errorf("-100/3 = %v", got)
	}
	got, _ = NewMoney(5, usd).Allocate([]int64{0, 3, 0, 1})
	if got[0].Minor != 0 || got[2].Minor != 0 || got[1].Minor+got[3].Minor != 5 {
		t.Errorf("zero weights got a share: %v", got)
	}
	for _, w := range [][]int64{nil, {0, 0}, {1, -1}} {
		if _, err := NewMoney(1, usd).Allocate(w); err == nil {
			t.Errorf("weights %v accepted", w)
		}
	}
	parts, err := MustDecimal("10.00").Allocate([]int64{1, 1, 1}, 2)
	if err != nil || parts[0].String() != "3.34" || parts[1].String() != "3.33" {
		t.Errorf("decimal allocate = %v %v", parts, err)
	}
	if _, err := MustDecimal("10.001").Allocate([]int64{1, 2}, 2); !errors.Is(err, ErrRounding) {
		t.Errorf("unrepresentable total: %v", err)
	}
}

// Invariant: allocation always sums back to the total exactly, and is stable.
func TestAllocateSumsExactly(t *testing.T) {
	f := func(total int64, raw []uint16) bool {
		if len(raw) == 0 {
			return true
		}
		weights := make([]int64, len(raw))
		var sum int64
		for i, w := range raw {
			weights[i] = int64(w)
			sum += int64(w)
		}
		if sum == 0 {
			return true
		}
		parts, err := NewMoney(total, usd).Allocate(weights)
		if err != nil {
			return false
		}
		acc := new(big.Int)
		for i, p := range parts {
			acc.Add(acc, big.NewInt(p.Minor))
			if weights[i] == 0 && p.Minor != 0 {
				return false
			}
		}
		again, _ := NewMoney(total, usd).Allocate(weights)
		for i := range parts {
			if parts[i].Minor != again[i].Minor {
				return false
			}
		}
		return acc.Cmp(big.NewInt(total)) == 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// Invariant: splitting and re-adding never drifts, however often it repeats.
func TestSplitAndReAddDoesNotDrift(t *testing.T) {
	f := func(total int64, n uint8) bool {
		parts := int(n%12) + 1
		weights := make([]int64, parts)
		for i := range weights {
			weights[i] = int64(i + 1)
		}
		cur := NewMoney(total, usd)
		for round := 0; round < 3; round++ {
			split, err := cur.Allocate(weights)
			if err != nil {
				return false
			}
			back := NewMoney(0, usd)
			for _, p := range split {
				if back, err = back.Add(p); err != nil {
					return false
				}
			}
			if back.Minor != cur.Minor {
				return false
			}
			cur = back
		}
		return cur.Minor == total
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// Invariant: widening a decimal's scale and rounding back is lossless, and
// money rebuilt from its decimal form is identical.
func TestScaleAndMoneyRoundTrip(t *testing.T) {
	f := func(u int64, extra uint8) bool {
		d, _ := NewDecimal(u, 2)
		wide, err := d.Round(2+int(extra%20), HalfEven)
		if err != nil {
			return false
		}
		back, err := wide.Round(2, HalfEven)
		if err != nil || back.String() != d.String() {
			return false
		}
		m := NewMoney(u, usd)
		m2, diff, err := MoneyFromDecimal(m.Decimal(), usd)
		return err == nil && m2 == m && diff.Sign() == 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// Invariant: the package never converts to binary fractions.
func TestNoBinaryFractionsInPackage(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(b)), "float") {
			t.Errorf("%s mentions float", f)
		}
	}
}
