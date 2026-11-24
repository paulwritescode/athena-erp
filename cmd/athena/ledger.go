package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"athena-erp/internal/ledger"
	"athena-erp/internal/store"
	"athena-erp/internal/types"
	"athena-erp/internal/workspace"
)

const ledgerUsage = `usage: athena ledger <noun> <verb> [--workspace slug] [--json]
  company save --name key --label text [--base-currency USD] [--disable] [--expected-revision N]
  company list
  period save --company key [--book main] --name key --label text --start YYYY-MM-DD --end YYYY-MM-DD [--fiscal-year key] [--expected-revision N]
  period list --company key [--book main]
  period close --company key [--book main] --name key --expected-revision N [--hard]
  period reopen --company key [--book main] --name key --expected-revision N
  currency save --code USD --label text [--exponent 2] [--disable] [--expected-revision N]
  currency list
  currency rate-save --from USD --to EUR --type spot --source ecb --at RFC3339 --rate 0.92
  currency rate-get --from USD --to EUR --type spot [--source ecb] --at RFC3339
  account type-save --name key --label text --normal-balance debit|credit
  account type-list
  account save --name key --label text --type key [--parent key] [--group] [--class text] [--control-role text] [--company key] [--disable] [--expected-revision N]
  account list [--company key]
  determination save --label text --account key [--match k=v,...] [--start YYYY-MM-DD] [--end YYYY-MM-DD] [--disable] [--id uuid] [--expected-revision N]
  determination list
  determination resolve --context k=v,... [--as-of YYYY-MM-DD]
  setting save --pack key --type key --label text --kind scalar|list [--list-rule replace|append|intersect] [--expected-revision N]
  setting list
  setting save-value --pack key --type key [--scope dim:value/...] --effective-from RFC3339 --value-json JSON
  setting publish --pack key --type key [--scope dim:value/...] --effective-from RFC3339 --revision N
  setting discard --pack key --type key [--scope dim:value/...] --effective-from RFC3339 --revision N
  setting list-values --pack key --type key [--published-only]
  setting resolve --pack key --type key [--scope k=v,...] --at RFC3339
  setting preview --pack key --type key [--scope dim:value/...] --effective-from RFC3339 --value-json JSON [--at-scope k=v,...]
  tree show --master account|branch|department|cost-centre [--company key] [--root key]
  uom family-save --name key --label text --base-unit key [--disable] [--expected-revision N]
  uom family-list
  uom conversion-save --family key --unit key --factor decimal [--item key] [--scale N] [--disable] [--expected-revision N]
  uom conversion-get --family key --unit key [--item key]
  uom conversion-list --family key
  uom convert --value decimal --from unit --to unit [--item key] [--mode half_up|half_even]`

func ledgerCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, ledgerUsage)
		return nil
	}
	noun, args := args[0], args[1:]
	var verb string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		verb, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("ledger "+noun+" "+verb, flag.ContinueOnError)
	f := &ledgerFlags{slug: flags.String("workspace", "", "workspace slug, when running from the home directory")}
	f.asJSON = flags.Bool("json", false, "print JSON")
	str := func(name, help string) *string { return flags.String(name, "", help) }
	f.name, f.label = str("name", "master key"), str("label", "display label")
	f.company, f.book = str("company", "company key"), str("book", "book (default main)")
	f.fiscalYear = str("fiscal-year", "fiscal year key")
	f.start, f.end = str("start", "start date YYYY-MM-DD"), str("end", "end date YYYY-MM-DD")
	f.code = str("code", "currency code")
	f.exponent = flags.Int("exponent", 2, "currency minor exponent")
	f.from, f.to = str("from", "rate from code"), str("to", "rate to code")
	f.rateType = str("type", "rate type, account type or setting type")
	f.source = str("source", "rate source")
	f.at = str("at", "instant RFC 3339")
	f.rate = str("rate", "decimal rate")
	f.normal = str("normal-balance", "debit or credit")
	f.parent = str("parent", "parent key")
	f.class = str("class", "account class")
	f.role = str("control-role", "control-account role")
	f.match = str("match", "determination pairs k=v,...")
	f.contextPairs = str("context", "resolve context k=v,...")
	f.pack = str("pack", "setting owner pack")
	f.base = str("base-currency", "company base currency")
	f.account = str("account", "account key")
	f.kind = str("kind", "scalar or list")
	f.listRule = str("list-rule", "replace, append or intersect")
	f.scope = str("scope", "scope chain dim:value/...")
	f.scopeMap = str("at-scope", "preview scope k=v,...")
	f.effective = str("effective-from", "instant RFC 3339")
	f.valueJSON = str("value-json", "JSON value")
	f.id = str("id", "determination id (updates)")
	f.revision = flags.Int64("expected-revision", 0, "revision edited, 0 creates")
	f.disable = flags.Bool("disable", false, "mark disabled")
	f.group = flags.Bool("group", false, "a group node")
	f.hard = flags.Bool("hard", false, "hard close (period close)")
	f.publishedOnly = flags.Bool("published-only", false, "published generations only")
	f.master = str("master", "tree master: account, branch, department or cost-centre")
	f.root = str("root", "subtree root key (tree show)")
	f.baseUnit = str("base-unit", "family base unit")
	f.family = str("family", "UoM family")
	f.unit = str("unit", "UoM unit")
	f.factor = str("factor", "base units per one unit, decimal")
	f.item = str("item", "item key for a conversion override")
	f.scale = flags.Int("scale", 6, "conversion fraction digits, 0 to 12")
	f.value = str("value", "quantity value, decimal")
	f.mode = str("mode", "half_up or half_even (default half_up)")
	f.asOf = str("as-of", "date YYYY-MM-DD (default today)")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	svc, ctx, cancel, err := ledgerService(*f.slug)
	if err != nil {
		return err
	}
	defer cancel()
	switch noun {
	case "company":
		return ledgerCompany(ctx, svc, out, verb, f)
	case "period":
		return ledgerPeriod(ctx, svc, out, verb, f)
	case "currency":
		return ledgerCurrency(ctx, svc, out, verb, f)
	case "account":
		return ledgerAccount(ctx, svc, out, verb, f)
	case "determination":
		return ledgerDetermination(ctx, svc, out, verb, f)
	case "setting":
		return ledgerSetting(ctx, svc, out, verb, f)
	case "tree":
		return ledgerTree(ctx, svc, out, verb, f)
	case "uom":
		return ledgerUoM(ctx, svc, out, verb, f)
	default:
		return fmt.Errorf("unknown ledger noun %q; use company, period, currency, account, determination, setting, tree or uom", noun)
	}
}

// ledgerFlags holds every flag one ledger invocation may take. Each noun
// reads only its own.
type ledgerFlags struct {
	slug                                      *string
	asJSON                                    *bool
	name, label, company, book                *string
	fiscalYear, start, end                    *string
	code                                      *string
	exponent                                  *int
	from, to, rateType, source, at, rate      *string
	normal, parent, class, role               *string
	match, contextPairs                       *string
	pack, kind, listRule                      *string
	base                                      *string
	account                                   *string
	scope, scopeMap, effective, valueJSON, id *string
	revision                                  *int64
	disable, group, hard, publishedOnly       *bool
	master, root, baseUnit                    *string
	family, unit, factor, item                *string
	value, mode, asOf                         *string
	scale                                     *int
}

// ledgerService opens the workspace database like the migrate command and
// returns the ledger service over it. The CLI acts as the operator.
func ledgerService(slug string) (*ledger.Service, context.Context, context.CancelFunc, error) {
	endpoint, _, err := migrationTarget(slug)
	if err != nil {
		return nil, nil, nil, err
	}
	connector, err := endpoint.Connector()
	if err != nil {
		return nil, nil, nil, err
	}
	db := sql.OpenDB(connector)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	release := func() { cancel(); stop(); _ = db.Close() }
	dialect := store.Dialect(store.MariaDB{})
	if endpoint.Driver == workspace.DriverPostgres {
		dialect = store.PostgreSQL{}
	}
	return &ledger.Service{DB: db, Dialect: dialect}, ctx, release, nil
}

func parsePairs(text string) (map[string]string, error) {
	out := map[string]string{}
	if text == "" {
		return out, nil
	}
	for _, pair := range strings.Split(text, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf("%q must be k=v pairs separated by commas", text)
		}
		out[key] = value
	}
	return out, nil
}

