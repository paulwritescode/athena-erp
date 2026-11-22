package server

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/ledger"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/posting"
	"athena-erp/internal/types"

	"connectrpc.com/connect/v2"
)

// postingFixture is operator services with seeded ledger masters, a
// submittable bill with lines, and the Rule engine attached so the generic
// hook runs beside the posting path.
type postingFixture struct {
	db      *sql.DB
	types   recordTypeService
	records recordService
	rules   ruleService
	masters *ledger.Service
}

func newPostingFixture(t *testing.T) *postingFixture {
	t.Helper()
	types, records := newServices(t)
	hooks := lifecycle.NewRegistry()
	registerRules(hooks, records.db)
	records.hooks = hooks
	f := &postingFixture{db: records.db, types: types, records: records,
		rules: newRuleService(records.db), masters: &ledger.Service{DB: records.db, Dialect: dialect}}
	seedPostingMasters(t, f.masters)
	mustSave(t, f.types, &athenav1.RecordType{Key: "bill", Label: "Bill", Kind: "document", Class: "posting", Submittable: true, Fields: []*athenav1.RecordTypeField{
		{Key: "total", Label: "Total", Type: "Number"},
	}})
	mustSave(t, f.types, &athenav1.RecordType{Key: "bill_line", Label: "Bill line", Kind: "child", ParentRecordTypeKey: "bill", Fields: []*athenav1.RecordTypeField{
		{Key: "amount", Label: "Amount", Type: "Number"},
	}})
	return f
}

