// Package posting_test holds the database-backed posting engine tests. It is
// an external test package so it can migrate with server.SystemMigrations
// without an import cycle.
package posting_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

func setup(t *testing.T) (*posting.Engine, *ledger.Service, *sql.DB) {
	t.Helper()
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, store.MariaDB{}, server.SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &posting.Engine{DB: db, Dialect: store.MariaDB{}},
		&ledger.Service{DB: db, Dialect: store.MariaDB{}}, db
}

// seedBasics creates company acme on USD with an open 2026 period, asset and
// income types, leaf accounts cash, bank, revenue and cost, a group, a
// disabled leaf, a control account, and a project dimension mandatory for
// revenue.
func seedBasics(t *testing.T, masters *ledger.Service) {
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
	} {
		if _, err := masters.SaveAccountType(ctx, accountType); err != nil {
			t.Fatal(err)
		}
	}
	for _, account := range []ledger.Account{
		{Name: "assets", Label: "Assets", Type: "asset", IsGroup: true},
		{Name: "cash", Label: "Cash", Type: "asset", Parent: "assets"},
		{Name: "bank", Label: "Bank", Type: "asset", Parent: "assets"},
		{Name: "revenue", Label: "Revenue", Type: "income"},
		{Name: "cost", Label: "Cost", Type: "asset"},
		{Name: "old", Label: "Old", Type: "asset", Disabled: true},
		{Name: "receivable", Label: "Receivable", Type: "asset", ControlRole: "receivable"},
	} {
		if _, err := masters.SaveAccount(ctx, account, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := masters.SaveDimension(ctx, ledger.Dimension{Name: "project", Label: "Project", Members: []string{"apollo", "zeus"}, MandatoryFor: []string{"revenue"}}, 0); err != nil {
		t.Fatal(err)
	}
}

func dec(value string) types.Decimal { return types.MustDecimal(value) }

func qty(value string) *types.Decimal {
	amount := types.MustDecimal(value)
	return &amount
}

// basePlan is a valid two-line plan; tests mutate a copy per case.
func basePlan() posting.Plan {
	return posting.Plan{
		VoucherType: "journal",
		VoucherID:   uuid.NewString(),
		Revision:    1,
		Event:       posting.EventSubmit,
		Book:        "main",
		PostingDate: "2026-06-15",
		Lines: []posting.Line{
			{Company: "acme", Account: "cash", DebitBase: dec("100.00")},
			{Company: "acme", Account: "revenue", CreditBase: dec("100.00"), Dimensions: map[string]string{"project": "apollo"}},
		},
	}
}

func opts() posting.PostOptions {
	return posting.PostOptions{Actor: "tester", Source: "api", RequestID: "req-1"}
}

func count(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var value string
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatalf("count %s: %v", query, err)
	}
	return value
}