func parseInstant(text, what string) (time.Time, error) {
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC 3339: %w", what, err)
	}
	return at, nil
}

func ledgerCompany(ctx context.Context, svc *ledger.Service, out io.Writer, verb string, f *ledgerFlags) error {
	switch verb {
	case "save":
		saved, err := svc.SaveCompany(ctx, ledger.Company{Name: *f.name, Label: *f.label, BaseCurrency: *f.base, Disabled: *f.disable}, *f.revision)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, saved)
		}
		fmt.Fprintf(out, "Saved company %s at revision %d.\n", saved.Name, saved.Revision)
		return nil
	case "list":
		companies, err := svc.ListCompanies(ctx)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, companies)
		}
		for _, company := range companies {
			fmt.Fprintf(out, "%s\t%s\tbase %s\trevision %d%s\n", company.Name, company.Label, orDash(company.BaseCurrency), company.Revision, disabledMark(company.Disabled))
		}
		return nil
	default:
		return fmt.Errorf("unknown ledger company command %q; use save or list", verb)
	}
}

func ledgerPeriod(ctx context.Context, svc *ledger.Service, out io.Writer, verb string, f *ledgerFlags) error {
	book := *f.book
	if book == "" {
		book = "main"
	}
	switch verb {
	case "save":
		saved, err := svc.SavePeriod(ctx, ledger.Period{Company: *f.company, Book: book, Name: *f.name, Label: *f.label, FiscalYear: *f.fiscalYear, StartDate: *f.start, EndDate: *f.end}, *f.revision)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, saved)
		}
		fmt.Fprintf(out, "Saved period %s at revision %d.\n", saved.Name, saved.Revision)
		return nil
	case "list":
		periods, err := svc.ListPeriods(ctx, *f.company, book)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, periods)
		}
		for _, period := range periods {
			fmt.Fprintf(out, "%s\t%s\t%s..%s\t%s\trevision %d\n", period.Name, period.Label, period.StartDate, period.EndDate, period.Status, period.Revision)
		}
		return nil
	case "close", "reopen":
		status := ledger.PeriodSoftClosed
		if verb == "close" && *f.hard {
			status = ledger.PeriodHardClosed
		} else if verb == "reopen" {
			status = ledger.PeriodOpen
		}
		saved, err := svc.SetPeriodStatus(ctx, *f.company, book, *f.name, status, "Local administrator", *f.revision)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, saved)
		}
		fmt.Fprintf(out, "Period %s is now %s.\n", saved.Name, saved.Status)
		return nil
	default:
		return fmt.Errorf("unknown ledger period command %q; use save, list, close or reopen", verb)
	}
}

func ledgerCurrency(ctx context.Context, svc *ledger.Service, out io.Writer, verb string, f *ledgerFlags) error {
	switch verb {
	case "save":
		saved, err := svc.SaveCurrency(ctx, ledger.Currency{Code: *f.code, Label: *f.label, Exponent: *f.exponent, Disabled: *f.disable}, *f.revision)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, saved)
		}
		fmt.Fprintf(out, "Saved currency %s at revision %d.\n", saved.Code, saved.Revision)
		return nil
	case "list":
		currencies, err := svc.ListCurrencies(ctx)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, currencies)
		}
		for _, currency := range currencies {
			fmt.Fprintf(out, "%s\t%s\texponent %d\trevision %d%s\n", currency.Code, currency.Label, currency.Exponent, currency.Revision, disabledMark(currency.Disabled))
		}
		return nil
	case "rate-save":
		at, err := parseInstant(*f.at, "--at")
		if err != nil {
			return err
		}
		rate, err := types.ParseDecimal(*f.rate)
		if err != nil {
			return fmt.Errorf("--rate must be a decimal number: %w", err)
		}
		saved, err := svc.SaveRate(ctx, ledger.ExchangeRate{From: *f.from, To: *f.to, RateType: *f.rateType, Source: *f.source, EffectiveAt: at, Rate: rate})
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, saved)
		}
		fmt.Fprintf(out, "Saved %s->%s %s at %s.\n", saved.From, saved.To, saved.Rate, saved.EffectiveAt.Format(time.RFC3339))
		return nil
	case "rate-get":
		at, err := parseInstant(*f.at, "--at")
		if err != nil {
			return err
		}
		rate, err := svc.GetRate(ctx, *f.from, *f.to, *f.rateType, *f.source, at)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, map[string]string{"rate": rate.String()})
		}
		fmt.Fprintf(out, "%s\n", rate)
		return nil
	default:
		return fmt.Errorf("unknown ledger currency command %q; use save, list, rate-save or rate-get", verb)
	}
}

