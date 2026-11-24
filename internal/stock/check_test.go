package stock_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"athena-erp/internal/ledger"
	"athena-erp/internal/stock"

	"github.com/google/uuid"
)

func TestCheckMatchesBalancedFlow(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)
	postMovement(t, db, stockSvc, engine, issueOf(receipt, "40"))
	delta := testDelta(receiptApplied.RowID)
	postDelta(t, db, stockSvc, engine, delta)

	report, err := stockSvc.CheckStockToGL(context.Background(), db, "acme", "main", "2026-12-31", "controller")
	if err != nil {
		t.Fatal(err)
	}
	if !report.Matched() {
		t.Fatalf("report matched nothing: %+v", report.Accounts)
	}
	if len(report.Accounts) != 1 || report.Accounts[0].Control != "stock" {
		t.Fatalf("accounts are %+v, want [stock]", report.Accounts)
	}
	if got := report.Accounts[0].Observed; got != "780.000000" {
		t.Fatalf("observed is %s, want 780.000000", got)
	}
	var logs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_integrity_log`).Scan(&logs); err != nil {
		t.Fatal(err)
	}
	if logs != 0 {
		t.Fatalf("check wrote %d log rows on a match, want none", logs)
	}
	early, err := stockSvc.CheckStockToGL(context.Background(), db, "acme", "main", "2026-06-15", "controller")
	if err != nil {
		t.Fatal(err)
	}
	if !early.Matched() {
		t.Fatalf("early cutoff matched nothing: %+v", early.Accounts)
	}
}

func rogueGLRow(t *testing.T, db *sql.DB, account, debit, postingDate string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO l_gl (id, company_name, book, account_name, period_name, posting_date, debit_base, credit_base, txn_debit, txn_credit, dimensions, voucher_type, voucher_id, voucher_revision, event, posting_group_id, is_reversal) VALUES (?, 'acme', 'main', ?, 'fy2026', ?, ?, 0, 0, 0, '{}', 'rogue', ?, 1, 'submit', ?, 0)`,
		uuid.NewString(), account, postingDate, debit, uuid.NewString(), uuid.NewString()); err != nil {
		t.Fatal(err)
	}
}

