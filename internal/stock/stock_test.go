package stock_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"athena-erp/internal/posting"
	"athena-erp/internal/stock"
	"athena-erp/internal/types"

	"github.com/google/uuid"
	"pgregory.net/rapid"
)

// TestAppendOnlyCostCorrectionProperty checks Property 21 over drawn stock
// histories: for any sequence of backdated movements, the hash of every
// historical stock ledger row is unchanged and the new cost appears only in
// appended delta rows.
//
// Feature: athena-batch-2, Property 21: Append-only cost correction.
//
// One shared database serves all iterations; fresh item and voucher ids
// keep iterations isolated. Each iteration posts a drawn movement history,
// snapshots every l_stock row it wrote, posts two batches of drawn cost
// deltas, and proves the snapshot byte-identical while the bin value moved
// only by the deltas' on-hand portions.
func TestAppendOnlyCostCorrectionProperty(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	rapid.Check(t, func(t *rapid.T) {
		itemID := uuid.NewString()
		base := testMovement()
		base.ItemID = itemID

		var onHand int64
		var receipts []string
		type openIssue struct {
			rowID      string
			unreturned int64
		}
		var issues []openIssue
		var vouchers []string

		movements := rapid.IntRange(1, 8).Draw(t, "movements")
		for i := 0; i < movements; i++ {
			kind := rapid.IntRange(0, 99).Draw(t, "kind")
			movementDate := drawDate(t, "movement")
			postingDate := drawDate(t, "posting")
			movement := base
			movement.MovementDate = movementDate
			movement.PostingDate = postingDate
			movement.VoucherID = uuid.NewString()
			switch {
			case kind < 45 || onHand == 0:
				qty := int64(rapid.IntRange(1, 50).Draw(t, "receiptQty"))
				cents := int64(rapid.IntRange(100, 5000).Draw(t, "receiptCost"))
				cost, err := types.NewDecimal(cents, 2)
				if err != nil {
					t.Fatal(err)
				}
				movement.Quantity = types.Quantity{Value: types.MustDecimal(fmt.Sprint(qty)), UoM: "each"}
				movement.UnitCost = cost
				_, applied := postOp(t, db, stockSvc, engine, movement)
				onHand += qty
				receipts = append(receipts, applied.RowID)
			case kind < 75:
				qty := int64(rapid.IntRange(1, int(onHand)).Draw(t, "issueQty"))
				issue := issueOf(movement, fmt.Sprint(qty))
				issue.MovementDate = movementDate
				issue.PostingDate = postingDate
				_, applied := postOp(t, db, stockSvc, engine, issue)
				onHand -= qty
				issues = append(issues, openIssue{rowID: applied.RowID, unreturned: qty})
				movement.VoucherID = issue.VoucherID
			default:
				var withReturns []openIssue
				for _, issue := range issues {
					if issue.unreturned > 0 {
						withReturns = append(withReturns, issue)
					}
				}
				if len(withReturns) == 0 {
					qty := int64(rapid.IntRange(1, 50).Draw(t, "receiptQty"))
					cost, err := types.NewDecimal(int64(rapid.IntRange(100, 5000).Draw(t, "receiptCost")), 2)
					if err != nil {
						t.Fatal(err)
					}
					movement.Quantity = types.Quantity{Value: types.MustDecimal(fmt.Sprint(qty)), UoM: "each"}
					movement.UnitCost = cost
					_, applied := postOp(t, db, stockSvc, engine, movement)
					onHand += qty
					receipts = append(receipts, applied.RowID)
					break
				}
				pick := withReturns[rapid.IntRange(0, len(withReturns)-1).Draw(t, "returnOf")]
				qty := int64(rapid.IntRange(1, int(pick.unreturned)).Draw(t, "returnQty"))
				movement.Quantity = types.Quantity{Value: types.MustDecimal(fmt.Sprint(qty)), UoM: "each"}
				movement.UnitCost = types.Decimal{}
				movement.VoucherType = "stock_return"
				movement.ReversesRowID = pick.rowID
				postOp(t, db, stockSvc, engine, movement)
				onHand += qty
				for j := range issues {
					if issues[j].rowID == pick.rowID {
						issues[j].unreturned -= qty
					}
				}
			}
			vouchers = append(vouchers, movement.VoucherID)
		}

		stocks := snapshotStockRows(t, db, itemID)
		binBefore := binValue(t, stockSvc, db, base)
		glBefore := scopedGLStock(t, db, vouchers)

		var onHandTotal types.Decimal
		for batch := 0; batch < 2; batch++ {
			deltas := rapid.IntRange(0, 3).Draw(t, "deltas")
			for i := 0; i < deltas; i++ {
				receipt := receipts[rapid.IntRange(0, len(receipts)-1).Draw(t, "deltaOf")]
				cents := int64(rapid.IntRange(-2000, 5000).Draw(t, "deltaTotal"))
				total, err := types.NewDecimal(cents, 2)
				if err != nil {
					t.Fatal(err)
				}
				delta := testDelta(receipt)
				delta.Total = total
				delta.PostingDate = drawDate(t, "deltaPosting")
				planned, _ := postDeltaOp(t, db, stockSvc, engine, delta)
				onHandTotal = onHandTotal.Add(planned.OnHand)
				vouchers = append(vouchers, delta.VoucherID)
			}
			if got := snapshotStockRows(t, db, itemID); !equalRows(stocks, got) {
				t.Fatalf("batch %d rewrote history: %d rows before, %d after", batch, len(stocks), len(got))
			}
		}

		binAfter := binValue(t, stockSvc, db, base)
		if got := binAfter.Sub(binBefore); !got.Equal(onHandTotal) {
			t.Fatalf("bin value moved by %s, want on-hand deltas %s", got.String(), onHandTotal.String())
		}
		glAfter := scopedGLStock(t, db, vouchers)
		if got := glAfter.Sub(glBefore); !got.Equal(onHandTotal) {
			t.Fatalf("GL stock moved by %s, want on-hand deltas %s", got.String(), onHandTotal.String())
		}
	})
}