func ledgerAccount(ctx context.Context, svc *ledger.Service, out io.Writer, verb string, f *ledgerFlags) error {
	switch verb {
	case "type-save":
		saved, err := svc.SaveAccountType(ctx, ledger.AccountType{Name: *f.name, Label: *f.label, NormalBalance: *f.normal})
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, saved)
		}
		fmt.Fprintf(out, "Saved account type %s.\n", saved.Name)
		return nil
	case "type-list":
		accountTypes, err := svc.ListAccountTypes(ctx)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, accountTypes)
		}
		for _, accountType := range accountTypes {
			fmt.Fprintf(out, "%s\t%s\t%s\n", accountType.Name, accountType.Label, accountType.NormalBalance)
		}
		return nil
	case "save":
		saved, err := svc.SaveAccount(ctx, ledger.Account{Name: *f.name, Label: *f.label, Type: *f.rateType, Parent: *f.parent, IsGroup: *f.group, Class: *f.class, ControlRole: *f.role, Company: *f.company, Disabled: *f.disable}, *f.revision)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, saved)
		}
		fmt.Fprintf(out, "Saved account %s at revision %d.\n", saved.Name, saved.Revision)
		return nil
	case "list":
		accounts, err := svc.ListAccounts(ctx, *f.company)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, accounts)
		}
		for _, account := range accounts {
			fmt.Fprintf(out, "%s\t%s\t%s%s\trevision %d%s\n", account.Name, account.Label, account.Type, groupMark(account.IsGroup), account.Revision, disabledMark(account.Disabled))
		}
		return nil
	default:
		return fmt.Errorf("unknown ledger account command %q; use type-save, type-list, save or list", verb)
	}
}

func ledgerDetermination(ctx context.Context, svc *ledger.Service, out io.Writer, verb string, f *ledgerFlags) error {
	switch verb {
	case "save":
		match, err := parsePairs(*f.match)
		if err != nil {
			return err
		}
		saved, err := svc.SaveDetermination(ctx, ledger.Determination{ID: *f.id, Label: *f.label, Account: *f.account, Match: match, EffectiveFrom: *f.start, EffectiveTo: *f.end, Disabled: *f.disable}, *f.revision)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, saved)
		}
		fmt.Fprintf(out, "Saved determination %s at revision %d.\n", saved.ID, saved.Revision)
		return nil
	case "list":
		mappings, err := svc.ListDeterminations(ctx)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, mappings)
		}
		for _, mapping := range mappings {
			fmt.Fprintf(out, "%s\t%s\t-> %s\t%s%s\trevision %d%s\n", mapping.ID, mapping.Label, mapping.Account, pairsText(mapping.Match), effectiveMark(mapping.EffectiveFrom, mapping.EffectiveTo), mapping.Revision, disabledMark(mapping.Disabled))
		}
		return nil
	case "resolve":
		context_, err := parsePairs(*f.contextPairs)
		if err != nil {
			return err
		}
		asOf := *f.asOf
		if asOf == "" {
			asOf = time.Now().UTC().Format(time.DateOnly)
		}
		resolved, err := svc.ResolveAccount(ctx, context_, asOf)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, resolved)
		}
		fmt.Fprintf(out, "%s\t%s\n", resolved.Account, resolved.Label)
		return nil
	default:
		return fmt.Errorf("unknown ledger determination command %q; use save, list or resolve", verb)
	}
}

