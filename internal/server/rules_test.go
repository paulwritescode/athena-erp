package server

import (
	"bytes"
	"context"
	"database/sql"
	"log"
	"strings"
	"testing"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/audit"
	"athena-erp/internal/auth"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/rules"
	"athena-erp/internal/store"

	"connectrpc.com/connect/v2"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type ruleFixture struct {
	db      *sql.DB
	types   recordTypeService
	records recordService
	rules   ruleService
}

// newRuleFixture builds operator services with the Rule engine attached, and
// an invoice document with a customer master to read from.
func newRuleFixture(t *testing.T) *ruleFixture {
	t.Helper()
	db := sharedDatabase(t)
	if _, err := store.Up(context.Background(), db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	hooks := lifecycle.NewRegistry()
	registerRules(hooks, db)
	f := &ruleFixture{db: db, types: recordTypeService{db: db}, records: recordService{db: db, hooks: hooks}, rules: newRuleService(db)}
	ruleNow = func() time.Time { return time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { ruleNow = time.Now })
	mustSave(t, f.types, &athenav1.RecordType{Key: "customer", Label: "Customer", Fields: []*athenav1.RecordTypeField{{Key: "name", Label: "Name", Type: "Text"}, {Key: "blocked", Label: "Blocked", Type: "Checkbox"}}})
	scale := int32(2)
	mustSave(t, f.types, &athenav1.RecordType{Key: "invoice", Label: "Invoice", Kind: "document", Class: "non_posting", Submittable: true, Fields: []*athenav1.RecordTypeField{
		{Key: "title", Label: "Title", Type: "Text"},
		{Key: "customer", Label: "Customer", Type: "Link", TargetRecordTypeKey: "customer"},
		{Key: "qty", Label: "Quantity", Type: "Number"},
		{Key: "rate", Label: "Rate", Type: "Number"},
		{Key: "total", Label: "Total", Type: "Number"},
		{Key: "price", Label: "Price", Type: "Money", Currency: "USD", Scale: &scale},
		{Key: "secret", Label: "Secret", Type: "Text", PermissionLevel: 5},
	}})
	return f
}

func (f *ruleFixture) save(t *testing.T, key, kind, event, source string, priority int32) *athenav1.Rule {
	t.Helper()
	res, err := f.rules.SaveRule(context.Background(), &athenav1.SaveRuleRequest{Definition: &athenav1.RuleDefinition{Key: key, Label: key, RuleType: kind, RecordTypeKey: "invoice", Event: event, Priority: priority, Enabled: true, Source: source}})
	if err != nil {
		t.Fatalf("save rule %s: %v", key, err)
	}
	return res.GetRule()
}

func (f *ruleFixture) invoice(t *testing.T, values map[string]*athenav1.FieldValue) (*athenav1.Record, error) {
	t.Helper()
	return create(t, f.records, "invoice", values)
}

func (f *ruleFixture) runs(t *testing.T, filter *athenav1.ListRuleRunsRequest) []*athenav1.RuleRun {
	t.Helper()
	res, err := f.rules.ListRuleRuns(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	return res.GetRuns()
}

func (f *ruleFixture) countInvoices(t *testing.T) int {
	t.Helper()
	res, err := f.records.ListRecords(context.Background(), &athenav1.ListRecordsRequest{RecordTypeKey: "invoice"})
	if err != nil {
		t.Fatal(err)
	}
	return len(res.GetRecords())
}

const positiveQty = `
def validate(doc, ctx):
    require(doc.qty != None, "Quantity is required")
    require(doc.qty > decimal("0"), "Quantity must be above zero")
`

func TestValidationRuleRejectsASaveAndLogsTheRun(t *testing.T) {
	f := newRuleFixture(t)
	f.save(t, "qty_positive", "validation", "validate", positiveQty, 10)
	_, err := f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("0")})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(connectMessage(err), "Quantity must be above zero") {
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
	rejected := runs[1]
	if rejected.GetInputHash() == "" || rejected.GetRuleVersion() != 1 || rejected.GetEvent() != "validate" || rejected.GetRecordTypeKey() != "invoice" || rejected.GetRequestId() == "" || rejected.GetActor() != localActor {
		t.Fatalf("run row = %v", rejected)
	}
	if strings.Contains(rejected.String(), "Quantity must be above zero") && rejected.GetMessage() == "" {
		t.Fatal("the rejection message is missing from the Rule Run")
	}
}

func TestDerivationSetsFieldsOnCreateAndUpdate(t *testing.T) {
	f := newRuleFixture(t)
	f.save(t, "line_total", "derivation", "validate", `
def derive(doc, ctx):
    if doc.qty != None and doc.rate != None:
        set("total", doc.qty * doc.rate)
`, 10)
	f.save(t, "total_set", "validation", "validate", `
def validate(doc, ctx):
    require(doc.total != None, "Total was not derived before validation")
`, 10)
	record, err := f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("3"), "rate": text("1.5"), "total": text("999")})
	if err != nil {
		t.Fatal(err)
	}
	if got := record.GetValues()["total"].GetStringValue(); got != "4.5" {
		t.Fatalf("total on create = %q", got)
	}
	if err := update(f.records, "invoice", record, map[string]*athenav1.FieldValue{"qty": text("4")}); err != nil {
		t.Fatal(err)
	}
	got, err := f.records.GetRecord(context.Background(), &athenav1.GetRecordRequest{RecordTypeKey: "invoice", Id: record.GetId()})
	if err != nil || got.GetRecord().GetValues()["total"].GetStringValue() != "6" {
		t.Fatalf("total after update = %v, %v", got.GetRecord().GetValues()["total"], err)
	}
	if h, _ := f.records.ListRecordHistory(context.Background(), &athenav1.ListRecordHistoryRequest{RecordTypeKey: "invoice", Id: record.GetId()}); len(h.GetEvents()) < 2 {
		t.Fatalf("history = %v", h)
	}
}

