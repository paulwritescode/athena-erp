// Settlement service tests (batch-2 G7). Functional coverage lives here
// from 7.2; Property 11 (allocation sums) and Property 12 (party ledger
// equals control) append in 7.3 and 7.5.
package posting_test

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"athena-erp/internal/ledger"
	"athena-erp/internal/posting"
	"athena-erp/internal/server"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
	"athena-erp/internal/types"

	"github.com/google/uuid"
	"pgregory.net/rapid"
)

func settleSetup(t *testing.T) (*posting.SettlementService, *sql.DB) {
	t.Helper()
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, store.MariaDB{}, server.SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	masters := &ledger.Service{DB: db, Dialect: store.MariaDB{}}
	ctx := context.Background()
	for _, currency := range []ledger.Currency{
		{Code: "USD", Label: "US dollar", Exponent: 2},
		{Code: "EUR", Label: "Euro", Exponent: 2},
		{Code: "KES", Label: "Kenyan shilling", Exponent: 2},
	} {
		if _, err := masters.SaveCurrency(ctx, currency, 0); err != nil {
			t.Fatal(err)
		}
	}
	for _, company := range []ledger.Company{
		{Name: "acme", Label: "Acme", BaseCurrency: "USD"},
		{Name: "nairobi", Label: "Nairobi", BaseCurrency: "KES"},
	} {
		if _, err := masters.SaveCompany(ctx, company, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := masters.SaveFiscalYear(ctx, ledger.FiscalYear{Name: "fy2026-" + company.Name, Label: "FY 2026", Company: company.Name, StartDate: "2026-01-01", EndDate: "2026-12-31"}, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := masters.SavePeriod(ctx, ledger.Period{Company: company.Name, Book: "main", Name: "fy2026", Label: "FY 2026", FiscalYear: "fy2026-" + company.Name, StartDate: "2026-01-01", EndDate: "2026-12-31"}, 0); err != nil {
			t.Fatal(err)
		}
	}
	for _, accountType := range []ledger.AccountType{
		{Name: "asset", Label: "Asset", NormalBalance: "debit"},
		{Name: "liability", Label: "Liability", NormalBalance: "credit"},
		{Name: "income", Label: "Income", NormalBalance: "credit"},
		{Name: "expense", Label: "Expense", NormalBalance: "debit"},
	} {
		if _, err := masters.SaveAccountType(ctx, accountType); err != nil {
			t.Fatal(err)
		}
	}
	for _, account := range []ledger.Account{
		{Name: "cash", Label: "Cash", Type: "asset"},
		{Name: "receivable", Label: "Receivable", Type: "asset", ControlRole: "receivable"},
		{Name: "payable", Label: "Payable", Type: "liability", ControlRole: "payable"},
		{Name: "revenue", Label: "Revenue", Type: "income"},
		{Name: "cost", Label: "Cost", Type: "expense"},
		{Name: "fx_gain", Label: "FX gain", Type: "income"},
		{Name: "fx_loss", Label: "FX loss", Type: "expense"},
	} {
		if _, err := masters.SaveAccount(ctx, account, 0); err != nil {
			t.Fatal(err)
		}
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
		PostingGroupID: group, PaymentCurrency: "USD",
		Actor: "tester", Source: "test", Lines: lines,
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
		"payment off item": func(req *posting.SettleRequest) { req.PaymentCurrency = "EUR" },
		"plain account":    func(req *posting.SettleRequest) { req.ControlAccount = "cash" },
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

// TestSettlementAllocationProperty checks Property 11 at the settlement:
// for any payment and any instalment set, the applied shares plus the
// unapplied remainder sum exactly to the payment, and the per-instalment
// shares match the id-ordered model whatever order the caller lists the
// items in, with every share within its residual and every residual stored.
//
// Feature: athena-batch-2, Property 11: Allocation sums.
//
// One shared database serves all iterations; fresh uuids keep source and
// settlement keys unique. Amounts stay small so proportional remainders and
// tie-breaks occur often, which is exactly where listing order would leak.
func TestSettlementAllocationProperty(t *testing.T) {
	ctx := context.Background()
	svc, db := settleSetup(t)
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 4).Draw(t, "items")
		minors := make([]int64, n)
		for i := range minors {
			minors[i] = int64(rapid.IntRange(1, 2000).Draw(t, "residual"))
		}
		side := posting.ItemSideDebit
		control := "receivable"
		if rapid.Bool().Draw(t, "payable") {
			side = posting.ItemSideCredit
			control = "payable"
		}
		sourceID := uuid.NewString()
		partyID := uuid.NewString()
		items := make([]posting.NewItem, n)
		for i := range items {
			amount, err := types.NewDecimal(minors[i], 2)
			if err != nil {
				t.Fatalf("residual: %v", err)
			}
			items[i] = posting.NewItem{
				Company: "acme", Book: "main", PartyType: "customer", PartyID: partyID,
				ControlAccount: control, SourceType: "invoice", SourceID: sourceID,
				SourceRevision: 2, InstalmentNo: i + 1, Currency: "USD", Side: side, Amount: amount,
			}
		}
		tx, err := db.Begin()
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		created, err := svc.CreateItemsInTx(ctx, tx, items)
		if err != nil {
			tx.Rollback()
			t.Fatalf("create: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
		payment, err := types.NewDecimal(int64(rapid.IntRange(1, 8000).Draw(t, "payment")), 2)
		if err != nil {
			t.Fatalf("payment: %v", err)
		}
		// A random listing order: shuffle by drawn keys.
		keys := make([]int, n)
		for i := range keys {
			keys[i] = rapid.IntRange(0, 1000000).Draw(t, "order")
		}
		order := make([]int, n)
		for i := range order {
			order[i] = i
		}
		sort.Slice(order, func(a, b int) bool { return keys[order[a]] < keys[order[b]] })
		targets := make([]posting.LineItem, n)
		for i, at := range order {
			targets[i] = posting.LineItem{ItemID: created[at].ID}
		}
		req := posting.SettleRequest{
			Company: "acme", Book: "main", PartyType: "customer", PartyID: partyID,
			ControlAccount: control, Side: side,
			VoucherType: "receipt", VoucherID: uuid.NewString(),
			Revision: 2, Event: posting.EventSubmit, PostingDate: "2026-06-20",
			PostingGroupID: uuid.NewString(), PaymentCurrency: "USD",
			Actor: "tester", Source: "test",
			Lines: []posting.SettleLine{{LineNo: 1, Amount: payment, Items: targets}},
		}
		result, err := svc.Apply(ctx, req)
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		// The model: residuals in stable id order through Allocate, each
		// share capped at its residual.
		byID := append([]posting.Item(nil), created...)
		sort.Slice(byID, func(a, b int) bool { return byID[a].ID < byID[b].ID })
		weights := make([]int64, n)
		for i := range byID {
			weights[i] = minors[indexOf(created, byID[i].ID)] * 10000
		}
		parts, err := payment.Allocate(weights, 6)
		if err != nil {
			t.Fatalf("model: %v", err)
		}
		want := map[string]types.Decimal{}
		for i := range byID {
			part := parts[i]
			if part.Cmp(byID[i].Residual) > 0 {
				part = byID[i].Residual
			}
			if part.Sign() > 0 {
				want[byID[i].ID] = part
			}
		}
		got := map[string]types.Decimal{}
		for _, application := range result.Applications {
			got[application.ItemID] = application.PaymentAmount
		}
		if len(got) != len(want) {
			t.Fatalf("got %d applications, model has %d", len(got), len(want))
		}
		for id, amount := range want {
			actual, ok := got[id]
			if !ok || !actual.Equal(amount) {
				t.Fatalf("item %s got %v, model has %v", id, actual, amount)
			}
		}
		total := types.Decimal{}
		for _, application := range result.Applications {
			total = total.Add(application.PaymentAmount)
			var residual string
			if err := db.QueryRow(`SELECT residual_amount FROM athena_party_items WHERE id = ?`, application.ItemID).Scan(&residual); err != nil {
				t.Fatalf("read residual: %v", err)
			}
			before := types.Decimal{}
			for _, item := range created {
				if item.ID == application.ItemID {
					before = item.Residual
				}
			}
			stored, err := types.ParseDecimal(residual)
			if err != nil {
				t.Fatalf("parse residual: %v", err)
			}
			if !stored.Equal(before.Sub(application.PaymentAmount)) {
				t.Fatalf("stored residual %v, want %v", stored, before.Sub(application.PaymentAmount))
			}
			var debit, credit string
			if err := db.QueryRow(`SELECT debit_base, credit_base FROM l_party WHERE application_id = ?`, application.ID).Scan(&debit, &credit); err != nil {
				t.Fatalf("read party row: %v", err)
			}
			zero := debit == "0.000000" || debit == "0"
			if side == posting.ItemSideDebit && !zero {
				t.Fatalf("receivable application debits %s", debit)
			}
			if side == posting.ItemSideCredit && zero {
				t.Fatalf("payable application misses its debit, got %s/%s", debit, credit)
			}
		}
		for _, unapplied := range result.Unapplied {
			total = total.Add(unapplied.Amount)
		}
		if !total.Equal(payment) {
			t.Fatalf("applied plus unapplied is %v, want %v", total, payment)
		}
	})
}

func indexOf(items []posting.Item, id string) int {
	for i := range items {
		if items[i].ID == id {
			return i
		}
	}
	return -1
}

func seedRate(t *testing.T, db *sql.DB, from, to, rate string, at time.Time) {
	t.Helper()
	masters := &ledger.Service{DB: db, Dialect: store.MariaDB{}}
	if _, err := masters.SaveRate(context.Background(), ledger.ExchangeRate{
		From: from, To: to, RateType: "spot", Source: "test",
		EffectiveAt: at, Rate: types.MustDecimal(rate),
	}); err != nil {
		t.Fatal(err)
	}
}

// backdateItem moves an item's booking instant back in time, simulating an
// invoice recorded earlier and settled later.
func backdateItem(t *testing.T, db *sql.DB, id, created string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE athena_party_items SET created_at = ? WHERE id = ?`, created, id); err != nil {
		t.Fatal(err)
	}
}

func setPeriodStatus(t *testing.T, db *sql.DB, company, status string) {
	t.Helper()
	ctx := context.Background()
	masters := &ledger.Service{DB: db, Dialect: store.MariaDB{}}
	period, err := masters.GetPeriod(ctx, company, "main", "fy2026")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := masters.SetPeriodStatus(ctx, company, "main", "fy2026", status, "tester", period.Revision); err != nil {
		t.Fatal(err)
	}
}

func fxRequest(voucher, group string, lines []posting.SettleLine) posting.SettleRequest {
	return posting.SettleRequest{
		Company: "nairobi", Book: "main", PartyType: "customer", PartyID: settleParty,
		ControlAccount: "receivable", Side: posting.ItemSideDebit,
		VoucherType: "receipt", VoucherID: voucher,
		Revision: 2, Event: posting.EventSubmit, PostingDate: "2026-07-15",
		PostingGroupID: group, PaymentCurrency: "USD",
		Actor: "tester", Source: "test",
		FX:    &posting.FXConfig{RateType: "spot", GainAccount: "fx_gain", LossAccount: "fx_loss"},
		Lines: lines,
	}
}

func fxItem(amount string, instalment int) posting.NewItem {
	item := settleItem(amount, instalment)
	item.Company = "nairobi"
	item.Currency = "USD"
	return item
}

func fxLegs(t *testing.T, db *sql.DB, group string) map[string][2]string {
	t.Helper()
	rows, err := db.Query(`SELECT account_name, debit_base, credit_base FROM l_gl WHERE posting_group_id = ? ORDER BY id`, group)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	legs := map[string][2]string{}
	for rows.Next() {
		var account, debit, credit string
		if err := rows.Scan(&account, &debit, &credit); err != nil {
			t.Fatal(err)
		}
		legs[account] = [2]string{debit, credit}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return legs
}

func TestApplyPostsRealisedGain(t *testing.T) {
	svc, db := settleSetup(t)
	seedRate(t, db, "USD", "KES", "130", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedRate(t, db, "USD", "KES", "132", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	created := createItems(t, svc, db, []posting.NewItem{fxItem("100.00", 1)})
	backdateItem(t, db, created[0].ID, "2026-06-15 12:00:00")
	result, err := svc.Apply(context.Background(), fxRequest(
		"aaaaaaaa-0000-0000-0000-000000000001", "aaaaaaaa-0000-0000-0000-000000000002",
		[]posting.SettleLine{{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}}},
	))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applications[0].BaseAmount.Equal(types.MustDecimal("13200")) {
		t.Fatalf("settled base is %v, want 13200", result.Applications[0].BaseAmount)
	}
	if result.FXGroupID == "" {
		t.Fatal("gain posts an FX group")
	}
	legs := fxLegs(t, db, result.FXGroupID)
	if len(legs) != 2 || legs["receivable"] != [2]string{"200.000000", "0.000000"} || legs["fx_gain"] != [2]string{"0.000000", "200.000000"} {
		t.Fatalf("FX legs are %v, want Dr receivable 200 / Cr fx_gain 200", legs)
	}
	// The post itself proves the control leg carried its item source: the
	// engine refuses sourceless control legs, and l_gl stores no sources.
}

func TestApplyPostsRealisedLoss(t *testing.T) {
	svc, db := settleSetup(t)
	seedRate(t, db, "USD", "KES", "132", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedRate(t, db, "USD", "KES", "130", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	created := createItems(t, svc, db, []posting.NewItem{fxItem("100.00", 1)})
	backdateItem(t, db, created[0].ID, "2026-06-15 12:00:00")
	result, err := svc.Apply(context.Background(), fxRequest(
		"aaaaaaaa-0000-0000-0000-000000000003", "aaaaaaaa-0000-0000-0000-000000000004",
		[]posting.SettleLine{{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}}},
	))
	if err != nil {
		t.Fatal(err)
	}
	legs := fxLegs(t, db, result.FXGroupID)
	if legs["fx_loss"] != [2]string{"200.000000", "0.000000"} || legs["receivable"] != [2]string{"0.000000", "200.000000"} {
		t.Fatalf("FX legs are %v, want Dr fx_loss 200 / Cr receivable 200", legs)
	}
}

func TestApplySkipsZeroFX(t *testing.T) {
	svc, db := settleSetup(t)
	seedRate(t, db, "USD", "KES", "130", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	created := createItems(t, svc, db, []posting.NewItem{fxItem("100.00", 1)})
	backdateItem(t, db, created[0].ID, "2026-06-15 12:00:00")
	result, err := svc.Apply(context.Background(), fxRequest(
		"aaaaaaaa-0000-0000-0000-000000000005", "aaaaaaaa-0000-0000-0000-000000000006",
		[]posting.SettleLine{{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}}},
	))
	if err != nil {
		t.Fatal(err)
	}
	if result.FXGroupID != "" {
		t.Fatal("zero gain posts no FX group")
	}
	if !result.Applications[0].BaseAmount.Equal(types.MustDecimal("13000")) {
		t.Fatalf("settled base is %v, want 13000", result.Applications[0].BaseAmount)
	}
}

func TestApplyRefusesForeignWithoutFX(t *testing.T) {
	svc, db := settleSetup(t)
	created := createItems(t, svc, db, []posting.NewItem{fxItem("100.00", 1)})
	req := fxRequest(
		"aaaaaaaa-0000-0000-0000-000000000007", "aaaaaaaa-0000-0000-0000-000000000008",
		[]posting.SettleLine{{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}}},
	)
	req.FX = nil
	if _, err := svc.Apply(context.Background(), req); !errors.Is(err, posting.ErrInvalid) {
		t.Fatalf("foreign items without FX config refuse, got %v", err)
	}
}

func TestApplyRespectsCloses(t *testing.T) {
	authority := &posting.CloseAuthority{Allowed: true, Reason: "year-end adjustment"}
	softFX := func() posting.SettleRequest {
		req := fxRequest(
			"aaaaaaaa-0000-0000-0000-000000000009", "aaaaaaaa-0000-0000-0000-000000000010",
			[]posting.SettleLine{{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: "ffffffff-ffff-ffff-ffff-ffffffffffff"}}}},
		)
		req.Authority = authority
		return req
	}
	t.Run("soft closed without authority refuses", func(t *testing.T) {
		svc, db := settleSetup(t)
		setPeriodStatus(t, db, "acme", ledger.PeriodSoftClosed)
		created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1)})
		req := settleRequest("bbbbbbbb-0000-0000-0000-000000000001", []posting.SettleLine{
			{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}},
		})
		err := applyErr(svc, req)
		if !errors.Is(err, posting.ErrInvalid) || !strings.Contains(err.Error(), "ledger:close") {
			t.Fatalf("soft close names the grant and reason, got %v", err)
		}
	})
	t.Run("soft closed with authority applies base-only", func(t *testing.T) {
		svc, db := settleSetup(t)
		setPeriodStatus(t, db, "acme", ledger.PeriodSoftClosed)
		created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1)})
		req := settleRequest("bbbbbbbb-0000-0000-0000-000000000002", []posting.SettleLine{
			{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}},
		})
		req.Authority = authority
		if _, err := svc.Apply(context.Background(), req); err != nil {
			t.Fatalf("adjustment with authority applies: %v", err)
		}
	})
	t.Run("soft closed refuses FX", func(t *testing.T) {
		svc, db := settleSetup(t)
		setPeriodStatus(t, db, "nairobi", ledger.PeriodSoftClosed)
		seedRate(t, db, "USD", "KES", "130", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		seedRate(t, db, "USD", "KES", "132", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
		created := createItems(t, svc, db, []posting.NewItem{fxItem("100.00", 1)})
		backdateItem(t, db, created[0].ID, "2026-06-15 12:00:00")
		req := softFX()
		req.Lines[0].Items[0].ItemID = created[0].ID
		err := applyErr(svc, req)
		if !errors.Is(err, posting.ErrInvalid) || !strings.Contains(err.Error(), "open period") {
			t.Fatalf("FX into soft close refuses for an open period, got %v", err)
		}
		var applications int
		if err := db.QueryRow(`SELECT COUNT(*) FROM athena_settlement_applications`).Scan(&applications); err != nil {
			t.Fatal(err)
		}
		if applications != 0 {
			t.Fatal("refused FX writes nothing")
		}
	})
	t.Run("hard closed refuses everything", func(t *testing.T) {
		svc, db := settleSetup(t)
		setPeriodStatus(t, db, "acme", ledger.PeriodHardClosed)
		created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1)})
		req := settleRequest("bbbbbbbb-0000-0000-0000-000000000003", []posting.SettleLine{
			{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}},
		})
		req.Authority = authority
		err := applyErr(svc, req)
		if !errors.Is(err, posting.ErrInvalid) || !strings.Contains(err.Error(), "hard-closed") {
			t.Fatalf("hard close refuses everything, got %v", err)
		}
	})
	t.Run("unknown book refuses", func(t *testing.T) {
		svc, _ := settleSetup(t)
		req := settleRequest("bbbbbbbb-0000-0000-0000-000000000004", []posting.SettleLine{
			{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: settleParty}}},
		})
		req.Book = "side"
		err := applyErr(svc, req)
		if !errors.Is(err, posting.ErrInvalid) || !strings.Contains(err.Error(), "not defined") {
			t.Fatalf("unknown book refuses, got %v", err)
		}
	})
	t.Run("uncovered date refuses", func(t *testing.T) {
		svc, _ := settleSetup(t)
		req := settleRequest("bbbbbbbb-0000-0000-0000-000000000005", []posting.SettleLine{
			{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: settleParty}}},
		})
		req.PostingDate = "2027-01-01"
		err := applyErr(svc, req)
		if !errors.Is(err, posting.ErrInvalid) || !strings.Contains(err.Error(), "no period covers") {
			t.Fatalf("uncovered date refuses, got %v", err)
		}
	})
}

func applyErr(svc *posting.SettlementService, req posting.SettleRequest) error {
	_, err := svc.Apply(context.Background(), req)
	return err
}

func reverseRequest(voucher string, revision int64) posting.ReverseRequest {
	return posting.ReverseRequest{
		Company: "acme", Book: "main", PartyType: "customer", PartyID: settleParty,
		ControlAccount: "receivable", Side: posting.ItemSideDebit,
		VoucherType: "receipt", VoucherID: voucher, Revision: revision,
		PostingDate: "2026-07-20", Actor: "tester", Source: "test",
	}
}

func TestReverseMirrorsSettlement(t *testing.T) {
	svc, db := settleSetup(t)
	created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1)})
	applyReq := settleRequest("cccccccc-0000-0000-0000-000000000002", []posting.SettleLine{
		{LineNo: 1, Amount: types.MustDecimal("130.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}},
	})
	applyReq.VoucherID = "cccccccc-0000-0000-0000-000000000001"
	applied, err := svc.Apply(context.Background(), applyReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.Unapplied) != 1 {
		t.Fatal("fixture carries unapplied cash")
	}
	result, err := svc.Reverse(context.Background(), reverseRequest("cccccccc-0000-0000-0000-000000000001", 3))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applications) != 1 || len(result.Items) != 1 {
		t.Fatalf("one mirror restores one item: %+v", result)
	}
	if !result.Items[0].Residual.Equal(types.MustDecimal("100.00")) || result.Items[0].Status != posting.ItemOpen {
		t.Fatalf("residual restores open: %+v", result.Items[0])
	}
	var isReversal bool
	var reverses string
	if err := db.QueryRow(`SELECT is_reversal, reverses_application_id FROM athena_settlement_applications WHERE id = ?`,
		result.Applications[0].ID).Scan(&isReversal, &reverses); err != nil {
		t.Fatal(err)
	}
	if !isReversal || reverses != applied.Applications[0].ID {
		t.Fatal("mirror flags its reversal link")
	}
	var debit, credit string
	var rowReversed bool
	if err := db.QueryRow(`SELECT debit_base, credit_base, is_reversal FROM l_party WHERE application_id = ?`,
		result.Applications[0].ID).Scan(&debit, &credit, &rowReversed); err != nil {
		t.Fatal(err)
	}
	if !rowReversed || debit != "100.000000" || credit != "0.000000" {
		t.Fatalf("mirror swaps to debit 100, got %s/%s", debit, credit)
	}
	var cashMirrors int
	if err := db.QueryRow(`SELECT COUNT(*) FROM l_party WHERE item_id IS NULL AND is_reversal = TRUE`).Scan(&cashMirrors); err != nil {
		t.Fatal(err)
	}
	if cashMirrors != 1 {
		t.Fatalf("unapplied cash mirrors back, got %d rows", cashMirrors)
	}
	if len(result.FXCancelGroups) != 0 {
		t.Fatal("base settlement unwinds no FX")
	}
	if _, err := svc.Reverse(context.Background(), reverseRequest("cccccccc-0000-0000-0000-000000000001", 4)); !errors.Is(err, posting.ErrAlreadyReversed) {
		t.Fatalf("second cancel reports ErrAlreadyReversed, got %v", err)
	}
	if _, err := svc.Reverse(context.Background(), reverseRequest("cccccccc-0000-0000-0000-000000009999", 3)); !errors.Is(err, posting.ErrInvalid) {
		t.Fatalf("unknown settlement reports ErrInvalid, got %v", err)
	}
}

func TestReverseUnwindsFX(t *testing.T) {
	svc, db := settleSetup(t)
	seedRate(t, db, "USD", "KES", "130", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedRate(t, db, "USD", "KES", "132", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	created := createItems(t, svc, db, []posting.NewItem{fxItem("100.00", 1)})
	backdateItem(t, db, created[0].ID, "2026-06-15 12:00:00")
	voucher := "dddddddd-0000-0000-0000-000000000001"
	if _, err := svc.Apply(context.Background(), fxRequest(
		voucher, "dddddddd-0000-0000-0000-000000000002",
		[]posting.SettleLine{{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}}},
	)); err != nil {
		t.Fatal(err)
	}
	req := reverseRequest(voucher, 3)
	req.Company = "nairobi"
	result, err := svc.Reverse(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.FXCancelGroups) != 1 {
		t.Fatalf("one FX post unwinds once: %+v", result)
	}
	rows, err := db.Query(`SELECT account_name, debit_base, credit_base FROM l_gl WHERE voucher_type = 'fx_settlement' AND voucher_id = ?`, voucher)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	net := map[string]string{}
	count := 0
	for rows.Next() {
		var account, debit, credit string
		if err := rows.Scan(&account, &debit, &credit); err != nil {
			t.Fatal(err)
		}
		count++
		balance := types.Decimal{}
		if net[account] != "" {
			var err error
			balance, err = types.ParseDecimal(net[account])
			if err != nil {
				t.Fatal(err)
			}
		}
		debitAmount, err := types.ParseDecimal(debit)
		if err != nil {
			t.Fatal(err)
		}
		creditAmount, err := types.ParseDecimal(credit)
		if err != nil {
			t.Fatal(err)
		}
		net[account] = balance.Add(debitAmount).Sub(creditAmount).String()
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("FX submit plus cancel is 4 rows, got %d", count)
	}
	for account, balance := range net {
		parsed, err := types.ParseDecimal(balance)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Sign() != 0 {
			t.Fatalf("account %s nets %s, want zero", account, balance)
		}
	}
}

func TestReverseRefusesClosedDate(t *testing.T) {
	svc, db := settleSetup(t)
	created := createItems(t, svc, db, []posting.NewItem{settleItem("100.00", 1)})
	voucher := "eeeeeeee-0000-0000-0000-000000000001"
	applyReq := settleRequest("eeeeeeee-0000-0000-0000-000000000002", []posting.SettleLine{
		{LineNo: 1, Amount: types.MustDecimal("100.00"), Items: []posting.LineItem{{ItemID: created[0].ID}}},
	})
	applyReq.VoucherID = voucher
	if _, err := svc.Apply(context.Background(), applyReq); err != nil {
		t.Fatal(err)
	}
	setPeriodStatus(t, db, "acme", ledger.PeriodSoftClosed)
	req := reverseRequest(voucher, 3)
	if _, err := svc.Reverse(context.Background(), req); !errors.Is(err, posting.ErrInvalid) {
		t.Fatalf("reversal into a soft date refuses, got %v", err)
	}
}

func TestReverseAcrossRevisions(t *testing.T) {
	svc, db := settleSetup(t)
	seedRate(t, db, "USD", "KES", "130", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedRate(t, db, "USD", "KES", "132", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	created := createItems(t, svc, db, []posting.NewItem{fxItem("100.00", 1)})
	backdateItem(t, db, created[0].ID, "2026-06-15 12:00:00")
	voucher := "ffffffff-0000-0000-0000-000000000001"
	for revision, amount := range map[int64]string{2: "40.00", 3: "30.00"} {
		req := fxRequest(voucher, uuid.NewString(), []posting.SettleLine{
			{LineNo: 1, Amount: types.MustDecimal(amount), Items: []posting.LineItem{{ItemID: created[0].ID}}},
		})
		req.Revision = revision
		if _, err := svc.Apply(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	req := reverseRequest(voucher, 4)
	req.Company = "nairobi"
	result, err := svc.Reverse(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applications) != 2 || len(result.FXCancelGroups) != 2 {
		t.Fatalf("both revisions unwind with both FX posts: %+v", result)
	}
	if !result.Items[0].Residual.Equal(types.MustDecimal("100.00")) {
		t.Fatalf("residual restores fully: %+v", result.Items[0])
	}
}

// equationFixture is one Property 12 iteration: a throwaway company with an
// open full-year period, so iterations never share a control balance.
type equationFixture struct {
	company string
	control string
	offset  string
	side    string
	base    string
}

func equationCompany(t *rapid.T, masters *ledger.Service, base string) equationFixture {
	tag := uuid.NewString()[:8]
	company := "c12" + tag
	ctx := context.Background()
	if _, err := masters.SaveCompany(ctx, ledger.Company{Name: company, Label: "Property 12", BaseCurrency: base}, 0); err != nil {
		t.Fatalf("company: %v", err)
	}
	if _, err := masters.SaveFiscalYear(ctx, ledger.FiscalYear{Name: "fy" + tag, Label: "FY", Company: company, StartDate: "2026-01-01", EndDate: "2026-12-31"}, 0); err != nil {
		t.Fatalf("year: %v", err)
	}
	if _, err := masters.SavePeriod(ctx, ledger.Period{Company: company, Book: "main", Name: "fy2026", Label: "FY 2026", FiscalYear: "fy" + tag, StartDate: "2026-01-01", EndDate: "2026-12-31"}, 0); err != nil {
		t.Fatalf("period: %v", err)
	}
	return equationFixture{company: company, base: base}
}

func controlBalance(t *rapid.T, db *sql.DB, company, control string) types.Decimal {
	var debit, credit string
	if err := db.QueryRow(`SELECT COALESCE(SUM(debit_base), 0), COALESCE(SUM(credit_base), 0) FROM l_gl WHERE company_name = ? AND account_name = ?`,
		company, control).Scan(&debit, &credit); err != nil {
		t.Fatalf("control balance: %v", err)
	}
	debitAmount, err := types.ParseDecimal(debit)
	if err != nil {
		t.Fatalf("parse debit: %v", err)
	}
	creditAmount, err := types.ParseDecimal(credit)
	if err != nil {
		t.Fatalf("parse credit: %v", err)
	}
	return debitAmount.Sub(creditAmount)
}

// assertBaseEquation checks R7.8 in one currency: signed open residuals
// equal the control balance. Settled rows carry zero and change nothing.
func assertBaseEquation(t *rapid.T, db *sql.DB, company, control string) {
	rows, err := db.Query(`SELECT side, residual_amount FROM athena_party_items WHERE company_name = ?`, company)
	if err != nil {
		t.Fatalf("residuals: %v", err)
	}
	defer rows.Close()
	total := types.Decimal{}
	for rows.Next() {
		var side, residual string
		if err := rows.Scan(&side, &residual); err != nil {
			t.Fatalf("scan residual: %v", err)
		}
		amount, err := types.ParseDecimal(residual)
		if err != nil {
			t.Fatalf("parse residual: %v", err)
		}
		if side == posting.ItemSideCredit {
			amount = amount.Neg()
		}
		total = total.Add(amount)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("residual rows: %v", err)
	}
	if balance := controlBalance(t, db, company, control); !balance.Equal(total) {
		t.Fatalf("control %v, signed residuals %v", balance, total)
	}
}

// assertFXEquation checks R7.8 across currencies: each residual valued at
// its invoice rate sums to the control balance. Rates are integral in these
// sequences so every conversion is exact; fractional-rate rounding dust has
// no booked-base column to pin it to and stays out of the equation.
func assertFXEquation(t *rapid.T, db *sql.DB, company, control string, invoiceRates map[string]types.Decimal) {
	rows, err := db.Query(`SELECT id, side, residual_amount FROM athena_party_items WHERE company_name = ?`, company)
	if err != nil {
		t.Fatalf("residuals: %v", err)
	}
	defer rows.Close()
	total := types.Decimal{}
	for rows.Next() {
		var id, side, residual string
		if err := rows.Scan(&id, &side, &residual); err != nil {
			t.Fatalf("scan residual: %v", err)
		}
		amount, err := types.ParseDecimal(residual)
		if err != nil {
			t.Fatalf("parse residual: %v", err)
		}
		rate, ok := invoiceRates[id]
		if !ok {
			t.Fatalf("item %s has no tracked invoice rate", id)
		}
		valued := amount.Mul(rate)
		if side == posting.ItemSideCredit {
			valued = valued.Neg()
		}
		total = total.Add(valued)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("residual rows: %v", err)
	}
	if balance := controlBalance(t, db, company, control); !balance.Equal(total) {
		t.Fatalf("control %v, booked residuals %v", balance, total)
	}
}

// equationInvoice posts one invoice's control legs through the engine and
// records its items: one leg pair per instalment, each control leg sourced
// to its item, in the given base amounts (equal to the instalments for base
// invoices, converted for foreign ones). It returns the created items.
func equationInvoice(t *rapid.T, svc *posting.SettlementService, db *sql.DB, engine *posting.Engine, fix equationFixture, partyID, currency string, minors []int64, onCreated func([]posting.Item), baseOf func(minor int64, itemID string) types.Decimal) []posting.Item {
	voucherID := uuid.NewString()
	items := make([]posting.NewItem, len(minors))
	for j, minor := range minors {
		amount, err := types.NewDecimal(minor, 2)
		if err != nil {
			t.Fatalf("instalment: %v", err)
		}
		items[j] = posting.NewItem{
			Company: fix.company, Book: "main", PartyType: "customer", PartyID: partyID,
			ControlAccount: fix.control, SourceType: "invoice", SourceID: voucherID,
			SourceRevision: 2, InstalmentNo: j + 1, Currency: currency, Side: fix.side, Amount: amount,
		}
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	created, err := svc.CreateItemsInTx(context.Background(), tx, items)
	if err != nil {
		tx.Rollback()
		t.Fatalf("create items: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit items: %v", err)
	}
	if onCreated != nil {
		onCreated(created)
	}
	var lines []posting.Line
	for j := range created {
		valued := baseOf(minors[j], created[j].ID)
		if fix.side == posting.ItemSideDebit {
			lines = append(lines,
				posting.Line{Company: fix.company, Account: fix.control, DebitBase: valued, SourceType: "party_item", SourceID: created[j].ID},
				posting.Line{Company: fix.company, Account: fix.offset, CreditBase: valued},
			)
		} else {
			lines = append(lines,
				posting.Line{Company: fix.company, Account: fix.offset, DebitBase: valued},
				posting.Line{Company: fix.company, Account: fix.control, CreditBase: valued, SourceType: "party_item", SourceID: created[j].ID},
			)
		}
	}
	_, err = engine.Post(context.Background(), posting.Plan{
		VoucherType: "invoice", VoucherID: voucherID, Revision: 2,
		Event: posting.EventSubmit, Book: "main", PostingDate: "2026-06-10", Lines: lines,
	}, posting.PostOptions{Actor: "tester", Source: "test"})
	if err != nil {
		t.Fatalf("post invoice: %v", err)
	}
	return created
}

// equationSettle applies one line over a random nonempty subset of the open
// items and posts the payment legs for the applied base only, returning the
// settlement voucher id when legs posted (empty otherwise) and the ids still
// open afterwards.
func equationSettle(t *rapid.T, svc *posting.SettlementService, db *sql.DB, engine *posting.Engine, fix equationFixture, partyID, currency, date string, open []string, fx *posting.FXConfig) (string, []string) {
	included := map[string]bool{}
	for _, id := range open {
		if rapid.Bool().Draw(t, "include") {
			included[id] = true
		}
	}
	if len(included) == 0 {
		included[open[0]] = true
	}
	targets := make([]posting.LineItem, 0, len(included))
	for _, id := range open {
		if included[id] {
			targets = append(targets, posting.LineItem{ItemID: id})
		}
	}
	payment, err := types.NewDecimal(int64(rapid.IntRange(1, 4000).Draw(t, "payment")), 2)
	if err != nil {
		t.Fatalf("payment: %v", err)
	}
	voucherID := uuid.NewString()
	result, err := svc.Apply(context.Background(), posting.SettleRequest{
		Company: fix.company, Book: "main", PartyType: "customer", PartyID: partyID,
		ControlAccount: fix.control, Side: fix.side,
		VoucherType: "receipt", VoucherID: voucherID,
		Revision: 2, Event: posting.EventSubmit, PostingDate: date,
		PostingGroupID: uuid.NewString(), PaymentCurrency: currency,
		Actor: "tester", Source: "test", FX: fx,
		Lines: []posting.SettleLine{{LineNo: 1, Amount: payment, Items: targets}},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	applied := types.Decimal{}
	for _, application := range result.Applications {
		applied = applied.Add(application.BaseAmount)
	}
	stillOpen := open[:0]
	for _, id := range open {
		settled := false
		for _, item := range result.Items {
			if item.ID == id && item.Status == posting.ItemSettled {
				settled = true
			}
		}
		if !settled {
			stillOpen = append(stillOpen, id)
		}
	}
	if applied.Sign() == 0 {
		return "", stillOpen
	}
	var legs []posting.Line
	for _, application := range result.Applications {
		if fix.side == posting.ItemSideDebit {
			legs = append(legs,
				posting.Line{Company: fix.company, Account: "cash", DebitBase: application.BaseAmount},
				posting.Line{Company: fix.company, Account: fix.control, CreditBase: application.BaseAmount, SourceType: "party_item", SourceID: application.ItemID},
			)
		} else {
			legs = append(legs,
				posting.Line{Company: fix.company, Account: fix.control, DebitBase: application.BaseAmount, SourceType: "party_item", SourceID: application.ItemID},
				posting.Line{Company: fix.company, Account: "cash", CreditBase: application.BaseAmount},
			)
		}
	}
	// Payment legs move the applied base only (R7.7 contract): unapplied
	// cash stays out of the control account until a later application moves
	// it, which is exactly what keeps the equation exact.
	if _, err := engine.Post(context.Background(), posting.Plan{
		VoucherType: "receipt", VoucherID: voucherID, Revision: 2,
		Event: posting.EventSubmit, Book: "main", PostingDate: date, Lines: legs,
	}, posting.PostOptions{Actor: "tester", Source: "test"}); err != nil {
		t.Fatalf("post payment: %v", err)
	}
	return voucherID, stillOpen
}

// equationReverse unwinds one settlement and mirrors its payment legs.
func equationReverse(t *rapid.T, svc *posting.SettlementService, engine *posting.Engine, fix equationFixture, partyID, voucherID, date string) {
	if _, err := svc.Reverse(context.Background(), posting.ReverseRequest{
		Company: fix.company, Book: "main", PartyType: "customer", PartyID: partyID,
		ControlAccount: fix.control, Side: fix.side,
		VoucherType: "receipt", VoucherID: voucherID, Revision: 3,
		PostingDate: date, Actor: "tester", Source: "test",
	}); err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if _, err := engine.Reverse(context.Background(), "receipt", voucherID, 2, "main",
		posting.ReverseOptions{Actor: "tester", Source: "test", PostingDate: date}); err != nil {
		t.Fatalf("mirror payment legs: %v", err)
	}
}

// TestPartyLedgerEqualsControl checks Property 12 in one currency: for any
// sequence of invoices, settlements and reversals, the signed party
// residuals equal the control balance after every completed operation.
//
// Feature: athena-batch-2, Property 12: Party ledger equals control.
func TestPartyLedgerEqualsControl(t *testing.T) {
	svc, db := settleSetup(t)
	engine := &posting.Engine{DB: db, Dialect: store.MariaDB{}}
	rapid.Check(t, func(t *rapid.T) {
		fix := equationCompany(t, &ledger.Service{DB: db, Dialect: store.MariaDB{}}, "USD")
		fix.control, fix.offset, fix.side = "receivable", "revenue", posting.ItemSideDebit
		if rapid.Bool().Draw(t, "payable") {
			fix.control, fix.offset, fix.side = "payable", "cost", posting.ItemSideCredit
		}
		partyID := uuid.NewString()
		var open []string
		for i, n := 0, rapid.IntRange(1, 2).Draw(t, "invoices"); i < n; i++ {
			total := int64(rapid.IntRange(100, 2000).Draw(t, "invoice total"))
			parts := []int64{total}
			if rapid.Bool().Draw(t, "split") {
				cut := int64(rapid.IntRange(1, int(total-1)).Draw(t, "cut"))
				parts = []int64{cut, total - cut}
			}
			created := equationInvoice(t, svc, db, engine, fix, partyID, "USD", parts, nil, func(minor int64, _ string) types.Decimal {
				amount, err := types.NewDecimal(minor, 2)
				if err != nil {
					t.Fatalf("instalment base: %v", err)
				}
				return amount
			})
			for _, item := range created {
				open = append(open, item.ID)
			}
			assertBaseEquation(t, db, fix.company, fix.control)
		}
		var settled []string
		for i, n := 0, rapid.IntRange(0, 2).Draw(t, "settlements"); i < n && len(open) > 0; i++ {
			voucherID, rest := equationSettle(t, svc, db, engine, fix, partyID, "USD", "2026-07-15", open, nil)
			open = rest
			if voucherID != "" {
				settled = append(settled, voucherID)
			}
			assertBaseEquation(t, db, fix.company, fix.control)
		}
		if len(settled) > 0 && rapid.Bool().Draw(t, "reverse") {
			equationReverse(t, svc, engine, fix, partyID, settled[len(settled)-1], "2026-08-20")
			assertBaseEquation(t, db, fix.company, fix.control)
		}
	})
}

// TestPartyLedgerEqualsControlFX checks Property 12 across currencies: with
// foreign items, each residual valued at its invoice rate sums to the
// control balance after every completed operation. Rates move between
// booking and settlement, so realised gains and losses post along the way.
//
// Feature: athena-batch-2, Property 12: Party ledger equals control.
func TestPartyLedgerEqualsControlFX(t *testing.T) {
	svc, db := settleSetup(t)
	masters := &ledger.Service{DB: db, Dialect: store.MariaDB{}}
	engine := &posting.Engine{DB: db, Dialect: store.MariaDB{}}
	ctx := context.Background()
	for _, curve := range []struct {
		rate string
		at   time.Time
	}{
		{"120", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"130", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
		{"125", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
	} {
		if _, err := masters.SaveRate(ctx, ledger.ExchangeRate{
			From: "USD", To: "KES", RateType: "spot", Source: "test",
			EffectiveAt: curve.at, Rate: types.MustDecimal(curve.rate),
		}); err != nil {
			t.Fatal(err)
		}
	}
	bookedAt := []string{"2026-03-01 12:00:00", "2026-07-15 12:00:00"}
	settleDates := []string{"2026-07-15", "2026-10-15"}
	fx := &posting.FXConfig{RateType: "spot", GainAccount: "fx_gain", LossAccount: "fx_loss"}
	rapid.Check(t, func(t *rapid.T) {
		fix := equationCompany(t, masters, "KES")
		fix.control, fix.offset, fix.side = "receivable", "revenue", posting.ItemSideDebit
		if rapid.Bool().Draw(t, "payable") {
			fix.control, fix.offset, fix.side = "payable", "cost", posting.ItemSideCredit
		}
		invoiceRates := map[string]types.Decimal{}
		partyID := uuid.NewString()
		var open []string
		for i, n := 0, rapid.IntRange(1, 2).Draw(t, "invoices"); i < n; i++ {
			total := int64(rapid.IntRange(100, 2000).Draw(t, "invoice total"))
			parts := []int64{total}
			if rapid.Bool().Draw(t, "split") {
				cut := int64(rapid.IntRange(1, int(total-1)).Draw(t, "cut"))
				parts = []int64{cut, total - cut}
			}
			created := equationInvoice(t, svc, db, engine, fix, partyID, "USD", parts,
				func(items []posting.Item) {
					for _, item := range items {
						stamp := bookedAt[rapid.IntRange(0, len(bookedAt)-1).Draw(t, "booked at")]
						if _, err := db.Exec(`UPDATE athena_party_items SET created_at = ? WHERE id = ?`, stamp, item.ID); err != nil {
							t.Fatalf("backdate: %v", err)
						}
						at, err := time.ParseInLocation("2006-01-02 15:04:05", stamp, time.UTC)
						if err != nil {
							t.Fatalf("parse instant: %v", err)
						}
						rate, err := masters.GetRate(ctx, "USD", "KES", "spot", "", at)
						if err != nil {
							t.Fatalf("invoice rate: %v", err)
						}
						invoiceRates[item.ID] = rate
					}
				},
				func(minor int64, itemID string) types.Decimal {
					amount, err := types.NewDecimal(minor, 2)
					if err != nil {
						t.Fatalf("instalment base: %v", err)
					}
					booked, err := amount.Mul(invoiceRates[itemID]).Round(6, types.HalfUp)
					if err != nil {
						t.Fatalf("book base: %v", err)
					}
					return booked
				})
			for _, item := range created {
				open = append(open, item.ID)
			}
			assertFXEquation(t, db, fix.company, fix.control, invoiceRates)
		}
		var settled []string
		for i, n := 0, rapid.IntRange(0, 2).Draw(t, "settlements"); i < n && len(open) > 0; i++ {
			date := settleDates[rapid.IntRange(0, len(settleDates)-1).Draw(t, "settle date")]
			voucherID, rest := equationSettle(t, svc, db, engine, fix, partyID, "USD", date, open, fx)
			open = rest
			if voucherID != "" {
				settled = append(settled, voucherID)
			}
			assertFXEquation(t, db, fix.company, fix.control, invoiceRates)
		}
		if len(settled) > 0 && rapid.Bool().Draw(t, "reverse") {
			equationReverse(t, svc, engine, fix, partyID, settled[len(settled)-1], "2026-11-20")
			assertFXEquation(t, db, fix.company, fix.control, invoiceRates)
		}
	})
}