func ledgerSetting(ctx context.Context, svc *ledger.Service, out io.Writer, verb string, f *ledgerFlags) error {
	switch verb {
	case "save":
		saved, err := svc.SaveSetting(ctx, ledger.Setting{OwnerPack: *f.pack, Type: *f.rateType, Label: *f.label, Kind: *f.kind, ListRule: *f.listRule}, *f.revision)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, saved)
		}
		fmt.Fprintf(out, "Saved setting %s/%s at revision %d.\n", saved.OwnerPack, saved.Type, saved.Revision)
		return nil
	case "list":
		settings, err := svc.ListSettings(ctx)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, settings)
		}
		for _, setting := range settings {
			fmt.Fprintf(out, "%s/%s\t%s\t%s\t%s\trevision %d\n", setting.OwnerPack, setting.Type, setting.Label, setting.Kind, orDash(setting.ListRule), setting.Revision)
		}
		return nil
	case "save-value", "publish", "discard", "list-values", "resolve", "preview":
		return ledgerSettingValue(ctx, svc, out, verb, f)
	default:
		return fmt.Errorf("unknown ledger setting command %q; use save, list, save-value, publish, discard, list-values, resolve or preview", verb)
	}
}

func ledgerSettingValue(ctx context.Context, svc *ledger.Service, out io.Writer, verb string, f *ledgerFlags) error {
	switch verb {
	case "save-value":
		at, err := parseInstant(*f.effective, "--effective-from")
		if err != nil {
			return err
		}
		var value any
		if err := jsonUnmarshal(*f.valueJSON, &value); err != nil {
			return fmt.Errorf("--value-json must be JSON: %w", err)
		}
		saved, err := svc.SaveSettingValue(ctx, ledger.SettingValue{OwnerPack: *f.pack, Type: *f.rateType, ScopeChain: *f.scope, EffectiveFrom: at, Value: value})
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, saved)
		}
		fmt.Fprintf(out, "Saved draft revision %d.\n", saved.Revision)
		return nil
	case "publish", "discard":
		at, err := parseInstant(*f.effective, "--effective-from")
		if err != nil {
			return err
		}
		if verb == "publish" {
			published, err := svc.PublishSettingValue(ctx, *f.pack, *f.rateType, *f.scope, at, *f.revision, "Local administrator")
			if err != nil {
				return err
			}
			if *f.asJSON {
				return writeJSON(out, published)
			}
			fmt.Fprintf(out, "Published revision %d.\n", published.Revision)
			return nil
		}
		if err := svc.DiscardSettingValue(ctx, *f.pack, *f.rateType, *f.scope, at, *f.revision); err != nil {
			return err
		}
		fmt.Fprintln(out, "Discarded the draft.")
		return nil
	case "list-values":
		values, err := svc.ListSettingValues(ctx, *f.pack, *f.rateType, *f.publishedOnly)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, values)
		}
		for _, value := range values {
			fmt.Fprintf(out, "%s\t%s\trevision %d\t%s\t%s\n", orGlobal(value.ScopeChain), value.EffectiveFrom.Format(time.RFC3339), value.Revision, value.Status, jsonText(value.Value))
		}
		return nil
	case "resolve":
		at, err := parseInstant(*f.at, "--at")
		if err != nil {
			return err
		}
		scope, err := parsePairs(*f.scopeMap)
		if err != nil {
			return err
		}
		resolved, err := svc.ResolveSetting(ctx, *f.pack, *f.rateType, scope, at)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, resolved)
		}
		fmt.Fprintf(out, "%s\n", jsonText(resolved.Value))
		return nil
	case "preview":
		at, err := parseInstant(*f.effective, "--effective-from")
		if err != nil {
			return err
		}
		var value any
		if err := jsonUnmarshal(*f.valueJSON, &value); err != nil {
			return fmt.Errorf("--value-json must be JSON: %w", err)
		}
		scope, err := parsePairs(*f.scopeMap)
		if err != nil {
			return err
		}
		changes, err := svc.DiffPreview(ctx, *f.pack, *f.rateType, scope, ledger.SettingValue{OwnerPack: *f.pack, Type: *f.rateType, ScopeChain: *f.scope, EffectiveFrom: at, Value: value})
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, changes)
		}
		for _, change := range changes {
			fmt.Fprintf(out, "%s\t%s -> %s\n", change.At.Format(time.RFC3339), jsonText(change.Before), jsonText(change.After))
		}
		return nil
	default:
		return fmt.Errorf("unknown ledger setting command %q", verb)
	}
}

func jsonUnmarshal(text string, out *any) error {
	return json.Unmarshal([]byte(text), out)
}

func jsonText(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "(not JSON)"
	}
	return string(raw)
}

