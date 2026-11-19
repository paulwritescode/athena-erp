package server

import (
	"context"
	"strings"
	"testing"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/expr"

	"connectrpc.com/connect/v2"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func fixedToday(t *testing.T) {
	t.Helper()
	previous := today
	today = func() time.Time { return time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { today = previous })
}

func numField(key, label string) *athenav1.RecordTypeField {
	return &athenav1.RecordTypeField{Key: key, Label: label, Type: "Number"}
}

func moneyField(key, label string) *athenav1.RecordTypeField {
	scale := int32(2)
	return &athenav1.RecordTypeField{Key: key, Label: label, Type: "Money", Currency: "USD", Scale: &scale}
}

func formulaOf(f *athenav1.RecordTypeField, source string) *athenav1.RecordTypeField {
	f.Formula = source
	return f
}

// valueOf returns the text of a field of a Record, "" for null.
func valueOf(r *athenav1.Record, key string) string {
	return r.GetValues()[key].GetStringValue()
}

func TestFormulasRunInDependencyOrder(t *testing.T) {
	recordTypes, records := newServices(t)
	// c reads b and b reads a, but c is declared first.
	saveRecordType(t, recordTypes, "calc",
		formulaOf(numField("c", "C"), "doc.b + 1"), formulaOf(numField("b", "B"), "doc.a * 2"), numField("a", "A"))
	record, err := create(t, records, "calc", map[string]*athenav1.FieldValue{"a": text("5")})
	if err != nil {
		t.Fatal(err)
	}
	if valueOf(record, "b") != "10" || valueOf(record, "c") != "11" {
		t.Fatalf("b = %q, c = %q", valueOf(record, "b"), valueOf(record, "c"))
	}
	updated, err := records.UpdateRecord(context.Background(), &athenav1.UpdateRecordRequest{RecordTypeKey: "calc", Id: record.GetId(), ExpectedRevision: record.GetRevision(), Values: map[string]*athenav1.FieldValue{"a": text("7.5")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.GetRecord(); valueOf(got, "b") != "15" || valueOf(got, "c") != "16" {
		t.Fatalf("after update b = %q, c = %q", valueOf(got, "b"), valueOf(got, "c"))
	}
}

func TestFormulaCycleAndBadExpressionsAreRejectedAtSave(t *testing.T) {
	recordTypes, _ := newServices(t)
	tests := map[string][]*athenav1.RecordTypeField{
		"cycle":         {formulaOf(numField("x", "X"), "doc.y + 1"), formulaOf(numField("y", "Y"), "doc.x + 1")},
		"self":          {formulaOf(numField("x", "X"), "doc.x + 1")},
		"unknown field": {formulaOf(numField("x", "X"), "doc.nope + 1")},
		"wrong kind":    {numField("a", "A"), formulaOf(numField("x", "X"), "doc.a > 1")},
		"float":         {numField("a", "A"), formulaOf(numField("x", "X"), "doc.a * 1.5")},
		"today":         {numField("a", "A"), formulaOf(&athenav1.RecordTypeField{Key: "d", Label: "D", Type: "Date"}, "today()")},
		"syntax":        {formulaOf(numField("x", "X"), "doc.a +")},
		"rule not bool": {{Key: "a", Label: "A", Type: "Number", RequiredIf: "dec(1)"}},
		"link formula":  {{Key: "a", Label: "A", Type: "Link", TargetRecordTypeKey: "calc", Formula: "'x'"}},
		"default":       {{Key: "a", Label: "A", Type: "Number", Formula: "dec(1)", DefaultValue: "1"}},
	}
	for name, fields := range tests {
		t.Run(name, func(t *testing.T) {
			err := saveType(t, recordTypes, &athenav1.RecordType{Key: "bad", Label: "Bad", Fields: fields})
			requireCode(t, err, connect.CodeInvalidArgument)
		})
	}
}

func TestClientCannotSetACalculatedField(t *testing.T) {
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "calc", numField("a", "A"), formulaOf(numField("b", "B"), "doc.a * 2"))
	_, err := create(t, records, "calc", map[string]*athenav1.FieldValue{"a": text("1"), "b": text("99")})
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestRequiredIfReadOnlyIfAndValidate(t *testing.T) {
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "rules",
		&athenav1.RecordTypeField{Key: "credit", Label: "Credit", Type: "Checkbox"},
		&athenav1.RecordTypeField{Key: "reason", Label: "Reason", Type: "Text", RequiredIf: "doc.credit"},
		&athenav1.RecordTypeField{Key: "rate", Label: "Rate", Type: "Number", Validate: "doc.rate >= dec(0)"},
		&athenav1.RecordTypeField{Key: "locked", Label: "Locked", Type: "Text", ReadOnlyIf: "doc.credit"})
	ctx := context.Background()
	_, err := create(t, records, "rules", map[string]*athenav1.FieldValue{"credit": {Kind: &athenav1.FieldValue_BoolValue{BoolValue: true}}})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = create(t, records, "rules", map[string]*athenav1.FieldValue{"rate": text("-1")})
	requireCode(t, err, connect.CodeInvalidArgument)
	plain, err := create(t, records, "rules", map[string]*athenav1.FieldValue{"locked": text("a")})
	if err != nil {
		t.Fatal(err)
	}
	credit, err := create(t, records, "rules", map[string]*athenav1.FieldValue{"credit": {Kind: &athenav1.FieldValue_BoolValue{BoolValue: true}}, "reason": text("refund"), "locked": text("a")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "rules", Id: plain.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"locked": text("b")}}); err != nil {
		t.Fatalf("unlocked record: %v", err)
	}
	_, err = records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "rules", Id: credit.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"locked": text("b")}})
	requireCode(t, err, connect.CodeInvalidArgument)
}

