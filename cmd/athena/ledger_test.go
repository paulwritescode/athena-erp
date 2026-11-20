package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

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
