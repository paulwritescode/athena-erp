package server

import (
	"context"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"

	"connectrpc.com/connect/v2"
)

// NOTE: like every DB-backed test on this branch, these stay red until
// migrations 21/22 land (see the migration 23 contiguity note).

func builderDef(key, ruleType, event string) *athenav1.RuleDefinition {
	return &athenav1.RuleDefinition{
		Key: key, Label: key, RuleType: ruleType, RecordTypeKey: "invoice", Event: event,
		Priority: 10, Enabled: true,
		ConditionTree: `{"root":{"all":true,"conditions":[{"field":"doc.qty","op":"more_than","values":["0"]}]}}`,
		Message:       "Quantity must be above zero.",
	}
}

func TestBuilderValidationRejectsAndLogs(t *testing.T) {
	f := newRuleFixture(t)
	saved, err := f.rules.SaveRule(context.Background(), &athenav1.SaveRuleRequest{Definition: builderDef("qty_positive", "validation", "validate")})
	if err != nil {
		t.Fatal(err)
	}
	if saved.GetRule().GetDefinition().GetConditionExpr() != `doc.qty > dec('0')` {
		t.Fatalf("expr = %q", saved.GetRule().GetDefinition().GetConditionExpr())
	}
	_, err = f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("0")})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(connectMessage(err), "Quantity must be above zero.") {
		t.Fatalf("message = %q", connectMessage(err))
	}
	if f.countInvoices(t) != 0 {
		t.Fatal("a rejected save left a document behind")
	}
	if _, err := f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("3")}); err != nil {
		t.Fatalf("valid save: %v", err)
	}
	runs := f.runs(t, &athenav1.ListRuleRunsRequest{RuleKey: "qty_positive"})
	if len(runs) != 2 || runs[0].GetOutcome() != "ok" || runs[1].GetOutcome() != "rejected" {
		t.Fatalf("runs = %v", runs)
	}
	if runs[1].GetMessage() != "Quantity must be above zero." {
		t.Fatalf("run message = %q", runs[1].GetMessage())
	}
}

func TestBuilderGuardBlocksSubmit(t *testing.T) {
	f := newRuleFixture(t)
	def := builderDef("needs_title", "guard", "before_submit")
	def.ConditionTree = `{"root":{"all":true,"conditions":[{"field":"doc.title","op":"is_not_empty"}]}}`
	def.Message = "Give the invoice a title before submitting."
	if _, err := f.rules.SaveRule(context.Background(), &athenav1.SaveRuleRequest{Definition: def}); err != nil {
		t.Fatal(err)
	}
	draft, err := f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("1")})
	if err != nil {
		t.Fatalf("a guard does not run on save: %v", err)
	}
	_, err = f.records.SubmitRecord(context.Background(), &athenav1.SubmitRecordRequest{RecordTypeKey: "invoice", Id: draft.GetId(), ExpectedRevision: draft.GetRevision()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(connectMessage(err), "Give the invoice a title") {
		t.Fatalf("message = %q", connectMessage(err))
	}
}

func TestBuilderDerivationSetsFields(t *testing.T) {
	f := newRuleFixture(t)
	def := builderDef("double_total", "derivation", "validate")
	def.Message = ""
	def.Sets = []*athenav1.DerivationSet{{Field: "total", ValueExpr: `doc.qty * dec('2')`}}
	if _, err := f.rules.SaveRule(context.Background(), &athenav1.SaveRuleRequest{Definition: def}); err != nil {
		t.Fatal(err)
	}
	created, err := f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("3")})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.records.GetRecord(context.Background(), &athenav1.GetRecordRequest{RecordTypeKey: "invoice", Id: created.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if total := got.GetRecord().GetValues()["total"].GetStringValue(); total != "6" {
		t.Fatalf("total = %q, want 6", total)
	}
}

func TestBuilderSaveValidation(t *testing.T) {
	f := newRuleFixture(t)
	ctx := context.Background()
	cases := []struct {
		name string
		edit func(*athenav1.RuleDefinition)
	}{
		{"both forms", func(d *athenav1.RuleDefinition) { d.Source = "def validate(doc, ctx):\n    pass\n" }},
		{"bad tree json", func(d *athenav1.RuleDefinition) { d.ConditionTree = "{oops" }},
		{"unknown field", func(d *athenav1.RuleDefinition) {
			d.ConditionTree = `{"root":{"all":true,"conditions":[{"field":"doc.nope","op":"is","values":["a"]}]}}`
		}},
		{"missing message", func(d *athenav1.RuleDefinition) { d.Message = "" }},
		{"transform tree", func(d *athenav1.RuleDefinition) { d.RuleType = "transform"; d.Event = ""; d.RecordTypeKey = "" }},
		{"derivation without sets", func(d *athenav1.RuleDefinition) { d.RuleType = "derivation"; d.Message = "" }},
		{"set unknown field", func(d *athenav1.RuleDefinition) {
			d.RuleType = "derivation"
			d.Message = ""
			d.Sets = []*athenav1.DerivationSet{{Field: "nope", ValueExpr: `dec('1')`}}
		}},
		{"set bad expression", func(d *athenav1.RuleDefinition) {
			d.RuleType = "derivation"
			d.Message = ""
			d.Sets = []*athenav1.DerivationSet{{Field: "total", ValueExpr: `doc.qty +`}}
		}},
	}
	for _, tc := range cases {
		def := builderDef("case_rule", "validation", "validate")
		tc.edit(def)
		if _, err := f.rules.SaveRule(ctx, &athenav1.SaveRuleRequest{Definition: def}); err == nil {
			t.Errorf("%s: saved", tc.name)
		}
	}
}

func TestBuilderSimulateRunsTheCondition(t *testing.T) {
	f := newRuleFixture(t)
	ctx := context.Background()
	def := builderDef("qty_positive", "validation", "validate")
	res, err := f.rules.SimulateRule(ctx, &athenav1.SimulateRuleRequest{
		Definition: def,
		Values:     map[string]*athenav1.FieldValue{"qty": text("0")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetOutcome() != "rejected" || !strings.Contains(res.GetMessage(), "Quantity must be above zero.") {
		t.Fatalf("simulate = %v", res)
	}
	ok, err := f.rules.SimulateRule(ctx, &athenav1.SimulateRuleRequest{
		Definition: def,
		Values:     map[string]*athenav1.FieldValue{"qty": text("5")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok.GetOutcome() != "ok" {
		t.Fatalf("simulate = %v", ok)
	}
}
