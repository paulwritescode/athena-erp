package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"athena-erp/internal/ledger"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
)

func ledgerRun(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := ledgerCommand(args, &out); err != nil {
		t.Fatalf("ledger %v: %v", args, err)
	}
	return out.String()
}

func ledgerMigrate(t *testing.T) {
	t.Helper()
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir()) // no home here, so the DSN fallback applies
	if err := migrateCommand([]string{"up"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
}

func TestLedgerCLICompanyAndPeriod(t *testing.T) {
	ledgerMigrate(t)

	if out := ledgerRun(t, "company", "save", "--name", "acme", "--label", "Acme"); !strings.Contains(out, "revision 1") {
		t.Fatalf("company save = %q", out)
	}
	if out := ledgerRun(t, "company", "list"); !strings.Contains(out, "acme") {
		t.Fatalf("company list = %q", out)
	}
	period := []string{"period", "save", "--company", "acme", "--name", "2026-01", "--label", "Jan", "--start", "2026-01-01", "--end", "2026-01-31"}
	if out := ledgerRun(t, period...); !strings.Contains(out, "revision 1") {
		t.Fatalf("period save = %q", out)
	}
	if out := ledgerRun(t, "period", "close", "--company", "acme", "--name", "2026-01", "--expected-revision", "1"); !strings.Contains(out, "soft_closed") {
		t.Fatalf("period close = %q", out)
	}
	if out := ledgerRun(t, "period", "reopen", "--company", "acme", "--name", "2026-01", "--expected-revision", "2"); !strings.Contains(out, "open") {
		t.Fatalf("period reopen = %q", out)
	}
	if out := ledgerRun(t, "period", "list", "--company", "acme"); !strings.Contains(out, "2026-01") {
		t.Fatalf("period list = %q", out)
	}
}

func TestLedgerCLICurrencyAccountDetermination(t *testing.T) {
	ledgerMigrate(t)

	ledgerRun(t, "currency", "save", "--code", "USD", "--label", "Dollar")
	ledgerRun(t, "currency", "save", "--code", "EUR", "--label", "Euro")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if out := ledgerRun(t, "currency", "rate-save", "--from", "USD", "--to", "EUR", "--type", "spot", "--source", "ecb", "--at", at, "--rate", "0.92"); !strings.Contains(out, "USD->EUR") {
		t.Fatalf("rate-save = %q", out)
	}
	if out := ledgerRun(t, "currency", "rate-get", "--from", "USD", "--to", "EUR", "--type", "spot", "--at", at); !strings.Contains(out, "0.92") {
		t.Fatalf("rate-get = %q", out)
	}
	ledgerRun(t, "account", "type-save", "--name", "asset", "--label", "Asset", "--normal-balance", "debit")
	ledgerRun(t, "account", "save", "--name", "assets", "--label", "Assets", "--type", "asset", "--group")
	ledgerRun(t, "account", "save", "--name", "cash", "--label", "Cash", "--type", "asset", "--parent", "assets")
	if out := ledgerRun(t, "account", "list", "--company", "acme"); !strings.Contains(out, "cash") {
		t.Fatalf("account list = %q", out)
	}
	if out := ledgerRun(t, "determination", "save", "--label", "petty", "--account", "cash", "--match", "company=acme"); !strings.Contains(out, "revision 1") {
		t.Fatalf("determination save = %q", out)
	}
	if out := ledgerRun(t, "determination", "resolve", "--context", "company=acme"); !strings.Contains(out, "cash") {
		t.Fatalf("determination resolve = %q", out)
	}
}

func TestLedgerCLISettingLifecycle(t *testing.T) {
	ledgerMigrate(t)

	ledgerRun(t, "setting", "save", "--pack", "core", "--type", "grace_days", "--label", "Grace", "--kind", "scalar")
	when := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	if out := ledgerRun(t, "setting", "save-value", "--pack", "core", "--type", "grace_days", "--effective-from", when, "--value-json", `"5"`); !strings.Contains(out, "revision 1") {
		t.Fatalf("save-value = %q", out)
	}
	if out := ledgerRun(t, "setting", "publish", "--pack", "core", "--type", "grace_days", "--effective-from", when, "--expected-revision", "1"); !strings.Contains(out, "Published") {
		t.Fatalf("publish = %q", out)
	}
	if out := ledgerRun(t, "setting", "resolve", "--pack", "core", "--type", "grace_days", "--at", time.Now().UTC().Add(72*time.Hour).Format(time.RFC3339)); !strings.Contains(out, "5") {
		t.Fatalf("resolve = %q", out)
	}
	if out := ledgerRun(t, "setting", "list-values", "--pack", "core", "--type", "grace_days"); !strings.Contains(out, "published") {
		t.Fatalf("list-values = %q", out)
	}
}

func TestLedgerCLITreeShow(t *testing.T) {
	ledgerMigrate(t)

	ledgerRun(t, "account", "type-save", "--name", "asset", "--label", "Asset", "--normal-balance", "debit")
	ledgerRun(t, "account", "save", "--name", "assets", "--label", "Assets", "--type", "asset", "--group")
	ledgerRun(t, "account", "save", "--name", "cash", "--label", "Cash", "--type", "asset", "--parent", "assets", "--group")
	ledgerRun(t, "account", "save", "--name", "bank", "--label", "Bank", "--type", "asset", "--parent", "assets")
	ledgerRun(t, "account", "save", "--name", "petty", "--label", "Petty", "--type", "asset", "--parent", "cash")
	out := ledgerRun(t, "tree", "show", "--master", "account")
	for _, want := range []string{"assets\tAssets\n", "  bank\tBank\n", "  cash\tCash\n", "    petty\tPetty\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tree show = %q, want %q", out, want)
		}
	}
	// Siblings print in name order however they were saved.
	if strings.Index(out, "bank") > strings.Index(out, "cash") {
		t.Fatalf("tree show is not sorted: %q", out)
	}
	if out := ledgerRun(t, "tree", "show", "--master", "account", "--root", "cash"); strings.Contains(out, "bank") || !strings.Contains(out, "petty") {
		t.Fatalf("tree --root cash = %q", out)
	}
	var nodes []treeNode
	if err := json.Unmarshal([]byte(ledgerRun(t, "tree", "show", "--master", "account", "--json")), &nodes); err != nil {
		t.Fatal(err)
	}
	depth := map[string]int{}
	for _, node := range nodes {
		depth[node.Name] = node.Depth
	}
	if len(nodes) != 4 || depth["assets"] != 0 || depth["cash"] != 1 || depth["petty"] != 2 {
		t.Fatalf("tree json = %+v", nodes)
	}
	if err := ledgerCommand([]string{"tree", "show", "--master", "ledger"}, &bytes.Buffer{}); err == nil {
		t.Fatal("tree show with a bad master succeeded")
	}
	if err := ledgerCommand([]string{"tree", "show", "--master", "account", "--root", "missing"}, &bytes.Buffer{}); err == nil {
		t.Fatal("tree show with a missing root succeeded")
	}
	// A second master renders through the same verb (branches have no save
	// noun, so the row goes through the service the CLI reads back from the
	// same database).
	svc, ctx := ledgerDirectService(t)
	ledgerRun(t, "company", "save", "--name", "acme", "--label", "Acme")
	if _, err := svc.SaveBranch(ctx, ledger.Branch{Name: "hq", Label: "HQ", Company: "acme", IsGroup: true}, 0); err != nil {
		t.Fatal(err)
	}
	if out := ledgerRun(t, "tree", "show", "--master", "branch", "--company", "acme"); !strings.Contains(out, "hq\tHQ") {
		t.Fatalf("branch tree = %q", out)
	}
}