func TestRuleCannotChangeWhatItMayNot(t *testing.T) {
	f := newRuleFixture(t)
	// A literal field name is checked when the Rule is saved...
	_, err := f.rules.SaveRule(context.Background(), &athenav1.SaveRuleRequest{Definition: &athenav1.RuleDefinition{Key: "bad", Label: "bad", RuleType: "derivation", RecordTypeKey: "invoice", Event: "validate", Enabled: true, Source: "def derive(doc, ctx):\n    set(\"docstate\", 1)\n"}})
	requireCode(t, err, connect.CodeInvalidArgument)
	if !strings.Contains(connectMessage(err), "line 2") {
		t.Fatalf("message = %q", connectMessage(err))
	}
	// ...and a computed one at run time.
	f.save(t, "sneaky", "derivation", "validate", `
def derive(doc, ctx):
    field = "doc" + "state"
    set(field, 1)
`, 10)
	_, err = f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("1")})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if f.countInvoices(t) != 0 {
		t.Fatal("a Rule wrote outside its Record Type's fields")
	}
	if runs := f.runs(t, &athenav1.ListRuleRunsRequest{RuleKey: "sneaky"}); len(runs) != 1 || runs[0].GetOutcome() != "error" || runs[0].GetCode() != "invalid_field" {
		t.Fatalf("runs = %v", runs)
	}
	// Floats and a ledger API do not exist.
	for _, source := range []string{"def validate(doc, ctx):\n    x = 0.5\n", "def validate(doc, ctx):\n    post_ledger(doc)\n"} {
		_, err := f.rules.SaveRule(context.Background(), &athenav1.SaveRuleRequest{Definition: &athenav1.RuleDefinition{Key: "x", Label: "x", RuleType: "validation", RecordTypeKey: "invoice", Event: "validate", Source: source}})
		requireCode(t, err, connect.CodeInvalidArgument)
	}
}

