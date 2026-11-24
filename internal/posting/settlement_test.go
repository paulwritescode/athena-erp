// Settlement service tests (batch-2 G7). Functional coverage lives here
// from 7.2; Property 11 (allocation sums) and Property 12 (party ledger
// equals control) append in 7.3 and 7.5.
package posting_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"athena-erp/internal/ledger"
	"athena-erp/internal/posting"
	"athena-erp/internal/server"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
	"athena-erp/internal/types"
)

func settleSetup(t *testing.T) (*posting.SettlementService, *sql.DB) {
	t.Helper()
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, store.MariaDB{}, server.SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	masters := &ledger.Service{DB: db, Dialect: store.MariaDB{}}
	ctx := context.Background()
	if _, err := masters.SaveCurrency(ctx, ledger.Currency{Code: "USD", Label: "US dollar", Exponent: 2}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := masters.SaveCompany(ctx, ledger.Company{Name: "acme", Label: "Acme", BaseCurrency: "USD"}, 0); err != nil {
		t.Fatal(err)
	}
	return &posting.SettlementService{DB: db, Dialect: store.MariaDB{}}, db
}

const settleParty = "11111111-1111-1111-1111-111111111111"

func settleItem(amount string, instalment int) posting.NewItem {
	return posting.NewItem{
		Company: "acme", Book: "main", PartyType: "customer", PartyID: settleParty,
		ControlAccount: "receivable", SourceType: "invoice", SourceID: "22222222-2222-2222-2222-222222222222",
		SourceRevision: 2, InstalmentNo: instalment, DueDate: "2026-07-01",
		Currency: "USD", Side: posting.ItemSideDebit, Amount: types.MustDecimal(amount),
	}
}

func createItems(t *testing.T, svc *posting.SettlementService, db *sql.DB, items []posting.NewItem) []posting.Item {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	created, err := svc.CreateItemsInTx(context.Background(), tx, items)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return created
}

func settleRequest(group string, lines []posting.SettleLine) posting.SettleRequest {
	return posting.SettleRequest{
		Company: "acme", Book: "main", PartyType: "customer", PartyID: settleParty,
		ControlAccount: "receivable", Side: posting.ItemSideDebit,
		VoucherType: "receipt", VoucherID: "33333333-3333-3333-3333-333333333333",
		Revision: 2, Event: posting.EventSubmit, PostingDate: "2026-06-20",
		PostingGroupID: group, PaymentCurrency: "USD", Lines: lines,
	}
}

func sumApplied(t *testing.T, result posting.ApplyResult) types.Decimal {
	t.Helper()
	total := types.Decimal{}
	for _, application := range result.Applications {
		total = total.Add(application.PaymentAmount)
	}
	for _, unapplied := range result.Unapplied {
		total = total.Add(unapplied.Amount)
	}
	return total
}

func TestCreateAndListOpenItems(t *testing.T) {
	svc, db := settleSetup(t)
	created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1), settleItem("50.00", 2)})
	if created[0].ID == "" || created[0].ID == created[1].ID {
		t.Fatalf("items need distinct ids: %+v", created)
	}
	if !created[0].Residual.Equal(types.MustDecimal("100.00")) || created[0].Status != posting.ItemOpen {
		t.Fatalf("new item carries its full residual open: %+v", created[0])
	}
	items, err := svc.ListOpenItems(context.Background(), db, "acme", "main", "customer", settleParty)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID > items[1].ID {
		t.Fatalf("open items list in stable id order, got %d", len(items))
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	_, err = svc.CreateItemsInTx(context.Background(), tx, []posting.NewItem{settleItem("10.00", 1)})
	if !errors.Is(err, posting.ErrItemExists) {
		t.Fatalf("duplicate instalment reports ErrItemExists, got %v", err)
	}
}