func TestPostHappyPath(t *testing.T) {
	ctx := context.Background()
	engine, masters, db := setup(t)
	seedBasics(t, masters)

	plan := basePlan()
	plan.Lines = append(plan.Lines,
		posting.Line{Company: "acme", Account: "bank", DebitBase: dec("25.50"), TxnCurrency: "USD", TxnDebit: dec("25.50"), Quantity: qty("2"), QuantityUOM: "pcs"},
		posting.Line{Company: "acme", Account: "cost", CreditBase: dec("25.50"), TxnCurrency: "USD", TxnCredit: dec("25.50")},
	)
	result, err := engine.Post(ctx, plan, opts())
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if result.PostingGroupID == "" || len(result.RowIDs) != 4 || result.PlanHash == "" {
		t.Fatalf("result: %+v", result)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM l_gl`); got != "4" {
		t.Fatalf("ledger rows = %s, want 4", got)
	}
	var company, book, account, period, debit, credit, currency, txnDebit, txnCredit, quantity, uom, dimensions, group string
	var date time.Time
	var isReversal bool
	err = db.QueryRow(`SELECT company_name, book, account_name, period_name, posting_date, debit_base, credit_base, txn_currency, txn_debit, txn_credit, quantity, quantity_uom, dimensions, posting_group_id, is_reversal FROM l_gl WHERE account_name = 'bank'`).
		Scan(&company, &book, &account, &period, &date, &debit, &credit, &currency, &txnDebit, &txnCredit, &quantity, &uom, &dimensions, &group, &isReversal)
	if err != nil {
		t.Fatalf("read bank row: %v", err)
	}
	if company != "acme" || book != "main" || period != "fy2026" || date.Format("2006-01-02") != "2026-06-15" || debit != "25.500000" || credit != "0.000000" ||
		currency != "USD" || txnDebit != "25.500000" || txnCredit != "0.000000" || quantity != "2.000000" || uom != "pcs" ||
		dimensions != "{}" || group != result.PostingGroupID || isReversal {
		t.Fatalf("bank row: %q %q %q %q %q %q %q %q %q %q %q %q %q %q %v",
			company, book, account, period, date, debit, credit, currency, txnDebit, txnCredit, quantity, uom, dimensions, group, isReversal)
	}
	if got := count(t, db, `SELECT dimensions FROM l_gl WHERE account_name = 'revenue'`); got != `{"project":"apollo"}` {
		t.Fatalf("revenue dimensions = %s", got)
	}
	balances := map[string][2]string{}
	rows, err := db.Query(`SELECT account_name, debit_total, credit_total FROM athena_balance_account`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, debitTotal, creditTotal string
		if err := rows.Scan(&name, &debitTotal, &creditTotal); err != nil {
			t.Fatal(err)
		}
		balances[name] = [2]string{debitTotal, creditTotal}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{
		"cash":    {"100.000000", "0.000000"},
		"revenue": {"0.000000", "100.000000"},
		"bank":    {"25.500000", "0.000000"},
		"cost":    {"0.000000", "25.500000"},
	}
	if fmt.Sprint(balances) != fmt.Sprint(want) {
		t.Fatalf("balances = %v, want %v", balances, want)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM athena_audit_log WHERE record_type_name = 'journal' AND action = 'posted'`); got != "1" {
		t.Fatalf("audit rows = %s, want 1", got)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM athena_outbox WHERE event_type = 'posting.posted'`); got != "1" {
		t.Fatalf("outbox rows = %s, want 1", got)
	}

	// A second post accumulates the running totals.
	second := basePlan()
	if _, err := engine.Post(ctx, second, opts()); err != nil {
		t.Fatalf("second post: %v", err)
	}
	if got := count(t, db, `SELECT debit_total FROM athena_balance_account WHERE account_name = 'cash'`); got != "200.000000" {
		t.Fatalf("cash debit total = %s, want 200.000000", got)
	}
}

func TestPostValidation(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		plan  func() posting.Plan
		opts  func() posting.PostOptions
		setup func(*testing.T, *ledger.Service)
		want  error
	}{
		{"no voucher type", func() posting.Plan { p := basePlan(); p.VoucherType = ""; return p }, opts, nil, posting.ErrInvalid},
		{"no voucher id", func() posting.Plan { p := basePlan(); p.VoucherID = ""; return p }, opts, nil, posting.ErrInvalid},
		{"revision zero", func() posting.Plan { p := basePlan(); p.Revision = 0; return p }, opts, nil, posting.ErrInvalid},
		{"no event", func() posting.Plan { p := basePlan(); p.Event = ""; return p }, opts, nil, posting.ErrInvalid},
		{"no book needs the book policy", func() posting.Plan { p := basePlan(); p.Book = ""; return p }, opts, nil, posting.ErrBookPolicyOpen},
		{"bad date", func() posting.Plan { p := basePlan(); p.PostingDate = "2026-13-40"; return p }, opts, nil, posting.ErrInvalid},
		{"no lines", func() posting.Plan { p := basePlan(); p.Lines = nil; return p }, opts, nil, posting.ErrInvalid},
		{"unknown company", func() posting.Plan { p := basePlan(); p.Lines[0].Company = "ghost"; return p }, opts, nil, posting.ErrInvalid},
		{"disabled company", func() posting.Plan { return basePlan() }, opts, func(t *testing.T, masters *ledger.Service) {
			t.Helper()
			company, err := masters.GetCompany(ctx, "acme")
			if err != nil {
				t.Fatal(err)
			}
			company.Disabled = true
			if _, err := masters.SaveCompany(ctx, company, company.Revision); err != nil {
				t.Fatal(err)
			}
		}, posting.ErrInvalid},
		{"company without base currency", func() posting.Plan {
			p := basePlan()
			p.Lines[0].Company, p.Lines[1].Company = "bare", "bare"
			return p
		}, opts, func(t *testing.T, masters *ledger.Service) {
			t.Helper()
			if _, err := masters.SaveCompany(ctx, ledger.Company{Name: "bare", Label: "Bare"}, 0); err != nil {
				t.Fatal(err)
			}
			if _, err := masters.SavePeriod(ctx, ledger.Period{Company: "bare", Book: "main", Name: "p", Label: "P", StartDate: "2026-01-01", EndDate: "2026-12-31"}, 0); err != nil {
				t.Fatal(err)
			}
		}, posting.ErrInvalid},
		{"unknown account", func() posting.Plan { p := basePlan(); p.Lines[0].Account = "ghost"; return p }, opts, nil, posting.ErrInvalid},
		{"group account", func() posting.Plan { p := basePlan(); p.Lines[0].Account = "assets"; return p }, opts, nil, posting.ErrInvalid},
		{"disabled account", func() posting.Plan { p := basePlan(); p.Lines[0].Account = "old"; return p }, opts, nil, posting.ErrInvalid},
		{"other company account", func() posting.Plan { p := basePlan(); p.Lines[0].Account = "petty"; return p }, opts, func(t *testing.T, masters *ledger.Service) {
			t.Helper()
			if _, err := masters.SaveCompany(ctx, ledger.Company{Name: "other", Label: "Other", BaseCurrency: "USD"}, 0); err != nil {
				t.Fatal(err)
			}
			if _, err := masters.SaveAccount(ctx, ledger.Account{Name: "petty", Label: "Petty", Type: "asset", Company: "other"}, 0); err != nil {
				t.Fatal(err)
			}
		}, posting.ErrInvalid},
		{"unbalanced base", func() posting.Plan { p := basePlan(); p.Lines[1].CreditBase = dec("99.99"); return p }, opts, nil, posting.ErrInvalid},
		{"zero line", func() posting.Plan { p := basePlan(); p.Lines[0].DebitBase = dec("0"); return p }, opts, nil, posting.ErrInvalid},
		{"both sides", func() posting.Plan { p := basePlan(); p.Lines[0].CreditBase = dec("1"); return p }, opts, nil, posting.ErrInvalid},
		{"negative amount", func() posting.Plan { p := basePlan(); p.Lines[0].DebitBase = dec("-1"); return p }, opts, nil, posting.ErrInvalid},
		{"seven fraction digits", func() posting.Plan { p := basePlan(); p.Lines[0].DebitBase = dec("1.0000001"); return p }, opts, nil, posting.ErrInvalid},
		{"txn without currency", func() posting.Plan { p := basePlan(); p.Lines[0].TxnDebit = dec("1"); return p }, opts, nil, posting.ErrInvalid},
		{"unknown txn currency", func() posting.Plan {
			p := basePlan()
			p.Lines[0].TxnCurrency, p.Lines[0].TxnDebit = "XXX", dec("100.00")
			p.Lines[1].TxnCurrency, p.Lines[1].TxnCredit = "XXX", dec("100.00")
			return p
		}, opts, nil, posting.ErrInvalid},
		{"txn wrong side", func() posting.Plan {
			p := basePlan()
			p.Lines[0].TxnCurrency, p.Lines[0].TxnCredit = "USD", dec("100.00")
			p.Lines[1].TxnCurrency, p.Lines[1].TxnDebit = "USD", dec("100.00")
			return p
		}, opts, nil, posting.ErrInvalid},
		{"unbalanced txn", func() posting.Plan {
			p := basePlan()
			p.Lines[0].TxnCurrency, p.Lines[0].TxnDebit = "USD", dec("100.00")
			p.Lines[1].TxnCurrency, p.Lines[1].TxnCredit = "USD", dec("99.00")
			return p
		}, opts, nil, posting.ErrInvalid},
		{"quantity without unit", func() posting.Plan { p := basePlan(); p.Lines[0].Quantity = qty("2"); return p }, opts, nil, posting.ErrInvalid},
		{"unit without quantity", func() posting.Plan { p := basePlan(); p.Lines[0].QuantityUOM = "pcs"; return p }, opts, nil, posting.ErrInvalid},
		{"negative quantity", func() posting.Plan {
			p := basePlan()
			p.Lines[0].Quantity, p.Lines[0].QuantityUOM = qty("-2"), "pcs"
			return p
		}, opts, nil, posting.ErrInvalid},
		{"unknown dimension", func() posting.Plan {
			p := basePlan()
			p.Lines[0].Dimensions = map[string]string{"ghost": "x"}
			return p
		}, opts, nil, posting.ErrInvalid},
		{"non member value", func() posting.Plan {
			p := basePlan()
			p.Lines[1].Dimensions = map[string]string{"project": "hades"}
			return p
		}, opts, nil, posting.ErrInvalid},
		{"missing mandatory dimension", func() posting.Plan { p := basePlan(); p.Lines[1].Dimensions = nil; return p }, opts, nil, posting.ErrInvalid},
		{"control account without source", func() posting.Plan {
			p := basePlan()
			p.Lines[0].Account = "receivable"
			return p
		}, opts, nil, posting.ErrInvalid},
		{"no covering period", func() posting.Plan { p := basePlan(); p.PostingDate = "2027-01-01"; return p }, opts, nil, posting.ErrInvalid},
		{"soft closed period", func() posting.Plan { return basePlan() }, opts, func(t *testing.T, masters *ledger.Service) {
			t.Helper()
			if _, err := masters.SetPeriodStatus(ctx, "acme", "main", "fy2026", ledger.PeriodSoftClosed, "tester", 1); err != nil {
				t.Fatal(err)
			}
		}, posting.ErrInvalid},
		{"hard closed period needs the correction window", func() posting.Plan { return basePlan() }, opts, func(t *testing.T, masters *ledger.Service) {
			t.Helper()
			if _, err := masters.SetPeriodStatus(ctx, "acme", "main", "fy2026", ledger.PeriodSoftClosed, "tester", 1); err != nil {
				t.Fatal(err)
			}
			if _, err := masters.SetPeriodStatus(ctx, "acme", "main", "fy2026", ledger.PeriodHardClosed, "tester", 2); err != nil {
				t.Fatal(err)
			}
		}, posting.ErrCorrectionWindowOpen},
		{"no actor", func() posting.Plan { return basePlan() }, func() posting.PostOptions { o := opts(); o.Actor = ""; return o }, nil, posting.ErrInvalid},
		{"no source", func() posting.Plan { return basePlan() }, func() posting.PostOptions { o := opts(); o.Source = ""; return o }, nil, posting.ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, masters, db := setup(t)
			seedBasics(t, masters)
			if tc.setup != nil {
				tc.setup(t, masters)
			}
			_, err := engine.Post(ctx, tc.plan(), tc.opts())
			if !errors.Is(err, tc.want) {
				t.Fatalf("post: %v, want %v", err, tc.want)
			}
			for _, table := range []string{"l_gl", "athena_balance_account", "athena_audit_log", "athena_outbox"} {
				// The audit log carries no rows for a failed post; other
				// writers (period close notifications) do not touch these.
				if table == "athena_audit_log" {
					if got := count(t, db, `SELECT COUNT(*) FROM athena_audit_log WHERE record_type_name = 'journal'`); got != "0" {
						t.Fatalf("audit rows = %s, want 0", got)
					}
					continue
				}
				if got := count(t, db, "SELECT COUNT(*) FROM "+table); got != "0" {
					t.Fatalf("%s rows = %s, want 0", table, got)
				}
			}
		})
	}
}

func TestControlAccountWithSourcePosts(t *testing.T) {
	ctx := context.Background()
	engine, masters, db := setup(t)
	seedBasics(t, masters)
	plan := basePlan()
	plan.Lines[0].Account = "receivable"
	plan.Lines[0].SourceType, plan.Lines[0].SourceID = "party_item", "item-1"
	if _, err := engine.Post(ctx, plan, opts()); err != nil {
		t.Fatalf("post: %v", err)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM l_gl`); got != "2" {
		t.Fatalf("ledger rows = %s, want 2", got)
	}
}