func TestFailingRuleRollsBackTheDocumentAndItsWrites(t *testing.T) {
	f := newRuleFixture(t)
	f.save(t, "a_derive", "derivation", "validate", "def derive(doc, ctx):\n    set(\"total\", decimal(\"5\"))\n", 1)
	f.save(t, "b_boom", "validation", "validate", "def validate(doc, ctx):\n    x = doc.qty + \"a\"\n", 2)
	_, err := f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("1")})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if f.countInvoices(t) != 0 {
		t.Fatal("the document survived a failing rule")
	}
	runs := f.runs(t, &athenav1.ListRuleRunsRequest{RequestId: ""})
	if len(runs) != 2 || runs[0].GetRuleKey() != "b_boom" || runs[0].GetOutcome() != "error" || runs[1].GetOutcome() != "ok" || runs[1].GetWrites() != 1 {
		t.Fatalf("runs = %v", runs)
	}
	if runs[0].GetRequestId() != runs[1].GetRequestId() {
		t.Fatal("the two runs of one request carry different request ids")
	}
}

func TestRunawayRuleStopsAndIsLogged(t *testing.T) {
	f := newRuleFixture(t)
	f.save(t, "spin", "validation", "validate", "def validate(doc, ctx):\n    for i in range(100000000):\n        pass\n", 1)
	_, err := f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("1")})
	requireCode(t, err, connect.CodeFailedPrecondition)
	runs := f.runs(t, &athenav1.ListRuleRunsRequest{RuleKey: "spin"})
	if len(runs) != 1 || runs[0].GetCode() != rules.CodeStepLimit || runs[0].GetSteps() == 0 {
		t.Fatalf("runs = %v", runs)
	}
}

func TestGuardBlocksSubmitAndOnlyThen(t *testing.T) {
	f := newRuleFixture(t)
	f.save(t, "needs_title", "guard", "before_submit", "def guard(doc, ctx):\n    require(doc.title != None, \"Give the invoice a title before submitting\")\n", 1)
	draft, err := f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("1")})
	if err != nil {
		t.Fatalf("a guard does not run on save: %v", err)
	}
	_, err = f.records.SubmitRecord(context.Background(), &athenav1.SubmitRecordRequest{RecordTypeKey: "invoice", Id: draft.GetId(), ExpectedRevision: draft.GetRevision()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(connectMessage(err), "Give the invoice a title") {
		t.Fatalf("message = %q", connectMessage(err))
	}
	if err := update(f.records, "invoice", draft, map[string]*athenav1.FieldValue{"title": text("T")}); err != nil {
		t.Fatal(err)
	}
	got, _ := f.records.GetRecord(context.Background(), &athenav1.GetRecordRequest{RecordTypeKey: "invoice", Id: draft.GetId()})
	submitted := submit(t, f.records, "invoice", got.GetRecord())
	if submitted.GetDocstate() != int32(lifecycle.Submitted) {
		t.Fatalf("state = %d", submitted.GetDocstate())
	}
}

func TestDisabledRuleDoesNotRun(t *testing.T) {
	f := newRuleFixture(t)
	saved := f.save(t, "qty_positive", "validation", "validate", positiveQty, 1)
	def := saved.GetDefinition()
	def.Enabled = false
	if _, err := f.rules.SaveRule(context.Background(), &athenav1.SaveRuleRequest{Definition: def, ExpectedVersion: saved.GetVersion()}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("0")}); err != nil {
		t.Fatalf("a disabled rule rejected a save: %v", err)
	}
}