func TestApplySettlesExactPayment(t *testing.T) {
	svc, db := settleSetup(t)
	created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1), settleItem("50.00", 2)})
	result, err := svc.Apply(context.Background(), settleRequest("44444444-4444-4444-4444-444444444444", []posting.SettleLine{
		{LineNo: 1, Amount: types.MustDecimal("150.00"), Items: []posting.LineItem{{ItemID: created[0].ID}, {ItemID: created[1].ID}}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applications) != 2 || len(result.Unapplied) != 0 {
		t.Fatalf("exact payment applies fully: %+v", result)
	}
	if !sumApplied(t, result).Equal(types.MustDecimal("150.00")) {
		t.Fatalf("applications sum to the payment: %+v", result)
	}
	for _, item := range result.Items {
		if item.Status != posting.ItemSettled || item.Residual.Sign() != 0 {
			t.Fatalf("fully paid item settles: %+v", item)
		}
	}
	var applications, partyRows, unapplied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_settlement_applications`).Scan(&applications); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*), COUNT(IF(item_id IS NULL, 1, NULL)) FROM l_party`).Scan(&partyRows, &unapplied); err != nil {
		t.Fatal(err)
	}
	if applications != 2 || partyRows != 2 || unapplied != 0 {
		t.Fatalf("applications=%d party rows=%d unapplied=%d", applications, partyRows, unapplied)
	}
	var credit string
	if err := db.QueryRow(`SELECT credit_base FROM l_party LIMIT 1`).Scan(&credit); err != nil {
		t.Fatal(err)
	}
	if credit == "0" || credit == "0.000000" {
		t.Fatalf("receivable applications settle by credit, got %s", credit)
	}
}

func TestApplyLeavesUnappliedRemainder(t *testing.T) {
	svc, db := settleSetup(t)
	created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1)})
	result, err := svc.Apply(context.Background(), settleRequest("55555555-5555-5555-5555-555555555555", []posting.SettleLine{
		{LineNo: 1, Amount: types.MustDecimal("130.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applications) != 1 || len(result.Unapplied) != 1 {
		t.Fatalf("overpayment applies the residual and keeps the rest: %+v", result)
	}
	if !result.Unapplied[0].Amount.Equal(types.MustDecimal("30.00")) || result.Unapplied[0].PartyRowID == "" {
		t.Fatalf("unapplied remainder carries its party row: %+v", result.Unapplied)
	}
	if !sumApplied(t, result).Equal(types.MustDecimal("130.00")) {
		t.Fatalf("applied plus unapplied sum to the payment: %+v", result)
	}
	var unapplied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM l_party WHERE item_id IS NULL`).Scan(&unapplied); err != nil {
		t.Fatal(err)
	}
	if unapplied != 1 {
		t.Fatalf("unapplied cash lives in l_party without an item, got %d rows", unapplied)
	}
	var items int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_party_items`).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if items != 1 {
		t.Fatalf("unapplied cash never becomes an item, got %d items", items)
	}
}

func TestApplyExplicitAmounts(t *testing.T) {
	svc, db := settleSetup(t)
	created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1), settleItem("50.00", 2)})
	result, err := svc.Apply(context.Background(), settleRequest("66666666-6666-6666-6666-666666666666", []posting.SettleLine{
		{LineNo: 1, Amount: types.MustDecimal("90.00"), Items: []posting.LineItem{
			{ItemID: created[0].ID, Amount: types.MustDecimal("60.00")},
			{ItemID: created[1].ID},
		}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applications) != 2 || len(result.Unapplied) != 0 {
		t.Fatalf("explicit plus auto shares cover the line: %+v", result)
	}
	byItem := map[string]types.Decimal{}
	for _, application := range result.Applications {
		byItem[application.ItemID] = application.PaymentAmount
	}
	if !byItem[created[0].ID].Equal(types.MustDecimal("60.00")) || !byItem[created[1].ID].Equal(types.MustDecimal("30.00")) {
		t.Fatalf("explicit first, remainder auto: %+v", result.Applications)
	}
	_, err = svc.Apply(context.Background(), settleRequest("77777777-7777-7777-7777-777777777777", []posting.SettleLine{
		{LineNo: 1, Amount: types.MustDecimal("90.00"), Items: []posting.LineItem{{ItemID: created[0].ID, Amount: types.MustDecimal("90.00")}}},
	}))
	if !errors.Is(err, posting.ErrInvalid) {
		t.Fatalf("explicit over-application refuses, got %v", err)
	}
}

func TestApplyRefusesMismatches(t *testing.T) {
	svc, db := settleSetup(t)
	created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1)})
	foreign := settleItem("100.00", 2)
	foreign.Currency = "EUR"
	createdForeign := createItems(t, svc, db, []posting.NewItem{foreign})
	payable := settleItem("100.00", 3)
	payable.Side = posting.ItemSideCredit
	createdPayable := createItems(t, svc, db, []posting.NewItem{payable})
	group := "88888888-8888-8888-8888-888888888888"
	for name, mutate := range map[string]func(*posting.SettleRequest){
		"unknown item": func(req *posting.SettleRequest) {
			req.Lines[0].Items[0].ItemID = "99999999-9999-9999-9999-999999999999"
		},
		"other party": func(req *posting.SettleRequest) { req.PartyID = "99999999-9999-9999-9999-999999999999" },
		"other side": func(req *posting.SettleRequest) {
			req.Lines[0].Items[0].ItemID = createdPayable[0].ID
		},
		"other currency": func(req *posting.SettleRequest) {
			req.Lines[0].Items[0].ItemID = createdForeign[0].ID
		},
		"payment off base": func(req *posting.SettleRequest) { req.PaymentCurrency = "EUR" },
	} {
		req := settleRequest(group, []posting.SettleLine{
			{LineNo: 1, Amount: types.MustDecimal("10.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}},
		})
		mutate(&req)
		if _, err := svc.Apply(context.Background(), req); !errors.Is(err, posting.ErrInvalid) {
			t.Fatalf("%s refuses with ErrInvalid, got %v", name, err)
		}
	}
	// Settling the item first makes a second application refuse as settled.
	if _, err := svc.Apply(context.Background(), settleRequest(group, []posting.SettleLine{
		{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}},
	})); err != nil {
		t.Fatal(err)
	}
	req := settleRequest("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", []posting.SettleLine{
		{LineNo: 1, Amount: types.MustDecimal("10.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}},
	})
	if _, err := svc.Apply(context.Background(), req); !errors.Is(err, posting.ErrInvalid) {
		t.Fatalf("settled item refuses, got %v", err)
	}
}

func TestApplyReplaysSameLineOnce(t *testing.T) {
	svc, db := settleSetup(t)
	created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1)})
	req := settleRequest("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", []posting.SettleLine{
		{LineNo: 1, Amount: types.MustDecimal("40.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}},
	})
	if _, err := svc.Apply(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	// The same voucher, line and item replays against the unique key even
	// though the item still has residual left.
	if _, err := svc.Apply(context.Background(), req); !errors.Is(err, posting.ErrAlreadyApplied) {
		t.Fatalf("replayed line reports ErrAlreadyApplied, got %v", err)
	}
}

func TestApplyOrderIndependent(t *testing.T) {
	// Each order runs in its own database against untouched residuals; the
	// per-instalment shares must match, keyed by instalment number since the
	// item ids differ per database.
	apply := func(order []int) map[int]string {
		svc, db := settleSetup(t)
		created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1), settleItem("60.00", 2), settleItem("40.00", 3)})
		byInstalment := map[int]string{}
		for _, item := range created {
			byInstalment[item.InstalmentNo] = item.ID
		}
		items := make([]posting.LineItem, len(order))
		for i, instalment := range order {
			items[i] = posting.LineItem{ItemID: byInstalment[instalment]}
		}
		result, err := svc.Apply(context.Background(), settleRequest("cccccccc-cccc-cccc-cccc-cccccccccccc", []posting.SettleLine{
			{LineNo: 1, Amount: types.MustDecimal("120.00"), Items: items},
		}))
		if err != nil {
			t.Fatal(err)
		}
		byItem := map[string]int{}
		for _, item := range created {
			byItem[item.ID] = item.InstalmentNo
		}
		got := map[int]string{}
		for _, application := range result.Applications {
			got[byItem[application.ItemID]] = application.PaymentAmount.String()
		}
		return got
	}
	first := apply([]int{1, 2, 3})
	second := apply([]int{3, 2, 1})
	for instalment, amount := range first {
		if second[instalment] != amount {
			t.Fatalf("instalment %d gets %s listed forward, %s listed backward", instalment, amount, second[instalment])
		}
	}
}

