package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/ledger"
	"athena-erp/internal/types"

	"connectrpc.com/connect/v2"
)

// ledgerService implements athena.v1.LedgerService over internal/ledger. The
// gate checks ledger:read or ledger:write; the handlers check again so a call
// that bypasses the gate is still refused.
type ledgerService struct {
	db *sql.DB
}

func newLedgerService(db *sql.DB) ledgerService { return ledgerService{db: db} }

func (s ledgerService) ready() error {
	if s.db == nil {
		return connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	return nil
}

func (s ledgerService) svc() *ledger.Service {
	return &ledger.Service{DB: s.db, Dialect: dialect}
}

func ledgerError(err error) error {
	var input *ledger.InputError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ledger.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err.Error())
	case errors.Is(err, ledger.ErrExists):
		return connect.NewError(connect.CodeAlreadyExists, err.Error())
	case errors.Is(err, ledger.ErrConflict):
		return connect.NewError(connect.CodeAborted, err.Error())
	case errors.Is(err, ledger.ErrAmbiguous):
		return connect.NewError(connect.CodeFailedPrecondition, err.Error())
	case errors.Is(err, ledger.ErrNoMatch), errors.Is(err, ledger.ErrNoGeneration):
		return connect.NewError(connect.CodeNotFound, err.Error())
	case errors.As(err, &input):
		return connect.NewError(connect.CodeInvalidArgument, err.Error())
	default:
		return internalError("ledger", err)
	}
}

func companyMessage(c ledger.Company) *athenav1.Company {
	return &athenav1.Company{Name: c.Name, Label: c.Label, BaseCurrency: c.BaseCurrency, Disabled: c.Disabled, Revision: c.Revision}
}

func branchMessage(b ledger.Branch) *athenav1.Branch {
	return &athenav1.Branch{Name: b.Name, Label: b.Label, Company: b.Company, Parent: b.Parent, IsGroup: b.IsGroup, Disabled: b.Disabled, Revision: b.Revision}
}

func departmentMessage(d ledger.Department) *athenav1.Department {
	return &athenav1.Department{Name: d.Name, Label: d.Label, Company: d.Company, Branch: d.Branch, Parent: d.Parent, IsGroup: d.IsGroup, Disabled: d.Disabled, Revision: d.Revision}
}

func costCentreMessage(c ledger.CostCentre) *athenav1.CostCentre {
	return &athenav1.CostCentre{Name: c.Name, Label: c.Label, Company: c.Company, Parent: c.Parent, IsGroup: c.IsGroup, Disabled: c.Disabled, Revision: c.Revision}
}

func dimensionMessage(d ledger.Dimension) *athenav1.DimensionDefinition {
	return &athenav1.DimensionDefinition{Name: d.Name, Label: d.Label, Members: d.Members, MandatoryFor: d.MandatoryFor, ValidFrom: d.ValidFrom, ValidTo: d.ValidTo, Disabled: d.Disabled, Revision: d.Revision}
}

func fiscalYearMessage(y ledger.FiscalYear) *athenav1.FiscalYear {
	return &athenav1.FiscalYear{Name: y.Name, Label: y.Label, Company: y.Company, StartDate: y.StartDate, EndDate: y.EndDate, Disabled: y.Disabled, Revision: y.Revision}
}

func periodMessage(p ledger.Period) *athenav1.Period {
	return &athenav1.Period{Company: p.Company, Book: p.Book, Name: p.Name, Label: p.Label, FiscalYear: p.FiscalYear, StartDate: p.StartDate, EndDate: p.EndDate, Status: p.Status, Revision: p.Revision}
}

func (s ledgerService) SaveCompany(ctx context.Context, req *athenav1.SaveCompanyRequest) (*athenav1.SaveCompanyResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetCompany()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "company is required")
	}
	saved, err := s.svc().SaveCompany(ctx, ledger.Company{Name: in.GetName(), Label: in.GetLabel(), BaseCurrency: in.GetBaseCurrency(), Disabled: in.GetDisabled()}, req.GetExpectedRevision())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveCompanyResponse{Company: companyMessage(saved)}, nil
}

func (s ledgerService) ListCompanies(ctx context.Context, _ *athenav1.ListCompaniesRequest) (*athenav1.ListCompaniesResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	companies, err := s.svc().ListCompanies(ctx)
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListCompaniesResponse{}
	for _, company := range companies {
		res.Companies = append(res.Companies, companyMessage(company))
	}
	return res, nil
}