// orderTypes saves an order with a stored total and a line Record Type that
// the total sums. The parent is saved first because a child needs it; its
// formulas are added once the child exists.
func orderTypes(t *testing.T, recordTypes recordTypeService) {
	t.Helper()
	ctx := context.Background()
	order := &athenav1.RecordType{Key: "order", Label: "Order", Kind: "master", Fields: []*athenav1.RecordTypeField{{Key: "note", Label: "Note", Type: "Text"}, numField("grand", "Grand"), moneyField("total", "Total")}}
	saved, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: order})
	if err != nil {
		t.Fatal(err)
	}
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "line", Label: "Line", Kind: "child", ParentRecordTypeKey: "order", Fields: []*athenav1.RecordTypeField{
		numField("qty", "Qty"), numField("rate", "Rate"), formulaOf(numField("line_total", "Line total"), "round(doc.qty * doc.rate, 2)"),
		formulaOf(moneyField("amount", "Amount"), "money(round(doc.qty * doc.rate, 2), 'USD', 2)")}})
	order.Fields[1].Formula = "sum(doc.line.line_total)"
	order.Fields[2].Formula = "sum(doc.line.amount)"
	order.Revision = saved.GetRecordType().GetRevision()
	if _, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: order, ExpectedRevision: saved.GetRecordType().GetRevision()}); err != nil {
		t.Fatal(err)
	}
}

func addLine(t *testing.T, records recordService, orderID, qty, rate string) *athenav1.Record {
	t.Helper()
	res, err := records.CreateRecord(context.Background(), &athenav1.CreateRecordRequest{RecordTypeKey: "line", ParentId: orderID, Values: map[string]*athenav1.FieldValue{"qty": text(qty), "rate": text(rate)}})
	if err != nil {
		t.Fatalf("add line: %v", err)
	}
	return res.GetRecord()
}

// recomputed evaluates the rollup expressions from the child rows, never from
// the stored total, and returns them as text.
func recomputed(t *testing.T, records recordService, orderID string) (string, string) {
	t.Helper()
	var out [2]string
	for i, source := range []string{"sum(doc.line.line_total)", "sum(doc.line.amount)"} {
		res, err := records.EvaluateExpression(context.Background(), &athenav1.EvaluateExpressionRequest{RecordTypeKey: "order", Expression: "amount(" + source + ")", Id: orderID})
		if i == 0 {
			res, err = records.EvaluateExpression(context.Background(), &athenav1.EvaluateExpressionRequest{RecordTypeKey: "order", Expression: source, Id: orderID})
		}
		if err != nil {
			t.Fatal(err)
		}
		out[i] = res.GetValue()
	}
	return out[0], out[1]
}