func TestConcurrentAppliesNeverOverApply(t *testing.T) {
	svc, db := settleSetup(t)
	created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1)})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			group := "dddddddd-dddd-dddd-dddd-dddddddddddd"
			voucher := "dddddddd-0000-0000-0000-dddddddddddd"
			if n == 1 {
				group = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
				voucher = "eeeeeeee-0000-0000-0000-eeeeeeeeeeee"
			}
			req := settleRequest(group, []posting.SettleLine{
				{LineNo: 1, Amount: types.MustDecimal("80.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}},
			})
			req.VoucherID = voucher
			_, errs[n] = svc.Apply(context.Background(), req)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("racing applies cap instead of failing: %v", err)
		}
	}
	var residual string
	if err := db.QueryRow(`SELECT residual_amount FROM athena_party_items WHERE id = ?`, created[0].ID).Scan(&residual); err != nil {
		t.Fatal(err)
	}
	var applied string
	if err := db.QueryRow(`SELECT COALESCE(SUM(payment_amount), 0) FROM athena_settlement_applications`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if residual != "0.000000" && residual != "0" {
		t.Fatalf("racing applies leave residual %s", residual)
	}
	if applied != "100.000000" && applied != "100.00" && applied != "100" {
		t.Fatalf("racing applies total %s, want 100", applied)
	}
}