func (s ledgerService) SaveBranch(ctx context.Context, req *athenav1.SaveBranchRequest) (*athenav1.SaveBranchResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetBranch()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "branch is required")
	}
	saved, err := s.svc().SaveBranch(ctx, ledger.Branch{Name: in.GetName(), Label: in.GetLabel(), Company: in.GetCompany(), Parent: in.GetParent(), IsGroup: in.GetIsGroup(), Disabled: in.GetDisabled()}, req.GetExpectedRevision())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveBranchResponse{Branch: branchMessage(saved)}, nil
}

func (s ledgerService) ListBranches(ctx context.Context, req *athenav1.ListBranchesRequest) (*athenav1.ListBranchesResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	branches, err := s.svc().ListBranches(ctx, req.GetCompany())
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListBranchesResponse{}
	for _, branch := range branches {
		res.Branches = append(res.Branches, branchMessage(branch))
	}
	return res, nil
}

func (s ledgerService) SaveDepartment(ctx context.Context, req *athenav1.SaveDepartmentRequest) (*athenav1.SaveDepartmentResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetDepartment()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "department is required")
	}
	saved, err := s.svc().SaveDepartment(ctx, ledger.Department{Name: in.GetName(), Label: in.GetLabel(), Company: in.GetCompany(), Branch: in.GetBranch(), Parent: in.GetParent(), IsGroup: in.GetIsGroup(), Disabled: in.GetDisabled()}, req.GetExpectedRevision())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveDepartmentResponse{Department: departmentMessage(saved)}, nil
}

func (s ledgerService) ListDepartments(ctx context.Context, req *athenav1.ListDepartmentsRequest) (*athenav1.ListDepartmentsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	departments, err := s.svc().ListDepartments(ctx, req.GetCompany())
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListDepartmentsResponse{}
	for _, department := range departments {
		res.Departments = append(res.Departments, departmentMessage(department))
	}
	return res, nil
}

func (s ledgerService) SaveCostCentre(ctx context.Context, req *athenav1.SaveCostCentreRequest) (*athenav1.SaveCostCentreResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetCostCentre()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "cost centre is required")
	}
	saved, err := s.svc().SaveCostCentre(ctx, ledger.CostCentre{Name: in.GetName(), Label: in.GetLabel(), Company: in.GetCompany(), Parent: in.GetParent(), IsGroup: in.GetIsGroup(), Disabled: in.GetDisabled()}, req.GetExpectedRevision())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveCostCentreResponse{CostCentre: costCentreMessage(saved)}, nil
}

func (s ledgerService) ListCostCentres(ctx context.Context, req *athenav1.ListCostCentresRequest) (*athenav1.ListCostCentresResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	centres, err := s.svc().ListCostCentres(ctx, req.GetCompany())
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListCostCentresResponse{}
	for _, centre := range centres {
		res.CostCentres = append(res.CostCentres, costCentreMessage(centre))
	}
	return res, nil
}

func (s ledgerService) SaveDimension(ctx context.Context, req *athenav1.SaveDimensionRequest) (*athenav1.SaveDimensionResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetDimension()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "dimension is required")
	}
	saved, err := s.svc().SaveDimension(ctx, ledger.Dimension{Name: in.GetName(), Label: in.GetLabel(), Members: in.GetMembers(), MandatoryFor: in.GetMandatoryFor(), ValidFrom: in.GetValidFrom(), ValidTo: in.GetValidTo(), Disabled: in.GetDisabled()}, req.GetExpectedRevision())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveDimensionResponse{Dimension: dimensionMessage(saved)}, nil
}

func (s ledgerService) ListDimensions(ctx context.Context, _ *athenav1.ListDimensionsRequest) (*athenav1.ListDimensionsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	dimensions, err := s.svc().ListDimensions(ctx)
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListDimensionsResponse{}
	for _, dimension := range dimensions {
		res.Dimensions = append(res.Dimensions, dimensionMessage(dimension))
	}
	return res, nil
}

func (s ledgerService) SaveFiscalYear(ctx context.Context, req *athenav1.SaveFiscalYearRequest) (*athenav1.SaveFiscalYearResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetFiscalYear()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "fiscal year is required")
	}
	saved, err := s.svc().SaveFiscalYear(ctx, ledger.FiscalYear{Name: in.GetName(), Label: in.GetLabel(), Company: in.GetCompany(), StartDate: in.GetStartDate(), EndDate: in.GetEndDate(), Disabled: in.GetDisabled()}, req.GetExpectedRevision())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveFiscalYearResponse{FiscalYear: fiscalYearMessage(saved)}, nil
}