func seedPostingMasters(t *testing.T, masters *ledger.Service) {
	t.Helper()
	ctx := context.Background()
	if _, err := masters.SaveCurrency(ctx, ledger.Currency{Code: "USD", Label: "US dollar", Exponent: 2}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := masters.SaveCompany(ctx, ledger.Company{Name: "acme", Label: "Acme", BaseCurrency: "USD"}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := masters.SaveFiscalYear(ctx, ledger.FiscalYear{Name: "fy2026", Label: "FY 2026", Company: "acme", StartDate: "2026-01-01", EndDate: "2026-12-31"}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := masters.SavePeriod(ctx, ledger.Period{Company: "acme", Book: "main", Name: "fy2026", Label: "FY 2026", FiscalYear: "fy2026", StartDate: "2026-01-01", EndDate: "2026-12-31"}, 0); err != nil {
		t.Fatal(err)
	}
	for _, accountType := range []ledger.AccountType{
		{Name: "asset", Label: "Asset", NormalBalance: "debit"},
		{Name: "income", Label: "Income", NormalBalance: "credit"},
	} {
		if _, err := masters.SaveAccountType(ctx, accountType); err != nil {
			t.Fatal(err)
		}
	}
	for _, account := range []ledger.Account{
		{Name: "cash", Label: "Cash", Type: "asset"},
		{Name: "revenue", Label: "Revenue", Type: "income"},
	} {
		if _, err := masters.SaveAccount(ctx, account, 0); err != nil {
			t.Fatal(err)
		}
	}
}

const billRuleV1 = `
def post(doc, ctx):
    return {
        "book": "main",
        "posting_date": "2026-06-15",
        "lines": [
            {"company": "acme", "account": "cash", "debit": doc.total},
            {"company": "acme", "account": "revenue", "credit": doc.total},
        ],
    }

def fixtures():
    return [{"name": "standard", "values": {"total": decimal("100.00")}}]
`

const billRuleV2 = `
def post(doc, ctx):
    return {
        "book": "main",
        "posting_date": "2026-06-16",
        "lines": [
            {"company": "acme", "account": "cash", "debit": doc.total},
            {"company": "acme", "account": "revenue", "credit": doc.total},
        ],
    }

def fixtures():
    return [{"name": "standard", "values": {"total": decimal("100.00")}}]
`

const billRuleBroken = `
def post(doc, ctx):
    return {
        "book": "main",
        "posting_date": "2026-06-15",
        "lines": [
            {"company": "acme", "account": "cash", "debit": doc.total},
            {"company": "acme", "account": "revenue", "credit": doc.total + decimal("1")},
        ],
    }

def fixtures():
    return [{"name": "standard", "values": {"total": decimal("100.00")}}]
`

const billLinesRule = `
def post(doc, ctx):
    legs = []
    total = decimal("0")
    for line in find("bill_line", {"parent_id": doc.id}):
        total = total + line.amount
        legs.append({"company": "acme", "account": "cash", "debit": line.amount})
    legs.append({"company": "acme", "account": "revenue", "credit": total})
    return {"book": "main", "posting_date": "2026-06-15", "lines": legs}

def fixtures():
    return [{"name": "two lines", "values": {},
             "children": {"bill_line": [{"amount": decimal("60.00")}, {"amount": decimal("40.00")}]}}]
`

func (f *postingFixture) saveRule(t *testing.T, key, source string, enabled bool, expected int64) (*athenav1.Rule, error) {
	t.Helper()
	res, err := f.rules.SaveRule(context.Background(), &athenav1.SaveRuleRequest{Definition: &athenav1.RuleDefinition{
		Key: key, Label: key, RuleType: "posting", RecordTypeKey: "bill", Event: "on_submit", Enabled: enabled, Source: source,
	}, ExpectedVersion: expected})
	if err != nil {
		return nil, err
	}
	return res.GetRule(), nil
}

func (f *postingFixture) bill(t *testing.T, total string) *athenav1.Record {
	t.Helper()
	values := map[string]*athenav1.FieldValue{}
	if total != "" {
		values["total"] = text(total)
	}
	record, err := create(t, f.records, "bill", values)
	if err != nil {
		t.Fatalf("create bill: %v", err)
	}
	return record
}

func (f *postingFixture) line(t *testing.T, bill *athenav1.Record, amount string) {
	t.Helper()
	_, err := f.records.CreateRecord(context.Background(), &athenav1.CreateRecordRequest{RecordTypeKey: "bill_line", ParentId: bill.GetId(), Values: map[string]*athenav1.FieldValue{"amount": text(amount)}})
	if err != nil {
		t.Fatalf("create bill line: %v", err)
	}
}

func queryInt(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var value int
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatalf("query %s: %v", query, err)
	}
	return value
}

func TestSubmitPostsThroughItsPostingRule(t *testing.T) {
	f := newPostingFixture(t)
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	bill := f.bill(t, "100")
	submitted := submit(t, f.records, "bill", bill)
	if submitted.GetDocstate() != int32(lifecycle.Submitted) || submitted.GetRevision() != 2 {
		t.Fatalf("submitted = state %d revision %d", submitted.GetDocstate(), submitted.GetRevision())
	}
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM l_gl WHERE voucher_type = 'bill' AND voucher_id = ?", bill.GetId()); got != 2 {
		t.Fatalf("ledger rows = %d, want 2", got)
	}
	var debit, credit string
	if err := f.db.QueryRow("SELECT SUM(debit_base), SUM(credit_base) FROM l_gl WHERE voucher_id = ?", bill.GetId()).Scan(&debit, &credit); err != nil {
		t.Fatal(err)
	}
	if debit != "100.000000" || credit != "100.000000" {
		t.Fatalf("posted totals = %s/%s", debit, credit)
	}
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM athena_posting_groups WHERE voucher_type = 'bill' AND voucher_id = ? AND event = 'submit' AND book = 'main'", bill.GetId()); got != 1 {
		t.Fatalf("posting groups = %d, want 1", got)
	}
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM athena_posting_log WHERE voucher_id = ? AND outcome = 'posted'", bill.GetId()); got != 1 {
		t.Fatalf("posted log rows = %d, want 1", got)
	}
	runs, err := f.rules.ListRuleRuns(context.Background(), &athenav1.ListRuleRunsRequest{RuleKey: "bill_post"})
	if err != nil {
		t.Fatal(err)
	}
	live := 0
	for _, run := range runs.GetRuns() {
		if !run.GetDryRun() {
			live++
		}
	}
	if live != 1 {
		t.Fatalf("live runs = %d, want 1: the generic engine must skip posting Rules", live)
	}
}