func TestRollupEqualsRecomputationFromChildRows(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	orderTypes(t, recordTypes)
	order, err := create(t, records, "order", map[string]*athenav1.FieldValue{"note": text("n")})
	if err != nil {
		t.Fatal(err)
	}
	check := func(step, wantGrand, wantTotal string) {
		t.Helper()
		got, err := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "order", Id: order.GetId()})
		if err != nil {
			t.Fatal(err)
		}
		grand, total := recomputed(t, records, order.GetId())
		stored := got.GetRecord()
		if numeric(valueOf(stored, "grand")) != numeric(grand) || numeric(valueOf(stored, "total")) != numeric(total) {
			t.Fatalf("%s: stored %s/%s, recomputed %s/%s", step, valueOf(stored, "grand"), valueOf(stored, "total"), grand, total)
		}
		if numeric(valueOf(stored, "grand")) != wantGrand || numeric(valueOf(stored, "total")) != wantTotal {
			t.Fatalf("%s: got %s/%s, want %s/%s", step, valueOf(stored, "grand"), valueOf(stored, "total"), wantGrand, wantTotal)
		}
		if stored.GetRevision() != order.GetRevision() {
			t.Fatalf("%s: a child edit changed the parent revision to %d", step, stored.GetRevision())
		}
	}
	check("empty", "0", "")
	first := addLine(t, records, order.GetId(), "3", "1.10")
	second := addLine(t, records, order.GetId(), "2", "0.333")
	check("two lines", "3.97", "3.97")
	updated, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "line", Id: first.GetId(), ExpectedRevision: first.GetRevision(), Values: map[string]*athenav1.FieldValue{"qty": text("10")}})
	if err != nil {
		t.Fatal(err)
	}
	check("edited", "11.67", "11.67")
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "line", Id: second.GetId(), ExpectedRevision: second.GetRevision()}); err != nil {
		t.Fatal(err)
	}
	check("deleted", "11", "11")
	_ = updated
	history, err := records.ListRecordHistory(ctx, &athenav1.ListRecordHistoryRequest{RecordTypeKey: "order", Id: order.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	recalculated := 0
	for _, event := range history.GetEvents() {
		if event.GetAction() == "recalculated" {
			recalculated++
		}
	}
	if recalculated != 4 {
		t.Fatalf("recalculated audit rows = %d, want 4", recalculated)
	}
}

// numeric drops trailing zeros and the currency suffix so "11.00" equals "11".
func numeric(s string) string {
	s, _, _ = strings.Cut(s, " ")
	if strings.Contains(s, ".") {
		s = strings.TrimSuffix(strings.TrimRight(s, "0"), ".")
	}
	return s
}

func TestChangingAFormulaRecalculatesStoredRecords(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saved, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "calc", Label: "Calc", Fields: []*athenav1.RecordTypeField{numField("a", "A"), numField("b", "B")}}})
	if err != nil {
		t.Fatal(err)
	}
	record, _ := create(t, records, "calc", map[string]*athenav1.FieldValue{"a": text("4")})
	definition := saved.GetRecordType()
	definition.Fields[1].Formula = "doc.a * 3"
	plan, err := recordTypes.PlanRecordTypeSave(ctx, &athenav1.PlanRecordTypeSaveRequest{RecordType: definition, ExpectedRevision: definition.GetRevision()})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, step := range plan.GetSteps() {
		found = found || (step.GetKind() == "recalculate" && step.GetAffectedRecords() == 1)
	}
	if !found || plan.GetRequiresConfirmation() {
		t.Fatalf("plan = %v", plan)
	}
	if _, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: definition, ExpectedRevision: definition.GetRevision()}); err != nil {
		t.Fatal(err)
	}
	got, _ := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "calc", Id: record.GetId()})
	if valueOf(got.GetRecord(), "b") != "12" {
		t.Fatalf("b = %q after the formula was added", valueOf(got.GetRecord(), "b"))
	}
}

