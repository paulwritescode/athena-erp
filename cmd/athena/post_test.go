package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPostCommands(t *testing.T) {
	sharedDSNEnv(t)
	run := func(command func([]string, io.Writer) error, args ...string) (string, error) {
		var out bytes.Buffer
		err := command(args, &out)
		return out.String(), err
	}
	if _, err := run(migrateCommand, "up"); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for _, args := range [][]string{
		{"currency", "save", "--code", "USD", "--label", "Dollar"},
		{"company", "save", "--name", "acme", "--label", "Acme", "--base-currency", "USD"},
		{"period", "save", "--company", "acme", "--name", "fy2026", "--label", "FY 2026", "--start", "2026-01-01", "--end", "2026-12-31"},
		{"account", "type-save", "--name", "asset", "--label", "Asset", "--normal-balance", "debit"},
		{"account", "type-save", "--name", "income", "--label", "Income", "--normal-balance", "credit"},
		{"account", "save", "--name", "cash", "--label", "Cash", "--type", "asset"},
		{"account", "save", "--name", "revenue", "--label", "Revenue", "--type", "income"},
	} {
		if _, err := run(ledgerCommand, args...); err != nil {
			t.Fatalf("ledger %v: %v", args, err)
		}
	}
	definition := write("bill.json", `{"key":"bill","label":"Bill","kind":"document","class":"posting","submittable":true,"fields":[{"key":"total","label":"Total","type":"Number"}]}`)
	if _, err := run(recordTypesCommand, "save", "--file", definition); err != nil {
		t.Fatalf("save bill type: %v", err)
	}
	rule := write("bill.star", `
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
    return [{"name": "standard", "values": {"total": decimal("100")}}]
`)
	if _, err := run(rulesCommand, "save", "bill_post", "--file", rule, "--type", "posting", "--record-type", "bill", "--event", "on_submit", "--enable"); err != nil {
		t.Fatalf("save posting rule: %v", err)
	}
	created, err := run(recordsCommand, "create", "bill", "--values", `{"total":"100"}`, "--json")
	if err != nil {
		t.Fatal(err)
	}
	id := regexp.MustCompile(`"id":\s+"([0-9a-f-]{36})"`).FindStringSubmatch(created)[1]

	if out, err := run(postCommand, "preview", "bill", id); err != nil || !strings.Contains(out, "plan 1: bill_post") || !strings.Contains(out, "cash debit 100") || !strings.Contains(out, "plan hash: ") {
		t.Fatalf("preview = %q, %v", out, err)
	}
	hash := regexp.MustCompile(`plan hash: ([0-9a-f]{64})`).FindStringSubmatch(mustPost(t, run, "preview", "bill", id))[1]
	if _, err := run(postCommand, "submit", "bill", id, "--revision", "1", "--preview-hash", "stale"); err == nil || !strings.Contains(err.Error(), "changed since it was previewed") {
		t.Fatalf("a stale preview hash must refuse the submit: %v", err)
	}
	if out, err := run(postCommand, "submit", "bill", id, "--revision", "1", "--preview-hash", hash); err != nil || !strings.Contains(out, "is now submitted (revision 2)") {
		t.Fatalf("submit = %q, %v", out, err)
	}
	if out, err := run(postCommand, "status", "bill", id); err != nil || !strings.Contains(out, "submit") || !strings.Contains(out, "2 rows") {
		t.Fatalf("status = %q, %v", out, err)
	}
	if out, err := run(postCommand, "log", "bill", id); err != nil || !strings.Contains(out, "posted") {
		t.Fatalf("log = %q, %v", out, err)
	}
	if out, err := run(postCommand, "cancel", "bill", id, "--revision", "2"); err != nil || !strings.Contains(out, "is now cancelled (revision 3)") {
		t.Fatalf("cancel = %q, %v", out, err)
	}
	if out, err := run(postCommand, "status", "bill", id); err != nil || !strings.Contains(out, "reversed") {
		t.Fatalf("status after cancel = %q, %v", out, err)
	}
}

func mustPost(t *testing.T, run func(func([]string, io.Writer) error, ...string) (string, error), args ...string) string {
	t.Helper()
	out, err := run(postCommand, args...)
	if err != nil {
		t.Fatalf("post %v: %v", args, err)
	}
	return out
}