func TestSubmitWithoutPostingRulesPostsNothing(t *testing.T) {
	f := newPostingFixture(t)
	mustSave(t, f.types, &athenav1.RecordType{Key: "memo", Label: "Memo", Kind: "document", Class: "non_posting", Submittable: true, Fields: []*athenav1.RecordTypeField{
		{Key: "body", Label: "Body", Type: "Text"},
	}})
	record, err := create(t, f.records, "memo", map[string]*athenav1.FieldValue{"body": text("hello")})
	if err != nil {
		t.Fatal(err)
	}
	submit(t, f.records, "memo", record)
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM l_gl"); got != 0 {
		t.Fatalf("ledger rows = %d, want 0", got)
	}
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM athena_posting_groups"); got != 0 {
		t.Fatalf("posting groups = %d, want 0", got)
	}
}

func TestCancelReversesThePost(t *testing.T) {
	f := newPostingFixture(t)
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	bill := f.bill(t, "100")
	submitted := submit(t, f.records, "bill", bill)
	if _, err := cancel(f.records, "bill", submitted); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM l_gl WHERE voucher_id = ?", bill.GetId()); got != 4 {
		t.Fatalf("ledger rows = %d, want 4", got)
	}
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM l_gl WHERE voucher_id = ? AND is_reversal = 1 AND reversed_row_id IS NOT NULL", bill.GetId()); got != 2 {
		t.Fatalf("mirror rows = %d, want 2", got)
	}
	var net string
	if err := f.db.QueryRow("SELECT SUM(debit_base - credit_base) FROM l_gl WHERE voucher_id = ?", bill.GetId()).Scan(&net); err != nil {
		t.Fatal(err)
	}
	if net != "0.000000" {
		t.Fatalf("submit+cancel nets to %s, want zero", net)
	}
}

func TestSubmitRefusesAChangedPreviewHash(t *testing.T) {
	f := newPostingFixture(t)
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	bill := f.bill(t, "100")
	_, err := f.records.SubmitRecord(context.Background(), &athenav1.SubmitRecordRequest{RecordTypeKey: "bill", Id: bill.GetId(), ExpectedRevision: 1, PostingPreviewHash: "stale"})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(err.Error(), "changed since it was previewed") {
		t.Fatalf("error = %v", err)
	}
	reloaded, err := f.records.GetRecord(context.Background(), &athenav1.GetRecordRequest{RecordTypeKey: "bill", Id: bill.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.GetRecord().GetDocstate() != int32(lifecycle.Draft) || reloaded.GetRecord().GetRevision() != 1 {
		t.Fatalf("a refused submit must leave the draft alone: %+v", reloaded.GetRecord())
	}
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM l_gl"); got != 0 {
		t.Fatalf("ledger rows = %d, want 0", got)
	}
	plan := posting.Plan{VoucherType: "bill", VoucherID: bill.GetId(), Revision: 2, Event: posting.EventSubmit, Book: "main", PostingDate: "2026-06-15", Lines: []posting.Line{
		{Company: "acme", Account: "cash", DebitBase: types.MustDecimal("100")},
		{Company: "acme", Account: "revenue", CreditBase: types.MustDecimal("100")},
	}}
	hash, err := posting.HashPlans([]posting.Plan{plan})
	if err != nil {
		t.Fatal(err)
	}
	res, err := f.records.SubmitRecord(context.Background(), &athenav1.SubmitRecordRequest{RecordTypeKey: "bill", Id: bill.GetId(), ExpectedRevision: 1, PostingPreviewHash: hash})
	if err != nil {
		t.Fatalf("submit with the approved hash: %v", err)
	}
	if res.GetRecord().GetDocstate() != int32(lifecycle.Submitted) {
		t.Fatalf("record = %+v", res.GetRecord())
	}
}

func TestPostingRuleChangeNeedsNamedApproval(t *testing.T) {
	f := newPostingFixture(t)
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save v1: %v", err)
	}
	_, err := f.saveRule(t, "bill_post", billRuleV2, true, 1)
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(err.Error(), "saved as disabled version 2") || !strings.Contains(err.Error(), "posting date 2026-06-15 became 2026-06-16") {
		t.Fatalf("approval error = %v", err)
	}
	stored, err := f.rules.GetRule(context.Background(), &athenav1.GetRuleRequest{Key: "bill_post"})
	if err != nil {
		t.Fatal(err)
	}
	if stored.GetRule().GetVersion() != 2 || stored.GetRule().GetDefinition().GetEnabled() {
		t.Fatalf("changed plan must land disabled: %+v", stored.GetRule())
	}
	approved, err := f.saveRule(t, "bill_post", billRuleV2, true, 2)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.GetVersion() != 3 || !approved.GetDefinition().GetEnabled() {
		t.Fatalf("approved = %+v", approved)
	}
	versions, err := f.rules.ListRuleVersions(context.Background(), &athenav1.ListRuleVersionsRequest{Key: "bill_post"})
	if err != nil {
		t.Fatal(err)
	}
	if len(versions.GetVersions()) != 3 || versions.GetVersions()[0].GetActor() == "" {
		t.Fatalf("versions = %+v: the approval must name who approved it", versions.GetVersions())
	}
}

