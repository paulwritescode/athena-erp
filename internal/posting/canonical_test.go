package posting

import (
	"testing"

	"athena-erp/internal/types"
)

// TestCanonicalDecimalText pins the hash normalization (review follow-up 4):
// equal values read alike whatever scale they carry.
func TestCanonicalDecimalText(t *testing.T) {
	for text, want := range map[string]string{
		"100":       "100",
		"100.00":    "100",
		"100.0":     "100",
		"25.50":     "25.5",
		"25.500000": "25.5",
		"0":         "0",
		"0.00":      "0",
		"0.000000":  "0",
		"-1.50":     "-1.5",
		"1.234":     "1.234",
		"1000000":   "1000000",
	} {
		if got := canonicalDecimalText(text); got != want {
			t.Fatalf("canonicalDecimalText(%q) = %q, want %q", text, got, want)
		}
	}
}

// TestHashPlans pins the R6.15 comparison: equal plans share one hash
// whatever decimal scales they carry, and any changed leg, book, date or
// order changes it.
func TestHashPlans(t *testing.T) {
	leg := func(debit, credit string) Line {
		return Line{Company: "acme", Account: "cash", DebitBase: types.MustDecimal(debit), CreditBase: types.MustDecimal(credit)}
	}
	plan := Plan{VoucherType: "bill", VoucherID: "id-1", Revision: 2, Event: EventSubmit, Book: "main", PostingDate: "2026-06-15",
		Lines: []Line{leg("100", "0"), leg("0", "100")}}
	rescaled := plan
	rescaled.Lines = []Line{leg("100.00", "0"), leg("0", "100.000")}
	first, err := HashPlans([]Plan{plan})
	if err != nil {
		t.Fatal(err)
	}
	if second, err := HashPlans([]Plan{rescaled}); err != nil || first != second {
		t.Fatalf("rescaled hash = %q, %v; want %q", second, err, first)
	}
	moved := plan
	moved.Lines = []Line{plan.Lines[1], plan.Lines[0]}
	changed := plan
	changed.Book = "audit"
	edited := plan
	edited.Lines = []Line{leg("120", "0"), leg("0", "120")}
	for _, tc := range []struct {
		name  string
		plans []Plan
	}{
		{"changed leg", []Plan{edited}},
		{"changed book", []Plan{changed}},
		{"changed order", []Plan{moved}},
		{"changed count", []Plan{plan, plan}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other, err := HashPlans(tc.plans)
			if err != nil {
				t.Fatal(err)
			}
			if other == first {
				t.Fatalf("hash did not change: %q", other)
			}
		})
	}
}
