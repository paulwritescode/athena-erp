package server

import (
	"context"
	"database/sql"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/ledger"
	"athena-erp/internal/posting"
	"athena-erp/internal/store"
	"athena-erp/internal/types"

	"connectrpc.com/connect/v2"
)

// settlementFixture seeds masters, receipt and customer types, and one open
// USD item; the operator context drives the RPCs like the CLI does.
type settlementFixture struct {
	db      *sql.DB
	ledger  ledgerService
	masters *ledger.Service
	itemID  string
}

func newSettlementFixture(t *testing.T) *settlementFixture {
	t.Helper()
	typesSvc, _ := newServices(t)
	db := typesSvc.db
	ctx := context.Background()
	masters := &ledger.Service{DB: db, Dialect: dialect}
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
	if _, err := masters.SaveAccountType(ctx, ledger.AccountType{Name: "asset", Label: "Asset", NormalBalance: "debit"}); err != nil {
		t.Fatal(err)
	}
	if _, err := masters.SaveAccount(ctx, ledger.Account{Name: "receivable", Label: "Receivable", Type: "asset", ControlRole: "receivable"}, 0); err != nil {
		t.Fatal(err)
	}
	mustSave(t, typesSvc, &athenav1.RecordType{Key: "receipt", Label: "Receipt", Kind: "document", Class: "non_posting", Submittable: true, Fields: []*athenav1.RecordTypeField{
		{Key: "total", Label: "Total", Type: "Number"},
	}})
	mustSave(t, typesSvc, &athenav1.RecordType{Key: "customer", Label: "Customer", Kind: "master", Fields: []*athenav1.RecordTypeField{
		{Key: "name", Label: "Name", Type: "Text"},
	}})
	settle := &posting.SettlementService{DB: db, Dialect: store.MariaDB{}}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	created, err := settle.CreateItemsInTx(ctx, tx, []posting.NewItem{{
		Company: "acme", Book: "main", PartyType: "customer", PartyID: "c1",
		ControlAccount: "receivable", SourceType: "invoice", SourceID: "i1",
		SourceRevision: 2, InstalmentNo: 1, Currency: "USD", Side: posting.ItemSideDebit,
		Amount: types.MustDecimal("100.00"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return &settlementFixture{db: db, ledger: newLedgerService(db), masters: masters, itemID: created[0].ID}
}

func decimalEqual(t *testing.T, got, want string) bool {
	t.Helper()
	gotAmount, err := types.ParseDecimal(got)
	if err != nil {
		t.Fatalf("parse %q: %v", got, err)
	}
	return gotAmount.Equal(types.MustDecimal(want))
}

func settleApplyRequest(itemID string) *athenav1.ApplySettlementRequest {
	return &athenav1.ApplySettlementRequest{
		Company: "acme", Book: "main", PartyType: "customer", PartyId: "c1",
		ControlAccount: "receivable", Side: posting.ItemSideDebit,
		VoucherType: "receipt", VoucherId: "r1", Revision: 2, PostingDate: "2026-06-20",
		PaymentCurrency: "USD",
		Lines:           []*athenav1.SettlementLine{{LineNo: 1, Amount: "100.00", Items: []*athenav1.SettlementLineItem{{ItemId: itemID}}}},
	}
}

func TestSettlementRoundTrip(t *testing.T) {
	f := newSettlementFixture(t)
	ctx := context.Background()
	open, err := f.ledger.ListOpenItems(ctx, &athenav1.ListOpenItemsRequest{Company: "acme", Book: "main", PartyType: "customer", PartyId: "c1"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(open.GetItems()) != 1 || !decimalEqual(t, open.GetItems()[0].GetResidualAmount(), "100.00") {
		t.Fatalf("open items = %+v", open.GetItems())
	}
	applied, err := f.ledger.ApplySettlement(ctx, settleApplyRequest(f.itemID))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(applied.GetApplications()) != 1 || !decimalEqual(t, applied.GetApplications()[0].GetBaseAmount(), "100.00") {
		t.Fatalf("applications = %+v", applied.GetApplications())
	}
	open, err = f.ledger.ListOpenItems(ctx, &athenav1.ListOpenItemsRequest{Company: "acme", Book: "main", PartyType: "customer", PartyId: "c1"})
	if err != nil {
		t.Fatalf("list after apply: %v", err)
	}
	if len(open.GetItems()) != 0 {
		t.Fatalf("settled item still listed: %+v", open.GetItems())
	}
	reversed, err := f.ledger.ReverseSettlement(ctx, &athenav1.ReverseSettlementRequest{
		Company: "acme", Book: "main", PartyType: "customer", PartyId: "c1",
		ControlAccount: "receivable", Side: posting.ItemSideDebit,
		VoucherType: "receipt", VoucherId: "r1", Revision: 3, PostingDate: "2026-07-20",
	})
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if len(reversed.GetApplications()) != 1 {
		t.Fatalf("mirrors = %+v", reversed.GetApplications())
	}
	open, err = f.ledger.ListOpenItems(ctx, &athenav1.ListOpenItemsRequest{Company: "acme", Book: "main", PartyType: "customer", PartyId: "c1"})
	if err != nil {
		t.Fatalf("list after reverse: %v", err)
	}
	if len(open.GetItems()) != 1 || !decimalEqual(t, open.GetItems()[0].GetResidualAmount(), "100.00") {
		t.Fatalf("restored item = %+v", open.GetItems())
	}
	if _, err := f.ledger.ReverseSettlement(ctx, &athenav1.ReverseSettlementRequest{
		Company: "acme", Book: "main", PartyType: "customer", PartyId: "c1",
		ControlAccount: "receivable", Side: posting.ItemSideDebit,
		VoucherType: "receipt", VoucherId: "r1", Revision: 4, PostingDate: "2026-07-21",
	}); connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("second reverse = %v, want Aborted", err)
	}
}

func TestApplySettlementValidation(t *testing.T) {
	f := newSettlementFixture(t)
	ctx := context.Background()
	bad := settleApplyRequest(f.itemID)
	bad.Lines[0].Amount = "ten"
	if _, err := f.ledger.ApplySettlement(ctx, bad); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("bad decimal = %v, want InvalidArgument", err)
	}
	unknown := settleApplyRequest(f.itemID)
	unknown.VoucherType = "nope"
	if _, err := f.ledger.ApplySettlement(ctx, unknown); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("unknown voucher type = %v, want NotFound", err)
	}
}

func clerkPrincipal(system []string, recordType string) *auth.Principal {
	permissions := map[string]bool{}
	for _, permission := range system {
		permissions[permission] = true
	}
	return &auth.Principal{Username: "clerk", SystemPermissions: permissions, Assignments: []auth.Assignment{{
		ID: "a", Active: true, Grants: []auth.Grant{{RecordType: recordType, Actions: map[string]bool{auth.ActionRead: true}}},
	}}}
}

func TestApplySettlementGrants(t *testing.T) {
	f := newSettlementFixture(t)
	ctx := context.Background()
	// No grants at all: the write gate denies first.
	denied := auth.WithPrincipal(ctx, clerkPrincipal(nil, "receipt"))
	if _, err := f.ledger.ApplySettlement(denied, settleApplyRequest(f.itemID)); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("grantless apply = %v, want PermissionDenied", err)
	}
	// A close reason without the close grant denies even with write access.
	writer := auth.WithPrincipal(ctx, clerkPrincipal([]string{auth.PermissionLedgerWrite}, "receipt"))
	reasoned := settleApplyRequest(f.itemID)
	reasoned.CloseReason = "late receipt"
	if _, err := f.ledger.ApplySettlement(writer, reasoned); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("unauthorized adjustment = %v, want PermissionDenied", err)
	}
	// The close grant plus a reason adjusts a soft-closed period and audits it.
	period, err := f.masters.GetPeriod(ctx, "acme", "main", "fy2026")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.masters.SetPeriodStatus(ctx, "acme", "main", "fy2026", ledger.PeriodSoftClosed, "tester", period.Revision); err != nil {
		t.Fatal(err)
	}
	closer := auth.WithPrincipal(ctx, clerkPrincipal([]string{auth.PermissionLedgerWrite, auth.PermissionLedgerClose}, "receipt"))
	if _, err := f.ledger.ApplySettlement(closer, reasoned); err != nil {
		t.Fatalf("authorized adjustment: %v", err)
	}
	var audits int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM athena_audit_log WHERE action = 'settlement_adjustment'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("adjustment audits = %d, want 1", audits)
	}
}

func TestListOpenItemsGrants(t *testing.T) {
	f := newSettlementFixture(t)
	ctx := context.Background()
	denied := auth.WithPrincipal(ctx, clerkPrincipal(nil, "customer"))
	req := &athenav1.ListOpenItemsRequest{Company: "acme", Book: "main", PartyType: "customer", PartyId: "c1"}
	if _, err := f.ledger.ListOpenItems(denied, req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("grantless list = %v, want PermissionDenied", err)
	}
	reader := auth.WithPrincipal(ctx, clerkPrincipal([]string{auth.PermissionLedgerRead}, "customer"))
	res, err := f.ledger.ListOpenItems(reader, req)
	if err != nil {
		t.Fatalf("list with grants: %v", err)
	}
	if len(res.GetItems()) != 1 {
		t.Fatalf("items = %+v", res.GetItems())
	}
}
