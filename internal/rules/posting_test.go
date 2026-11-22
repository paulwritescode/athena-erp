package rules

import (
	"context"
	"strings"
	"testing"

	"athena-erp/internal/types"
)

const postingRule = `
def post(doc, ctx):
    return {
        "book": "main",
        "posting_date": "2026-11-20",
        "lines": [
            {"company": "acme", "account": "receivable", "debit": doc.total},
            {"company": "acme", "account": "revenue", "credit": doc.total,
             "dimensions": {"region": "north"}},
        ],
    }

def fixtures():
    return [
        {"name": "standard sale", "values": {"total": decimal("100.00")}},
        {"name": "with lines", "values": {"total": decimal("50.00")},
         "children": {"invoice_line": [{"item": "apples", "qty": 2}]}},
    ]
`

func TestPostingRuleCollectsItsPlan(t *testing.T) {
	p := compile(t, Posting, postingRule)
	if p.Type.Entry() != "post" {
		t.Fatalf("entry = %q", p.Type.Entry())
	}
	out := p.Run(context.Background(), input(map[string]any{"total": types.MustDecimal("100.00")}), &fakeHost{}, DefaultLimits)
	if out.Status != OK {
		t.Fatalf("outcome = %+v", out)
	}
	if out.Plan == nil {
		t.Fatalf("no plan collected: %+v", out)
	}
	plan := out.Plan
	if plan.Book != "main" || plan.PostingDate != "2026-11-20" || len(plan.Lines) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	first, second := plan.Lines[0], plan.Lines[1]
	if first.Company != "acme" || first.Account != "receivable" || first.DebitBase.String() != "100.00" || first.CreditBase.Sign() != 0 {
		t.Fatalf("first line = %+v", first)
	}
	if second.CreditBase.String() != "100.00" || second.Dimensions["region"] != "north" {
		t.Fatalf("second line = %+v", second)
	}
}