// ledgerDirectService migrates a throwaway database and returns the ledger
// service over it for rows no CLI noun can save. The CLI reads the same
// database back through the DSN fallback.
func ledgerDirectService(t *testing.T) (*ledger.Service, context.Context) {
	t.Helper()
	db, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir())
	if err := migrateCommand([]string{"up"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return &ledger.Service{DB: db, Dialect: store.Dialect(store.MariaDB{})}, context.Background()
}

func TestOrderTreeRefusesCycles(t *testing.T) {
	nodes := []treeNode{{Name: "a", Parent: "b"}, {Name: "b", Parent: "a"}}
	if _, err := orderTree(nodes, "a"); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("orderTree = %v, want a cycle error", err)
	}
}

func TestLedgerCLIUoM(t *testing.T) {
	ledgerMigrate(t)

	if out := ledgerRun(t, "uom", "family-save", "--name", "mass", "--label", "Mass", "--base-unit", "kg"); !strings.Contains(out, "revision 1") {
		t.Fatalf("family-save = %q", out)
	}
	if out := ledgerRun(t, "uom", "family-list"); !strings.Contains(out, "mass") || !strings.Contains(out, "base kg") {
		t.Fatalf("family-list = %q", out)
	}
	if out := ledgerRun(t, "uom", "conversion-save", "--family", "mass", "--unit", "g", "--factor", "0.001"); !strings.Contains(out, "revision 1") {
		t.Fatalf("conversion-save = %q", out)
	}
	if out := ledgerRun(t, "uom", "conversion-get", "--family", "mass", "--unit", "g"); !strings.Contains(out, "factor 0.001") || !strings.Contains(out, "default") {
		t.Fatalf("conversion-get = %q", out)
	}
	if out := ledgerRun(t, "uom", "conversion-save", "--family", "mass", "--unit", "g", "--factor", "0.002", "--item", "widget"); !strings.Contains(out, "revision 1") {
		t.Fatalf("override save = %q", out)
	}
	if out := ledgerRun(t, "uom", "conversion-list", "--family", "mass"); !strings.Contains(out, "item widget") {
		t.Fatalf("conversion-list = %q", out)
	}
	if out := ledgerRun(t, "uom", "convert", "--value", "1500", "--from", "g", "--to", "kg"); !strings.Contains(out, "1500 g = 1.500000 kg") {
		t.Fatalf("convert = %q", out)
	}
	if out := ledgerRun(t, "uom", "convert", "--value", "1500", "--from", "g", "--to", "kg", "--item", "widget"); !strings.Contains(out, "1500 g = 3.000000 kg") {
		t.Fatalf("override convert = %q", out)
	}
	ledgerRun(t, "uom", "family-save", "--name", "length", "--label", "Length", "--base-unit", "m")
	if err := ledgerCommand([]string{"uom", "convert", "--value", "1", "--from", "g", "--to", "m"}, &bytes.Buffer{}); err == nil {
		t.Fatal("cross-family convert succeeded")
	}
	if err := ledgerCommand([]string{"uom", "convert", "--value", "1", "--from", "g", "--to", "kg", "--mode", "up"}, &bytes.Buffer{}); err == nil {
		t.Fatal("convert with a bad mode succeeded")
	}
	if err := ledgerCommand([]string{"uom", "family-save", "--name", "mass", "--label", "Mass", "--base-unit", "kg"}, &bytes.Buffer{}); err == nil {
		t.Fatal("duplicate family-save succeeded")
	}
}

