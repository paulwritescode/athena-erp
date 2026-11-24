package stock_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"athena-erp/internal/posting"
	"athena-erp/internal/stock"
	"athena-erp/internal/types"

	"github.com/google/uuid"
)

func testCount(m stock.Movement, counted string) stock.Count {
	return stock.Count{
		Company: m.Company, Book: m.Book, Owner: m.Owner,
		ItemType: m.ItemType, ItemID: m.ItemID, Location: m.Location,
		Counted:        types.Quantity{Value: dec(counted), UoM: "each"},
		CountDate:      "2026-06-15",
		PostingDate:    "2026-06-16",
		ControlAccount: "stock",
		OffsetAccount:  "cogs",
		Valued:         m.Valued,
		VoucherType:    "stock_count",
		VoucherID:      uuid.NewString(),
		Revision:       1,
		SourceLineNo:   1,
	}
}

func postCount(t *testing.T, db *sql.DB, stockSvc *stock.Service, engine *posting.Engine, c stock.Count) (stock.PlannedCount, stock.AppliedMovement, bool) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	planned, err := stockSvc.PlanCount(ctx, tx, c)
	if err != nil {
		t.Fatal(err)
	}
	groupID := ""
	if planned.Movement != nil && len(planned.Movement.Legs) > 0 {
		result, err := engine.PostInTx(ctx, tx, posting.Plan{
			VoucherType: c.VoucherType, VoucherID: c.VoucherID, Revision: c.Revision,
			Event: posting.EventSubmit, Book: c.Book, PostingDate: c.PostingDate, Lines: planned.Movement.Legs,
		}, posting.PostOptions{Actor: "tester", Source: "test", OperationID: uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		groupID = result.PostingGroupID
	}
	applied, posted, err := stockSvc.ApplyCount(ctx, tx, planned, groupID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return planned, applied, posted
}

func TestCountShrinkagePostsIssue(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)

	planned, _, posted := postCount(t, db, stockSvc, engine, testCount(receipt, "95"))
	if !posted {
		t.Fatal("count posted nothing, want a shrinkage issue")
	}
	if got := planned.Delta.String(); got != "-5.000000" {
		t.Fatalf("delta is %s, want -5.000000", got)
	}
	bin, _ := binOf(t, stockSvc, db, receipt)
	if got := bin.Qty.String(); got != "95.000000" {
		t.Fatalf("bin holds %s, want 95.000000", got)
	}
	if got := bin.Value.String(); got != "950.000000" {
		t.Fatalf("bin value is %s, want 950.000000", got)
	}
	if got := glStockBalance(t, db); got != "950.000000" {
		t.Fatalf("GL stock balance is %s, want 950.000000", got)
	}
}

func TestCountGainComesInAtAverage(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)
	postMovement(t, db, stockSvc, engine, issueOf(receipt, "40"))

	planned, _, posted := postCount(t, db, stockSvc, engine, testCount(receipt, "70"))
	if !posted {
		t.Fatal("count posted nothing, want a gain receipt")
	}
	if got := planned.Delta.String(); got != "10.000000" {
		t.Fatalf("delta is %s, want 10.000000", got)
	}
	bin, _ := binOf(t, stockSvc, db, receipt)
	if got := bin.Value.String(); got != "700.000000" {
		t.Fatalf("bin value is %s, want 700.000000", got)
	}
}

func TestCountGainIntoEmptyBinCostsZero(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	receipt.Quantity = types.Quantity{Value: dec("10"), UoM: "each"}
	receipt.UnitCost = dec("5")
	postMovement(t, db, stockSvc, engine, receipt)
	postMovement(t, db, stockSvc, engine, issueOf(receipt, "10"))

	_, _, posted := postCount(t, db, stockSvc, engine, testCount(receipt, "3"))
	if !posted {
		t.Fatal("count posted nothing, want a gain receipt")
	}
	bin, _ := binOf(t, stockSvc, db, receipt)
	if got := bin.Qty.String(); got != "3.000000" {
		t.Fatalf("bin holds %s, want 3.000000", got)
	}
	if got := bin.Value.String(); got != "0.000000" {
		t.Fatalf("bin value is %s, want 0.000000 (no cost basis)", got)
	}
}

func TestCountZeroDeltaPostsNothing(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)

	count := testCount(receipt, "100")
	_, _, posted := postCount(t, db, stockSvc, engine, count)
	if posted {
		t.Fatal("count posted, want a no-op")
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM l_stock WHERE voucher_id = ?`, count.VoucherID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("count wrote %d rows, want none", rows)
	}
	var groups int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_posting_groups WHERE voucher_id = ?`, count.VoucherID).Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if groups != 0 {
		t.Fatalf("count claimed %d groups, want none", groups)
	}
}

func TestCountAdjustsMovementsSinceCutoff(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	receipt.MovementDate = "2026-06-15"
	postMovement(t, db, stockSvc, engine, receipt)
	issue := issueOf(receipt, "20")
	issue.MovementDate = "2026-06-16"
	issue.PostingDate = "2026-06-16"
	postMovement(t, db, stockSvc, engine, issue)

	// The count saw 100 on the 15th; the issue of the 16th happened after.
	count := testCount(receipt, "100")
	count.CountDate = "2026-06-15"
	planned, _, posted := postCount(t, db, stockSvc, engine, count)
	if posted {
		t.Fatal("count posted, want a no-op after the cutoff adjustment")
	}
	if got := planned.Expected.String(); got != "100.000000" {
		t.Fatalf("expected is %s, want 100.000000", got)
	}
}

func TestCountValidation(t *testing.T) {
	stockSvc, _, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	valid := testCount(receipt, "100")
	cases := map[string]func(*stock.Count){
		"unknown company": func(c *stock.Count) { c.Company = "ghost" },
		"same accounts":   func(c *stock.Count) { c.OffsetAccount = c.ControlAccount },
		"bad count date":  func(c *stock.Count) { c.CountDate = "15-06-2026" },
		"bad unit":        func(c *stock.Count) { c.Counted.UoM = "Each" },
		"zero revision":   func(c *stock.Count) { c.Revision = 0 },
	}
	for name, mutate := range cases {
		count := valid
		count.VoucherID = uuid.NewString()
		mutate(&count)
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = stockSvc.PlanCount(context.Background(), tx, count)
		tx.Rollback()
		if !errors.Is(err, stock.ErrInvalid) {
			t.Errorf("%s: error is %v, want ErrInvalid", name, err)
		}
	}
}