func (s ledgerService) ListFiscalYears(ctx context.Context, req *athenav1.ListFiscalYearsRequest) (*athenav1.ListFiscalYearsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	years, err := s.svc().ListFiscalYears(ctx, req.GetCompany())
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListFiscalYearsResponse{}
	for _, year := range years {
		res.FiscalYears = append(res.FiscalYears, fiscalYearMessage(year))
	}
	return res, nil
}

func (s ledgerService) SavePeriod(ctx context.Context, req *athenav1.SavePeriodRequest) (*athenav1.SavePeriodResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetPeriod()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "period is required")
	}
	saved, err := s.svc().SavePeriod(ctx, ledger.Period{Company: in.GetCompany(), Book: in.GetBook(), Name: in.GetName(), Label: in.GetLabel(), FiscalYear: in.GetFiscalYear(), StartDate: in.GetStartDate(), EndDate: in.GetEndDate()}, req.GetExpectedRevision())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SavePeriodResponse{Period: periodMessage(saved)}, nil
}

func (s ledgerService) SetPeriodStatus(ctx context.Context, req *athenav1.SetPeriodStatusRequest) (*athenav1.SetPeriodStatusResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	saved, err := s.svc().SetPeriodStatus(ctx, req.GetCompany(), req.GetBook(), req.GetName(), req.GetStatus(), actorName(ctx), req.GetExpectedRevision())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SetPeriodStatusResponse{Period: periodMessage(saved)}, nil
}

func (s ledgerService) ListPeriods(ctx context.Context, req *athenav1.ListPeriodsRequest) (*athenav1.ListPeriodsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	periods, err := s.svc().ListPeriods(ctx, req.GetCompany(), req.GetBook())
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListPeriodsResponse{}
	for _, period := range periods {
		res.Periods = append(res.Periods, periodMessage(period))
	}
	return res, nil
}

func currencyMessage(c ledger.Currency) *athenav1.Currency {
	return &athenav1.Currency{Code: c.Code, Label: c.Label, Exponent: int32(c.Exponent), Disabled: c.Disabled, Revision: c.Revision}
}

func rateMessage(r ledger.ExchangeRate) *athenav1.ExchangeRate {
	return &athenav1.ExchangeRate{FromCode: r.From, ToCode: r.To, RateType: r.RateType, Source: r.Source, EffectiveAt: r.EffectiveAt.UTC().Format(time.RFC3339), Rate: r.Rate.String()}
}

func (s ledgerService) SaveCurrency(ctx context.Context, req *athenav1.SaveCurrencyRequest) (*athenav1.SaveCurrencyResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetCurrency()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "currency is required")
	}
	saved, err := s.svc().SaveCurrency(ctx, ledger.Currency{Code: in.GetCode(), Label: in.GetLabel(), Exponent: int(in.GetExponent()), Disabled: in.GetDisabled()}, req.GetExpectedRevision())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveCurrencyResponse{Currency: currencyMessage(saved)}, nil
}

func (s ledgerService) ListCurrencies(ctx context.Context, _ *athenav1.ListCurrenciesRequest) (*athenav1.ListCurrenciesResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	currencies, err := s.svc().ListCurrencies(ctx)
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListCurrenciesResponse{}
	for _, currency := range currencies {
		res.Currencies = append(res.Currencies, currencyMessage(currency))
	}
	return res, nil
}

func (s ledgerService) SaveRate(ctx context.Context, req *athenav1.SaveRateRequest) (*athenav1.SaveRateResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetRate()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "rate is required")
	}
	at, err := time.Parse(time.RFC3339, in.GetEffectiveAt())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "effective_at must be RFC 3339")
	}
	rate, err := types.ParseDecimal(in.GetRate())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "rate must be a decimal number")
	}
	saved, err := s.svc().SaveRate(ctx, ledger.ExchangeRate{From: in.GetFromCode(), To: in.GetToCode(), RateType: in.GetRateType(), Source: in.GetSource(), EffectiveAt: at, Rate: rate})
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveRateResponse{Rate: rateMessage(saved)}, nil
}