func TestPostingRuleWithABrokenPlanCannotActivate(t *testing.T) {
	f := newPostingFixture(t)
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save v1: %v", err)
	}
	if _, err := f.saveRule(t, "bill_post", billRuleBroken, true, 1); err == nil {
		t.Fatalf("an unbalanced plan must refuse the save")
	} else {
		requireCode(t, err, connect.CodeInvalidArgument)
		if !strings.Contains(err.Error(), `fixture "standard"`) {
			t.Fatalf("error = %v", err)
		}
	}
	stored, err := f.rules.GetRule(context.Background(), &athenav1.GetRuleRequest{Key: "bill_post"})
	if err != nil {
		t.Fatal(err)
	}
	if stored.GetRule().GetVersion() != 1 {
		t.Fatalf("a refused save must store nothing: %+v", stored.GetRule())
	}
	// A broken draft may still be saved disabled; enabling replays the
	// fixtures and refuses while the masters cannot post it.
	draft, err := f.saveRule(t, "bill_post", billRuleBroken, false, 1)
	if err != nil {
		t.Fatalf("save disabled draft: %v", err)
	}
	if draft.GetVersion() != 2 || draft.GetDefinition().GetEnabled() {
		t.Fatalf("draft = %+v", draft)
	}
	if _, err := f.saveRule(t, "bill_post", billRuleBroken, true, 2); err == nil {
		t.Fatalf("enabling a broken Rule must replay its fixtures and refuse")
	} else {
		requireCode(t, err, connect.CodeInvalidArgument)
	}
}

func TestPostingRuleReadsItsVouchersLines(t *testing.T) {
	f := newPostingFixture(t)
	if _, err := f.saveRule(t, "bill_lines", billLinesRule, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM "+recordTypeTableName("bill_line")); got != 0 {
		t.Fatalf("staged fixture children leaked: %d rows", got)
	}
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM "+recordTypeTableName("bill")+" WHERE `id` LIKE 'fixture:%'"); got != 0 {
		t.Fatalf("staged fixture headers leaked: %d rows", got)
	}
	first := f.bill(t, "")
	f.line(t, first, "60")
	f.line(t, first, "40")
	submit(t, f.records, "bill", first)
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM l_gl WHERE voucher_id = ?", first.GetId()); got != 3 {
		t.Fatalf("ledger rows = %d, want 3", got)
	}
	second := f.bill(t, "")
	f.line(t, second, "25")
	submit(t, f.records, "bill", second)
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM l_gl WHERE voucher_id = ?", second.GetId()); got != 2 {
		t.Fatalf("second voucher rows = %d, want 2: lines must not leak across vouchers", got)
	}
	var credit string
	if err := f.db.QueryRow("SELECT SUM(credit_base) FROM l_gl WHERE voucher_id = ?", second.GetId()).Scan(&credit); err != nil {
		t.Fatal(err)
	}
	if credit != "25.000000" {
		t.Fatalf("second voucher credit = %s", credit)
	}
}