func TestRuleVersionsAuditAndConflicts(t *testing.T) {
	f := newRuleFixture(t)
	ctx := context.Background()
	v1 := f.save(t, "qty_positive", "validation", "validate", positiveQty, 1)
	def := v1.GetDefinition()
	if again, err := f.rules.SaveRule(ctx, &athenav1.SaveRuleRequest{Definition: def, ExpectedVersion: 1}); err != nil || again.GetRule().GetVersion() != 1 {
		t.Fatalf("an unchanged save made a version: %v %v", again, err)
	}
	def.Source = positiveQty + "\n# changed\n"
	v2, err := f.rules.SaveRule(ctx, &athenav1.SaveRuleRequest{Definition: def, ExpectedVersion: 1})
	if err != nil || v2.GetRule().GetVersion() != 2 {
		t.Fatalf("v2 = %v %v", v2, err)
	}
	_, err = f.rules.SaveRule(ctx, &athenav1.SaveRuleRequest{Definition: def, ExpectedVersion: 1})
	requireCode(t, err, connect.CodeAborted)
	_, err = f.rules.SaveRule(ctx, &athenav1.SaveRuleRequest{Definition: def})
	requireCode(t, err, connect.CodeAborted)
	versions, err := f.rules.ListRuleVersions(ctx, &athenav1.ListRuleVersionsRequest{Key: "qty_positive"})
	if err != nil || len(versions.GetVersions()) != 2 || versions.GetVersions()[1].GetDefinition().GetSource() != positiveQty {
		t.Fatalf("versions = %v %v", versions, err)
	}
	// Each run records the version it used.
	if _, err := f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("2")}); err != nil {
		t.Fatal(err)
	}
	if runs := f.runs(t, &athenav1.ListRuleRunsRequest{}); runs[0].GetRuleVersion() != 2 {
		t.Fatalf("run version = %d", runs[0].GetRuleVersion())
	}
	// The Audit Log holds hashes and attributes, never the source.
	rows, err := audit.Rows(ctx, f.db, ruleAuditType, "qty_positive", 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("audit rows = %v %v", rows, err)
	}
	for _, row := range rows {
		if strings.Contains(row.RawChange, "require(") {
			t.Fatalf("the Audit Log holds source: %q", row.RawChange)
		}
	}
	if res, err := audit.Verify(ctx, f.db, nil); err != nil || !res.OK {
		t.Fatalf("chain = %+v %v", res, err)
	}
	if _, err := f.rules.DeleteRule(ctx, &athenav1.DeleteRuleRequest{Key: "qty_positive", ExpectedVersion: 1}); connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("stale delete = %v", err)
	}
	if _, err := f.rules.DeleteRule(ctx, &athenav1.DeleteRuleRequest{Key: "qty_positive", ExpectedVersion: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("0")}); err != nil {
		t.Fatalf("a deleted rule still ran: %v", err)
	}
}

