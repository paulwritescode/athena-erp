package server

import (
	"context"
	"fmt"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/ledger"
	"athena-erp/internal/posting"
	"athena-erp/internal/types"
)

// creationFixture is a bill that posts to the receivable control account for
// one customer: submit records the open items, cancel voids them.
type creationFixture struct {
	*postingFixture
	customerID string
}

func newCreationFixture(t *testing.T, rule string) *creationFixture {
	t.Helper()
	f := newPostingFixture(t)
	ctx := context.Background()
	if _, err := f.masters.SaveAccount(ctx, ledger.Account{Name: "receivable", Label: "Receivable", Type: "asset", ControlRole: "receivable"}, 0); err != nil {
		t.Fatal(err)
	}
	mustSave(t, f.types, &athenav1.RecordType{Key: "customer", Label: "Customer", Kind: "master", Fields: []*athenav1.RecordTypeField{
		{Key: "name", Label: "Name", Type: "Text"},
	}})
	customer, err := create(t, f.records, "customer", map[string]*athenav1.FieldValue{"name": text("Acme Corp")})
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	// Both rule shapes take the customer id per control leg.
	if _, err := f.saveRule(t, "bill_post", strings.ReplaceAll(rule, "CUSTOMER_ID", customer.GetId()), true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	return &creationFixture{postingFixture: f, customerID: customer.GetId()}
}

const creationRuleSingle = `
def post(doc, ctx):
    return {
        "book": "main",
        "posting_date": "2026-06-15",
        "lines": [
            {"company": "acme", "account": "receivable", "debit": doc.total, "source_type": "customer", "source_id": "CUSTOMER_ID"},
            {"company": "acme", "account": "revenue", "credit": doc.total},
        ],
    }

def fixtures():
    return [{"name": "standard", "values": {"total": decimal("100.00")}}]
`

const creationRuleSplit = `
def post(doc, ctx):
    return {
        "book": "main",
        "posting_date": "2026-06-15",
        "lines": [
            {"company": "acme", "account": "receivable", "debit": "60.00", "source_type": "customer", "source_id": "CUSTOMER_ID"},
            {"company": "acme", "account": "receivable", "debit": "40.00", "source_type": "customer", "source_id": "CUSTOMER_ID"},
            {"company": "acme", "account": "revenue", "credit": doc.total},
        ],
    }

def fixtures():
    return [{"name": "standard", "values": {"total": decimal("100.00")}}]
`

func submitBill(t *testing.T, f *creationFixture, total string) *athenav1.Record {
	t.Helper()
	bill := f.bill(t, total)
	res, err := f.records.SubmitRecord(context.Background(), &athenav1.SubmitRecordRequest{RecordTypeKey: "bill", Id: bill.GetId(), ExpectedRevision: 1})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return res.GetRecord()
}

func queryItems(t *testing.T, f *creationFixture, voucherID string) []map[string]string {
	t.Helper()
	rows, err := f.db.Query(`SELECT instalment_no, party_type, party_id, control_account_name, currency, side, original_amount, residual_amount, status, source_voucher_revision FROM athena_party_items WHERE source_voucher_type = 'bill' AND source_voucher_id = ? ORDER BY instalment_no`, voucherID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]string
	for rows.Next() {
		var no, rev int64
		var partyType, partyID, account, currency, side, original, residual, status string
		if err := rows.Scan(&no, &partyType, &partyID, &account, &currency, &side, &original, &residual, &status, &rev); err != nil {
			t.Fatal(err)
		}
		out = append(out, map[string]string{
			"no": fmt.Sprint(no), "party": partyType + "/" + partyID, "account": account,
			"currency": currency, "side": side, "original": original, "residual": residual,
			"status": status, "rev": fmt.Sprint(rev),
		})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSubmitCreatesOpenItems(t *testing.T) {
	f := newCreationFixture(t, creationRuleSingle)
	bill := submitBill(t, f, "100")
	items := queryItems(t, f, bill.GetId())
	if len(items) != 1 {
		t.Fatalf("items = %+v, want one", items)
	}
	item := items[0]
	if item["no"] != "1" || item["party"] != "customer/"+f.customerID || item["account"] != "receivable" {
		t.Fatalf("item identity = %+v", item)
	}
	if item["original"] != "100.000000" || item["residual"] != "100.000000" || item["status"] != "open" || item["side"] != "debit" || item["currency"] != "USD" {
		t.Fatalf("item amounts = %+v", item)
	}
}

func TestSubmitCreatesOneItemPerInstalment(t *testing.T) {
	f := newCreationFixture(t, creationRuleSplit)
	bill := submitBill(t, f, "100")
	items := queryItems(t, f, bill.GetId())
	if len(items) != 2 || items[0]["no"] != "1" || items[1]["no"] != "2" {
		t.Fatalf("items = %+v, want instalments 1 and 2", items)
	}
	if items[0]["residual"] != "60.000000" || items[1]["residual"] != "40.000000" {
		t.Fatalf("instalment residuals = %+v", items)
	}
	// The control-account invariant (R7.8) at submit: signed residuals equal
	// the control balance.
	var balance string
	if err := f.db.QueryRow(`SELECT COALESCE(SUM(debit_base), 0) - COALESCE(SUM(credit_base), 0) FROM l_gl WHERE company_name = 'acme' AND account_name = 'receivable'`).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != "100.000000" {
		t.Fatalf("control balance = %s, want 100", balance)
	}
}

func TestSubmitCreationReplays(t *testing.T) {
	f := newCreationFixture(t, creationRuleSingle)
	bill := submitBill(t, f, "100")
	submitted, err := f.records.SubmitRecord(context.Background(), &athenav1.SubmitRecordRequest{RecordTypeKey: "bill", Id: bill.GetId(), ExpectedRevision: 2})
	if err == nil {
		t.Fatalf("second submit accepted: %+v", submitted.GetRecord())
	}
	if got := len(queryItems(t, f, bill.GetId())); got != 1 {
		t.Fatalf("items after refused resubmit = %d, want 1", got)
	}
	// The hook itself tolerates a replayed revision: same plan twice keeps
	// one item set.
	plan := posting.Plan{
		VoucherType: "bill", VoucherID: "replay", Revision: 2,
		Event: posting.EventSubmit, Book: "main", PostingDate: "2026-06-15",
		Lines: []posting.Line{{
			Company: "acme", Account: "receivable", DebitBase: types.MustDecimal("50.00"),
			SourceType: "customer", SourceID: f.customerID,
		}},
	}
	for i := 0; i < 2; i++ {
		tx, err := f.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.records.createVoucherItems(context.Background(), tx, "bill", "replay", plan, 0); err != nil {
			tx.Rollback()
			t.Fatalf("hook run %d: %v", i, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(queryItems(t, f, "replay")); got != 1 {
		t.Fatalf("items after hook replay = %d, want 1", got)
	}
}

func TestCancelVoidsOpenItems(t *testing.T) {
	f := newCreationFixture(t, creationRuleSplit)
	bill := submitBill(t, f, "100")
	res, err := f.records.CancelRecord(context.Background(), &athenav1.CancelRecordRequest{RecordTypeKey: "bill", Id: bill.GetId(), ExpectedRevision: 2})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	_ = res
	items := queryItems(t, f, bill.GetId())
	if len(items) != 2 {
		t.Fatalf("items = %+v, want both instalments", items)
	}
	for _, item := range items {
		if item["status"] != "cancelled" || item["residual"] != "0.000000" {
			t.Fatalf("voided item = %+v", item)
		}
		if item["original"] == "0.000000" || item["original"] == "0" {
			t.Fatalf("voided item lost its original: %+v", item)
		}
	}
	// The invariant holds at zero: mirrored legs net the control account
	// while the voided residuals contribute nothing.
	var balance string
	if err := f.db.QueryRow(`SELECT COALESCE(SUM(debit_base), 0) - COALESCE(SUM(credit_base), 0) FROM l_gl WHERE company_name = 'acme' AND account_name = 'receivable'`).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != "0.000000" && balance != "0" {
		t.Fatalf("control balance after cancel = %s, want 0", balance)
	}
}

func TestCancelRefusesUnreversedSettlement(t *testing.T) {
	f := newCreationFixture(t, creationRuleSingle)
	bill := submitBill(t, f, "100")
	settle := &posting.SettlementService{DB: f.db, Dialect: dialect}
	if _, err := settle.Apply(context.Background(), posting.SettleRequest{
		Company: "acme", Book: "main", PartyType: "customer", PartyID: f.customerID,
		ControlAccount: "receivable", Side: posting.ItemSideDebit,
		VoucherType: "receipt", VoucherID: "r1", Revision: 2, Event: posting.EventSubmit,
		PostingDate: "2026-06-20", PostingGroupID: "g1", PaymentCurrency: "USD",
		Actor: "tester", Source: "test",
		Lines: []posting.SettleLine{{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: itemIDOf(t, f, bill.GetId())}}}},
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	_, err := f.records.CancelRecord(context.Background(), &athenav1.CancelRecordRequest{RecordTypeKey: "bill", Id: bill.GetId(), ExpectedRevision: 2})
	if err == nil || !strings.Contains(err.Error(), "reverse its settlements first") {
		t.Fatalf("cancel with open settlement = %v", err)
	}
}

func itemIDOf(t *testing.T, f *creationFixture, voucherID string) string {
	t.Helper()
	var id string
	if err := f.db.QueryRow(`SELECT id FROM athena_party_items WHERE source_voucher_type = 'bill' AND source_voucher_id = ?`, voucherID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestControlSourceReferencesVerified(t *testing.T) {
	f := newCreationFixture(t, creationRuleSingle)
	bill := submitBill(t, f, "100")
	reference := func(sourceID string) posting.Plan {
		return posting.Plan{
			VoucherType: "bill", VoucherID: "ref", Revision: 2,
			Event: posting.EventSubmit, Book: "main", PostingDate: "2026-06-15",
			Lines: []posting.Line{{
				Company: "acme", Account: "receivable", DebitBase: types.MustDecimal("10.00"),
				SourceType: "party_item", SourceID: sourceID,
			}},
		}
	}
	run := func(plan posting.Plan) error {
		tx, err := f.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		_, err = f.records.createVoucherItems(context.Background(), tx, "bill", "ref", plan, 0)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	var realID string
	if err := f.db.QueryRow(`SELECT id FROM athena_party_items WHERE source_voucher_id = ?`, bill.GetId()).Scan(&realID); err != nil {
		t.Fatal(err)
	}
	if err := run(reference(realID)); err != nil {
		t.Fatalf("existing reference: %v", err)
	}
	if err := run(reference("missing")); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing reference = %v", err)
	}
}

func TestSubmitRefusesUnknownParty(t *testing.T) {
	f := newCreationFixture(t, creationRuleSingle)
	bill := f.bill(t, "100")
	// Swap the rule's party for a missing record by re-saving with a bogus id.
	bogus := `
def post(doc, ctx):
    return {
        "book": "main",
        "posting_date": "2026-06-15",
        "lines": [
            {"company": "acme", "account": "receivable", "debit": doc.total, "source_type": "customer", "source_id": "missing"},
            {"company": "acme", "account": "revenue", "credit": doc.total},
        ],
    }

def fixtures():
    return [{"name": "standard", "values": {"total": decimal("100.00")}}]
`
	if _, err := f.saveRule(t, "bill_post", bogus, true, 1); err != nil && !strings.Contains(err.Error(), "save again") {
		t.Fatalf("save bogus rule: %v", err)
	}
	if _, err := f.saveRule(t, "bill_post", bogus, true, 2); err != nil {
		t.Fatalf("approve bogus rule: %v", err)
	}
	_, err := f.records.SubmitRecord(context.Background(), &athenav1.SubmitRecordRequest{RecordTypeKey: "bill", Id: bill.GetId(), ExpectedRevision: 1})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("submit with unknown party = %v", err)
	}
}
