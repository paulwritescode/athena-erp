package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"athena-erp/internal/posting"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
	"athena-erp/internal/types"
)

func TestSettleCommands(t *testing.T) {
	db, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir())
	run := func(command func([]string, io.Writer) error, args ...string) (string, error) {
		var out bytes.Buffer
		err := command(args, &out)
		return out.String(), err
	}
	if _, err := run(migrateCommand, "up"); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	for _, args := range [][]string{
		{"currency", "save", "--code", "USD", "--label", "Dollar"},
		{"company", "save", "--name", "acme", "--label", "Acme", "--base-currency", "USD"},
		{"period", "save", "--company", "acme", "--name", "fy2026", "--label", "FY 2026", "--start", "2026-01-01", "--end", "2026-12-31"},
		{"account", "type-save", "--name", "asset", "--label", "Asset", "--normal-balance", "debit"},
		{"account", "save", "--name", "receivable", "--label", "Receivable", "--type", "asset", "--control-role", "receivable"},
	} {
		if _, err := run(ledgerCommand, args...); err != nil {
			t.Fatalf("ledger %v: %v", args, err)
		}
	}
	definition := `{"key":"%s","label":"%s","kind":"%s"%s,"fields":[{"key":"total","label":"Total","type":"Number"}]}`
	shapes := map[string][2]string{"receipt": {"document", `,"class":"non_posting","submittable":true`}, "customer": {"master", ""}}
	for _, key := range []string{"receipt", "customer"} {
		shape := shapes[key]
		path := t.TempDir() + "/" + key + ".json"
		if err := os.WriteFile(path, []byte(fmt.Sprintf(definition, key, key, shape[0], shape[1])), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := run(recordTypesCommand, "save", "--file", path); err != nil {
			t.Fatalf("save %s type: %v", key, err)
		}
	}
	// Items have no CLI surface (instalments come from invoices); the test
	// records one through the service, like the invoice submit path will.
	settle := &posting.SettlementService{DB: db, Dialect: store.MariaDB{}}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	created, err := settle.CreateItemsInTx(context.Background(), tx, []posting.NewItem{{
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
	itemID := created[0].ID

	open, err := run(settleCommand, "open-items", "--company", "acme", "--book", "main", "--party-type", "customer", "--party-id", "c1")
	if err != nil {
		t.Fatalf("open-items: %v", err)
	}
	if !strings.Contains(open, itemID) || !strings.Contains(open, "100.00") {
		t.Fatalf("open-items printed %q", open)
	}
	applied, err := run(settleCommand, "apply",
		"--company", "acme", "--book", "main", "--party-type", "customer", "--party-id", "c1",
		"--control-account", "receivable", "--side", "debit",
		"--voucher-type", "receipt", "--voucher-id", "r1", "--revision", "2",
		"--date", "2026-06-20", "--currency", "USD",
		"--lines", `[{"line_no":1,"amount":"100.00","items":[{"item_id":"`+itemID+`"}]}]`)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(applied, itemID) {
		t.Fatalf("apply printed %q", applied)
	}
	reversed, err := run(settleCommand, "reverse",
		"--company", "acme", "--book", "main", "--party-type", "customer", "--party-id", "c1",
		"--control-account", "receivable", "--side", "debit",
		"--voucher-type", "receipt", "--voucher-id", "r1", "--revision", "3",
		"--date", "2026-07-20")
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if !strings.Contains(reversed, "reversed 1 application(s)") {
		t.Fatalf("reverse printed %q", reversed)
	}
	if _, err := run(settleCommand, "apply", "--lines", "nope"); err == nil {
		t.Fatal("bad lines JSON accepted")
	}
}
