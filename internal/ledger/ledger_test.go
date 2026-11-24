// Package ledger_test holds the database-backed ledger master tests. It is an
// external test package so it can migrate with server.SystemMigrations
// without an import cycle.
package ledger_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"athena-erp/internal/ledger"
	"athena-erp/internal/store"
	"athena-erp/internal/types"
)

func service(t *testing.T) *ledger.Service {
	t.Helper()
	db := sharedDatabase(t)
	return &ledger.Service{DB: db, Dialect: store.MariaDB{}}
}

func mustCompany(t *testing.T, s *ledger.Service, name string) ledger.Company {
	t.Helper()
	company, err := s.SaveCompany(context.Background(), ledger.Company{Name: name, Label: name}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return company
}

func TestCompanyLifecycle(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	company, err := s.SaveCompany(ctx, ledger.Company{Name: "acme", Label: "Acme"}, 0)
	if err != nil || company.Revision != 1 {
		t.Fatalf("create: %+v %v", company, err)
	}
	if _, err := s.SaveCompany(ctx, ledger.Company{Name: "acme", Label: "Again"}, 0); !errors.Is(err, ledger.ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	company.Label = "Acme Ltd"
	company, err = s.SaveCompany(ctx, company, 1)
	if err != nil || company.Revision != 2 {
		t.Fatalf("update: %+v %v", company, err)
	}
	if _, err := s.SaveCompany(ctx, company, 1); !errors.Is(err, ledger.ErrConflict) {
		t.Fatalf("stale: %v", err)
	}
	if _, err := s.SaveCompany(ctx, ledger.Company{Name: "Bad Name!", Label: "x"}, 0); err == nil {
		t.Fatal("bad name accepted")
	}
	if _, err := s.SaveCompany(ctx, ledger.Company{Name: "nope", Label: "x", BaseCurrency: "XX"}, 0); err == nil {
		t.Fatal("unknown base currency accepted")
	}
	got, err := s.GetCompany(ctx, "acme")
	if err != nil || got.Label != "Acme Ltd" || got.Revision != 2 {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := s.GetCompany(ctx, "missing"); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestNestedMastersCheckParentsGroupsAndCycles(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustCompany(t, s, "acme")
	mustCompany(t, s, "globex")
	if _, err := s.SaveBranch(ctx, ledger.Branch{Name: "north", Label: "North", Company: "missing"}, 0); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("branch without company: %v", err)
	}
	north, err := s.SaveBranch(ctx, ledger.Branch{Name: "north", Label: "North", Company: "acme", IsGroup: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	north1, err := s.SaveBranch(ctx, ledger.Branch{Name: "north_1", Label: "N1", Company: "acme", Parent: "north"}, 0)
	if err != nil {
		t.Fatalf("child branch: %v", err)
	}
	// A leaf cannot parent, and a parent must be a group in the same company.
	if _, err := s.SaveBranch(ctx, ledger.Branch{Name: "deep", Label: "D", Company: "acme", Parent: "north_1"}, 0); err == nil {
		t.Fatal("leaf parent accepted")
	}
	if _, err := s.SaveBranch(ctx, ledger.Branch{Name: "x", Label: "X", Company: "globex", Parent: "north"}, 0); err == nil {
		t.Fatal("cross-company parent accepted")
	}
	// Closing the cycle is refused.
	north1.IsGroup = true
	if _, err := s.SaveBranch(ctx, north1, 1); err != nil {
		t.Fatal(err)
	}
	north.Parent = "north_1"
	north.IsGroup = true
	if _, err := s.SaveBranch(ctx, north, 1); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle: %v", err)
	}
	// A node with children must stay a group.
	north.Parent = ""
	north.IsGroup = false
	if _, err := s.SaveBranch(ctx, north, 1); err == nil || !strings.Contains(err.Error(), "must be a group") {
		t.Fatalf("ungroup with children: %v", err)
	}
	branches, err := s.ListBranches(ctx, "acme")
	if err != nil || len(branches) != 2 {
		t.Fatalf("branches: %v %v", branches, err)
	}
}

func TestDepartmentBranchLinkAndCostCentre(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustCompany(t, s, "acme")
	mustCompany(t, s, "globex")
	if _, err := s.SaveBranch(ctx, ledger.Branch{Name: "north", Label: "N", Company: "acme"}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveDepartment(ctx, ledger.Department{Name: "sales", Label: "S", Company: "globex", Branch: "north"}, 0); err == nil {
		t.Fatal("cross-company branch accepted")
	}
	if _, err := s.SaveDepartment(ctx, ledger.Department{Name: "sales", Label: "S", Company: "acme", Branch: "north"}, 0); err != nil {
		t.Fatalf("department: %v", err)
	}
	if _, err := s.SaveCostCentre(ctx, ledger.CostCentre{Name: "cc1", Label: "C", Company: "acme"}, 0); err != nil {
		t.Fatalf("cost centre: %v", err)
	}
	if _, err := s.GetCostCentre(ctx, "missing"); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestDimensionMembersMandatoryAndValidity(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustCompany(t, s, "acme")
	if _, err := s.SaveAccountType(ctx, ledger.AccountType{Name: "asset", Label: "Asset", NormalBalance: "debit"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveAccount(ctx, ledger.Account{Name: "cash", Label: "Cash", Type: "asset"}, 0); err != nil {
		t.Fatal(err)
	}
	good := ledger.Dimension{Name: "project", Label: "Project", Members: []string{"p1", "p2"}, MandatoryFor: []string{"cash"}, ValidFrom: "2026-01-01", ValidTo: "2026-12-31"}
	if _, err := s.SaveDimension(ctx, good, 0); err != nil {
		t.Fatalf("dimension: %v", err)
	}
	bad := good
	bad.Name = "bad"
	bad.MandatoryFor = []string{"missing"}
	if _, err := s.SaveDimension(ctx, bad, 0); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("unknown mandatory account: %v", err)
	}
	bad = good
	bad.Name = "bad2"
	bad.ValidFrom, bad.ValidTo = "2026-12-31", "2026-01-01"
	if _, err := s.SaveDimension(ctx, bad, 0); err == nil {
		t.Fatal("reversed validity accepted")
	}
	got, err := s.GetDimension(ctx, "project")
	if err != nil || len(got.Members) != 2 || len(got.MandatoryFor) != 1 {
		t.Fatalf("get: %+v %v", got, err)
	}
}

func TestFiscalYearAndPeriodOverlap(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustCompany(t, s, "acme")
	if _, err := s.SaveFiscalYear(ctx, ledger.FiscalYear{Name: "fy26", Label: "FY26", Company: "acme", StartDate: "2026-04-01", EndDate: "2027-03-31"}, 0); err != nil {
		t.Fatalf("fiscal year: %v", err)
	}
	jan, err := s.SavePeriod(ctx, ledger.Period{Company: "acme", Name: "2026-01", Label: "Jan", StartDate: "2026-01-01", EndDate: "2026-01-31"}, 0)
	if err != nil || jan.Book != "main" || jan.Status != ledger.PeriodOpen {
		t.Fatalf("jan: %+v %v", jan, err)
	}
	// Adjacent is fine; sharing one date is not.
	if _, err := s.SavePeriod(ctx, ledger.Period{Company: "acme", Name: "2026-02", Label: "Feb", StartDate: "2026-02-01", EndDate: "2026-02-28"}, 0); err != nil {
		t.Fatalf("adjacent: %v", err)
	}
	if _, err := s.SavePeriod(ctx, ledger.Period{Company: "acme", Name: "overlap", Label: "O", StartDate: "2026-01-31", EndDate: "2026-02-05"}, 0); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("overlap: %v", err)
	}
	// Another book is another calendar.
	if _, err := s.SavePeriod(ctx, ledger.Period{Company: "acme", Book: "tax", Name: "2026-01", Label: "Jan", StartDate: "2026-01-01", EndDate: "2026-01-31"}, 0); err != nil {
		t.Fatalf("other book: %v", err)
	}
	// Moving February back over January is refused.
	feb := ledger.Period{Company: "acme", Name: "2026-02", Label: "Feb", StartDate: "2026-01-15", EndDate: "2026-02-28"}
	if _, err := s.SavePeriod(ctx, feb, 1); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("move into overlap: %v", err)
	}
}

func TestPeriodCloseAndReopen(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustCompany(t, s, "acme")
	if _, err := s.SavePeriod(ctx, ledger.Period{Company: "acme", Name: "2026-01", Label: "Jan", StartDate: "2026-01-01", EndDate: "2026-01-31"}, 0); err != nil {
		t.Fatal(err)
	}
	soft, err := s.SetPeriodStatus(ctx, "acme", "main", "2026-01", ledger.PeriodSoftClosed, "ana", 1)
	if err != nil || soft.Status != ledger.PeriodSoftClosed || soft.Revision != 2 {
		t.Fatalf("soft close: %+v %v", soft, err)
	}
	open, err := s.SetPeriodStatus(ctx, "acme", "main", "2026-01", ledger.PeriodOpen, "ana", 2)
	if err != nil || open.Status != ledger.PeriodOpen {
		t.Fatalf("reopen soft: %+v %v", open, err)
	}
	hard, err := s.SetPeriodStatus(ctx, "acme", "main", "2026-01", ledger.PeriodHardClosed, "ana", 3)
	if err != nil || hard.Status != ledger.PeriodHardClosed {
		t.Fatalf("hard close: %+v %v", hard, err)
	}
	if _, err := s.SetPeriodStatus(ctx, "acme", "main", "2026-01", ledger.PeriodOpen, "ana", 4); err == nil || !strings.Contains(err.Error(), "approved reopen") {
		t.Fatalf("reopen hard: %v", err)
	}
	if _, err := s.SetPeriodStatus(ctx, "acme", "main", "2026-01", "bogus", "ana", 4); err == nil {
		t.Fatal("bogus status accepted")
	}
	found, err := s.PeriodFor(ctx, "acme", "main", "2026-01-15")
	if err != nil || found.Name != "2026-01" {
		t.Fatalf("period for: %+v %v", found, err)
	}
	if _, err := s.PeriodFor(ctx, "acme", "main", "2027-05-05"); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("gap: %v", err)
	}
}

func TestCurrencyAndRateLookup(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	if _, err := s.SaveCurrency(ctx, ledger.Currency{Code: "USD", Label: "Dollar", Exponent: 2}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveCurrency(ctx, ledger.Currency{Code: "EUR", Label: "Euro", Exponent: 2}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveCurrency(ctx, ledger.Currency{Code: "usd", Label: "x"}, 0); err == nil {
		t.Fatal("lowercase code accepted")
	}
	day1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for _, rate := range []ledger.ExchangeRate{
		{From: "USD", To: "EUR", RateType: "spot", Source: "ecb", EffectiveAt: day1, Rate: mustDecimal(t, "0.90")},
		{From: "USD", To: "EUR", RateType: "spot", Source: "ecb", EffectiveAt: day2, Rate: mustDecimal(t, "0.92")},
		{From: "USD", To: "EUR", RateType: "spot", Source: "fed", EffectiveAt: day1, Rate: mustDecimal(t, "0.91")},
	} {
		if _, err := s.SaveRate(ctx, rate); err != nil {
			t.Fatalf("rate: %v", err)
		}
	}
	got, err := s.GetRate(ctx, "USD", "EUR", "spot", "ecb", day2)
	if err != nil || !got.Equal(mustDecimal(t, "0.92")) {
		t.Fatalf("exact: %v %v", got, err)
	}
	got, err = s.GetRate(ctx, "USD", "EUR", "spot", "ecb", day1.Add(12*time.Hour))
	if err != nil || !got.Equal(mustDecimal(t, "0.90")) {
		t.Fatalf("earlier instant: %v %v", got, err)
	}
	if _, err := s.GetRate(ctx, "USD", "EUR", "spot", "", day2); !errors.Is(err, ledger.ErrAmbiguous) {
		t.Fatalf("disagreeing sources: %v", err)
	}
	same, err := s.GetRate(ctx, "USD", "USD", "spot", "", day2)
	if err != nil || same.String() != "1" {
		t.Fatalf("same code: %v %v", same, err)
	}
	if _, err := s.GetRate(ctx, "USD", "EUR", "spot", "ecb", day1.Add(-time.Hour)); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("before any rate: %v", err)
	}
}

func mustDecimal(t *testing.T, text string) types.Decimal {
	t.Helper()
	d, err := types.ParseDecimal(text)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