func rogueStockRow(t *testing.T, db *sql.DB, itemID, value, postingDate string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO l_stock (id, company_name, book, owner, item_type, item_id, control_account_name, location_name, base_uom, qty_base, value_base, base_currency, movement_date, posting_date, dimensions, voucher_type, voucher_id, voucher_revision, source_line_no, event, posting_group_id, is_reversal) VALUES (?, 'acme', 'main', 'company', 'test_widget', ?, 'stock', 'wh_main', 'each', 0, ?, 'USD', ?, ?, '{}', 'rogue', ?, 1, 1, 'submit', ?, 0)`,
		uuid.NewString(), itemID, value, postingDate, postingDate, uuid.NewString(), uuid.NewString()); err != nil {
		t.Fatal(err)
	}
}

func TestCheckFlagsGLHeavyBreak(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)
	postMovement(t, db, stockSvc, engine, issueOf(receipt, "40"))
	postDelta(t, db, stockSvc, engine, testDelta(receiptApplied.RowID))
	rogueGLRow(t, db, "stock", "100", "2026-06-20")

	report, err := stockSvc.CheckStockToGL(context.Background(), db, "acme", "main", "2026-12-31", "controller")
	if err != nil {
		t.Fatal(err)
	}
	if report.Matched() {
		t.Fatal("report matched, want a break")
	}
	got := report.Accounts[0]
	if got.Expected != "880.000000" || got.Observed != "780.000000" {
		t.Fatalf("expected/observed is %s/%s, want 880.000000/780.000000", got.Expected, got.Observed)
	}
	if got.LogID == "" {
		t.Fatal("break has no log id")
	}
	var owner, check, status, detail string
	if err := db.QueryRow(`SELECT owner, check_name, status, detail FROM athena_integrity_log WHERE id = ?`, got.LogID).Scan(&owner, &check, &status, &detail); err != nil {
		t.Fatal(err)
	}
	if owner != "controller" || check != stock.StockToGLCheck || status != "open" {
		t.Fatalf("log row is %s/%s/%s, want controller/stock_to_gl/open", owner, check, status)
	}
	if detail == "" || detail == "{}" {
		t.Fatalf("log detail is %q, want a breakdown", detail)
	}
}

func TestCheckFlagsStockHeavyBreak(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)
	rogueStockRow(t, db, receipt.ItemID, "50", "2026-06-20")

	report, err := stockSvc.CheckStockToGL(context.Background(), db, "acme", "main", "2026-12-31", "controller")
	if err != nil {
		t.Fatal(err)
	}
	if report.Matched() {
		t.Fatal("report matched, want a break")
	}
	if got := report.Accounts[0]; got.Expected != "1000.000000" || got.Observed != "1050.000000" {
		t.Fatalf("expected/observed is %s/%s, want 1000.000000/1050.000000", got.Expected, got.Observed)
	}
}

func TestCheckRespectsCutoff(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)
	rogueGLRow(t, db, "stock", "100", "2026-09-01")
	rogueStockRow(t, db, receipt.ItemID, "50", "2026-09-01")

	early, err := stockSvc.CheckStockToGL(context.Background(), db, "acme", "main", "2026-06-30", "controller")
	if err != nil {
		t.Fatal(err)
	}
	if !early.Matched() {
		t.Fatalf("early cutoff matched nothing: %+v", early.Accounts)
	}
	late, err := stockSvc.CheckStockToGL(context.Background(), db, "acme", "main", "2026-12-31", "controller")
	if err != nil {
		t.Fatal(err)
	}
	if late.Matched() {
		t.Fatal("late cutoff matched, want a break")
	}
	if got := late.Accounts[0]; got.Expected != "1100.000000" || got.Observed != "1050.000000" {
		t.Fatalf("expected/observed is %s/%s, want 1100.000000/1050.000000", got.Expected, got.Observed)
	}
}

func TestCheckSkipsIdenticalOpenRow(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)
	rogueGLRow(t, db, "stock", "100", "2026-06-20")

	first, err := stockSvc.CheckStockToGL(context.Background(), db, "acme", "main", "2026-12-31", "controller")
	if err != nil {
		t.Fatal(err)
	}
	second, err := stockSvc.CheckStockToGL(context.Background(), db, "acme", "main", "2026-12-31", "controller")
	if err != nil {
		t.Fatal(err)
	}
	if first.Accounts[0].LogID != second.Accounts[0].LogID {
		t.Fatalf("second run logged %s, want reuse of %s", second.Accounts[0].LogID, first.Accounts[0].LogID)
	}
	var logs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_integrity_log`).Scan(&logs); err != nil {
		t.Fatal(err)
	}
	if logs != 1 {
		t.Fatalf("log holds %d rows after identical rerun, want 1", logs)
	}
	rogueGLRow(t, db, "stock", "25", "2026-06-21")
	third, err := stockSvc.CheckStockToGL(context.Background(), db, "acme", "main", "2026-12-31", "controller")
	if err != nil {
		t.Fatal(err)
	}
	if third.Accounts[0].LogID == first.Accounts[0].LogID {
		t.Fatal("changed break reused the log row, want a new row")
	}
}

func TestCheckPerAccountAndValidation(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)
	ctx := context.Background()
	if _, err := masters.SaveAccount(ctx, ledger.Account{Name: "stock_b", Label: "Stock B", Type: "asset", Parent: "assets"}, 0); err != nil {
		t.Fatal(err)
	}

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)
	other := testMovement()
	other.ControlAccount = "stock_b"
	postMovement(t, db, stockSvc, engine, other)
	rogueGLRow(t, db, "stock_b", "10", "2026-06-20")

	report, err := stockSvc.CheckStockToGL(ctx, db, "acme", "main", "2026-12-31", "controller")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Accounts) != 2 {
		t.Fatalf("report has %d accounts, want 2", len(report.Accounts))
	}
	if !report.Accounts[0].Match || report.Accounts[1].Match {
		t.Fatalf("matches are %v/%v, want true/false in stock/stock_b order", report.Accounts[0].Match, report.Accounts[1].Match)
	}
	if _, err := stockSvc.CheckStockToGL(ctx, db, "ghost", "main", "2026-12-31", "controller"); !errors.Is(err, stock.ErrInvalid) {
		t.Fatalf("unknown company error is %v, want ErrInvalid", err)
	}
	if _, err := stockSvc.CheckStockToGL(ctx, db, "acme", "main", "31-12-2026", "controller"); !errors.Is(err, stock.ErrInvalid) {
		t.Fatalf("bad cutoff error is %v, want ErrInvalid", err)
	}
	if _, err := stockSvc.CheckStockToGL(ctx, db, "acme", "main", "2026-12-31", ""); !errors.Is(err, stock.ErrInvalid) {
		t.Fatalf("missing owner error is %v, want ErrInvalid", err)
	}
}
