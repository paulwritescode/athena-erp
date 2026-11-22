// Package posting_test holds the database-backed posting engine tests. It is
// an external test package so it can migrate with server.SystemMigrations
// without an import cycle.
package posting_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
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

// drawBalancedLines draws free lines over accounts, then a plug line (or a
// pair when the free lines already net to zero), so the result always
// balances in base currency. Properties 8 and 9 share it.
func drawBalancedLines(t *rapid.T, accounts []string) []posting.Line {
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
	return lines
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
		lines := drawBalancedLines(t, accounts)
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

func opOpts(operationID string) posting.PostOptions {
	options := opts()
	options.OperationID = operationID
	return options
}

func actorOpts(actor, operationID string) posting.PostOptions {
	options := opts()
	options.Actor = actor
	options.OperationID = operationID
	return options
}

func equalResults(a, b posting.Result) bool {
	return a.PostingGroupID == b.PostingGroupID && slices.Equal(a.RowIDs, b.RowIDs) &&
		a.PlanHash == b.PlanHash && a.OperationID == b.OperationID && a.Replayed == b.Replayed
}

func logCount(t *testing.T, db *sql.DB, operationID, outcome, code string) string {
	t.Helper()
	return count(t, db, `SELECT COUNT(*) FROM athena_posting_log WHERE operation_id = ? AND outcome = ? AND code = ?`, operationID, outcome, code)
}

func TestOperationReplay(t *testing.T) {
	ctx := context.Background()
	engine, masters, db := setup(t)
	seedBasics(t, masters)
	plan := basePlan()
	operationID := uuid.NewString()

	first, err := engine.Post(ctx, plan, opOpts(operationID))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if first.Replayed || first.OperationID != operationID || first.PlanHash == "" {
		t.Fatalf("first result: %+v", first)
	}
	second, err := engine.Post(ctx, plan, opOpts(operationID))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	want := first
	want.Replayed = true
	if !equalResults(second, want) {
		t.Fatalf("replay = %+v, want %+v", second, want)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM l_gl`); got != "2" {
		t.Fatalf("ledger rows = %s, want 2", got)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM athena_operations`); got != "1" {
		t.Fatalf("operation rows = %s, want 1", got)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM athena_posting_groups`); got != "1" {
		t.Fatalf("group rows = %s, want 1", got)
	}
	if got := logCount(t, db, operationID, posting.OutcomePosted, posting.CodeOK); got != "1" {
		t.Fatalf("posted log rows = %s, want 1", got)
	}
	if got := logCount(t, db, operationID, posting.OutcomeReplayed, posting.CodeOK); got != "1" {
		t.Fatalf("replayed log rows = %s, want 1", got)
	}
}

func TestOperationConflict(t *testing.T) {
	ctx := context.Background()
	engine, masters, db := setup(t)
	seedBasics(t, masters)
	plan := basePlan()
	operationID := uuid.NewString()
	if _, err := engine.Post(ctx, plan, opOpts(operationID)); err != nil {
		t.Fatalf("post: %v", err)
	}
	changed := plan
	changed.Lines = append([]posting.Line(nil), plan.Lines...)
	changed.Lines[0].DebitBase = dec("101.00")
	changed.Lines[1].CreditBase = dec("101.00")
	if _, err := engine.Post(ctx, changed, opOpts(operationID)); !errors.Is(err, posting.ErrConflict) {
		t.Fatalf("same voucher changed payload: %v, want %v", err, posting.ErrConflict)
	}
	other := basePlan()
	if _, err := engine.Post(ctx, other, opOpts(operationID)); !errors.Is(err, posting.ErrConflict) {
		t.Fatalf("other voucher same id: %v, want %v", err, posting.ErrConflict)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM l_gl`); got != "2" {
		t.Fatalf("ledger rows = %s, want 2", got)
	}
	if got := logCount(t, db, operationID, posting.OutcomeConflict, posting.CodeConflict); got != "2" {
		t.Fatalf("conflict log rows = %s, want 2", got)
	}
}

func TestDifferentOperationSameVoucher(t *testing.T) {
	ctx := context.Background()
	engine, masters, db := setup(t)
	seedBasics(t, masters)
	plan := basePlan()

	first, err := engine.Post(ctx, plan, opOpts(uuid.NewString()))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	otherID := uuid.NewString()
	second, err := engine.Post(ctx, plan, opOpts(otherID))
	if err != nil {
		t.Fatalf("second operation: %v", err)
	}
	// Post-once returns the first result verbatim, including its operation
	// id, with only the replay flag set.
	want := first
	want.Replayed = true
	if !equalResults(second, want) {
		t.Fatalf("second = %+v, want %+v", second, want)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM l_gl`); got != "2" {
		t.Fatalf("ledger rows = %s, want 2", got)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM athena_posting_groups`); got != "1" {
		t.Fatalf("group rows = %s, want 1", got)
	}
	if got := logCount(t, db, otherID, posting.OutcomeReplayed, posting.CodeDuplicateGroup); got != "1" {
		t.Fatalf("duplicate-group log rows = %s, want 1", got)
	}
}

// TestGroupCollisionAcrossActorsFindsWinner is the regression test for
// review B1: operation ids are namespaced per actor, so several rows can
// share one id string, and a group collision must replay the row whose
// stored result points at the collided group — not whichever row a bare
// LIMIT 1 scan returns first. Carol posts first under the shared id, so a
// scan-ordered lookup would hand her row to bob; bob must still get alice's
// result for alice's voucher exactly.
func TestGroupCollisionAcrossActorsFindsWinner(t *testing.T) {
	ctx := context.Background()
	engine, masters, db := setup(t)
	seedBasics(t, masters)

	carolPlan := basePlan()
	carol, err := engine.Post(ctx, carolPlan, actorOpts("carol", "op-1"))
	if err != nil {
		t.Fatalf("carol: %v", err)
	}
	alicePlan := basePlan()
	alice, err := engine.Post(ctx, alicePlan, actorOpts("alice", "op-1"))
	if err != nil {
		t.Fatalf("alice: %v", err)
	}
	bob, err := engine.Post(ctx, alicePlan, actorOpts("bob", "op-1"))
	if err != nil {
		t.Fatalf("bob: %v", err)
	}
	want := alice
	want.Replayed = true
	if !equalResults(bob, want) {
		t.Fatalf("bob = %+v, want %+v", bob, want)
	}
	if bob.PostingGroupID == carol.PostingGroupID {
		t.Fatalf("bob got carol's group %q for alice's voucher", bob.PostingGroupID)
	}
	var groupID string
	err = db.QueryRow(`SELECT id FROM athena_posting_groups WHERE voucher_type = 'journal' AND voucher_id = ?`, alicePlan.VoucherID).Scan(&groupID)
	if err != nil {
		t.Fatal(err)
	}
	if groupID != alice.PostingGroupID {
		t.Fatalf("group row = %q, alice's result points at %q", groupID, alice.PostingGroupID)
	}
	if rows := voucherRows(t, db, alicePlan.VoucherID); rows != 2 {
		t.Fatalf("alice rows = %d, want 2", rows)
	}
	if rows := voucherRows(t, db, carolPlan.VoucherID); rows != 2 {
		t.Fatalf("carol rows = %d, want 2", rows)
	}
}

func TestOperationScaleInsensitive(t *testing.T) {
	ctx := context.Background()
	engine, masters, db := setup(t)
	seedBasics(t, masters)
	plan := basePlan()
	plan.Lines[0].DebitBase = dec("100.00")
	plan.Lines[1].CreditBase = dec("100.00")
	operationID := uuid.NewString()
	first, err := engine.Post(ctx, plan, opOpts(operationID))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	rescaled := plan
	rescaled.Lines = append([]posting.Line(nil), plan.Lines...)
	rescaled.Lines[0].DebitBase = dec("100.0")
	rescaled.Lines[1].CreditBase = dec("100.0000")
	second, err := engine.Post(ctx, rescaled, opOpts(operationID))
	if err != nil {
		t.Fatalf("rescaled replay: %v", err)
	}
	if !second.Replayed || second.PlanHash != first.PlanHash {
		t.Fatalf("rescaled = %+v, want a replay of %+v", second, first)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM l_gl`); got != "2" {
		t.Fatalf("ledger rows = %s, want 2", got)
	}
}

func TestGeneratedOperationID(t *testing.T) {
	ctx := context.Background()
	engine, masters, _ := setup(t)
	seedBasics(t, masters)
	result, err := engine.Post(ctx, basePlan(), opts())
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if result.OperationID == "" || result.Replayed {
		t.Fatalf("result: %+v", result)
	}
}

func TestPostingLogOutcomes(t *testing.T) {
	ctx := context.Background()
	engine, masters, db := setup(t)
	seedBasics(t, masters)

	posted := uuid.NewString()
	if _, err := engine.Post(ctx, basePlan(), opOpts(posted)); err != nil {
		t.Fatal(err)
	}
	replayed := uuid.NewString()
	replayPlan := basePlan()
	if _, err := engine.Post(ctx, replayPlan, opOpts(replayed)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Post(ctx, replayPlan, opOpts(replayed)); err != nil {
		t.Fatal(err)
	}
	conflicted := uuid.NewString()
	if _, err := engine.Post(ctx, basePlan(), opOpts(conflicted)); err != nil {
		t.Fatal(err)
	}
	changed := basePlan()
	changed.Lines[1].CreditBase = dec("50.00")
	if _, err := engine.Post(ctx, changed, opOpts(conflicted)); !errors.Is(err, posting.ErrConflict) {
		t.Fatal("changed payload did not conflict")
	}
	duplicate := uuid.NewString()
	duplicatePlan := basePlan()
	if _, err := engine.Post(ctx, duplicatePlan, opOpts(uuid.NewString())); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Post(ctx, duplicatePlan, opOpts(duplicate)); err != nil {
		t.Fatal(err)
	}
	invalid := uuid.NewString()
	unbalanced := basePlan()
	unbalanced.Lines[1].CreditBase = dec("50.00")
	if _, err := engine.Post(ctx, unbalanced, opOpts(invalid)); !errors.Is(err, posting.ErrInvalid) {
		t.Fatal("unbalanced plan posted")
	}
	policy := uuid.NewString()
	bookless := basePlan()
	bookless.Book = ""
	if _, err := engine.Post(ctx, bookless, opOpts(policy)); !errors.Is(err, posting.ErrBookPolicyOpen) {
		t.Fatal("bookless plan posted")
	}
	for operationID, want := range map[string][2]string{
		posted:     {posting.OutcomePosted, posting.CodeOK},
		replayed:   {posting.OutcomeReplayed, posting.CodeOK},
		conflicted: {posting.OutcomeConflict, posting.CodeConflict},
		duplicate:  {posting.OutcomeReplayed, posting.CodeDuplicateGroup},
		invalid:    {posting.OutcomeRejected, posting.CodeInvalid},
		policy:     {posting.OutcomeRejected, posting.CodeBookPolicyOpen},
	} {
		if got := logCount(t, db, operationID, want[0], want[1]); got != "1" {
			t.Fatalf("operation %s: (%s, %s) rows = %s, want 1", operationID, want[0], want[1], got)
		}
	}
	if got := count(t, db, `SELECT COUNT(*) FROM athena_posting_log WHERE plan_hash = ''`); got != "0" {
		t.Fatalf("log rows without a plan hash = %s, want 0", got)
	}
}

func TestHardClosedPeriodLogsCorrectionWindow(t *testing.T) {
	ctx := context.Background()
	engine, masters, db := setup(t)
	seedBasics(t, masters)
	if _, err := masters.SetPeriodStatus(ctx, "acme", "main", "fy2026", ledger.PeriodSoftClosed, "tester", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := masters.SetPeriodStatus(ctx, "acme", "main", "fy2026", ledger.PeriodHardClosed, "tester", 2); err != nil {
		t.Fatal(err)
	}
	operationID := uuid.NewString()
	if _, err := engine.Post(ctx, basePlan(), opOpts(operationID)); !errors.Is(err, posting.ErrCorrectionWindowOpen) {
		t.Fatal("hard-closed period posted")
	}
	if got := logCount(t, db, operationID, posting.OutcomeRejected, posting.CodeCorrectionWindowOpen); got != "1" {
		t.Fatalf("correction-window log rows = %s, want 1", got)
	}
}

func TestRevisionPostsAgain(t *testing.T) {
	ctx := context.Background()
	engine, masters, db := setup(t)
	seedBasics(t, masters)
	plan := basePlan()
	operationID := uuid.NewString()
	if _, err := engine.Post(ctx, plan, opOpts(operationID)); err != nil {
		t.Fatalf("revision 1: %v", err)
	}
	amended := plan
	amended.Revision = 2
	if _, err := engine.Post(ctx, amended, opOpts(uuid.NewString())); err != nil {
		t.Fatalf("revision 2: %v", err)
	}
	// The same operation id cannot follow the voucher to its new revision:
	// the payload changed, so it conflicts.
	if _, err := engine.Post(ctx, amended, opOpts(operationID)); !errors.Is(err, posting.ErrConflict) {
		t.Fatalf("reused id on revision 2 did not conflict")
	}
	if got := count(t, db, `SELECT COUNT(*) FROM l_gl`); got != "4" {
		t.Fatalf("ledger rows = %s, want 4", got)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM athena_posting_groups`); got != "2" {
		t.Fatalf("group rows = %s, want 2", got)
	}
}

func TestNilDialectDegradesGracefully(t *testing.T) {
	ctx := context.Background()
	_, masters, db := setup(t)
	seedBasics(t, masters)
	engine := &posting.Engine{DB: db}
	plan := basePlan()
	if _, err := db.Exec(`INSERT INTO athena_posting_groups (id, voucher_type, voucher_id, voucher_revision, event, book, operation_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), plan.VoucherType, plan.VoucherID, plan.Revision, plan.Event, plan.Book, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	// No Dialect means no classification: the collision comes back as a raw
	// database error instead of panicking or replaying.
	_, err := engine.Post(ctx, plan, opts())
	if err == nil || errors.Is(err, posting.ErrConflict) {
		t.Fatalf("post: %v, want a raw collision error", err)
	}
	if got := count(t, db, `SELECT COUNT(*) FROM athena_posting_log WHERE outcome = 'error'`); got != "1" {
		t.Fatalf("error log rows = %s, want 1", got)
	}
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

// fatalf is what the shared database helpers need: both *testing.T and
// *rapid.T supply it.
type fatalf interface {
	Fatalf(format string, args ...any)
}

func voucherRows(t fatalf, db *sql.DB, voucherID string) int {
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM l_gl WHERE voucher_type = 'journal' AND voucher_id = ?`, voucherID).Scan(&rows); err != nil {
		t.Fatalf("read rows: %v", err)
	}
	return rows
}

func balanceSnapshot(t fatalf, db *sql.DB) map[string][2]string {
	rows, err := db.Query(`SELECT account_name, debit_total, credit_total FROM athena_balance_account`)
	if err != nil {
		t.Fatalf("read balances: %v", err)
	}
	defer rows.Close()
	out := map[string][2]string{}
	for rows.Next() {
		var name, debit, credit string
		if err := rows.Scan(&name, &debit, &credit); err != nil {
			t.Fatalf("read balances: %v", err)
		}
		out[name] = [2]string{debit, credit}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read balances: %v", err)
	}
	return out
}

type replayOutcome struct {
	result posting.Result
	err    error
}

// TestIdempotentReplayProperty checks Property 9 over drawn plans with
// concurrent replays: for any command and any number of replays with one
// operation id and payload, the ledgers equal the state after the first
// replay and every replay returns the same result; any other payload under
// the id conflicts; and another operation id for the same voucher revision
// returns the first result without posting again.
//
// Feature: athena-batch-2, Property 9: Idempotent replay.
//
// One shared database serves all iterations. Each iteration also races fresh
// posts of one plan, so the losers must take the collision paths and still
// return the single winner's result.
func TestIdempotentReplayProperty(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if _, err := store.Up(ctx, db, store.MariaDB{}, server.SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	engine := &posting.Engine{DB: db, Dialect: store.MariaDB{}}
	masters := &ledger.Service{DB: db, Dialect: store.MariaDB{}}
	seedBasics(t, masters)
	accounts := []string{"cash", "bank", "cost"}
	rapid.Check(t, func(t *rapid.T) {
		lines := drawBalancedLines(t, accounts)
		plan := posting.Plan{
			VoucherType: "journal", VoucherID: uuid.NewString(), Revision: 1,
			Event: posting.EventSubmit, Book: "main", PostingDate: "2026-06-15", Lines: lines,
		}
		operationID := uuid.NewString()
		first, err := engine.Post(ctx, plan, opOpts(operationID))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		want := first
		want.Replayed = true

		n := rapid.IntRange(2, 6).Draw(t, "replays")
		before := balanceSnapshot(t, db)
		start := make(chan struct{})
		outcomes := make(chan replayOutcome, n)
		for i := 0; i < n; i++ {
			go func() {
				<-start
				result, err := engine.Post(ctx, plan, opOpts(operationID))
				outcomes <- replayOutcome{result, err}
			}()
		}
		close(start)
		for i := 0; i < n; i++ {
			outcome := <-outcomes
			if outcome.err != nil {
				t.Fatalf("replay: %v", outcome.err)
			}
			if !equalResults(outcome.result, want) {
				t.Fatalf("replay = %+v, want %+v", outcome.result, want)
			}
		}
		if after := balanceSnapshot(t, db); fmt.Sprint(after) != fmt.Sprint(before) {
			t.Fatalf("balances moved under replay: %v vs %v", before, after)
		}
		if rows := voucherRows(t, db, plan.VoucherID); rows != len(lines) {
			t.Fatalf("rows = %d, want %d", rows, len(lines))
		}

		bad := plan
		bad.Lines = append([]posting.Line(nil), lines...)
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
		if _, err := engine.Post(ctx, bad, opOpts(operationID)); !errors.Is(err, posting.ErrConflict) {
			t.Fatalf("changed payload: %v, want %v", err, posting.ErrConflict)
		}
		if rows := voucherRows(t, db, plan.VoucherID); rows != len(lines) {
			t.Fatalf("rows after conflict = %d, want %d", rows, len(lines))
		}

		second, err := engine.Post(ctx, plan, opOpts(uuid.NewString()))
		if err != nil {
			t.Fatalf("second operation: %v", err)
		}
		if !equalResults(second, want) {
			t.Fatalf("second operation = %+v, want %+v", second, want)
		}
		if rows := voucherRows(t, db, plan.VoucherID); rows != len(lines) {
			t.Fatalf("rows after second operation = %d, want %d", rows, len(lines))
		}

		// Fresh posts racing each other: exactly one wins the group, and
		// every loser returns the winner's result.
		m := rapid.IntRange(2, 4).Draw(t, "racers")
		race := plan
		race.VoucherID = uuid.NewString()
		raceOp := uuid.NewString()
		raceStart := make(chan struct{})
		raceOutcomes := make(chan replayOutcome, m)
		for i := 0; i < m; i++ {
			go func() {
				<-raceStart
				result, err := engine.Post(ctx, race, opOpts(raceOp))
				raceOutcomes <- replayOutcome{result, err}
			}()
		}
		close(raceStart)
		raced := make([]replayOutcome, m)
		for i := 0; i < m; i++ {
			raced[i] = <-raceOutcomes
			if raced[i].err != nil {
				t.Fatalf("race: %v", raced[i].err)
			}
		}
		var winner posting.Result
		winners := 0
		for _, outcome := range raced {
			if !outcome.result.Replayed {
				winners++
				winner = outcome.result
			}
		}
		if winners != 1 {
			t.Fatalf("racers produced %d winners, want 1", winners)
		}
		wantRace := winner
		wantRace.Replayed = true
		for _, outcome := range raced {
			if outcome.result.Replayed && !equalResults(outcome.result, wantRace) {
				t.Fatalf("loser = %+v, want %+v", outcome.result, wantRace)
			}
		}
		if rows := voucherRows(t, db, race.VoucherID); rows != len(lines) {
			t.Fatalf("race rows = %d, want %d", rows, len(lines))
		}
	})
}

// TestPostEmitsEnvelopeAndOutbox checks a post writes the posting.posted
// envelope and its linked pending outbox row in the posting transaction.
func TestPostEmitsEnvelopeAndOutbox(t *testing.T) {
	ctx := context.Background()
	engine, masters, db := setup(t)
	engine.Workspace = "testws"
	seedBasics(t, masters)
	plan := basePlan()
	if _, err := engine.Post(ctx, plan, opts()); err != nil {
		t.Fatalf("post: %v", err)
	}
	var eventType, producer, subjectType, subjectID, actor, workspace, correlation, status string
	var revision int64
	if err := db.QueryRow(`SELECT e.event_type, e.producer, e.subject_type, e.subject_id,
		e.subject_revision, e.actor, e.workspace, e.correlation_id, o.status
		FROM athena_events e JOIN athena_outbox o ON o.event_id = e.id`).Scan(
		&eventType, &producer, &subjectType, &subjectID, &revision,
		&actor, &workspace, &correlation, &status); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	if eventType != "posting.posted" || producer != "posting" || subjectType != "journal" ||
		subjectID != plan.VoucherID || revision != 1 || actor != "tester" ||
		workspace != "testws" || correlation != "req-1" || status != "pending" {
		t.Fatalf("envelope mismatch: %q %q %q/%q rev %d actor %q workspace %q correlation %q outbox %q",
			eventType, producer, subjectType, subjectID, revision, actor, workspace, correlation, status)
	}
}