func TestChildChangeThatBreaksTheParentIsRefused(t *testing.T) {
	ctx := context.Background()
	recordTypes, _ := newServices(t)
	orderTypes(t, recordTypes)
	list, _ := recordTypes.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	var line *athenav1.RecordType
	for _, item := range list.GetRecordTypes() {
		if item.GetKey() == "line" {
			line = item
		}
	}
	line.Fields = line.Fields[:2] // drops the fields the parent sums
	_, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: line, ExpectedRevision: line.GetRevision(), ConfirmDataLoss: true})
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestPreviewRecordCalculatesAndReportsStates(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "calc",
		numField("a", "A"), formulaOf(numField("b", "B"), "doc.a * 2"),
		&athenav1.RecordTypeField{Key: "flag", Label: "Flag", Type: "Checkbox"},
		&athenav1.RecordTypeField{Key: "why", Label: "Why", Type: "Text", RequiredIf: "doc.flag", DependsOn: "doc.flag"})
	res, err := records.PreviewRecord(ctx, &athenav1.PreviewRecordRequest{RecordTypeKey: "calc", Values: map[string]*athenav1.FieldValue{"a": text("2.5"), "flag": {Kind: &athenav1.FieldValue_BoolValue{BoolValue: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetComputed()["b"].GetStringValue() != "5" {
		t.Fatalf("computed = %v", res.GetComputed())
	}
	states := map[string]*athenav1.FieldState{}
	for _, state := range res.GetStates() {
		states[state.GetKey()] = state
	}
	if !states["b"].GetReadOnly() || !states["why"].GetRequired() || states["why"].GetHidden() {
		t.Fatalf("states = %v", states)
	}
	hidden, _ := records.PreviewRecord(ctx, &athenav1.PreviewRecordRequest{RecordTypeKey: "calc", Values: map[string]*athenav1.FieldValue{"a": text("x")}})
	for _, state := range hidden.GetStates() {
		if state.GetKey() == "why" && (!state.GetHidden() || state.GetRequired()) {
			t.Fatalf("why = %v", state)
		}
	}
	if hidden.GetComputed()["b"].GetNullValue() != true {
		t.Fatalf("an unparsable input should leave the formula empty: %v", hidden.GetComputed())
	}
}

var filterFields = []*athenav1.RecordTypeField{
	{Key: "title", Label: "Title", Type: "Text"},
	numField("qty", "Qty"), numField("rate", "Rate"),
	moneyField("price", "Price"), moneyField("fee", "Fee"),
	{Key: "due", Label: "Due", Type: "Date"},
	{Key: "paid", Label: "Paid", Type: "Checkbox"},
}

// Rows cover null in every column, case and trailing-space differences and
// values that differ only in scale.
var filterRows = []map[string]string{
	{"title": "Acme", "qty": "2", "rate": "1.5", "price": "10.00", "fee": "1.00", "due": "2026-11-19", "paid": "true"},
	{"title": "acme", "qty": "2.50", "rate": "0.1", "price": "10.00", "fee": "2.00", "due": "2026-12-25", "paid": "false"},
	{"title": "Acme ", "qty": "-1", "rate": "3", "price": "0.50", "due": "2026-01-01"},
	{"title": "Ünï", "qty": "0", "price": "99.99", "fee": "99.99", "paid": "true"},
	{"qty": "1000000.000001", "rate": "0.000001", "due": "2027-03-01", "paid": "false"},
	{},
}

var filters = []string{
	`doc.qty > 1`, `doc.qty == 2`, `doc.qty == dec("2.5")`, `doc.qty != 2`, `!(doc.qty < 1)`, `doc.qty == null`, `doc.qty != null`,
	`doc.qty * doc.rate > dec("2")`, `doc.qty + doc.rate - 1 >= 0`, `coalesce(doc.rate, dec(1)) < 1`, `round(doc.qty * doc.rate, 1) == dec("0.3")`,
	`doc.title == "Acme"`, `doc.title == "acme"`, `doc.title != "Acme"`, `doc.title.startsWith("Ac")`, `doc.title.startsWith("")`, `doc.title == null`, `doc.title == "Ünï"`,
	`doc.price > doc.fee`, `doc.price == doc.fee`, `doc.price + doc.fee > money(dec("10.5"), "USD", 2)`, `doc.price <= money(dec("0.50"), "USD", 2)`, `coalesce(doc.fee, doc.price) > doc.price`,
	`doc.due < today()`, `doc.due > add_days(today(), 30)`, `doc.due >= add_months(today(), -11)`, `doc.due == null || doc.due < today()`, `doc.due == coalesce(doc.due, today())`,
	`doc.paid`, `!doc.paid`, `doc.paid && doc.qty > 1`, `doc.paid || doc.qty == null`, `doc.paid == false`,
	`doc.qty > 1 ? doc.paid : !doc.paid`, `doc.revision == 1`, `doc.revision > 0 && doc.qty == null`,
}

func TestSQLFilterReturnsTheRowsInMemoryEvaluationDoes(t *testing.T) {
	ctx := context.Background()
	fixedToday(t)
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "item", filterFields...)
	for _, row := range filterRows {
		values := map[string]*athenav1.FieldValue{}
		for key, value := range row {
			if key == "paid" {
				values[key] = &athenav1.FieldValue{Kind: &athenav1.FieldValue_BoolValue{BoolValue: value == "true"}}
				continue
			}
			values[key] = text(value)
		}
		if _, err := create(t, records, "item", values); err != nil {
			t.Fatalf("row %v: %v", row, err)
		}
	}
	all, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "item"})
	if err != nil || len(all.GetRecords()) != len(filterRows) {
		t.Fatalf("list: %v", err)
	}
	for _, source := range filters {
		t.Run(source, func(t *testing.T) {
			want := map[string]bool{}
			for _, record := range all.GetRecords() {
				res, err := records.EvaluateExpression(ctx, &athenav1.EvaluateExpressionRequest{RecordTypeKey: "item", Expression: source, Id: record.GetId()})
				if err != nil {
					t.Fatalf("evaluate: %v", err)
				}
				if !res.GetSqlTranslatable() {
					t.Fatalf("should translate: %s", res.GetSqlReason())
				}
				want[record.GetId()] = res.GetValue() == "true"
			}
			got, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "item", Filter: source})
			if err != nil {
				t.Fatalf("filter: %v", err)
			}
			found := map[string]bool{}
			for _, record := range got.GetRecords() {
				found[record.GetId()] = true
			}
			for id, expected := range want {
				if found[id] != expected {
					t.Errorf("record %s: in memory %v, in SQL %v", id, expected, found[id])
				}
			}
			if len(found) > 0 && len(want) == 0 {
				t.Error("unexpected rows")
			}
		})
	}
}

