package main

import (
	"bytes"
	"strings"
	"testing"

	"athena-erp/internal/ledger"
	"athena-erp/internal/testdb"
)

// The CLI opens the ledger service through ledgerService on the workspace
// DSN, which sets no parseTime: DATE columns must still scan. A connector
// without ParseTime fails reads with "unsupported Scan, storing driver.Value
// type []uint8 into type *time.Time" (found by the F7 E2E script on a real
// workspace; unit tests never saw it because test databases set parseTime).
func TestLedgerServiceReadsDatesWithoutParseTime(t *testing.T) {
	_, cfg := testdb.NewWithConfig(t)
	cfg.ParseTime = false
	if strings.Contains(cfg.FormatDSN(), "parseTime=true") {
		t.Fatal("test DSN still sets parseTime")
	}
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir()) // no home here, so the DSN fallback applies
	if err := migrateCommand([]string{"up"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc, ctx, cancel, err := ledgerService("")
	if err != nil {
		t.Fatalf("ledgerService: %v", err)
	}
	defer cancel()
	if _, err := svc.SaveCompany(ctx, ledger.Company{Name: "acme", Label: "Acme"}, 0); err != nil {
		t.Fatalf("save company: %v", err)
	}
	if _, err := svc.SavePeriod(ctx, ledger.Period{Company: "acme", Book: "main", Name: "2026-01", Label: "Jan", StartDate: "2026-01-01", EndDate: "2026-01-31"}, 0); err != nil {
		t.Fatalf("save period: %v", err)
	}
	periods, err := svc.ListPeriods(ctx, "acme", "main")
	if err != nil {
		t.Fatalf("list periods: %v", err)
	}
	if len(periods) != 1 || periods[0].StartDate != "2026-01-01" || periods[0].EndDate != "2026-01-31" {
		t.Fatalf("periods = %+v", periods)
	}
	if _, err := svc.SaveAccountType(ctx, ledger.AccountType{Name: "asset", Label: "Asset", NormalBalance: "debit"}); err != nil {
		t.Fatalf("save account type: %v", err)
	}
	if _, err := svc.SaveAccount(ctx, ledger.Account{Name: "cash", Label: "Cash", Type: "asset"}, 0); err != nil {
		t.Fatalf("save account: %v", err)
	}
	if _, err := svc.SaveDetermination(ctx, ledger.Determination{Label: "petty", Account: "cash", Match: map[string]string{"company": "acme"}, EffectiveFrom: "2026-01-01", EffectiveTo: "2027-01-01"}, 0); err != nil {
		t.Fatalf("save determination: %v", err)
	}
	mappings, err := svc.ListDeterminations(ctx)
	if err != nil {
		t.Fatalf("list determinations: %v", err)
	}
	if len(mappings) != 1 || mappings[0].EffectiveFrom != "2026-01-01" || mappings[0].EffectiveTo != "2027-01-01" {
		t.Fatalf("determinations = %+v", mappings)
	}
}