func TestPostingRuleNeedsPostAndFixtures(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"no post", "def transform(doc, ctx):\n    return {}\n\ndef fixtures():\n    return []\n", "must define def post(doc, ctx)"},
		{"no fixtures", "def post(doc, ctx):\n    return {}\n", "must define def fixtures()"},
		{"fixtures with params", "def post(doc, ctx):\n    return {}\n\ndef fixtures(doc):\n    return []\n", "fixtures must take no parameters"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Compile(Posting, tc.source, Refs{}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPostingPlanShapeErrorsNameTheLeg(t *testing.T) {
	body := func(plan string) string {
		return "def post(doc, ctx):\n    return " + plan + "\n\ndef fixtures():\n    return [{\"name\": \"a\", \"values\": {}}]\n"
	}
	for _, tc := range []struct{ name, plan, want string }{
		{"none", "None", "not nothing"},
		{"list", "[]", "must return a dict"},
		{"missing book", `{"posting_date": "2026-11-20", "lines": []}`, `needs "book"`},
		{"unknown key", `{"book": "main", "posting_date": "2026-11-20", "lines": [], "ledger": "gl"}`, `no key "ledger"`},
		{"line not dict", `{"book": "main", "posting_date": "2026-11-20", "lines": [14]}`, "line 1 must be a dict"},
		{"unknown line key", `{"book": "main", "posting_date": "2026-11-20", "lines": [{"debits": decimal("1")}]}`, `no key "debits"`},
		{"bad amount", `{"book": "main", "posting_date": "2026-11-20", "lines": [{"debit": "a lot"}]}`, "not a decimal number"},
		{"money refused", `{"book": "main", "posting_date": "2026-11-20", "lines": [{"debit": money(100, "USD")}]}`, "not money"},
		{"bad dimensions", `{"book": "main", "posting_date": "2026-11-20", "lines": [{"debit": 1, "dimensions": [1]}]}`, "dimensions must be a dict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := compile(t, Posting, body(tc.plan))
			out := p.Run(context.Background(), input(nil), &fakeHost{}, DefaultLimits)
			if out.Status != Errored || !strings.Contains(out.Message, tc.want) {
				t.Fatalf("outcome = %+v, want %q", out, tc.want)
			}
			if out.Plan != nil {
				t.Fatalf("a rejected plan must not be collected: %+v", out.Plan)
			}
		})
	}
}

func TestPostingRuleMayReadButNotSet(t *testing.T) {
	if !Posting.Reads() || Posting.Writes() {
		t.Fatalf("posting reads=%v writes=%v", Posting.Reads(), Posting.Writes())
	}
	p := compile(t, Posting, `
def post(doc, ctx):
    peer = get("invoice", "other")
    return {"book": "main", "posting_date": "2026-11-20", "lines": []}

def fixtures():
    return [{"name": "a", "values": {}}]
`)
	host := &fakeHost{records: map[string]Row{"invoice/other": {Values: map[string]any{"total": types.MustDecimal("1")}}}}
	out := p.Run(context.Background(), input(nil), host, DefaultLimits)
	if out.Status != OK || out.Reads != 1 {
		t.Fatalf("outcome = %+v", out)
	}
	if _, err := Compile(Posting, `
def post(doc, ctx):
    set("total", decimal("1"))
    return {"book": "main", "posting_date": "2026-11-20", "lines": []}

def fixtures():
    return [{"name": "a", "values": {}}]
`, Refs{}); err == nil {
		t.Fatalf("set must not compile in a posting Rule")
	}
}

func TestRunFixturesReturnsEachCase(t *testing.T) {
	p := compile(t, Posting, postingRule)
	fixtures, err := p.RunFixtures(context.Background(), DefaultLimits, (&fakeHost{}).Currency)
	if err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	if len(fixtures) != 2 || fixtures[0].Name != "standard sale" {
		t.Fatalf("fixtures = %+v", fixtures)
	}
	if total, ok := fixtures[0].Values["total"].(types.Decimal); !ok || total.String() != "100.00" {
		t.Fatalf("values = %+v", fixtures[0].Values)
	}
	rows := fixtures[1].Children["invoice_line"]
	if len(rows) != 1 || rows[0]["item"] != "apples" {
		t.Fatalf("children = %+v", fixtures[1].Children)
	}
}

func TestRunFixturesRefusesReadsAndBadShapes(t *testing.T) {
	withFixtures := func(body string) string {
		return "def post(doc, ctx):\n    return {\"book\": \"b\", \"posting_date\": \"2026-11-20\", \"lines\": []}\n\ndef fixtures():\n    return " + body + "\n"
	}
	for _, tc := range []struct{ name, body, want string }{
		{"empty", "[]", "at least one case"},
		{"not list", "{}", "must return a list"},
		{"no name", `[{"values": {}}]`, "needs a name"},
		{"dup name", `[{"name": "a", "values": {}}, {"name": "a", "values": {}}]`, `named "a"`},
		{"no values", `[{"name": "a"}]`, "needs values"},
		{"unknown key", `[{"name": "a", "values": {}, "date": "today"}]`, `no key "date"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := compile(t, Posting, withFixtures(tc.body))
			if _, err := p.RunFixtures(context.Background(), DefaultLimits, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	p := compile(t, Posting, "def post(doc, ctx):\n    return {\"book\": \"b\", \"posting_date\": \"2026-11-20\", \"lines\": []}\n\ndef fixtures():\n    return get(\"invoice\", \"x\")\n")
	if _, err := p.RunFixtures(context.Background(), DefaultLimits, nil); err == nil {
		t.Fatalf("fixtures must not read Records")
	}
	if _, err := compile(t, Validation, "def validate(doc, ctx):\n    pass\n").RunFixtures(context.Background(), DefaultLimits, nil); err == nil {
		t.Fatalf("only a posting Rule has fixtures")
	}
}