func TestPostingRejectionRefusesTheSubmit(t *testing.T) {
	f := newPostingFixture(t)
	guarded := `
def post(doc, ctx):
    if doc.total == None:
        fail("no_total", "Total is required to post")
    return {
        "book": "main",
        "posting_date": "2026-06-15",
        "lines": [
            {"company": "acme", "account": "cash", "debit": doc.total},
            {"company": "acme", "account": "revenue", "credit": doc.total},
        ],
    }

def fixtures():
    return [{"name": "standard", "values": {"total": decimal("10")}}]
`
	if _, err := f.saveRule(t, "bill_post", guarded, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	bill := f.bill(t, "")
	_, err := f.records.SubmitRecord(context.Background(), &athenav1.SubmitRecordRequest{RecordTypeKey: "bill", Id: bill.GetId(), ExpectedRevision: 1})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(err.Error(), "Total is required to post") {
		t.Fatalf("error = %v", err)
	}
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM l_gl"); got != 0 {
		t.Fatalf("ledger rows = %d, want 0", got)
	}
}

func TestTwoPostingRulesMayNotShareABook(t *testing.T) {
	f := newPostingFixture(t)
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	if _, err := f.saveRule(t, "bill_post_again", billRuleV1, true, 0); err != nil {
		t.Fatalf("save second rule: %v", err)
	}
	bill := f.bill(t, "100")
	_, err := f.records.SubmitRecord(context.Background(), &athenav1.SubmitRecordRequest{RecordTypeKey: "bill", Id: bill.GetId(), ExpectedRevision: 1})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(err.Error(), "both post to book") {
		t.Fatalf("error = %v", err)
	}
}

func TestDiffFixturePlans(t *testing.T) {
	line := func(account, debit, credit string) posting.Line {
		return posting.Line{Company: "acme", Account: account, DebitBase: types.MustDecimal(debit), CreditBase: types.MustDecimal(credit)}
	}
	before := posting.Plan{Book: "main", PostingDate: "2026-06-15", Lines: []posting.Line{line("cash", "100", "0"), line("revenue", "0", "100")}}
	same := posting.Plan{Book: "main", PostingDate: "2026-06-15", Lines: []posting.Line{line("cash", "100.00", "0"), line("revenue", "0", "100")}}
	if diff := diffFixturePlans([]fixturePlan{{name: "a", plan: before}}, []fixturePlan{{name: "a", plan: same}}); diff != "" {
		t.Fatalf("equal values at different scales must not diff: %q", diff)
	}
	after := posting.Plan{Book: "main", PostingDate: "2026-06-16", Lines: []posting.Line{line("cash", "120", "0"), line("revenue", "0", "120")}}
	diff := diffFixturePlans([]fixturePlan{{name: "a", plan: before}}, []fixturePlan{{name: "a", plan: after}})
	for _, want := range []string{"posting date 2026-06-15 became 2026-06-16", "line 1: debit 100 became 120", "line 2: credit 100 became 120"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("diff = %q, want %q", diff, want)
		}
	}
	diff = diffFixturePlans([]fixturePlan{{name: "a", plan: before}}, []fixturePlan{{name: "a", plan: before}, {name: "b", plan: before}})
	if !strings.Contains(diff, `fixture "b" is new`) {
		t.Fatalf("diff = %q", diff)
	}
}

func TestOperationIDIsStablePerGroup(t *testing.T) {
	first := operationID("submit", "bill", "id-1", 2, "main")
	if second := operationID("submit", "bill", "id-1", 2, "main"); first != second {
		t.Fatalf("unstable: %q vs %q", first, second)
	}
	seen := map[string]bool{first: true}
	for _, other := range []string{
		operationID("cancel", "bill", "id-1", 2, "main"),
		operationID("submit", "bill", "id-1", 3, "main"),
		operationID("submit", "bill", "id-1", 2, "audit"),
	} {
		if len(other) > 140 {
			t.Fatalf("operation id exceeds the 140-character limit: %q", other)
		}
		if seen[other] {
			t.Fatalf("collision: %q", other)
		}
		seen[other] = true
	}
}