// TestBalancedPostingsProperty checks Property 8 over drawn plans: for any
// valid Posting_Plan the ledger rows written balance per company and book in
// base currency, and any unbalanced plan writes nothing.
//
// Feature: athena-batch-2, Property 8: Balanced postings.
//
// One shared database serves all iterations; fresh uuids keep voucher ids
// unique. Free lines are drawn, then a plug line (or pair) balances them, so
// every drawn plan is valid by construction and the property checks what the
// writer stored.
func TestBalancedPostingsProperty(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if _, err := store.Up(ctx, db, store.MariaDB{}, server.SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	engine := &posting.Engine{DB: db, Dialect: store.MariaDB{}}
	masters := &ledger.Service{DB: db, Dialect: store.MariaDB{}}
	seedBasics(t, masters)
	accounts := []string{"cash", "bank", "cost"}
	sums := func(voucherID string) (rows int, debit, credit string) {
		err := db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(debit_base), 0), COALESCE(SUM(credit_base), 0) FROM l_gl WHERE voucher_type = 'journal' AND voucher_id = ?`, voucherID).
			Scan(&rows, &debit, &credit)
		if err != nil {
			t.Fatalf("read rows: %v", err)
		}
		return rows, debit, credit
	}
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 5).Draw(t, "freeLines")
		useTxn := rapid.Bool().Draw(t, "txn")
		lines := make([]posting.Line, 0, n+2)
		net := types.MustDecimal("0")
		for i := 0; i < n; i++ {
			minor := rapid.IntRange(1, 100000).Draw(t, "minor")
			amount, err := types.NewDecimal(int64(minor), 2)
			if err != nil {
				t.Fatalf("decimal: %v", err)
			}
			account := accounts[rapid.IntRange(0, len(accounts)-1).Draw(t, "account")]
			line := posting.Line{Company: "acme", Account: account}
			if rapid.Bool().Draw(t, "debit") {
				line.DebitBase = amount
				net = net.Add(amount)
			} else {
				line.CreditBase = amount
				net = net.Sub(amount)
			}
			if useTxn {
				line.TxnCurrency = "USD"
				line.TxnDebit, line.TxnCredit = line.DebitBase, line.CreditBase
			}
			lines = append(lines, line)
		}
		plugAccount := accounts[rapid.IntRange(0, len(accounts)-1).Draw(t, "plugAccount")]
		switch net.Sign() {
		case 0:
			extra, err := types.NewDecimal(int64(rapid.IntRange(1, 100000).Draw(t, "extraMinor")), 2)
			if err != nil {
				t.Fatalf("decimal: %v", err)
			}
			debit, credit := posting.Line{Company: "acme", Account: plugAccount, DebitBase: extra}, posting.Line{Company: "acme", Account: plugAccount, CreditBase: extra}
			if useTxn {
				debit.TxnCurrency, debit.TxnDebit = "USD", extra
				credit.TxnCurrency, credit.TxnCredit = "USD", extra
			}
			lines = append(lines, debit, credit)
		case 1:
			plug := posting.Line{Company: "acme", Account: plugAccount, CreditBase: net}
			if useTxn {
				plug.TxnCurrency, plug.TxnCredit = "USD", net
			}
			lines = append(lines, plug)
		default:
			plug := posting.Line{Company: "acme", Account: plugAccount, DebitBase: net.Neg()}
			if useTxn {
				plug.TxnCurrency, plug.TxnDebit = "USD", net.Neg()
			}
			lines = append(lines, plug)
		}
		plan := posting.Plan{
			VoucherType: "journal", VoucherID: uuid.NewString(), Revision: 1,
			Event: posting.EventSubmit, Book: "main", PostingDate: "2026-06-15", Lines: lines,
		}
		if _, err := engine.Post(ctx, plan, opts()); err != nil {
			t.Fatalf("post: %v", err)
		}
		rows, debit, credit := sums(plan.VoucherID)
		if rows != len(lines) {
			t.Fatalf("rows = %d, want %d", rows, len(lines))
		}
		debitTotal, err := types.ParseDecimal(debit)
		if err != nil {
			t.Fatalf("parse debit: %v", err)
		}
		creditTotal, err := types.ParseDecimal(credit)
		if err != nil {
			t.Fatalf("parse credit: %v", err)
		}
		if !debitTotal.Equal(creditTotal) {
			t.Fatalf("stored totals debit %s credit %s", debit, credit)
		}

		// The same plan with one debit bumped a cent is unbalanced and must
		// write nothing.
		bad := posting.Plan{
			VoucherType: "journal", VoucherID: uuid.NewString(), Revision: 1,
			Event: posting.EventSubmit, Book: "main", PostingDate: "2026-06-15",
			Lines: append([]posting.Line(nil), lines...),
		}
		bumped := false
		for i := range bad.Lines {
			if bad.Lines[i].DebitBase.Sign() > 0 {
				bad.Lines[i].DebitBase = bad.Lines[i].DebitBase.Add(types.MustDecimal("0.01"))
				bumped = true
				break
			}
		}
		if !bumped {
			t.Fatal("drawn plan has no debit line")
		}
		if _, err := engine.Post(ctx, bad, opts()); !errors.Is(err, posting.ErrInvalid) {
			t.Fatalf("unbalanced post: %v, want %v", err, posting.ErrInvalid)
		}
		if rows, _, _ := sums(bad.VoucherID); rows != 0 {
			t.Fatalf("unbalanced plan wrote %d rows", rows)
		}
	})
}

func TestPostInTxJoinsCallerTransaction(t *testing.T) {
	ctx := context.Background()
	engine, masters, db := setup(t)
	seedBasics(t, masters)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.PostInTx(ctx, tx, basePlan(), opts()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM l_gl`); got != "0" {
		t.Fatalf("ledger rows after rollback = %s, want 0", got)
	}

	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.PostInTx(ctx, tx, basePlan(), opts()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM l_gl`); got != "2" {
		t.Fatalf("ledger rows after commit = %s, want 2", got)
	}
}
