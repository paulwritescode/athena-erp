package stock_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"athena-erp/internal/posting"
	"athena-erp/internal/stock"
	"athena-erp/internal/types"

	"github.com/google/uuid"
)

func testDelta(receiptRowID string) stock.CostDelta {
	return stock.CostDelta{
		ReceiptRowID:  receiptRowID,
		Total:         dec("300"),
		OffsetAccount: "clearing",
		COGSAccount:   "cogs",
		VoucherType:   "freight",
		VoucherID:     uuid.NewString(),
		Revision:      1,
		SourceLineNo:  1,
		PostingDate:   "2026-07-20",
	}
}

// postDelta runs the delta seam in one transaction: plan, post the value
// legs, apply under the group.
func postDelta(t *testing.T, db *sql.DB, stockSvc *stock.Service, engine *posting.Engine, d stock.CostDelta) (stock.PlannedDelta, stock.AppliedDelta) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	planned, err := stockSvc.PlanCostDelta(ctx, tx, d)
	if err != nil {
		t.Fatal(err)
	}
	groupID := ""
	if len(planned.Legs) > 0 {
		result, err := engine.PostInTx(ctx, tx, posting.Plan{
			VoucherType: d.VoucherType, VoucherID: d.VoucherID, Revision: d.Revision,
			Event: posting.EventSubmit, Book: planned.Receipt.Bin.Book, PostingDate: d.PostingDate, Lines: planned.Legs,
		}, posting.PostOptions{Actor: "tester", Source: "test", OperationID: uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		groupID = result.PostingGroupID
	}
	applied, err := stockSvc.ApplyCostDelta(ctx, tx, planned, groupID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return planned, applied
}

func glBalance(t *testing.T, db *sql.DB, account string) string {
	t.Helper()
	var balance string
	if err := db.QueryRow(`SELECT COALESCE(SUM(debit_base - credit_base), 0) FROM l_gl WHERE company_name = 'acme' AND book = 'main' AND account_name = ?`, account).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	return balance
}

func layerCost(t *testing.T, db *sql.DB, receiptRowID string) string {
	t.Helper()
	var cost string
	if err := db.QueryRow(`SELECT unit_cost FROM athena_stock_layers WHERE receipt_row_id = ?`, receiptRowID).Scan(&cost); err != nil {
		t.Fatal(err)
	}
	return cost
}

func TestFreightSplitsOnHandAndSold(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)
	postMovement(t, db, stockSvc, engine, issueOf(receipt, "40"))

	planned, applied := postDelta(t, db, stockSvc, engine, testDelta(receiptApplied.RowID))
	if got := planned.OnHand.String(); got != "180.000000" {
		t.Fatalf("on-hand split is %s, want 180.000000", got)
	}
	if got := planned.Sold.String(); got != "120.000000" {
		t.Fatalf("sold split is %s, want 120.000000", got)
	}
	if len(planned.Legs) != 3 {
		t.Fatalf("delta has %d legs, want 3", len(planned.Legs))
	}
	bin, _ := binOf(t, stockSvc, db, receipt)
	if got := bin.Value.String(); got != "780.000000" {
		t.Fatalf("bin value is %s, want 780.000000", got)
	}
	if got := bin.Qty.String(); got != "60.000000" {
		t.Fatalf("bin holds %s, want 60.000000", got)
	}
	if got := layerCost(t, db, receiptApplied.RowID); got != "10.000000" {
		t.Fatalf("layer unit cost is %s, want 10.000000 (deltas never rewrite it)", got)
	}
	var economic, postingDate time.Time
	var onHand, sold string
	if err := db.QueryRow(`SELECT economic_date, posting_date, on_hand_delta, sold_delta FROM l_stock_value WHERE id = ?`, applied.RowID).Scan(&economic, &postingDate, &onHand, &sold); err != nil {
		t.Fatal(err)
	}
	if got := economic.Format("2006-01-02") + "/" + postingDate.Format("2006-01-02"); got != "2026-06-15/2026-07-20" {
		t.Fatalf("delta dates are %s, want 2026-06-15/2026-07-20", got)
	}
	if onHand != "180.000000" || sold != "120.000000" {
		t.Fatalf("stored split is %s/%s, want 180.000000/120.000000", onHand, sold)
	}
	if got := glBalance(t, db, "stock"); got != "780.000000" {
		t.Fatalf("GL stock balance is %s, want 780.000000", got)
	}
	if got := glBalance(t, db, "cogs"); got != "520.000000" {
		t.Fatalf("GL cogs balance is %s, want 520.000000", got)
	}
	if got := glBalance(t, db, "clearing"); got != "-1300.000000" {
		t.Fatalf("GL clearing balance is %s, want -1300.000000", got)
	}
}

func TestDeltaFullySoldSkipsStockLeg(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	receipt.Quantity = types.Quantity{Value: dec("10"), UoM: "each"}
	receipt.UnitCost = dec("5")
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)
	postMovement(t, db, stockSvc, engine, issueOf(receipt, "10"))

	delta := testDelta(receiptApplied.RowID)
	delta.Total = dec("50")
	planned, _ := postDelta(t, db, stockSvc, engine, delta)
	if got := planned.OnHand.String(); got != "0.000000" {
		t.Fatalf("on-hand split is %s, want 0.000000", got)
	}
	if got := planned.Sold.String(); got != "50.000000" {
		t.Fatalf("sold split is %s, want 50.000000", got)
	}
	if len(planned.Legs) != 2 {
		t.Fatalf("delta has %d legs, want 2", len(planned.Legs))
	}
	bin, _ := binOf(t, stockSvc, db, receipt)
	if got := bin.Value.String(); got != "0.000000" {
		t.Fatalf("bin value is %s, want 0.000000", got)
	}
	if got := glBalance(t, db, "stock"); got != "0.000000" {
		t.Fatalf("GL stock balance is %s, want 0.000000", got)
	}
}