func (s ledgerService) GetRate(ctx context.Context, req *athenav1.GetRateRequest) (*athenav1.GetRateResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	at, err := time.Parse(time.RFC3339, req.GetAt())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "at must be RFC 3339")
	}
	rate, err := s.svc().GetRate(ctx, req.GetFromCode(), req.GetToCode(), req.GetRateType(), req.GetSource(), at)
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.GetRateResponse{Rate: rate.String()}, nil
}

func accountTypeMessage(t ledger.AccountType) *athenav1.AccountType {
	return &athenav1.AccountType{Name: t.Name, Label: t.Label, NormalBalance: t.NormalBalance}
}

func accountMessage(a ledger.Account) *athenav1.Account {
	return &athenav1.Account{Name: a.Name, Label: a.Label, Type: a.Type, Parent: a.Parent, IsGroup: a.IsGroup, Class: a.Class, ControlRole: a.ControlRole, Company: a.Company, Disabled: a.Disabled, Revision: a.Revision}
}

func (s ledgerService) SaveAccountType(ctx context.Context, req *athenav1.SaveAccountTypeRequest) (*athenav1.SaveAccountTypeResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetAccountType()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "account type is required")
	}
	saved, err := s.svc().SaveAccountType(ctx, ledger.AccountType{Name: in.GetName(), Label: in.GetLabel(), NormalBalance: in.GetNormalBalance()})
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveAccountTypeResponse{AccountType: accountTypeMessage(saved)}, nil
}

func (s ledgerService) ListAccountTypes(ctx context.Context, _ *athenav1.ListAccountTypesRequest) (*athenav1.ListAccountTypesResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	accountTypes, err := s.svc().ListAccountTypes(ctx)
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListAccountTypesResponse{}
	for _, accountType := range accountTypes {
		res.AccountTypes = append(res.AccountTypes, accountTypeMessage(accountType))
	}
	return res, nil
}

func (s ledgerService) SaveAccount(ctx context.Context, req *athenav1.SaveAccountRequest) (*athenav1.SaveAccountResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetAccount()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "account is required")
	}
	saved, err := s.svc().SaveAccount(ctx, ledger.Account{Name: in.GetName(), Label: in.GetLabel(), Type: in.GetType(), Parent: in.GetParent(), IsGroup: in.GetIsGroup(), Class: in.GetClass(), ControlRole: in.GetControlRole(), Company: in.GetCompany(), Disabled: in.GetDisabled()}, req.GetExpectedRevision())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveAccountResponse{Account: accountMessage(saved)}, nil
}

func (s ledgerService) ListAccounts(ctx context.Context, req *athenav1.ListAccountsRequest) (*athenav1.ListAccountsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	accounts, err := s.svc().ListAccounts(ctx, req.GetCompany())
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListAccountsResponse{}
	for _, account := range accounts {
		res.Accounts = append(res.Accounts, accountMessage(account))
	}
	return res, nil
}

func determinationMessage(d ledger.Determination) *athenav1.Determination {
	return &athenav1.Determination{Id: d.ID, Label: d.Label, Account: d.Account, Match: d.Match, Disabled: d.Disabled, Revision: d.Revision}
}

func (s ledgerService) SaveDetermination(ctx context.Context, req *athenav1.SaveDeterminationRequest) (*athenav1.SaveDeterminationResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetDetermination()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "determination is required")
	}
	saved, err := s.svc().SaveDetermination(ctx, ledger.Determination{ID: in.GetId(), Label: in.GetLabel(), Account: in.GetAccount(), Match: in.GetMatch(), Disabled: in.GetDisabled()}, req.GetExpectedRevision())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveDeterminationResponse{Determination: determinationMessage(saved)}, nil
}

func (s ledgerService) ListDeterminations(ctx context.Context, _ *athenav1.ListDeterminationsRequest) (*athenav1.ListDeterminationsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	determinations, err := s.svc().ListDeterminations(ctx)
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListDeterminationsResponse{}
	for _, determination := range determinations {
		res.Determinations = append(res.Determinations, determinationMessage(determination))
	}
	return res, nil
}

func (s ledgerService) ResolveAccount(ctx context.Context, req *athenav1.ResolveAccountRequest) (*athenav1.ResolveAccountResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	determination, err := s.svc().ResolveAccount(ctx, req.GetContext())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.ResolveAccountResponse{Determination: determinationMessage(determination)}, nil
}

func settingMessage(s ledger.Setting) *athenav1.Setting {
	return &athenav1.Setting{OwnerPack: s.OwnerPack, Type: s.Type, Label: s.Label, Kind: s.Kind, ListRule: s.ListRule, Revision: s.Revision}
}