func orDash(text string) string {
	if text == "" {
		return "-"
	}
	return text
}

func orGlobal(chain string) string {
	if chain == "" {
		return "global"
	}
	return chain
}

func disabledMark(disabled bool) string {
	if disabled {
		return "\tdisabled"
	}
	return ""
}

func groupMark(group bool) string {
	if group {
		return "\tgroup"
	}
	return ""
}

func pairsText(pairs map[string]string) string {
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = key + "=" + pairs[key]
	}
	return strings.Join(parts, ",")
}

// effectiveMark shows a determination's end-exclusive range, or nothing
// when the mapping is fully open.
func effectiveMark(from, to string) string {
	if from == "" && to == "" {
		return ""
	}
	return fmt.Sprintf("\teffective %s..%s", from, to)
}

// treeNode is one master row placed in its hierarchy.
type treeNode struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Parent   string `json:"parent"`
	Depth    int    `json:"depth"`
	Disabled bool   `json:"disabled"`
}

func ledgerTree(ctx context.Context, svc *ledger.Service, out io.Writer, verb string, f *ledgerFlags) error {
	if verb != "show" {
		return fmt.Errorf("unknown ledger tree command %q; use show", verb)
	}
	nodes, err := ledgerTreeNodes(ctx, svc, *f.master, *f.company)
	if err != nil {
		return err
	}
	ordered, err := orderTree(nodes, *f.root)
	if err != nil {
		return err
	}
	if *f.asJSON {
		return writeJSON(out, ordered)
	}
	for _, node := range ordered {
		fmt.Fprintf(out, "%s%s\t%s%s\n", strings.Repeat("  ", node.Depth), node.Name, node.Label, disabledMark(node.Disabled))
	}
	return nil
}

func ledgerTreeNodes(ctx context.Context, svc *ledger.Service, master, company string) ([]treeNode, error) {
	switch master {
	case "account":
		accounts, err := svc.ListAccounts(ctx, company)
		if err != nil {
			return nil, err
		}
		nodes := make([]treeNode, len(accounts))
		for i, account := range accounts {
			nodes[i] = treeNode{Name: account.Name, Label: account.Label, Parent: account.Parent, Disabled: account.Disabled}
		}
		return nodes, nil
	case "branch":
		branches, err := svc.ListBranches(ctx, company)
		if err != nil {
			return nil, err
		}
		nodes := make([]treeNode, len(branches))
		for i, branch := range branches {
			nodes[i] = treeNode{Name: branch.Name, Label: branch.Label, Parent: branch.Parent, Disabled: branch.Disabled}
		}
		return nodes, nil
	case "department":
		departments, err := svc.ListDepartments(ctx, company)
		if err != nil {
			return nil, err
		}
		nodes := make([]treeNode, len(departments))
		for i, department := range departments {
			nodes[i] = treeNode{Name: department.Name, Label: department.Label, Parent: department.Parent, Disabled: department.Disabled}
		}
		return nodes, nil
	case "cost-centre":
		centres, err := svc.ListCostCentres(ctx, company)
		if err != nil {
			return nil, err
		}
		nodes := make([]treeNode, len(centres))
		for i, centre := range centres {
			nodes[i] = treeNode{Name: centre.Name, Label: centre.Label, Parent: centre.Parent, Disabled: centre.Disabled}
		}
		return nodes, nil
	default:
		return nil, fmt.Errorf("tree --master must be account, branch, department or cost-centre")
	}
}