// postOp runs the movement seam in one transaction for a rapid iteration.
func postOp(t *rapid.T, db *sql.DB, stockSvc *stock.Service, engine *posting.Engine, m stock.Movement) (stock.PlannedMovement, stock.AppliedMovement) {
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

// postDeltaOp runs the delta seam in one transaction for a rapid iteration.
func postDeltaOp(t *rapid.T, db *sql.DB, stockSvc *stock.Service, engine *posting.Engine, d stock.CostDelta) (stock.PlannedDelta, stock.AppliedDelta) {
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

// drawDate draws a 2026 date on the 15th of a drawn month, always inside
// the seeded fiscal year.
func drawDate(t *rapid.T, name string) string {
	month := rapid.IntRange(1, 12).Draw(t, name)
	return fmt.Sprintf("2026-%02d-15", month)
}

// snapshotStockRows reads every l_stock column of an item's rows in id
// order. Comparing snapshots before and after later writes proves no
// historical row changed.
func snapshotStockRows(t *rapid.T, db *sql.DB, itemID string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT id, company_name, book, owner, item_type, item_id, control_account_name, location_name, COALESCE(lot_no, ''), COALESCE(serial_no, ''), base_uom, qty_base, value_base, base_currency, movement_date, posting_date, dimensions, voucher_type, voucher_id, voucher_revision, source_line_no, event, posting_group_id, is_reversal, COALESCE(reversed_row_id, ''), created_at FROM l_stock WHERE item_id = ? ORDER BY id`, itemID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		values := make([]any, 26)
		scanned := make([]any, 26)
		for i := range values {
			scanned[i] = &values[i]
		}
		if err := rows.Scan(scanned...); err != nil {
			t.Fatal(err)
		}
		parts := make([]string, 26)
		for i, value := range values {
			parts[i] = fmt.Sprintf("%v", value)
		}
		out = append(out, strings.Join(parts, "\x1f"))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func equalRows(before, after []string) bool {
	if len(before) != len(after) {
		return false
	}
	for i := range before {
		if before[i] != after[i] {
			return false
		}
	}
	return true
}

func binValue(t *rapid.T, stockSvc *stock.Service, db *sql.DB, m stock.Movement) types.Decimal {
	bin, found, err := stockSvc.BinBalance(context.Background(), db, stock.BinKey{
		Company: m.Company, Book: m.Book, Owner: m.Owner, ItemID: m.ItemID,
		Location: m.Location, LotNo: m.LotNo, SerialNo: m.SerialNo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("bin missing")
	}
	return bin.Value
}

func scopedGLStock(t *rapid.T, db *sql.DB, vouchers []string) types.Decimal {
	placeholders := make([]string, len(vouchers))
	args := make([]any, len(vouchers))
	for i, voucher := range vouchers {
		placeholders[i] = "?"
		args[i] = voucher
	}
	var balance string
	err := db.QueryRow(fmt.Sprintf(`SELECT COALESCE(SUM(debit_base - credit_base), 0) FROM l_gl WHERE account_name = 'stock' AND voucher_id IN (%s)`, strings.Join(placeholders, ",")),
		args...).Scan(&balance)
	if err != nil {
		t.Fatal(err)
	}
	total, err := types.ParseDecimal(balance)
	if err != nil {
		t.Fatal(err)
	}
	return total
}