func TestDeltaNegativeTotalFlipsSides(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)

	delta := testDelta(receiptApplied.RowID)
	delta.Total = dec("-200")
	planned, _ := postDelta(t, db, stockSvc, engine, delta)
	if got := planned.OnHand.String(); got != "-200.000000" {
		t.Fatalf("on-hand split is %s, want -200.000000", got)
	}
	bin, _ := binOf(t, stockSvc, db, receipt)
	if got := bin.Value.String(); got != "800.000000" {
		t.Fatalf("bin value is %s, want 800.000000", got)
	}
	if got := glBalance(t, db, "stock"); got != "800.000000" {
		t.Fatalf("GL stock balance is %s, want 800.000000", got)
	}
}

func TestDeltaCancelMirrors(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)
	delta := testDelta(receiptApplied.RowID)
	postDelta(t, db, stockSvc, engine, delta)

	applied := cancelVoucher(t, db, stockSvc, engine, delta.VoucherType, delta.VoucherID, true)
	if len(applied.RowIDs) != 1 {
		t.Fatalf("delta cancel wrote %d rows, want 1", len(applied.RowIDs))
	}
	bin, _ := binOf(t, stockSvc, db, receipt)
	if got := bin.Value.String(); got != "1000.000000" {
		t.Fatalf("bin value is %s after cancel, want 1000.000000", got)
	}
	var mirrors int
	if err := db.QueryRow(`SELECT COUNT(*) FROM l_stock_value WHERE voucher_id = ? AND event = 'cancel' AND is_reversal = 1`, delta.VoucherID).Scan(&mirrors); err != nil {
		t.Fatal(err)
	}
	if mirrors != 1 {
		t.Fatalf("delta cancel wrote %d flagged mirrors, want 1", mirrors)
	}
	if got := layerCost(t, db, receiptApplied.RowID); got != "10.000000" {
		t.Fatalf("layer unit cost is %s after cancel, want 10.000000", got)
	}
	if got := glBalance(t, db, "stock"); got != "1000.000000" {
		t.Fatalf("GL stock balance is %s after cancel, want 1000.000000", got)
	}
}

