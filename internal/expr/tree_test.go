package expr

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func treeVars() map[string]Variable {
	return map[string]Variable{
		"doc.name":     {Name: "doc.name", Kind: String},
		"doc.qty":      {Name: "doc.qty", Kind: Decimal},
		"doc.total":    {Name: "doc.total", Kind: Money, Currency: "USD", Scale: 2},
		"doc.due":      {Name: "doc.due", Kind: Date},
		"doc.paid":     {Name: "doc.paid", Kind: Bool},
		"doc.docstate": {Name: "doc.docstate", Kind: Int},
	}
}

func TestTreeRoundTrip(t *testing.T) {
	cond := func(field string, op Operator, values ...string) Condition {
		return Condition{Field: field, Op: op, Values: values}
	}
	cases := []struct {
		name string
		tree Tree
		want string
	}{
		{"string is", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.name", OpIs, "Acme")}}}, `doc.name == 'Acme'`},
		{"string is not", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.name", OpIsNot, "Acme")}}}, `doc.name != 'Acme'`},
		{"quote escape", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.name", OpIs, `o'clock\side`)}}}, `doc.name == 'o\'clock\\side'`},
		{"contains", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.name", OpContains, "ac.me")}}}, `doc.name.matches('(?s).*ac\\.me.*')`},
		{"not contains", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.name", OpNotContains, "x")}}}, `!(doc.name.matches('(?s).*x.*'))`},
		{"starts with", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.name", OpStartsWith, "Ac")}}}, `doc.name.startsWith('Ac')`},
		{"string empty", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.name", OpIsEmpty)}}}, `doc.name == null`},
		{"string not empty", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.name", OpIsNotEmpty)}}}, `doc.name != null`},
		{"any of", Tree{Root: Group{All: false, Conditions: []Condition{cond("doc.name", OpIsAnyOf, "a", "b")}}}, `(doc.name == 'a' || doc.name == 'b')`},
		{"none of", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.name", OpIsNoneOf, "a", "b")}}}, `(doc.name != 'a' && doc.name != 'b')`},
		{"decimal gt", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.qty", OpGT, "1.50")}}}, `doc.qty > dec('1.5')`},
		{"decimal empty", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.qty", OpIsEmpty)}}}, `doc.qty == null`},
		{"money lte", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.total", OpLTE, "10.50")}}}, `doc.total <= money(dec('10.5'), 'USD', 2)`},
		{"date before", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.due", OpBefore, "2026-12-01")}}}, `doc.due < date('2026-12-01')`},
		{"date on or after", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.due", OpOnOrAfter, "2026-01-01")}}}, `doc.due >= date('2026-01-01')`},
		{"last n days", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.due", OpLastNDays, "30")}}}, `(doc.due >= add_days(today(), -30) && doc.due <= today())`},
		{"next n days", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.due", OpNextNDays, "7")}}}, `(doc.due >= today() && doc.due <= add_days(today(), 7))`},
		{"checked", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.paid", OpIsChecked)}}}, `doc.paid == true`},
		{"not checked", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.paid", OpIsNotChecked)}}}, `doc.paid == false`},
		{"docstate is", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.docstate", OpIs, "1")}}}, `doc.docstate == 1`},
		{"int negative", Tree{Root: Group{All: true, Conditions: []Condition{cond("doc.docstate", OpGTE, "-2")}}}, `doc.docstate >= -2`},
		{
			"nested groups",
			Tree{Root: Group{All: true,
				Conditions: []Condition{cond("doc.paid", OpIsChecked)},
				Groups: []Group{{All: false, Conditions: []Condition{
					cond("doc.name", OpIs, "a"),
					cond("doc.qty", OpGT, "2"),
				}}},
			}},
			`(doc.paid == true && (doc.name == 'a' || doc.qty > dec('2')))`,
		},
	}
	vars := treeVars()
	for _, tc := range cases {
		src, err := CompileTree(tc.tree, vars)
		if err != nil {
			t.Fatalf("%s: compile: %v", tc.name, err)
		}
		if src != tc.want {
			t.Fatalf("%s: source = %q, want %q", tc.name, src, tc.want)
		}
		back, err := ParseTree(src, vars)
		if err != nil {
			t.Fatalf("%s: parse back: %v", tc.name, err)
		}
		// Normalize the single-condition root the parser always builds.
		if !reflect.DeepEqual(back, tc.tree) {
			t.Fatalf("%s: round trip:\n%#v\nwant:\n%#v", tc.name, back, tc.tree)
		}
		raw, err := json.Marshal(tc.tree)
		if err != nil {
			t.Fatal(err)
		}
		var stored Tree
		if err := json.Unmarshal(raw, &stored); err != nil {
			t.Fatal(err)
		}
		resrc, err := CompileTree(stored, vars)
		if err != nil || resrc != src {
			t.Fatalf("%s: JSON round trip = %q, %v", tc.name, resrc, err)
		}
	}
}

func TestTreeSingleChoiceNormalizes(t *testing.T) {
	vars := treeVars()
	src, err := CompileTree(Tree{Root: Group{All: true, Conditions: []Condition{{Field: "doc.name", Op: OpIsAnyOf, Values: []string{"a"}}}}}, vars)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseTree(src, vars)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Root.Conditions) != 1 || back.Root.Conditions[0].Op != OpIs {
		t.Fatalf("single any_of parsed as %#v, want is", back.Root.Conditions)
	}
}

func TestTreeCompileErrors(t *testing.T) {
	vars := treeVars()
	cases := []struct {
		name string
		tree Tree
		want string
	}{
		{"empty root", Tree{}, "at least one condition"},
		{"nested single", Tree{Root: Group{All: true,
			Conditions: []Condition{
				{Field: "doc.name", Op: OpIs, Values: []string{"a"}},
				{Field: "doc.name", Op: OpIsNot, Values: []string{"b"}},
			},
			Groups: []Group{
				{All: true, Conditions: []Condition{
					{Field: "doc.name", Op: OpIs, Values: []string{"c"}},
				}},
			},
		}}, "nested group needs at least two"},
		{"unknown field", Tree{Root: Group{All: true, Conditions: []Condition{{Field: "doc.nope", Op: OpIs, Values: []string{"a"}}}}}, `unknown field "doc.nope"`},
		{"op on kind", Tree{Root: Group{All: true, Conditions: []Condition{{Field: "doc.qty", Op: OpContains, Values: []string{"a"}}}}}, "cannot be used"},
		{"bad decimal", Tree{Root: Group{All: true, Conditions: []Condition{{Field: "doc.qty", Op: OpGT, Values: []string{"abc"}}}}}, "not a number"},
		{"bad date", Tree{Root: Group{All: true, Conditions: []Condition{{Field: "doc.due", Op: OpBefore, Values: []string{"yesterday"}}}}}, "not a YYYY-MM-DD date"},
		{"bad days", Tree{Root: Group{All: true, Conditions: []Condition{{Field: "doc.due", Op: OpLastNDays, Values: []string{"-1"}}}}}, "not a day count"},
		{"bad int", Tree{Root: Group{All: true, Conditions: []Condition{{Field: "doc.docstate", Op: OpIs, Values: []string{"1.5"}}}}}, "not a whole number"},
		{"money fit", Tree{Root: Group{All: true, Conditions: []Condition{{Field: "doc.total", Op: OpIs, Values: []string{"1.234"}}}}}, "does not fit USD"},
		{"value count", Tree{Root: Group{All: true, Conditions: []Condition{{Field: "doc.name", Op: OpIs}}}}, "exactly one value"},
	}
	for _, tc := range cases {
		_, err := CompileTree(tc.tree, vars)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestTreeParseRefusals(t *testing.T) {
	vars := treeVars()
	for _, src := range []string{
		`'Acme' == doc.name`,                        // field on the right
		`lower(doc.name) == 'a'`,                    // non-canonical call
		`doc.qty + dec('1') > dec('2')`,             // arithmetic
		`doc.name == 'a' && lower(doc.name) == 'a'`, // one bad conjunct
		`doc.qty > 1`,                               // bare int on a decimal
		`doc.paid != true`,                          // non-canonical bool form
		`doc.due > add_days(today(), 3)`,            // lone bound, no pair
		`doc.name`,                                  // not boolean
		`doc.nope == 'a'`,                           // unknown field
	} {
		if _, err := ParseTree(src, vars); err == nil || !strings.Contains(err.Error(), "not a builder expression") {
			t.Errorf("%q: err = %v, want not-a-builder refusal", src, err)
		}
	}
}
