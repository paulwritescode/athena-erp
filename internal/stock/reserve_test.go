package stock_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"athena-erp/internal/posting"
	"athena-erp/internal/stock"
	"athena-erp/internal/types"

	"github.com/google/uuid"
)

func testReservation(m stock.Movement, qty string) stock.Reservation {
	return stock.Reservation{
		Company: m.Company, Book: m.Book, Owner: m.Owner,
		ItemType: m.ItemType, ItemID: m.ItemID, Location: m.Location,
		Quantity:   types.Quantity{Value: dec(qty), UoM: "each"},
		HolderType: "cart", HolderID: uuid.NewString(),
	}
}

func hold(t *testing.T, db *sql.DB, stockSvc *stock.Service, r stock.Reservation) stock.HeldReservation {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	held, err := stockSvc.Reserve(context.Background(), tx, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return held
}

func TestReserveReducesAvailability(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)

	held := hold(t, db, stockSvc, testReservation(receipt, "30"))
	if held.Qty.String() != "30.000000" {
		t.Fatalf("held %s, want 30.000000", held.Qty)
	}
	avail, err := stockSvc.Available(context.Background(), db, stock.BinKey{
		Company: receipt.Company, Book: receipt.Book, Owner: receipt.Owner, ItemID: receipt.ItemID,
		Location: receipt.Location,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := avail.Available.String(); got != "70.000000" {
		t.Fatalf("available is %s, want 70.000000", got)
	}
	if got := avail.Reserved.String(); got != "30.000000" {
		t.Fatalf("reserved is %s, want 30.000000", got)
	}

	second := testReservation(receipt, "71")
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := stockSvc.Reserve(context.Background(), tx, second); !errors.Is(err, stock.ErrInsufficientAvailable) {
		t.Fatalf("over-hold error is %v, want ErrInsufficientAvailable", err)
	}
}

func TestCompetingTakesSerializeOnBinLock(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	receipt.Quantity = types.Quantity{Value: dec("10"), UoM: "each"}
	postMovement(t, db, stockSvc, engine, receipt)

	ctx := context.Background()
	connA, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connA.Close()
	txA, err := connA.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer txA.Rollback()
	if _, err := stockSvc.Reserve(ctx, txA, testReservation(receipt, "10")); err != nil {
		t.Fatal(err)
	}
	connB, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connB.Close()
	if _, err := connB.ExecContext(ctx, `SET SESSION innodb_lock_wait_timeout = 1`); err != nil {
		t.Fatal(err)
	}
	txB, err := connB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer txB.Rollback()
	_, err = stockSvc.Reserve(ctx, txB, testReservation(receipt, "1"))
	if err == nil || errors.Is(err, stock.ErrInsufficientAvailable) {
		t.Fatalf("second take error is %v, want a lock wait (not an availability decision)", err)
	}
	if !strings.Contains(err.Error(), "1205") {
		t.Fatalf("second take error is %v, want lock wait timeout 1205", err)
	}
}

func TestReleaseConsumeExpireLifecycle(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)

	held := hold(t, db, stockSvc, testReservation(receipt, "30"))
	release := func(id string, qty string) {
		t.Helper()
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		amount := types.Decimal{}
		if qty != "" {
			amount = dec(qty)
		}
		if err := stockSvc.ReleaseReservation(context.Background(), tx, id, amount); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	release(held.ID, "10")
	avail, err := stockSvc.Available(context.Background(), db, stock.BinKey{
		Company: receipt.Company, Book: receipt.Book, Owner: receipt.Owner, ItemID: receipt.ItemID,
		Location: receipt.Location,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := avail.Available.String(); got != "80.000000" {
		t.Fatalf("available is %s after partial release, want 80.000000", got)
	}
	release(held.ID, "")
	avail, err = stockSvc.Available(context.Background(), db, stock.BinKey{
		Company: receipt.Company, Book: receipt.Book, Owner: receipt.Owner, ItemID: receipt.ItemID,
		Location: receipt.Location,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := avail.Available.String(); got != "100.000000" {
		t.Fatalf("available is %s after full release, want 100.000000", got)
	}

	consumed := hold(t, db, stockSvc, testReservation(receipt, "25"))
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	issue := issueOf(receipt, "25")
	planned, err := stockSvc.PlanMovement(context.Background(), tx, issue)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.PostInTx(context.Background(), tx, posting.Plan{
		VoucherType: issue.VoucherType, VoucherID: issue.VoucherID, Revision: issue.Revision,
		Event: posting.EventSubmit, Book: issue.Book, PostingDate: issue.PostingDate, Lines: planned.Legs,
	}, posting.PostOptions{Actor: "tester", Source: "test", OperationID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stockSvc.ApplyMovement(context.Background(), tx, planned, result.PostingGroupID); err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.ConsumeReservation(context.Background(), tx, consumed.ID, types.Decimal{}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	avail, err = stockSvc.Available(context.Background(), db, stock.BinKey{
		Company: receipt.Company, Book: receipt.Book, Owner: receipt.Owner, ItemID: receipt.ItemID,
		Location: receipt.Location,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := avail.OnHand.String(); got != "75.000000" {
		t.Fatalf("on-hand is %s after consume+issue, want 75.000000", got)
	}
	if got := avail.Reserved.String(); got != "0.000000" {
		t.Fatalf("reserved is %s after consume, want 0.000000", got)
	}

	expiring := testReservation(receipt, "5")
	expiring.ExpiresAt = "2026-01-01 00:00:00"
	hold(t, db, stockSvc, expiring)
	keeping := testReservation(receipt, "5")
	keeping.ExpiresAt = "2027-01-01 00:00:00"
	hold(t, db, stockSvc, keeping)
	tx, err = db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := stockSvc.ExpireReservations(context.Background(), tx, "acme", "main", "2026-06-15 12:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if expired != 1 {
		t.Fatalf("expired %d holds, want 1", expired)
	}
	avail, err = stockSvc.Available(context.Background(), db, stock.BinKey{
		Company: receipt.Company, Book: receipt.Book, Owner: receipt.Owner, ItemID: receipt.ItemID,
		Location: receipt.Location,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := avail.Reserved.String(); got != "5.000000" {
		t.Fatalf("reserved is %s after expiry, want 5.000000", got)
	}
}

func TestReholdAfterReleaseReactivates(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)

	first := testReservation(receipt, "10")
	held := hold(t, db, stockSvc, first)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stockSvc.ReleaseReservation(context.Background(), tx, held.ID, types.Decimal{}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	again := first
	again.Quantity = types.Quantity{Value: dec("15"), UoM: "each"}
	again.ExpiresAt = "2027-06-01 00:00:00"
	reheld := hold(t, db, stockSvc, again)
	if reheld.ID != held.ID {
		t.Fatalf("re-hold id is %s, want revived %s", reheld.ID, held.ID)
	}
	if reheld.Qty.String() != "15.000000" {
		t.Fatalf("re-hold qty is %s, want 15.000000", reheld.Qty)
	}
	avail, err := stockSvc.Available(context.Background(), db, stock.BinKey{
		Company: receipt.Company, Book: receipt.Book, Owner: receipt.Owner, ItemID: receipt.ItemID,
		Location: receipt.Location,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := avail.Reserved.String(); got != "15.000000" {
		t.Fatalf("reserved is %s after re-hold, want 15.000000", got)
	}
}

func TestDuplicateHoldRejected(t *testing.T) {
	stockSvc, engine, masters, db := setup(t)
	seedStockBasics(t, masters)

	receipt := testMovement()
	postMovement(t, db, stockSvc, engine, receipt)

	first := testReservation(receipt, "10")
	hold(t, db, stockSvc, first)
	second := first
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := stockSvc.Reserve(context.Background(), tx, second); !errors.Is(err, stock.ErrDuplicateHold) {
		t.Fatalf("duplicate hold error is %v, want ErrDuplicateHold", err)
	}
}