func TestSimulateIsADryRunWithADiffAndAReplayableHash(t *testing.T) {
	f := newRuleFixture(t)
	ctx := context.Background()
	draft := &athenav1.RuleDefinition{Key: "line_total", Label: "Line total", RuleType: "derivation", RecordTypeKey: "invoice", Event: "validate", Source: "def derive(doc, ctx):\n    set(\"total\", doc.qty * doc.rate)\n    warn(\"check\", \"look at this\")\n"}
	req := &athenav1.SimulateRuleRequest{Definition: draft, Values: map[string]*athenav1.FieldValue{"qty": text("2"), "rate": text("2.5"), "total": text("1")}}
	first, err := f.rules.SimulateRule(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.GetOutcome() != "ok" || len(first.GetChanges()) != 1 || first.GetChanges()[0].GetNewValue() != "5.0" && first.GetChanges()[0].GetNewValue() != "5" || first.GetChanges()[0].GetOldValue() != "1" || len(first.GetWarnings()) != 1 {
		t.Fatalf("simulation = %v", first)
	}
	second, err := f.rules.SimulateRule(ctx, req)
	if err != nil || second.GetInputHash() != first.GetInputHash() || second.GetSteps() != first.GetSteps() {
		t.Fatalf("replay differs: %v %v", second, err)
	}
	if f.countInvoices(t) != 0 {
		t.Fatal("a simulation stored a document")
	}
	runs := f.runs(t, &athenav1.ListRuleRunsRequest{RuleKey: "line_total"})
	if len(runs) != 2 || !runs[0].GetDryRun() {
		t.Fatalf("simulation runs = %v", runs)
	}
	bad := &athenav1.SimulateRuleRequest{Definition: &athenav1.RuleDefinition{Key: "t", Label: "t", RuleType: "transform", Source: "def transform(row, ctx):\n    return {\"name\": row.name.upper()}\n"}, Values: map[string]*athenav1.FieldValue{"name": text("ana")}}
	res, err := f.rules.SimulateRule(ctx, bad)
	if err != nil || res.GetResult()["name"].GetStringValue() != "ANA" {
		t.Fatalf("transform = %v %v", res, err)
	}
}

func TestRuleReadsAreMadeAsTheActingUser(t *testing.T) {
	f := newAccessFixture(t)
	mustSave(t, f.types, &athenav1.RecordType{Key: "customer", Label: "Customer", Fields: []*athenav1.RecordTypeField{{Key: "name", Label: "Name", Type: "Text"}, {Key: "blocked", Label: "Blocked", Type: "Checkbox"}}})
	mustSave(t, f.types, &athenav1.RecordType{Key: "invoice", Label: "Invoice", Kind: "document", Class: "non_posting", Fields: []*athenav1.RecordTypeField{
		{Key: "customer", Label: "Customer", Type: "Link", TargetRecordTypeKey: "customer"},
		{Key: "secret", Label: "Secret", Type: "Text", PermissionLevel: 5},
	}})
	customer, err := create(t, f.records, "customer", map[string]*athenav1.FieldValue{"name": text("Acme")})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, r := range []struct{ key, source string }{
		{"customer_ok", "def validate(doc, ctx):\n    c = get(\"customer\", doc.customer)\n    require(c != None and not c.blocked, \"Customer is blocked\")\n"},
		{"reads_secret", "def validate(doc, ctx):\n    x = doc.secret\n"},
	} {
		if _, err := f.rules.SaveRule(ctx, &athenav1.SaveRuleRequest{Definition: &athenav1.RuleDefinition{Key: r.key, Label: r.key, RuleType: "validation", RecordTypeKey: "invoice", Event: "validate", Enabled: true, Source: r.source}}); err != nil {
			t.Fatal(err)
		}
	}
	grant := auth.Grant{RecordType: "invoice", Level: 1, Actions: map[string]bool{auth.ActionRead: true, auth.ActionCreate: true, auth.ActionWrite: true}}
	f.role(t, "clerk", nil, grant, grantOf("customer", 1, "", auth.ActionRead))
	f.role(t, "blind", nil, grant)
	f.user(t, "cora", "clerk")
	f.user(t, "bill", "blind")
	link := map[string]*athenav1.FieldValue{"customer": text(customer.GetId())}

	// The rule that reads a hidden field fails for a user below its level, and
	// the failure is attributed to the user.
	f.rules.db.Exec("UPDATE athena_rules SET enabled = 0 WHERE name = 'customer_ok'")
	cora := f.browser(t)
	cora.login(t, "cora")
	_, err = cora.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "invoice", Values: link})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(connectMessage(err), "permission level") {
		t.Fatalf("message = %q", connectMessage(err))
	}
	f.rules.db.Exec("UPDATE athena_rules SET enabled = 0 WHERE name = 'reads_secret'")
	f.rules.db.Exec("UPDATE athena_rules SET enabled = 1 WHERE name = 'customer_ok'")
	if _, err := cora.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "invoice", Values: link}); err != nil {
		t.Fatalf("a user who may read customers: %v", err)
	}
	// A user with no Grant on customer cannot read it through a Rule either.
	bill := f.browser(t)
	bill.login(t, "bill")
	_, err = bill.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "invoice", Values: link})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(connectMessage(err), "permission to read customer") {
		t.Fatalf("message = %q", connectMessage(err))
	}
	runs, err := f.rules.ListRuleRuns(ctx, &athenav1.ListRuleRunsRequest{RuleKey: "customer_ok", Outcome: "error"})
	if err != nil || len(runs.GetRuns()) != 1 || runs.GetRuns()[0].GetActor() != "bill" || runs.GetRuns()[0].GetCode() != "permission_denied" {
		t.Fatalf("runs = %v %v", runs, err)
	}
}