// orderTree lays master rows out depth-first from the roots (or from root
// when set). Saves refuse cycles, but hand edits can corrupt the links, so
// a revisit fails naming the node instead of looping.
func orderTree(nodes []treeNode, root string) ([]treeNode, error) {
	byName := make(map[string]treeNode, len(nodes))
	children := make(map[string][]treeNode)
	for _, node := range nodes {
		byName[node.Name] = node
		children[node.Parent] = append(children[node.Parent], node)
	}
	for _, siblings := range children {
		sort.Slice(siblings, func(i, j int) bool { return siblings[i].Name < siblings[j].Name })
	}
	starts := children[""]
	if root != "" {
		top, ok := byName[root]
		if !ok {
			return nil, fmt.Errorf("unknown tree root %q", root)
		}
		starts = []treeNode{top}
	}
	var ordered []treeNode
	visiting := map[string]bool{}
	var visit func(node treeNode, depth int) error
	visit = func(node treeNode, depth int) error {
		if visiting[node.Name] {
			return fmt.Errorf("cycle at %q: the parent links are corrupt", node.Name)
		}
		visiting[node.Name] = true
		node.Depth = depth
		ordered = append(ordered, node)
		for _, child := range children[node.Name] {
			if err := visit(child, depth+1); err != nil {
				return err
			}
		}
		delete(visiting, node.Name)
		return nil
	}
	for _, start := range starts {
		if err := visit(start, 0); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func ledgerUoM(ctx context.Context, svc *ledger.Service, out io.Writer, verb string, f *ledgerFlags) error {
	switch verb {
	case "family-save":
		saved, err := svc.SaveUoMFamily(ctx, ledger.UoMFamily{Name: *f.name, Label: *f.label, BaseUnit: *f.baseUnit, Disabled: *f.disable}, *f.revision)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, saved)
		}
		fmt.Fprintf(out, "Saved UoM family %s at revision %d.\n", saved.Name, saved.Revision)
		return nil
	case "family-list":
		families, err := svc.ListUoMFamilies(ctx)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, families)
		}
		for _, family := range families {
			fmt.Fprintf(out, "%s\t%s\tbase %s\trevision %d%s\n", family.Name, family.Label, family.BaseUnit, family.Revision, disabledMark(family.Disabled))
		}
		return nil
	case "conversion-save":
		factor, err := types.ParseDecimal(*f.factor)
		if err != nil {
			return fmt.Errorf("factor %q is not a decimal: %w", *f.factor, err)
		}
		saved, err := svc.SaveUoMConversion(ctx, ledger.UoMConversion{Family: *f.family, Unit: *f.unit, Item: *f.item, Factor: factor, Scale: *f.scale, Disabled: *f.disable}, *f.revision)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, saved)
		}
		fmt.Fprintf(out, "Saved UoM conversion %s/%s at revision %d.\n", saved.Family, saved.Unit, saved.Revision)
		return nil
	case "conversion-get":
		conversion, err := svc.GetUoMConversion(ctx, *f.family, *f.unit, *f.item)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, conversion)
		}
		fmt.Fprintf(out, "%s\t%s\t%s\tfactor %s\tscale %d%s\n", conversion.Family, conversion.Unit, conversionItemText(conversion.Item), conversion.Factor.String(), conversion.Scale, disabledMark(conversion.Disabled))
		return nil
	case "conversion-list":
		conversions, err := svc.ListUoMConversions(ctx, *f.family)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, conversions)
		}
		for _, conversion := range conversions {
			fmt.Fprintf(out, "%s\t%s\t%s\tfactor %s\tscale %d%s\n", conversion.Family, conversion.Unit, conversionItemText(conversion.Item), conversion.Factor.String(), conversion.Scale, disabledMark(conversion.Disabled))
		}
		return nil
	case "convert":
		value, err := types.ParseDecimal(*f.value)
		if err != nil {
			return fmt.Errorf("value %q is not a decimal: %w", *f.value, err)
		}
		qty, err := types.NewQuantity(value, *f.from)
		if err != nil {
			return fmt.Errorf("unit %q is not a unit of measure", *f.from)
		}
		mode := types.RoundingMode(*f.mode)
		if *f.mode == "" {
			mode = types.HalfUp
		}
		if !mode.Valid() {
			return fmt.Errorf("mode %q must be half_up or half_even", *f.mode)
		}
		converted, err := svc.ConvertQuantity(ctx, qty, *f.to, *f.item, mode)
		if err != nil {
			return err
		}
		if *f.asJSON {
			return writeJSON(out, map[string]string{"value": converted.Value.String(), "unit": converted.UoM})
		}
		fmt.Fprintf(out, "%s %s = %s\n", value.String(), *f.from, converted.String())
		return nil
	default:
		return fmt.Errorf("unknown ledger uom command %q; use family-save, family-list, conversion-save, conversion-get, conversion-list or convert", verb)
	}
}

func conversionItemText(item string) string {
	if item == "" {
		return "default"
	}
	return "item " + item
}
