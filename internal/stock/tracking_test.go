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

func postTransfer(t *testing.T, db *sql.DB, stockSvc *stock.Service, engine *posting.Engine, tr stock.Transfer) (stock.PlannedTransfer, stock.AppliedTransfer) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	planned, err := stockSvc.PlanTransfer(ctx, tx, tr)
	if err != nil {
		t.Fatal(err)
	}
	groupID := ""
	if len(planned.Legs) > 0 {
		result, err := engine.PostInTx(ctx, tx, posting.Plan{
			VoucherType: tr.VoucherType, VoucherID: tr.VoucherID, Revision: tr.Revision,
			Event: posting.EventSubmit, Book: tr.Book, PostingDate: tr.PostingDate, Lines: planned.Legs,
		}, posting.PostOptions{Actor: "tester", Source: "test", OperationID: uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		groupID = result.PostingGroupID
	}
	applied, err := stockSvc.ApplyTransfer(ctx, tx, planned, groupID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return planned, applied
}

func testTransfer(m stock.Movement, toLocation string, qty string) stock.Transfer {
	return stock.Transfer{
		Company: m.Company, Book: m.Book, ItemType: m.ItemType, ItemID: m.ItemID,
		ControlAccount: m.ControlAccount, TransitAccount: "clearing",
		FromOwner: m.Owner, FromLocation: m.Location, ToOwner: m.Owner, ToLocation: toLocation,
		LotNo: m.LotNo, SerialNo: m.SerialNo,
		Quantity:     types.Quantity{Value: dec(qty), UoM: "each"},
		Valued:       m.Valued,
		MovementDate: "2026-06-15", PostingDate: "2026-06-15",
		VoucherType: "stock_transfer", VoucherID: uuid.NewString(), Revision: 1,
	}
}

func serialStatus(t *testing.T, db *sql.DB, itemID, serial string) string {
	t.Helper()
	var status string
	if err := db.QueryRow(`SELECT status FROM athena_serials WHERE item_id = ? AND serial_no = ?`, itemID, serial).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestSerialLifecycle(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	receipt.Quantity = types.Quantity{Value: dec("2"), UoM: "each"}
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.RegisterSerials(context.Background(), tx, receiptApplied.RowID, []string{"S1", "S2"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := serialStatus(t, db, receipt.ItemID, "S1"); got != stock.SerialOnHand {
		t.Fatalf("S1 is %s, want on_hand", got)
	}

	issue := issueOf(receipt, "2")
	_, issueApplied := postMovement(t, db, stockSvc, engine, issue)
	tx, err = db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.IssueSerials(context.Background(), tx, issueApplied.RowID, []stock.SerialOut{{SerialNo: "S1", Status: stock.SerialIssued}, {SerialNo: "S2", Status: stock.SerialIssued}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := serialStatus(t, db, receipt.ItemID, "S2"); got != stock.SerialIssued {
		t.Fatalf("S2 is %s, want issued", got)
	}

	back := receipt
	back.VoucherID = uuid.NewString()
	back.VoucherType = "stock_return"
	back.Quantity = types.Quantity{Value: dec("1"), UoM: "each"}
	back.UnitCost = types.Decimal{}
	back.ReversesRowID = issueApplied.RowID
	_, backApplied := postMovement(t, db, stockSvc, engine, back)
	tx, err = db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.ReturnSerials(context.Background(), tx, backApplied.RowID, []string{"S1"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := serialStatus(t, db, receipt.ItemID, "S1"); got != stock.SerialOnHand {
		t.Fatalf("S1 is %s after return, want on_hand", got)
	}

	tx, err = db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := stockSvc.RegisterSerials(context.Background(), tx, receiptApplied.RowID, []string{"S1"}); !errors.Is(err, stock.ErrInvalid) {
		t.Fatalf("short serial list error is %v, want ErrInvalid", err)
	}
	other := testMovement()
	other.ItemID = receipt.ItemID
	other.VoucherID = uuid.NewString()
	other.Quantity = types.Quantity{Value: dec("1"), UoM: "each"}
	_, otherApplied := postMovement(t, db, stockSvc, engine, other)
	if err := stockSvc.RegisterSerials(context.Background(), tx, otherApplied.RowID, []string{"S1"}); !errors.Is(err, stock.ErrDuplicateSerial) {
		t.Fatalf("duplicate serial error is %v, want ErrDuplicateSerial", err)
	}
}

func TestRentalTrackedNotConsumed(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	// One chair: unvalued receipt, serial registered, held for an event,
	// issued on rent, returned. No cost is ever consumed.
	chair := testMovement()
	chair.ItemType = "test_chair"
	chair.Valued = false
	chair.UnitCost = types.Decimal{}
	chair.Quantity = types.Quantity{Value: dec("1"), UoM: "each"}
	chair.SerialNo = "CH-1"
	_, receiptApplied := postMovement(t, db, stockSvc, engine, chair)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.RegisterSerials(ctx, tx, receiptApplied.RowID, []string{"CH-1"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	event := testReservation(chair, "1")
	event.SerialNo = "CH-1"
	event.HolderType = "event"
	event.HolderID = "EVT-1"
	held := hold(t, db, stockSvc, event)
	if got := serialStatus(t, db, chair.ItemID, "CH-1"); got != stock.SerialReserved {
		t.Fatalf("CH-1 is %s after hold, want reserved", got)
	}

	out := issueOf(chair, "1")
	out.Valued = false
	_, outApplied := postMovement(t, db, stockSvc, engine, out)
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.IssueSerials(ctx, tx, outApplied.RowID, []stock.SerialOut{{SerialNo: "CH-1", Status: stock.SerialOnRent}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := serialStatus(t, db, chair.ItemID, "CH-1"); got != stock.SerialOnRent {
		t.Fatalf("CH-1 is %s after issue, want on_rent", got)
	}
	var holdStatus string
	if err := db.QueryRow(`SELECT status FROM athena_reservations WHERE id = ?`, held.ID).Scan(&holdStatus); err != nil {
		t.Fatal(err)
	}
	if holdStatus != stock.ReserveConsumed {
		t.Fatalf("event hold is %s after issue, want consumed", holdStatus)
	}

	back := chair
	back.VoucherID = uuid.NewString()
	back.VoucherType = "stock_return"
	back.ReversesRowID = outApplied.RowID
	_, backApplied := postMovement(t, db, stockSvc, engine, back)
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.ReturnSerials(ctx, tx, backApplied.RowID, []string{"CH-1"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := serialStatus(t, db, chair.ItemID, "CH-1"); got != stock.SerialOnHand {
		t.Fatalf("CH-1 is %s after return, want on_hand", got)
	}
	bin, _ := binOf(t, stockSvc, db, chair)
	if got := bin.Qty.String(); got != "1.000000" {
		t.Fatalf("bin holds %s, want 1.000000", got)
	}
	if got := bin.Value.String(); got != "0.000000" {
		t.Fatalf("bin value is %s, want 0.000000 (never consumed)", got)
	}
	var gl int
	if err := db.QueryRow(`SELECT COUNT(*) FROM l_gl WHERE voucher_id IN (?, ?, ?)`, chair.VoucherID, out.VoucherID, back.VoucherID).Scan(&gl); err != nil {
		t.Fatal(err)
	}
	if gl != 0 {
		t.Fatalf("rental posted %d GL rows, want none", gl)
	}
	var layers int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_stock_layers WHERE item_id = ?`, chair.ItemID).Scan(&layers); err != nil {
		t.Fatal(err)
	}
	if layers != 0 {
		t.Fatalf("rental created %d layers, want none", layers)
	}
}

func TestSerialCancelReversals(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	receipt.Quantity = types.Quantity{Value: dec("1"), UoM: "each"}
	receipt.SerialNo = "S9"
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.RegisterSerials(ctx, tx, receiptApplied.RowID, []string{"S9"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	issue := issueOf(receipt, "1")
	_, issueApplied := postMovement(t, db, stockSvc, engine, issue)
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.IssueSerials(ctx, tx, issueApplied.RowID, []stock.SerialOut{{SerialNo: "S9", Status: stock.SerialIssued}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	back := receipt
	back.VoucherID = uuid.NewString()
	back.VoucherType = "stock_return"
	back.UnitCost = types.Decimal{}
	back.ReversesRowID = issueApplied.RowID
	_, backApplied := postMovement(t, db, stockSvc, engine, back)
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.ReturnSerials(ctx, tx, backApplied.RowID, []string{"S9"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	cancelVoucher(t, db, stockSvc, engine, back.VoucherType, back.VoucherID, true)
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.ReverseReturnSerials(ctx, tx, backApplied.RowID, stock.SerialIssued); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := serialStatus(t, db, receipt.ItemID, "S9"); got != stock.SerialIssued {
		t.Fatalf("S9 is %s after return cancel, want issued", got)
	}
	cancelVoucher(t, db, stockSvc, engine, issue.VoucherType, issue.VoucherID, true)
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.ReverseIssueSerials(ctx, tx, issueApplied.RowID, stock.SerialOnHand); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := serialStatus(t, db, receipt.ItemID, "S9"); got != stock.SerialOnHand {
		t.Fatalf("S9 is %s after issue cancel, want on_hand", got)
	}
	cancelVoucher(t, db, stockSvc, engine, receipt.VoucherType, receipt.VoucherID, true)
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.ReverseReceiptSerials(ctx, tx, receiptApplied.RowID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var serials int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_serials WHERE item_id = ?`, receipt.ItemID).Scan(&serials); err != nil {
		t.Fatal(err)
	}
	if serials != 0 {
		t.Fatalf("%d serials remain after receipt cancel, want none", serials)
	}
}

func TestReturnRequiresIssueBin(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)
	_, issueApplied := postMovement(t, db, stockSvc, engine, issueOf(receipt, "10"))
	// A return to another location or owner is a transfer, not a return:
	// return there after returning here.
	for name, mutate := range map[string]func(*stock.Movement){
		"third location": func(m *stock.Movement) { m.Location = "locC" },
		"other owner":    func(m *stock.Movement) { m.Owner = "consignment" },
	} {
		back := receipt
		back.VoucherID = uuid.NewString()
		back.VoucherType = "stock_return"
		back.UnitCost = types.Decimal{}
		back.ReversesRowID = issueApplied.RowID
		mutate(&back)
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = stockSvc.PlanMovement(context.Background(), tx, back)
		tx.Rollback()
		if !errors.Is(err, stock.ErrInvalid) {
			t.Errorf("%s: error is %v, want ErrInvalid", name, err)
		}
	}
}

func TestCrossLocationReturnCancelRestoresIssueBin(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)
	ctx := context.Background()
	serialTx := func(work func(*sql.Tx) error) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if err := work(tx); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	receipt := testMovement()
	receipt.Quantity = types.Quantity{Value: dec("1"), UoM: "each"}
	receipt.SerialNo = "X1"
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)
	serialTx(func(tx *sql.Tx) error {
		return stockSvc.RegisterSerials(ctx, tx, receiptApplied.RowID, []string{"X1"})
	})
	postTransfer(t, db, stockSvc, engine, testTransfer(receipt, "locB", "1"))

	issued := issueOf(receipt, "1")
	issued.Location = "locB"
	_, issuedApplied := postMovement(t, db, stockSvc, engine, issued)
	serialTx(func(tx *sql.Tx) error {
		return stockSvc.IssueSerials(ctx, tx, issuedApplied.RowID, []stock.SerialOut{{SerialNo: "X1", Status: stock.SerialIssued}})
	})
	back := receipt
	back.Location = "locB"
	back.VoucherID = uuid.NewString()
	back.VoucherType = "stock_return"
	back.UnitCost = types.Decimal{}
	back.ReversesRowID = issuedApplied.RowID
	_, backApplied := postMovement(t, db, stockSvc, engine, back)
	serialTx(func(tx *sql.Tx) error {
		return stockSvc.ReturnSerials(ctx, tx, backApplied.RowID, []string{"X1"})
	})

	cancelVoucher(t, db, stockSvc, engine, back.VoucherType, back.VoucherID, true)
	serialTx(func(tx *sql.Tx) error {
		return stockSvc.ReverseReturnSerials(ctx, tx, backApplied.RowID, stock.SerialIssued)
	})
	if got := serialStatus(t, db, receipt.ItemID, "X1"); got != stock.SerialIssued {
		t.Fatalf("X1 is %s after return cancel, want issued", got)
	}
	var location string
	if err := db.QueryRow(`SELECT location_name FROM athena_serials WHERE item_id = ? AND serial_no = 'X1'`, receipt.ItemID).Scan(&location); err != nil {
		t.Fatal(err)
	}
	if location != "locB" {
		t.Fatalf("X1 location is %s after return cancel, want locB (the issue bin)", location)
	}

	// A unit transferred away after its return blocks the return cancel
	// instead of being silently rewritten.
	moved := testMovement()
	moved.Quantity = types.Quantity{Value: dec("1"), UoM: "each"}
	moved.SerialNo = "X2"
	_, movedReceipt := postMovement(t, db, stockSvc, engine, moved)
	serialTx(func(tx *sql.Tx) error {
		return stockSvc.RegisterSerials(ctx, tx, movedReceipt.RowID, []string{"X2"})
	})
	movedIssue := issueOf(moved, "1")
	_, movedIssued := postMovement(t, db, stockSvc, engine, movedIssue)
	serialTx(func(tx *sql.Tx) error {
		return stockSvc.IssueSerials(ctx, tx, movedIssued.RowID, []stock.SerialOut{{SerialNo: "X2", Status: stock.SerialIssued}})
	})
	movedBack := moved
	movedBack.VoucherID = uuid.NewString()
	movedBack.VoucherType = "stock_return"
	movedBack.UnitCost = types.Decimal{}
	movedBack.ReversesRowID = movedIssued.RowID
	_, movedBackApplied := postMovement(t, db, stockSvc, engine, movedBack)
	serialTx(func(tx *sql.Tx) error {
		return stockSvc.ReturnSerials(ctx, tx, movedBackApplied.RowID, []string{"X2"})
	})
	postTransfer(t, db, stockSvc, engine, testTransfer(moved, "locC", "1"))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := stockSvc.ReverseReturnSerials(ctx, tx, movedBackApplied.RowID, stock.SerialIssued); !errors.Is(err, stock.ErrCancelBlocked) {
		t.Fatalf("moved-unit return cancel error is %v, want ErrCancelBlocked", err)
	}
}

func TestBatchFEFOPicksOldestExpiry(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receive := func(lot, qty, cost, expiry string) {
		t.Helper()
		movement := testMovement()
		movement.ItemID = "flour-item"
		movement.ItemType = "test_flour"
		movement.LotNo = lot
		movement.Quantity = types.Quantity{Value: dec(qty), UoM: "each"}
		movement.UnitCost = dec(cost)
		_, applied := postMovement(t, db, stockSvc, engine, movement)
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if err := stockSvc.EnsureBatch(context.Background(), tx, applied.RowID, lot, expiry, "2026-06-01"); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	receive("L1", "100", "2", "2026-09-01")
	receive("L2", "50", "2", "2026-07-01")
	receive("L3", "200", "2", "")

	picks, short, err := stockSvc.PickFEFO(context.Background(), db, "acme", "main", "", "flour-item", "wh_main", dec("120"))
	if err != nil {
		t.Fatal(err)
	}
	if short.Sign() != 0 {
		t.Fatalf("shortfall is %s, want 0", short)
	}
	if len(picks) != 2 || picks[0].LotNo != "L2" || picks[0].Qty.String() != "50.000000" || picks[1].LotNo != "L1" || picks[1].Qty.String() != "70.000000" {
		t.Fatalf("picks are %+v, want [(L2 50) (L1 70)]", picks)
	}
	picks, short, err = stockSvc.PickFEFO(context.Background(), db, "acme", "main", "", "flour-item", "wh_main", dec("400"))
	if err != nil {
		t.Fatal(err)
	}
	if short.String() != "50.000000" {
		t.Fatalf("shortfall is %s, want 50.000000", short)
	}
	if len(picks) != 3 || picks[2].LotNo != "L3" {
		t.Fatalf("last pick is %+v, want L3 last (no expiry)", picks)
	}
}

func TestTransferPreservesCompanyValue(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)

	planned, _ := postTransfer(t, db, stockSvc, engine, testTransfer(receipt, "quarantine", "30"))
	if len(planned.Legs) != 4 {
		t.Fatalf("transfer has %d legs, want 4", len(planned.Legs))
	}
	atMain, _ := binOf(t, stockSvc, db, receipt)
	if got := atMain.Qty.String(); got != "70.000000" {
		t.Fatalf("main bin holds %s, want 70.000000", got)
	}
	moved := receipt
	moved.Location = "quarantine"
	atQuarantine, found := binOf(t, stockSvc, db, moved)
	if !found {
		t.Fatal("quarantine bin missing")
	}
	if got := atQuarantine.Qty.String(); got != "30.000000" {
		t.Fatalf("quarantine bin holds %s, want 30.000000", got)
	}
	if got := atQuarantine.Value.String(); got != "300.000000" {
		t.Fatalf("quarantine bin value is %s, want 300.000000", got)
	}
	if got := glStockBalance(t, db); got != "1000.000000" {
		t.Fatalf("GL stock balance is %s, want 1000.000000 (unchanged)", got)
	}
	if got := glBalance(t, db, "clearing"); got != "-1000.000000" {
		t.Fatalf("GL clearing balance is %s, want -1000.000000 (transit nets zero)", got)
	}

	back := testTransfer(receipt, "wh_main", "30")
	back.FromLocation = "quarantine"
	postTransfer(t, db, stockSvc, engine, back)
	atMain, _ = binOf(t, stockSvc, db, receipt)
	if got := atMain.Qty.String(); got != "100.000000" {
		t.Fatalf("main bin holds %s after return, want 100.000000", got)
	}
}

func TestTransferCancelUnwindsBothLines(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)
	transfer := testTransfer(receipt, "quarantine", "30")
	postTransfer(t, db, stockSvc, engine, transfer)

	cancelled := cancelVoucher(t, db, stockSvc, engine, transfer.VoucherType, transfer.VoucherID, true)
	if len(cancelled.RowIDs) != 2 {
		t.Fatalf("cancel wrote %d rows, want 2", len(cancelled.RowIDs))
	}
	atMain, _ := binOf(t, stockSvc, db, receipt)
	if got := atMain.Qty.String(); got != "100.000000" {
		t.Fatalf("main bin holds %s after cancel, want 100.000000", got)
	}
	moved := receipt
	moved.Location = "quarantine"
	atQuarantine, _ := binOf(t, stockSvc, db, moved)
	if got := atQuarantine.Qty.String(); got != "0.000000" {
		t.Fatalf("quarantine bin holds %s after cancel, want 0.000000", got)
	}
	if got := glStockBalance(t, db); got != "1000.000000" {
		t.Fatalf("GL stock balance is %s after cancel, want 1000.000000", got)
	}
}

func TestTransferHeldStockRefused(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)
	hold(t, db, stockSvc, testReservation(receipt, "90"))

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := stockSvc.PlanTransfer(context.Background(), tx, testTransfer(receipt, "quarantine", "20")); !errors.Is(err, stock.ErrInsufficientAvailable) {
		t.Fatalf("held transfer error is %v, want ErrInsufficientAvailable", err)
	}
}

func TestTransferSerialMovesUnit(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	receipt.Quantity = types.Quantity{Value: dec("1"), UoM: "each"}
	receipt.SerialNo = "T1"
	_, receiptApplied := postMovement(t, db, stockSvc, engine, receipt)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.RegisterSerials(context.Background(), tx, receiptApplied.RowID, []string{"T1"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	postTransfer(t, db, stockSvc, engine, testTransfer(receipt, "loc2", "1"))
	var location string
	if err := db.QueryRow(`SELECT location_name FROM athena_serials WHERE item_id = ? AND serial_no = 'T1'`, receipt.ItemID).Scan(&location); err != nil {
		t.Fatal(err)
	}
	if location != "loc2" {
		t.Fatalf("serial location is %s, want loc2", location)
	}
}
