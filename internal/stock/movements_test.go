// Package stock_test drives the stock seam the way production will: plan
// with the stock service, post the value legs with the G6 engine, apply
// under the resulting group. It is an external test package so it can
// migrate with server.SystemMigrations without an import cycle.
package stock_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"athena-erp/internal/ledger"
	"athena-erp/internal/posting"
	"athena-erp/internal/server"
	"athena-erp/internal/stock"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
	"athena-erp/internal/types"

	"github.com/google/uuid"
)

func setup(t *testing.T) (*stock.Service, *posting.Engine, *ledger.Service, *sql.DB) {
	t.Helper()
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, store.MariaDB{}, server.SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &stock.Service{DB: db, Dialect: store.MariaDB{}},
		&posting.Engine{DB: db, Dialect: store.MariaDB{}},
		&ledger.Service{DB: db, Dialect: store.MariaDB{}}, db
}

// seedStockBasics creates company acme on USD with an open 2026 period,
// asset, income and expense types, leaf accounts stock, clearing and cogs,
// and a units family (each base, box of 12).
func seedStockBasics(t *testing.T, masters *ledger.Service) {
	t.Helper()
	ctx := context.Background()
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
	for _, accountType := range []ledger.AccountType{
		{Name: "asset", Label: "Asset", NormalBalance: "debit"},
		{Name: "income", Label: "Income", NormalBalance: "credit"},
		{Name: "expense", Label: "Expense", NormalBalance: "debit"},
	} {
		if _, err := masters.SaveAccountType(ctx, accountType); err != nil {
			t.Fatal(err)
		}
	}
	for _, account := range []ledger.Account{
		{Name: "assets", Label: "Assets", Type: "asset", IsGroup: true},
		{Name: "stock", Label: "Stock", Type: "asset", Parent: "assets"},
		{Name: "clearing", Label: "Clearing", Type: "asset", Parent: "assets"},
		{Name: "cogs", Label: "Cost of sales", Type: "expense"},
	} {
		if _, err := masters.SaveAccount(ctx, account, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := masters.SaveUoMFamily(ctx, ledger.UoMFamily{Name: "units", Label: "Units", BaseUnit: "each"}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := masters.SaveUoMConversion(ctx, ledger.UoMConversion{Family: "units", Unit: "box", Factor: types.MustDecimal("12"), Scale: 6}, 0); err != nil {
		t.Fatal(err)
	}
}

func dec(value string) types.Decimal { return types.MustDecimal(value) }

// testMovement is a valued receipt template with fresh item and voucher
// ids; tests mutate a copy per case.
func testMovement() stock.Movement {
	return stock.Movement{
		Company: "acme", Book: "main", Owner: "company",
		ItemType: "test_widget", ItemID: uuid.NewString(),
		ControlAccount: "stock", OffsetAccount: "clearing",
		Location:     "wh_main",
		Quantity:     types.Quantity{Value: dec("100"), UoM: "each"},
		UnitCost:     dec("10"),
		Valued:       true,
		MovementDate: "2026-06-15", PostingDate: "2026-06-15",
		VoucherType: "stock_receipt", VoucherID: uuid.NewString(), Revision: 1, SourceLineNo: 1,
	}
}

func issueOf(receipt stock.Movement, qty string) stock.Movement {
	out := receipt
	out.Quantity = types.Quantity{Value: dec(qty).Neg(), UoM: receipt.Quantity.UoM}
	out.UnitCost = types.Decimal{}
	out.OffsetAccount = "cogs"
	out.VoucherType = "stock_issue"
	out.VoucherID = uuid.NewString()
	out.ReversesRowID = ""
	return out
}

// postMovement runs the production seam in one transaction: plan, post the
// value legs, apply under the group.
func postMovement(t *testing.T, db *sql.DB, stockSvc *stock.Service, engine *posting.Engine, m stock.Movement) (stock.PlannedMovement, stock.AppliedMovement) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	planned, err := stockSvc.PlanMovement(ctx, tx, m)
	if err != nil {
		t.Fatal(err)
	}
	groupID := ""
	if len(planned.Legs) > 0 {
		result, err := engine.PostInTx(ctx, tx, posting.Plan{
			VoucherType: m.VoucherType, VoucherID: m.VoucherID, Revision: m.Revision,
			Event: posting.EventSubmit, Book: m.Book, PostingDate: m.PostingDate, Lines: planned.Legs,
		}, posting.PostOptions{Actor: "tester", Source: "test", OperationID: uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		groupID = result.PostingGroupID
	}
	applied, err := stockSvc.ApplyMovement(ctx, tx, planned, groupID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return planned, applied
}

func cancelVoucher(t *testing.T, db *sql.DB, stockSvc *stock.Service, engine *posting.Engine, voucherType, voucherID string, withGL bool) stock.AppliedCancel {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	planned, err := stockSvc.PlanCancel(ctx, tx, voucherType, voucherID, 1, "main", "2026-06-16", "2026-06-16")
	if err != nil {
		t.Fatal(err)
	}
	groupID := ""
	if withGL {
		result, err := engine.ReverseInTx(ctx, tx, voucherType, voucherID, 1, "main", posting.ReverseOptions{Actor: "tester", Source: "test", OperationID: uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		groupID = result.PostingGroupID
	}
	applied, err := stockSvc.ApplyCancel(ctx, tx, planned, groupID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return applied
}

func binOf(t *testing.T, stockSvc *stock.Service, db *sql.DB, m stock.Movement) (stock.BinState, bool) {
	t.Helper()
	bin, found, err := stockSvc.BinBalance(context.Background(), db, stock.BinKey{
		Company: m.Company, Book: m.Book, Owner: m.Owner, ItemID: m.ItemID,
		Location: m.Location, LotNo: m.LotNo, SerialNo: m.SerialNo,
	})
	if err != nil {
		t.Fatal(err)
	}
	return bin, found
}

func glStockBalance(t *testing.T, db *sql.DB) string {
	t.Helper()
	var balance string
	if err := db.QueryRow(`SELECT COALESCE(SUM(debit_base - credit_base), 0) FROM l_gl WHERE company_name = 'acme' AND book = 'main' AND account_name = 'stock'`).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	return balance
}

func layerResidual(t *testing.T, db *sql.DB, receiptRowID string) string {
	t.Helper()
	var residual string
	if err := db.QueryRow(`SELECT residual_qty FROM athena_stock_layers WHERE receipt_row_id = ?`, receiptRowID).Scan(&residual); err != nil {
		t.Fatal(err)
	}
	return residual
}

func edgeCount(t *testing.T, db *sql.DB, rowID string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_item_applications WHERE issue_row_id = ? AND is_reversal = 0`, rowID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestReceiptsBlendAndIssueTakesAverage(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	first := testMovement()
	planned, applied := postMovement(t, db, stockSvc, engine, first)
	if got := planned.Value.String(); got != "1000.000000" {
		t.Fatalf("first receipt value is %s, want 1000.000000", got)
	}
	second := first
	second.Quantity = types.Quantity{Value: dec("50"), UoM: "each"}
	second.UnitCost = dec("16")
	second.VoucherID = uuid.NewString()
	planned, _ = postMovement(t, db, stockSvc, engine, second)
	if got := planned.Average.String(); got != "12.000000" {
		t.Fatalf("blended average is %s, want 12.000000", got)
	}
	issue := issueOf(first, "60")
	planned, issueApplied := postMovement(t, db, stockSvc, engine, issue)
	if got := planned.Value.String(); got != "-720.000000" {
		t.Fatalf("issue value is %s, want -720.000000", got)
	}
	bin, found := binOf(t, stockSvc, db, first)
	if !found {
		t.Fatal("bin missing after issue")
	}
	if got := bin.Qty.String(); got != "90.000000" {
		t.Fatalf("bin holds %s, want 90.000000", got)
	}
	if got := bin.Value.String(); got != "1080.000000" {
		t.Fatalf("bin value is %s, want 1080.000000", got)
	}
	if got := layerResidual(t, db, applied.RowID); got != "40.000000" {
		t.Fatalf("first layer residual is %s, want 40.000000", got)
	}
	if got := edgeCount(t, db, issueApplied.RowID); got != 1 {
		t.Fatalf("issue has %d application edges, want 1", got)
	}
	if got := glStockBalance(t, db); got != "1080.000000" {
		t.Fatalf("GL stock balance is %s, want 1080.000000", got)
	}
	// A second issue spans both layers oldest first.
	older := issue
	older.VoucherID = uuid.NewString()
	older.Quantity = types.Quantity{Value: dec("45").Neg(), UoM: "each"}
	_, spanned := postMovement(t, db, stockSvc, engine, older)
	if got := edgeCount(t, db, spanned.RowID); got != 2 {
		t.Fatalf("spanning issue has %d application edges, want 2", got)
	}
	if got := layerResidual(t, db, applied.RowID); got != "0.000000" {
		t.Fatalf("first layer residual is %s, want 0.000000", got)
	}
}

func TestNegativeStockRefusedForValuedOnly(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	receipt.Quantity = types.Quantity{Value: dec("10"), UoM: "each"}
	postMovement(t, db, stockSvc, engine, receipt)

	over := issueOf(receipt, "11")
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = stockSvc.PlanMovement(context.Background(), tx, over)
	tx.Rollback()
	if !errors.Is(err, stock.ErrNegativeStock) {
		t.Fatalf("over-issue error is %v, want ErrNegativeStock", err)
	}

	plain := testMovement()
	plain.Valued = false
	plain.UnitCost = types.Decimal{}
	plain.Quantity = types.Quantity{Value: dec("10"), UoM: "each"}
	postMovement(t, db, stockSvc, engine, plain)
	plainIssue := issueOf(plain, "11")
	plainIssue.Valued = false
	postMovement(t, db, stockSvc, engine, plainIssue)
	bin, found := binOf(t, stockSvc, db, plain)
	if !found || bin.Qty.String() != "-1.000000" {
		t.Fatalf("unvalued bin is %v, want -1.000000", bin.Qty)
	}
}

func TestReturnTakesIssueCost(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)
	issue := issueOf(receipt, "40")
	_, issueApplied := postMovement(t, db, stockSvc, engine, issue)

	back := receipt
	back.VoucherID = uuid.NewString()
	back.VoucherType = "stock_return"
	back.Quantity = types.Quantity{Value: dec("10"), UoM: "each"}
	back.UnitCost = types.Decimal{}
	back.ReversesRowID = issueApplied.RowID
	planned, _ := postMovement(t, db, stockSvc, engine, back)
	if got := planned.UnitCost.String(); got != "10.000000" {
		t.Fatalf("return unit cost is %s, want 10.000000", got)
	}
	if got := planned.Value.String(); got != "100.000000" {
		t.Fatalf("return value is %s, want 100.000000", got)
	}
	bin, _ := binOf(t, stockSvc, db, receipt)
	if got := bin.Qty.String(); got != "70.000000" {
		t.Fatalf("bin holds %s, want 70.000000", got)
	}
	if got := layerResidual(t, db, receiptApplied.RowID); got != "70.000000" {
		t.Fatalf("layer residual is %s, want 70.000000", got)
	}

	tooMuch := back
	tooMuch.VoucherID = uuid.NewString()
	tooMuch.Quantity = types.Quantity{Value: dec("40"), UoM: "each"}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := stockSvc.PlanMovement(context.Background(), tx, tooMuch); !errors.Is(err, stock.ErrOverReturn) {
		t.Fatalf("over-return error is %v, want ErrOverReturn", err)
	}
	ofReceipt := back
	ofReceipt.VoucherID = uuid.NewString()
	ofReceipt.ReversesRowID = receiptApplied.RowID
	if _, err := stockSvc.PlanMovement(context.Background(), tx, ofReceipt); !errors.Is(err, stock.ErrInvalid) {
		t.Fatalf("return of receipt error is %v, want ErrInvalid", err)
	}
}

func TestCancelMirrorsSubmit(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)
	issue := issueOf(receipt, "30")
	postMovement(t, db, stockSvc, engine, issue)

	applied := cancelVoucher(t, db, stockSvc, engine, issue.VoucherType, issue.VoucherID, true)
	if len(applied.RowIDs) != 1 {
		t.Fatalf("cancel wrote %d rows, want 1", len(applied.RowIDs))
	}
	bin, _ := binOf(t, stockSvc, db, receipt)
	if got := bin.Qty.String(); got != "100.000000" {
		t.Fatalf("bin holds %s after cancel, want 100.000000", got)
	}
	if got := bin.Value.String(); got != "1000.000000" {
		t.Fatalf("bin value is %s after cancel, want 1000.000000", got)
	}
	if got := layerResidual(t, db, receiptApplied.RowID); got != "100.000000" {
		t.Fatalf("layer residual is %s after cancel, want 100.000000", got)
	}
	var mirrors int
	if err := db.QueryRow(`SELECT COUNT(*) FROM l_stock WHERE voucher_id = ? AND event = 'cancel' AND is_reversal = 1`, issue.VoucherID).Scan(&mirrors); err != nil {
		t.Fatal(err)
	}
	if mirrors != 1 {
		t.Fatalf("cancel wrote %d flagged mirrors, want 1", mirrors)
	}
	if got := glStockBalance(t, db); got != "1000.000000" {
		t.Fatalf("GL stock balance is %s after cancel, want 1000.000000", got)
	}
	// Cancelling again replays the first cancel.
	again := cancelVoucher(t, db, stockSvc, engine, issue.VoucherType, issue.VoucherID, true)
	if !again.Replayed || len(again.RowIDs) != 1 || again.RowIDs[0] != applied.RowIDs[0] {
		t.Fatalf("second cancel is %+v, want a replay of %s", again, applied.RowIDs[0])
	}
}

func TestCancelBlockedByDownstreamIssue(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	receipt.Quantity = types.Quantity{Value: dec("50"), UoM: "each"}
	postMovement(t, db, stockSvc, engine, receipt)
	postMovement(t, db, stockSvc, engine, issueOf(receipt, "50"))

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := stockSvc.PlanCancel(context.Background(), tx, receipt.VoucherType, receipt.VoucherID, 1, "main", "2026-06-16", "2026-06-16"); !errors.Is(err, stock.ErrCancelBlocked) {
		t.Fatalf("consumed receipt cancel error is %v, want ErrCancelBlocked", err)
	}
	if _, err := stockSvc.PlanCancel(context.Background(), tx, "stock_receipt", uuid.NewString(), 1, "main", "2026-06-16", "2026-06-16"); !errors.Is(err, stock.ErrNoSubmit) {
		t.Fatalf("unknown voucher cancel error is %v, want ErrNoSubmit", err)
	}
}

func TestPostTwiceReplays(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	movement := testMovement()
	_, first := postMovement(t, db, stockSvc, engine, movement)
	if first.Replayed {
		t.Fatal("first post replays, want a fresh write")
	}
	_, second := postMovement(t, db, stockSvc, engine, movement)
	if !second.Replayed || second.RowID != first.RowID {
		t.Fatalf("second post is %+v, want a replay of %s", second, first.RowID)
	}
	bin, _ := binOf(t, stockSvc, db, movement)
	if got := bin.Qty.String(); got != "100.000000" {
		t.Fatalf("bin holds %s after double post, want 100.000000", got)
	}
}

func TestQuantityOnlyMoveClaimsGroup(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	movement := testMovement()
	movement.Valued = false
	movement.UnitCost = types.Decimal{}
	planned, applied := postMovement(t, db, stockSvc, engine, movement)
	if len(planned.Legs) != 0 {
		t.Fatalf("quantity-only move has %d legs, want none", len(planned.Legs))
	}
	var groups int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_posting_groups WHERE voucher_id = ? AND event = 'submit'`, movement.VoucherID).Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if groups != 1 {
		t.Fatalf("quantity-only move claimed %d groups, want 1", groups)
	}
	var gl int
	if err := db.QueryRow(`SELECT COUNT(*) FROM l_gl WHERE posting_group_id = ?`, applied.PostingGroupID).Scan(&gl); err != nil {
		t.Fatal(err)
	}
	if gl != 0 {
		t.Fatalf("quantity-only move posted %d GL rows, want none", gl)
	}
}

func TestRebuildBinsRestoresCorruptedBin(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)
	postMovement(t, db, stockSvc, engine, issueOf(receipt, "40"))
	if _, err := db.Exec(`UPDATE athena_stock_bins SET on_hand = 999, value_on_hand = 1 WHERE item_id = ?`, receipt.ItemID); err != nil {
		t.Fatal(err)
	}
	count, err := stockSvc.RebuildBins(context.Background(), db, "acme", "main")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rebuild wrote %d bins, want 1", count)
	}
	bin, found := binOf(t, stockSvc, db, receipt)
	if !found {
		t.Fatal("bin missing after rebuild")
	}
	if got := bin.Qty.String(); got != "60.000000" {
		t.Fatalf("rebuilt bin holds %s, want 60.000000", got)
	}
	if got := bin.Value.String(); got != "600.000000" {
		t.Fatalf("rebuilt bin value is %s, want 600.000000", got)
	}
	if count, err := stockSvc.RebuildBins(context.Background(), db, "acme", "other"); err != nil || count != 0 {
		t.Fatalf("empty rebuild is %d, %v; want 0, nil", count, err)
	}
}

func TestBoxQuantityConvertsToBase(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	movement := testMovement()
	movement.Quantity = types.Quantity{Value: dec("2"), UoM: "box"}
	movement.UnitCost = dec("1")
	planned, _ := postMovement(t, db, stockSvc, engine, movement)
	if got := planned.BaseQty.String(); got != "24.000000" {
		t.Fatalf("base quantity is %s, want 24.000000", got)
	}
	if got := planned.Value.String(); got != "24.000000" {
		t.Fatalf("value is %s, want 24.000000", got)
	}
	bin, _ := binOf(t, stockSvc, db, movement)
	if bin.UoM != "each" || bin.Qty.String() != "24.000000" {
		t.Fatalf("bin is %s %s, want 24.000000 each", bin.Qty, bin.UoM)
	}
}

func TestMovementValidation(t *testing.T) {
	stockSvc, _, masters, db := setup(t)
	seedStockBasics(t, masters)

	valid := testMovement()
	cases := map[string]func(*stock.Movement){
		"unknown company":   func(m *stock.Movement) { m.Company = "ghost" },
		"group control":     func(m *stock.Movement) { m.ControlAccount = "assets" },
		"same accounts":     func(m *stock.Movement) { m.OffsetAccount = m.ControlAccount },
		"zero quantity":     func(m *stock.Movement) { m.Quantity.Value = dec("0") },
		"bad unit":          func(m *stock.Movement) { m.Quantity.UoM = "Each" },
		"unknown unit":      func(m *stock.Movement) { m.Quantity.UoM = "smoot" },
		"issue with cost":   func(m *stock.Movement) { m.Quantity.Value = dec("-5") },
		"negative receipt":  func(m *stock.Movement) { m.UnitCost = dec("-2") },
		"return of nothing": func(m *stock.Movement) { m.ReversesRowID = uuid.NewString(); m.UnitCost = types.Decimal{} },
		"negative return":   func(m *stock.Movement) { m.ReversesRowID = uuid.NewString(); m.Quantity.Value = dec("-5") },
		"zero revision":     func(m *stock.Movement) { m.Revision = 0 },
		"zero source line":  func(m *stock.Movement) { m.SourceLineNo = 0 },
		"bad movement date": func(m *stock.Movement) { m.MovementDate = "15-06-2026" },
		"missing location":  func(m *stock.Movement) { m.Location = "" },
	}
	for name, mutate := range cases {
		movement := valid
		movement.VoucherID = uuid.NewString()
		movement.ItemID = uuid.NewString()
		mutate(&movement)
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = stockSvc.PlanMovement(context.Background(), tx, movement)
		tx.Rollback()
		if !errors.Is(err, stock.ErrInvalid) {
			t.Errorf("%s: error is %v, want ErrInvalid", name, err)
		}
	}
}