func TestRebuildResumesFromWatermark(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	first := testMovement()
	_, firstApplied := postMovement(t, db, stockSvc, engine, first)
	second := testMovement()
	postMovement(t, db, stockSvc, engine, second)
	if _, err := db.Exec(`UPDATE athena_stock_bins SET on_hand = 999, value_on_hand = 1`); err != nil {
		t.Fatal(err)
	}
	// A previous run died after the first receipt's row: resume must fix
	// only what lies beyond the watermark.
	if _, err := db.Exec(`INSERT INTO athena_stock_rebuilds (id, company_name, book, state, watermark_row_id) VALUES (?, 'acme', 'main', 'running', ?)`, uuid.NewString(), firstApplied.RowID); err != nil {
		t.Fatal(err)
	}
	pending, err := stockSvc.PendingRebuild(context.Background(), db, "acme", "main")
	if err != nil || !pending {
		t.Fatalf("pending rebuild is %v, %v; want true, nil", pending, err)
	}
	count, err := stockSvc.RebuildBins(context.Background(), db, "acme", "main")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("rebuild wrote %d bins, want 2", count)
	}
	stale, _ := binOf(t, stockSvc, db, first)
	if got := stale.Qty.String(); got != "999.000000" {
		t.Fatalf("pre-watermark bin holds %s, want 999.000000 (untouched)", got)
	}
	fixed, _ := binOf(t, stockSvc, db, second)
	if got := fixed.Qty.String(); got != "100.000000" {
		t.Fatalf("resumed bin holds %s, want 100.000000", got)
	}
	if got := fixed.Value.String(); got != "1000.000000" {
		t.Fatalf("resumed bin value is %s, want 1000.000000", got)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM athena_stock_rebuilds WHERE company_name = 'acme' AND book = 'main' ORDER BY started_at DESC LIMIT 1`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "completed" {
		t.Fatalf("rebuild state is %s, want completed", state)
	}
	if pending, err := stockSvc.PendingRebuild(context.Background(), db, "acme", "main"); err != nil || pending {
		t.Fatalf("pending rebuild is %v, %v; want false, nil", pending, err)
	}
}

func TestPendingRebuildEmpty(t *testing.T) {
	stockSvc, _, masters, db := setup(t)
	seedStockBasics(t, masters)

	if pending, err := stockSvc.PendingRebuild(context.Background(), db, "acme", "main"); err != nil || pending {
		t.Fatalf("pending rebuild is %v, %v; want false, nil", pending, err)
	}
	if count, err := stockSvc.RebuildBins(context.Background(), db, "acme", "main"); err != nil || count != 0 {
		t.Fatalf("empty rebuild is %d, %v; want 0, nil", count, err)
	}
}

func TestCostDeltaValidation(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)
	issue := issueOf(receipt, "10")
	_, issueApplied := postMovement(t, db, stockSvc, engine, issue)
	plain := testMovement()
	plain.Valued = false
	plain.UnitCost = types.Decimal{}
	_, plainApplied := postMovement(t, db, stockSvc, engine, plain)

	valid := testDelta(receiptApplied.RowID)
	cases := map[string]func(*stock.CostDelta){
		"unknown receipt":  func(d *stock.CostDelta) { d.ReceiptRowID = uuid.NewString() },
		"issue as receipt": func(d *stock.CostDelta) { d.ReceiptRowID = issueApplied.RowID },
		"unvalued receipt": func(d *stock.CostDelta) { d.ReceiptRowID = plainApplied.RowID },
		"group offset":     func(d *stock.CostDelta) { d.OffsetAccount = "assets" },
		"same accounts":    func(d *stock.CostDelta) { d.COGSAccount = d.OffsetAccount },
		"missing cogs":     func(d *stock.CostDelta) { d.COGSAccount = "ghost" },
		"bad posting date": func(d *stock.CostDelta) { d.PostingDate = "20-07-2026" },
		"zero revision":    func(d *stock.CostDelta) { d.Revision = 0 },
	}
	for name, mutate := range cases {
		delta := valid
		delta.VoucherID = uuid.NewString()
		mutate(&delta)
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = stockSvc.PlanCostDelta(context.Background(), tx, delta)
		tx.Rollback()
		if !errors.Is(err, stock.ErrInvalid) {
			t.Errorf("%s: error is %v, want ErrInvalid", name, err)
		}
	}
}