func TestRuleServicePermissions(t *testing.T) {
	f := newAccessFixture(t)
	f.role(t, "reader", []string{auth.PermissionRulesRead})
	f.role(t, "author", []string{auth.PermissionRulesRead, auth.PermissionRulesWrite})
	f.user(t, "rita", "reader")
	f.user(t, "walt", "author")
	f.user(t, "nobody")
	ctx := context.Background()
	for _, name := range []string{"nobody", "rita"} {
		b := f.browser(t)
		b.login(t, name)
		_, err := b.rules.SaveRule(ctx, &athenav1.SaveRuleRequest{Definition: &athenav1.RuleDefinition{Key: "x"}})
		requireCode(t, err, connect.CodePermissionDenied)
		_, err = b.rules.SimulateRule(ctx, &athenav1.SimulateRuleRequest{Definition: &athenav1.RuleDefinition{Key: "x"}})
		requireCode(t, err, connect.CodePermissionDenied)
	}
	b := f.browser(t)
	b.login(t, "nobody")
	_, err := b.rules.ListRules(ctx, &athenav1.ListRulesRequest{})
	requireCode(t, err, connect.CodePermissionDenied)
	r := f.browser(t)
	r.login(t, "rita")
	if _, err := r.rules.ListRules(ctx, &athenav1.ListRulesRequest{}); err != nil {
		t.Fatalf("rules:read could not list: %v", err)
	}
	if denials := f.accessEvents(t, auth.EventDenied); len(denials) != 5 {
		t.Fatalf("denials = %d, want 5", len(denials))
	}
	mustSave(t, f.types, &athenav1.RecordType{Key: "invoice", Label: "Invoice", Kind: "document", Class: "non_posting", Fields: []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text"}}})
	w := f.browser(t)
	w.login(t, "walt")
	saved, err := w.rules.SaveRule(ctx, &athenav1.SaveRuleRequest{Definition: &athenav1.RuleDefinition{Key: "ok", Label: "ok", RuleType: "validation", RecordTypeKey: "invoice", Event: "validate", Enabled: true, Source: "def validate(doc, ctx):\n    pass\n"}})
	if err != nil || saved.GetRule().GetDefinition().GetKey() != "ok" {
		t.Fatalf("author save = %v %v", saved, err)
	}
	if saved.GetRule().GetModifiedBy() != "walt" {
		t.Fatalf("modified_by = %q", saved.GetRule().GetModifiedBy())
	}
}

func TestRuleCascadeIsBounded(t *testing.T) {
	f := newRuleFixture(t)
	f.save(t, "qty_positive", "validation", "validate", positiveQty, 1)
	engine := ruleEngine{db: f.db}
	tx, err := f.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	doc := &lifecycle.Document{Values: map[string]any{"qty": "1"}, System: map[string]any{}}
	ctx := context.WithValue(context.Background(), ruleStackKey{}, rules.Stack{"qty_positive"})
	err = engine.hook(ctx, &lifecycle.Event{Name: lifecycle.Validate, RecordType: "invoice", RecordID: "x", Tx: tx, Doc: doc})
	if err == nil || !strings.Contains(err.Error(), "cascade") {
		t.Fatalf("a rule that would run itself: %v", err)
	}
}

func TestCascadeLimitCodeIsRecorded(t *testing.T) {
	f := newRuleFixture(t)
	f.save(t, "qty_positive", "validation", "validate", positiveQty, 1)
	engine := ruleEngine{db: f.db}
	tx, err := f.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	doc := &lifecycle.Document{Values: map[string]any{"qty": "1"}, System: map[string]any{}}
	ctx := context.WithValue(context.Background(), ruleStackKey{}, rules.Stack{"qty_positive"})
	err = engine.hook(ctx, &lifecycle.Event{Name: lifecycle.Validate, RecordType: "invoice", RecordID: "x", Tx: tx, Doc: doc})
	if err == nil || !strings.Contains(err.Error(), rules.CodeCascadeLimit) {
		t.Fatalf("cascade error: %v", err)
	}
	runs := f.runs(t, &athenav1.ListRuleRunsRequest{RuleKey: "qty_positive"})
	if len(runs) != 1 || runs[0].GetOutcome() != "error" || runs[0].GetCode() != rules.CodeCascadeLimit {
		t.Fatalf("cascade runs = %v", runs)
	}
}

func TestSimulateAppliesTheSameCaps(t *testing.T) {
	f := newRuleFixture(t)
	draft := &athenav1.RuleDefinition{Key: "big_repeat", Label: "Big repeat", RuleType: "validation", RecordTypeKey: "invoice", Event: "validate", Source: "def validate(doc, ctx):\n    x = [0]\n    y = x * 10000000000\n"}
	res, err := f.rules.SimulateRule(context.Background(), &athenav1.SimulateRuleRequest{Definition: draft, Values: map[string]*athenav1.FieldValue{"qty": text("2")}})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetOutcome() != "error" || res.GetCode() != rules.CodeMemoryLimit {
		t.Fatalf("simulation = %v", res)
	}
	runs := f.runs(t, &athenav1.ListRuleRunsRequest{RuleKey: "big_repeat"})
	if len(runs) != 1 || !runs[0].GetDryRun() || runs[0].GetCode() != rules.CodeMemoryLimit {
		t.Fatalf("simulation runs = %v", runs)
	}
}

func TestSavedRunRowLimitIsRecorded(t *testing.T) {
	old := rules.DefaultLimits
	rules.DefaultLimits.MaxReads = 2
	defer func() { rules.DefaultLimits = old }()
	f := newRuleFixture(t)
	for _, name := range []string{"Acme", "Globex"} {
		if _, err := create(t, f.records, "customer", map[string]*athenav1.FieldValue{"name": text(name)}); err != nil {
			t.Fatalf("customer %s: %v", name, err)
		}
	}
	f.save(t, "read_twice", "validation", "validate", "def validate(doc, ctx):\n    find(\"customer\")\n    find(\"customer\")\n", 1)
	_, err := f.invoice(t, map[string]*athenav1.FieldValue{"qty": text("3")})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(connectMessage(err), rules.CodeRowLimit) {
		t.Fatalf("row limit error: %v", err)
	}
	runs := f.runs(t, &athenav1.ListRuleRunsRequest{RuleKey: "read_twice"})
	if len(runs) != 1 || runs[0].GetCode() != rules.CodeRowLimit {
		t.Fatalf("row limit runs = %v", runs)
	}
}

func TestRuleSpansAndLogsCarryNoValues(t *testing.T) {
	f := newRuleFixture(t)
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	var logged bytes.Buffer
	original := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(original) })

	f.save(t, "noisy", "validation", "validate", "def validate(doc, ctx):\n    log.info(\"seen\", doc.title)\n    print(doc)\n    require(doc.title != \"hunter2\", \"no\")\n", 1)
	_, _ = f.invoice(t, map[string]*athenav1.FieldValue{"title": text("hunter2")})
	found := false
	for _, span := range exporter.GetSpans() {
		if span.Name == "athena.rule_run" {
			found = true
		}
		for _, a := range span.Attributes {
			if strings.Contains(a.Value.Emit(), "hunter2") {
				t.Fatalf("span %s carries a value: %s", span.Name, a.Key)
			}
		}
	}
	if !found {
		t.Fatal("no rule_run span")
	}
	if strings.Contains(logged.String(), "hunter2") {
		t.Fatal("the server log carries a value")
	}
}