func settingValueMessage(v ledger.SettingValue) *athenav1.SettingValue {
	payload, _ := json.Marshal(v.Value)
	return &athenav1.SettingValue{OwnerPack: v.OwnerPack, Type: v.Type, ScopeChain: v.ScopeChain, EffectiveFrom: v.EffectiveFrom.UTC().Format(time.RFC3339), Revision: v.Revision, ValueJson: string(payload), Status: v.Status}
}

func (s ledgerService) SaveSetting(ctx context.Context, req *athenav1.SaveSettingRequest) (*athenav1.SaveSettingResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetSetting()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "setting is required")
	}
	saved, err := s.svc().SaveSetting(ctx, ledger.Setting{OwnerPack: in.GetOwnerPack(), Type: in.GetType(), Label: in.GetLabel(), Kind: in.GetKind(), ListRule: in.GetListRule()}, req.GetExpectedRevision())
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveSettingResponse{Setting: settingMessage(saved)}, nil
}

func (s ledgerService) ListSettings(ctx context.Context, _ *athenav1.ListSettingsRequest) (*athenav1.ListSettingsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	settings, err := s.svc().ListSettings(ctx)
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListSettingsResponse{}
	for _, setting := range settings {
		res.Settings = append(res.Settings, settingMessage(setting))
	}
	return res, nil
}

func (s ledgerService) SaveSettingValue(ctx context.Context, req *athenav1.SaveSettingValueRequest) (*athenav1.SaveSettingValueResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	in := req.GetValue()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "value is required")
	}
	at, err := time.Parse(time.RFC3339, in.GetEffectiveFrom())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "effective_from must be RFC 3339")
	}
	var value any
	if err := json.Unmarshal([]byte(in.GetValueJson()), &value); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "value_json must be JSON")
	}
	saved, err := s.svc().SaveSettingValue(ctx, ledger.SettingValue{OwnerPack: in.GetOwnerPack(), Type: in.GetType(), ScopeChain: in.GetScopeChain(), EffectiveFrom: at, Value: value})
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.SaveSettingValueResponse{Value: settingValueMessage(saved)}, nil
}

func (s ledgerService) PublishSettingValue(ctx context.Context, req *athenav1.PublishSettingValueRequest) (*athenav1.PublishSettingValueResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	at, err := time.Parse(time.RFC3339, req.GetEffectiveFrom())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "effective_from must be RFC 3339")
	}
	published, err := s.svc().PublishSettingValue(ctx, req.GetOwnerPack(), req.GetType(), req.GetScopeChain(), at, req.GetRevision(), actorName(ctx))
	if err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.PublishSettingValueResponse{Value: settingValueMessage(published)}, nil
}

func (s ledgerService) DiscardSettingValue(ctx context.Context, req *athenav1.DiscardSettingValueRequest) (*athenav1.DiscardSettingValueResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerWrite); err != nil {
		return nil, err
	}
	at, err := time.Parse(time.RFC3339, req.GetEffectiveFrom())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "effective_from must be RFC 3339")
	}
	if err := s.svc().DiscardSettingValue(ctx, req.GetOwnerPack(), req.GetType(), req.GetScopeChain(), at, req.GetRevision()); err != nil {
		return nil, ledgerError(err)
	}
	return &athenav1.DiscardSettingValueResponse{}, nil
}

func (s ledgerService) ListSettingValues(ctx context.Context, req *athenav1.ListSettingValuesRequest) (*athenav1.ListSettingValuesResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	values, err := s.svc().ListSettingValues(ctx, req.GetOwnerPack(), req.GetType(), req.GetPublishedOnly())
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.ListSettingValuesResponse{}
	for _, value := range values {
		res.Values = append(res.Values, settingValueMessage(value))
	}
	return res, nil
}

func (s ledgerService) ResolveSetting(ctx context.Context, req *athenav1.ResolveSettingRequest) (*athenav1.ResolveSettingResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	at, err := time.Parse(time.RFC3339, req.GetAt())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "at must be RFC 3339")
	}
	resolved, err := s.svc().ResolveSetting(ctx, req.GetOwnerPack(), req.GetType(), req.GetScope(), at)
	if err != nil {
		return nil, ledgerError(err)
	}
	payload, err := json.Marshal(resolved.Value)
	if err != nil {
		return nil, internalError("encode setting value", err)
	}
	res := &athenav1.ResolveSettingResponse{ValueJson: string(payload)}
	for _, winner := range resolved.Winners {
		res.Winners = append(res.Winners, settingValueMessage(winner))
	}
	return res, nil
}