func TestLedgerCLIDeterminationEffective(t *testing.T) {
	ledgerMigrate(t)

	ledgerRun(t, "account", "type-save", "--name", "asset", "--label", "Asset", "--normal-balance", "debit")
	ledgerRun(t, "account", "save", "--name", "cash", "--label", "Cash", "--type", "asset")
	ledgerRun(t, "account", "save", "--name", "bank", "--label", "Bank", "--type", "asset")
	ledgerRun(t, "determination", "save", "--label", "old", "--account", "cash", "--match", "company=acme", "--start", "2025-01-01", "--end", "2026-01-01")
	ledgerRun(t, "determination", "save", "--label", "new", "--account", "bank", "--match", "company=acme", "--start", "2026-01-01")
	if out := ledgerRun(t, "determination", "list"); !strings.Contains(out, "effective 2025-01-01..2026-01-01") || !strings.Contains(out, "effective 2026-01-01..") {
		t.Fatalf("determination list = %q", out)
	}
	if out := ledgerRun(t, "determination", "resolve", "--context", "company=acme", "--as-of", "2025-06-01"); !strings.Contains(out, "cash") {
		t.Fatalf("resolve as-of 2025 = %q", out)
	}
	if out := ledgerRun(t, "determination", "resolve", "--context", "company=acme", "--as-of", "2026-06-01"); !strings.Contains(out, "bank") {
		t.Fatalf("resolve as-of 2026 = %q", out)
	}
	if err := ledgerCommand([]string{"determination", "resolve", "--context", "company=acme", "--as-of", "2024-06-01"}, &bytes.Buffer{}); err == nil {
		t.Fatal("resolve outside every range succeeded")
	}
	if err := ledgerCommand([]string{"determination", "resolve", "--context", "company=acme", "--as-of", "June"}, &bytes.Buffer{}); err == nil {
		t.Fatal("resolve with a bad as-of succeeded")
	}
}
