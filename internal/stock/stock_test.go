package stock_test

import (
	"context"
	"database/sql"
	"errors"
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

// TestAvailabilityProperty checks Property 23 over drawn reservation sets:
// available always equals on-hand minus active holds, takes beyond it
// fail, and no set of holds exceeds on-hand for one item and location.
//
// Feature: athena-batch-2, Property 23: Availability.
//
// One shared database serves all iterations; fresh item ids isolate them.
// Each iteration funds two locations, draws a set of holds across holders
// (some over, some duplicated), then draws releases, consumes and an
// expiry sweep, asserting an independent arithmetic model against the
// service after every step.
func TestAvailabilityProperty(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		itemID := uuid.NewString()
		locations := []string{"wh_main", "wh_east"}
		type modelHold struct {
			id      string
			qty     int64
			expires bool
		}
		type locModel struct {
			onHand int64
			holds  map[string]*modelHold
		}
		models := map[string]*locModel{}
		for _, location := range locations {
			funding := testMovement()
			funding.ItemID = itemID
			funding.Location = location
			funding.Quantity = types.Quantity{Value: types.MustDecimal(fmt.Sprint(rapid.IntRange(50, 100).Draw(t, "fund"))), UoM: "each"}
			postOp(t, db, stockSvc, engine, funding)
			onHand, err := decimalInt(funding.Quantity.Value)
			if err != nil {
				t.Fatal(err)
			}
			models[location] = &locModel{onHand: onHand, holds: map[string]*modelHold{}}
		}
		assertLocations := func() {
			for _, location := range locations {
				model := models[location]
				var reserved int64
				for _, hold := range model.holds {
					reserved += hold.qty
				}
				avail, err := stockSvc.Available(ctx, db, stock.BinKey{
					Company: "acme", Book: "main", Owner: "company", ItemID: itemID, Location: location,
				})
				if err != nil {
					t.Fatal(err)
				}
				if got := avail.OnHand.String(); got != fmt.Sprintf("%d.000000", model.onHand) {
					t.Fatalf("%s on-hand is %s, want %d", location, got, model.onHand)
				}
				if got := avail.Reserved.String(); got != fmt.Sprintf("%d.000000", reserved) {
					t.Fatalf("%s reserved is %s, want %d", location, got, reserved)
				}
				if got := avail.Available.String(); got != fmt.Sprintf("%d.000000", model.onHand-reserved) {
					t.Fatalf("%s available is %s, want %d", location, got, model.onHand-reserved)
				}
			}
		}

		attempts := rapid.IntRange(1, 8).Draw(t, "attempts")
		for i := 0; i < attempts; i++ {
			location := locations[rapid.IntRange(0, 1).Draw(t, "location")]
			holder := fmt.Sprintf("holder-%d", rapid.IntRange(1, 3).Draw(t, "holder"))
			qty := int64(rapid.IntRange(1, 60).Draw(t, "holdQty"))
			model := models[location]
			var reserved int64
			for _, hold := range model.holds {
				reserved += hold.qty
			}
			_, dup := model.holds[holder]
			r := testReservationFor(t, itemID, location, holder, qty)
			held, err := tryReserve(ctx, db, stockSvc, r)
			switch {
			case qty > model.onHand-reserved:
				if !errors.Is(err, stock.ErrInsufficientAvailable) {
					t.Fatalf("over-hold error is %v, want ErrInsufficientAvailable", err)
				}
			case dup:
				if !errors.Is(err, stock.ErrDuplicateHold) {
					t.Fatalf("double-hold error is %v, want ErrDuplicateHold", err)
				}
			default:
				if err != nil {
					t.Fatalf("hold error is %v, want success", err)
				}
				model.holds[holder] = &modelHold{id: held.ID, qty: qty, expires: r.ExpiresAt != "" && r.ExpiresAt < "2026-06-15 12:00:00"}
			}
			assertLocations()
		}

		lifecycle := rapid.IntRange(0, 4).Draw(t, "lifecycle")
		for i := 0; i < lifecycle; i++ {
			type candidate struct {
				location string
				holder   string
				hold     *modelHold
			}
			var actives []candidate
			for _, location := range locations {
				for holder, hold := range models[location].holds {
					actives = append(actives, candidate{location, holder, hold})
				}
			}
			choice := rapid.IntRange(0, 99).Draw(t, "lifecycleOp")
			switch {
			case len(actives) == 0 || choice < 25:
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := stockSvc.ExpireReservations(ctx, tx, "acme", "main", "2026-06-15 12:00:00"); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
				// The sweep is workspace-wide and may free earlier
				// iterations' holds too, so assert per-row statuses for
				// this iteration's holds instead of the freed count.
				for _, location := range locations {
					for holder, hold := range models[location].holds {
						var status string
						if err := db.QueryRow(`SELECT status FROM athena_reservations WHERE id = ?`, hold.id).Scan(&status); err != nil {
							t.Fatal(err)
						}
						if hold.expires && status != stock.ReserveExpired {
							t.Fatalf("past-expiry hold is %s, want expired", status)
						}
						if !hold.expires && status != stock.ReserveActive {
							t.Fatalf("live hold is %s, want active", status)
						}
						if hold.expires {
							delete(models[location].holds, holder)
						}
					}
				}
			default:
				pick := actives[rapid.IntRange(0, len(actives)-1).Draw(t, "lifecycleHold")]
				part := int64(rapid.IntRange(1, int(pick.hold.qty)).Draw(t, "lifecycleQty"))
				amount := types.MustDecimal(fmt.Sprint(part))
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				if choice < 65 {
					if err := stockSvc.ReleaseReservation(ctx, tx, pick.hold.id, amount); err != nil {
						t.Fatal(err)
					}
				} else if err := stockSvc.ConsumeReservation(ctx, tx, pick.hold.id, amount); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
				if left := pick.hold.qty - part; left == 0 {
					delete(models[pick.location].holds, pick.holder)
				} else {
					pick.hold.qty = left
				}
			}
			assertLocations()
		}

		for _, location := range locations {
			var active, onHand string
			err := db.QueryRow(`SELECT (SELECT COALESCE(SUM(qty), 0) FROM athena_reservations WHERE company_name = 'acme' AND book = 'main' AND owner = 'company' AND item_id = ? AND location_name = ? AND status = 'active'), on_hand FROM athena_stock_bins WHERE company_name = 'acme' AND book = 'main' AND owner = 'company' AND item_id = ? AND location_name = ?`,
				itemID, location, itemID, location).Scan(&active, &onHand)
			if err != nil {
				t.Fatal(err)
			}
			heldTotal, err := types.ParseDecimal(active)
			if err != nil {
				t.Fatal(err)
			}
			stockTotal, err := types.ParseDecimal(onHand)
			if err != nil {
				t.Fatal(err)
			}
			if heldTotal.Cmp(stockTotal) > 0 {
				t.Fatalf("%s holds %s exceed on-hand %s", location, active, onHand)
			}
		}
	})
}