func (s ledgerService) PreviewSettingChange(ctx context.Context, req *athenav1.PreviewSettingChangeRequest) (*athenav1.PreviewSettingChangeResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	in := req.GetDraft()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "draft is required")
	}
	at, err := time.Parse(time.RFC3339, in.GetEffectiveFrom())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "effective_from must be RFC 3339")
	}
	var value any
	if err := json.Unmarshal([]byte(in.GetValueJson()), &value); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "value_json must be JSON")
	}
	changes, err := s.svc().DiffPreview(ctx, req.GetOwnerPack(), req.GetType(), req.GetScope(), ledger.SettingValue{
		OwnerPack: in.GetOwnerPack(), Type: in.GetType(), ScopeChain: in.GetScopeChain(), EffectiveFrom: at, Revision: in.GetRevision(), Value: value,
	})
	if err != nil {
		return nil, ledgerError(err)
	}
	res := &athenav1.PreviewSettingChangeResponse{}
	for _, change := range changes {
		before, _ := json.Marshal(change.Before)
		after, _ := json.Marshal(change.After)
		res.Changes = append(res.Changes, &athenav1.SettingChange{At: change.At.UTC().Format(time.RFC3339), BeforeJson: string(before), AfterJson: string(after)})
	}
	return res, nil
}

func (s ledgerService) GetReadiness(ctx context.Context, req *athenav1.GetReadinessRequest) (*athenav1.GetReadinessResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionLedgerRead); err != nil {
		return nil, err
	}
	svc := s.svc()
	res := &athenav1.GetReadinessResponse{}
	check := func(name string, ok bool, detail string) {
		res.Checks = append(res.Checks, &athenav1.ReadinessCheck{Name: name, Ok: ok, Detail: detail})
	}
	company, err := svc.GetCompany(ctx, req.GetCompany())
	if err != nil {
		check("Company set up", false, "save it with: athena ledger company save --name "+req.GetCompany()+" --label ...")
		return res, nil
	}
	check("Company set up", true, company.Label)
	if company.BaseCurrency == "" {
		check("Base currency chosen", false, "set it with: athena ledger company save --name "+company.Name+" --base-currency USD --expected-revision "+revision(company.Revision))
	} else {
		check("Base currency chosen", true, company.BaseCurrency)
	}
	currencies, err := svc.ListCurrencies(ctx)
	if err != nil {
		return nil, ledgerError(err)
	}
	check("Currencies available", len(currencies) > 0, orAction(len(currencies) > 0, "save one with: athena ledger currency save --code USD --label ..."))
	today := time.Now().UTC().Format("2006-01-02")
	if _, err := svc.PeriodFor(ctx, company.Name, "main", today); err != nil {
		check("Today falls in an open period", false, "save one with: athena ledger period save --company "+company.Name+" --name ... --start ... --end ...")
	} else {
		check("Today falls in an open period", true, today)
	}
	accountTypes, err := svc.ListAccountTypes(ctx)
	if err != nil {
		return nil, ledgerError(err)
	}
	check("Account types available", len(accountTypes) > 0, orAction(len(accountTypes) > 0, "save one with: athena ledger account type-save --name asset ..."))
	accounts, err := svc.ListAccounts(ctx, company.Name)
	if err != nil {
		return nil, ledgerError(err)
	}
	leaves := 0
	for _, account := range accounts {
		if !account.IsGroup && !account.Disabled {
			leaves++
		}
	}
	check("Posting accounts available", leaves > 0, orAction(leaves > 0, "save one with: athena ledger account save --name ... --type ..."))
	mappings, err := svc.ListDeterminations(ctx)
	if err != nil {
		return nil, ledgerError(err)
	}
	enabled := 0
	for _, mapping := range mappings {
		if !mapping.Disabled {
			enabled++
		}
	}
	check("Posting rules available", enabled > 0, orAction(enabled > 0, "save one with: athena ledger determination save --label ... --account ...; per-action coverage arrives with the posting engine"))
	return res, nil
}

// orAction returns no detail for a passing check, so the desk shows the
// action only where something is missing.
func orAction(ok bool, action string) string {
	if ok {
		return ""
	}
	return action
}

func revision(revision int64) string {
	return strconv.FormatInt(revision, 10)
}
