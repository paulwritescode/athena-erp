package server

import (
	"context"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"

	"connectrpc.com/connect/v2"
)

func TestLedgerCompanyRoundTrip(t *testing.T) {
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := newLedgerService(db)
	ctx := context.Background()
	saved, err := svc.SaveCompany(ctx, &athenav1.SaveCompanyRequest{Company: &athenav1.Company{Name: "acme", Label: "Acme"}})
	if err != nil || saved.GetCompany().GetRevision() != 1 {
		t.Fatalf("save: %v", saved)
	}
	listed, err := svc.ListCompanies(ctx, &athenav1.ListCompaniesRequest{})
	if err != nil || len(listed.GetCompanies()) != 1 {
		t.Fatalf("list: %v", listed)
	}
	_, err = svc.SaveCompany(ctx, &athenav1.SaveCompanyRequest{Company: &athenav1.Company{Name: "acme", Label: "Again"}})
	requireCode(t, err, connect.CodeAlreadyExists)
	_, err = svc.SaveCompany(ctx, &athenav1.SaveCompanyRequest{Company: &athenav1.Company{Name: "Bad Name!", Label: "x"}})
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestLedgerReadinessNamesActions(t *testing.T) {
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := newLedgerService(db)
	ctx := context.Background()
	res, err := svc.GetReadiness(ctx, &athenav1.GetReadinessRequest{Company: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetChecks()) != 1 || res.GetChecks()[0].GetOk() {
		t.Fatalf("missing company: %v", res.GetChecks())
	}
	if _, err := svc.SaveCompany(ctx, &athenav1.SaveCompanyRequest{Company: &athenav1.Company{Name: "acme", Label: "Acme"}}); err != nil {
		t.Fatal(err)
	}
	res, err = svc.GetReadiness(ctx, &athenav1.GetReadinessRequest{Company: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetChecks()) != 7 || !res.GetChecks()[0].GetOk() || res.GetChecks()[1].GetOk() {
		t.Fatalf("checks: %v", res.GetChecks())
	}
	for _, check := range res.GetChecks()[1:] {
		if check.GetDetail() == "" {
			t.Fatalf("check %q names no action", check.GetName())
		}
	}
}

func TestLedgerPeriodStatusMapping(t *testing.T) {
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := newLedgerService(db)
	ctx := context.Background()
	if _, err := svc.SaveCompany(ctx, &athenav1.SaveCompanyRequest{Company: &athenav1.Company{Name: "acme", Label: "Acme"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SavePeriod(ctx, &athenav1.SavePeriodRequest{Period: &athenav1.Period{Company: "acme", Name: "2026-01", Label: "Jan", StartDate: "2026-01-01", EndDate: "2026-01-31"}}); err != nil {
		t.Fatal(err)
	}
	closed, err := svc.SetPeriodStatus(ctx, &athenav1.SetPeriodStatusRequest{Company: "acme", Book: "main", Name: "2026-01", Status: "hard_closed", ExpectedRevision: 1})
	if err != nil || closed.GetPeriod().GetStatus() != "hard_closed" {
		t.Fatalf("close: %v", closed)
	}
	_, err = svc.SetPeriodStatus(ctx, &athenav1.SetPeriodStatusRequest{Company: "acme", Book: "main", Name: "2026-01", Status: "open", ExpectedRevision: 2})
	requireCode(t, err, connect.CodeInvalidArgument)
}