func testReservationFor(t *rapid.T, itemID, location, holder string, qty int64) stock.Reservation {
	expiry := ""
	switch rapid.IntRange(0, 2).Draw(t, "expiry") {
	case 1:
		expiry = "2026-01-01 00:00:00"
	case 2:
		expiry = "2027-01-01 00:00:00"
	}
	return stock.Reservation{
		Company: "acme", Book: "main", Owner: "company",
		ItemType: "test_widget", ItemID: itemID, Location: location,
		Quantity:   types.Quantity{Value: types.MustDecimal(fmt.Sprint(qty)), UoM: "each"},
		HolderType: "cart", HolderID: holder, ExpiresAt: expiry,
	}
}

func tryReserve(ctx context.Context, db *sql.DB, stockSvc *stock.Service, r stock.Reservation) (stock.HeldReservation, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return stock.HeldReservation{}, err
	}
	defer tx.Rollback()
	held, err := stockSvc.Reserve(ctx, tx, r)
	if err != nil {
		return stock.HeldReservation{}, err
	}
	if err := tx.Commit(); err != nil {
		return stock.HeldReservation{}, err
	}
	return held, nil
}

func decimalInt(d types.Decimal) (int64, error) {
	whole, err := d.Rescale(0)
	if err != nil {
		return 0, err
	}
	var out int64
	if _, err := fmt.Sscan(whole.String(), &out); err != nil {
		return 0, err
	}
	return out, nil
}