func TestFilterRefusesWhatCannotRunInTheDatabase(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	orderTypes(t, recordTypes)
	saveRecordType(t, recordTypes, "plain", numField("qty", "Qty"), &athenav1.RecordTypeField{Key: "title", Label: "Title", Type: "Text"})
	for key, source := range map[string]string{"order": `sum(doc.line.line_total) > dec(1)`, "plain": `lower(doc.title) == "a"`} {
		_, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: key, Filter: source})
		requireCode(t, err, connect.CodeInvalidArgument)
	}
	for _, source := range []string{`doc.qty`, `doc.nope > 1`, `doc.qty >`, `doc.title.matches("a")`} {
		_, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "plain", Filter: source})
		requireCode(t, err, connect.CodeInvalidArgument)
	}
}

func TestExpressionSpansNeverCarryRecordValues(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	saveRecordType(t, recordTypes, "calc", numField("a", "A"), formulaOf(numField("b", "B"), "doc.a * 2"), &athenav1.RecordTypeField{Key: "who", Label: "Who", Type: "Text"})
	const secret = "s3cret-777.125"
	record, err := create(t, records, "calc", map[string]*athenav1.FieldValue{"a": text("777.125"), "who": text(secret)})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = records.PreviewRecord(ctx, &athenav1.PreviewRecordRequest{RecordTypeKey: "calc", Id: record.GetId(), Values: map[string]*athenav1.FieldValue{"who": text(secret)}})
	_, _ = records.EvaluateExpression(ctx, &athenav1.EvaluateExpressionRequest{RecordTypeKey: "calc", Expression: `doc.who == "` + "s3cret" + `"`, Id: record.GetId()})
	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("spans = %d, want create, preview and evaluate", len(spans))
	}
	for _, span := range spans {
		for _, attribute := range span.Attributes {
			text := attribute.Value.Emit()
			if strings.Contains(text, "s3cret") || strings.Contains(text, "777.125") || strings.Contains(text, "1554.25") {
				t.Fatalf("span %s attribute %s carries a value or an expression", span.Name, attribute.Key)
			}
		}
	}
}

func TestExpressionKindsMatchFieldTypes(t *testing.T) {
	cases := map[string]expr.Kind{"Number": expr.Decimal, "Currency": expr.Decimal, "Quantity": expr.Decimal, "Money": expr.Money, "Date": expr.Date, "Checkbox": expr.Bool, "Text": expr.String, "Link": expr.String, "Enum": expr.String}
	for fieldType, want := range cases {
		if got := exprKind(recordTypeField{Type: fieldType}); got != want {
			t.Errorf("%s: %s, want %s", fieldType, got, want)
		}
	}
}
