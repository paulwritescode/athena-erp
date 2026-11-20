package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Period statuses. A soft close restricts normal posting; a hard close needs
// a separately approved reopen, which does not exist yet, or a correction in
// an open period at posting time.
const (
	PeriodOpen       = "open"
	PeriodSoftClosed = "soft_closed"
	PeriodHardClosed = "hard_closed"
)

// FiscalYear groups the periods of one company.
type FiscalYear struct {
	Name      string
	Label     string
	Company   string
	StartDate string
	EndDate   string
	Disabled  bool
	Revision  int64
}

// Period is one posting window in a company and book. Dates are inclusive:
// a period covers every date from StartDate to EndDate, and two periods in
// one calendar must share no date.
type Period struct {
	Company    string
	Book       string
	Name       string
	Label      string
	FiscalYear string
	StartDate  string
	EndDate    string
	Status     string
	Revision   int64
}

func checkDateRange(from, to string) error {
	if from != "" && !validDate(from) {
		return inputErrorf("%q is not a date as YYYY-MM-DD", from)
	}
	if to != "" && !validDate(to) {
		return inputErrorf("%q is not a date as YYYY-MM-DD", to)
	}
	if from != "" && to != "" && to < from {
		return inputErrorf("the end date is before the start date")
	}
	return nil
}

func validDate(value string) bool {
	if len(value) != 10 || value[4] != '-' || value[7] != '-' {
		return false
	}
	for i, c := range value {
		if i == 4 || i == 7 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	month, day := value[5:7], value[8:10]
	return month >= "01" && month <= "12" && day >= "01" && day <= "31"
}

func (s *Service) SaveFiscalYear(ctx context.Context, year FiscalYear, expected int64) (FiscalYear, error) {
	if err := checkName("fiscal year", year.Name); err != nil {
		return FiscalYear{}, err
	}
	if err := checkLabel(year.Label); err != nil {
		return FiscalYear{}, err
	}
	if _, err := s.GetCompany(ctx, year.Company); err != nil {
		return FiscalYear{}, err
	}
	if year.StartDate == "" || year.EndDate == "" {
		return FiscalYear{}, inputErrorf("a fiscal year needs a start and an end date")
	}
	if err := checkDateRange(year.StartDate, year.EndDate); err != nil {
		return FiscalYear{}, err
	}
	if expected == 0 {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_fiscal_years (name, label, company_name, start_date, end_date, disabled) VALUES (?, ?, ?, ?, ?, ?)`,
			year.Name, year.Label, year.Company, year.StartDate, year.EndDate, year.Disabled)
		if err != nil {
			return FiscalYear{}, s.conflict(err, "fiscal year", year.Name)
		}
		year.Revision = 1
		return year, nil
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_fiscal_years SET label = ?, company_name = ?, start_date = ?, end_date = ?, disabled = ?, revision = revision + 1 WHERE name = ? AND revision = ?`,
		year.Label, year.Company, year.StartDate, year.EndDate, year.Disabled, year.Name, expected)
	if err != nil {
		return FiscalYear{}, err
	}
	return year, s.cas(ctx, result, "athena_fiscal_years", "name", year.Name, expected, &year.Revision)
}

func (s *Service) ListFiscalYears(ctx context.Context, company string) ([]FiscalYear, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, label, company_name, start_date, end_date, disabled, revision FROM athena_fiscal_years WHERE company_name = ? ORDER BY start_date`, company)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FiscalYear
	for rows.Next() {
		var year FiscalYear
		var start, end time.Time
		if err := rows.Scan(&year.Name, &year.Label, &year.Company, &start, &end, &year.Disabled, &year.Revision); err != nil {
			return nil, err
		}
		year.StartDate, year.EndDate = scanDate(start), scanDate(end)
		out = append(out, year)
	}
	return out, rows.Err()
}

func (s *Service) GetFiscalYear(ctx context.Context, name string) (FiscalYear, error) {
	var year FiscalYear
	var start, end time.Time
	err := s.DB.QueryRowContext(ctx, `SELECT name, label, company_name, start_date, end_date, disabled, revision FROM athena_fiscal_years WHERE name = ?`, name).
		Scan(&year.Name, &year.Label, &year.Company, &start, &end, &year.Disabled, &year.Revision)
	if err != nil {
		return FiscalYear{}, notFound(err, "fiscal year", name)
	}
	year.StartDate, year.EndDate = scanDate(start), scanDate(end)
	return year, nil
}

// SavePeriod stores a period after checking its dates against every other
// period in its calendar. The status moves only through SetPeriodStatus.
func (s *Service) SavePeriod(ctx context.Context, period Period, expected int64) (Period, error) {
	if err := checkName("period", period.Name); err != nil {
		return Period{}, err
	}
	if err := checkLabel(period.Label); err != nil {
		return Period{}, err
	}
	if _, err := s.GetCompany(ctx, period.Company); err != nil {
		return Period{}, err
	}
	if period.Book == "" {
		period.Book = "main"
	}
	if err := checkName("book", period.Book); err != nil {
		return Period{}, err
	}
	if period.StartDate == "" || period.EndDate == "" {
		return Period{}, inputErrorf("a period needs a start and an end date")
	}
	if err := checkDateRange(period.StartDate, period.EndDate); err != nil {
		return Period{}, err
	}
	if period.FiscalYear != "" {
		year, err := s.GetFiscalYear(ctx, period.FiscalYear)
		if err != nil {
			return Period{}, err
		}
		if year.Company != period.Company {
			return Period{}, inputErrorf("fiscal year %q belongs to another company", period.FiscalYear)
		}
	}
	if err := s.checkOverlap(ctx, period); err != nil {
		return Period{}, err
	}
	if expected == 0 {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_periods (company_name, book, name, label, fiscal_year_name, start_date, end_date) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			period.Company, period.Book, period.Name, period.Label, nullText(period.FiscalYear), period.StartDate, period.EndDate)
		if err != nil {
			return Period{}, s.conflict(err, "period", period.Name)
		}
		period.Status, period.Revision = PeriodOpen, 1
		return period, nil
	}
	current, err := s.GetPeriod(ctx, period.Company, period.Book, period.Name)
	if err != nil {
		return Period{}, err
	}
	if current.Revision != expected {
		return Period{}, fmt.Errorf("%w: expected revision %d", ErrConflict, expected)
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_periods SET label = ?, fiscal_year_name = ?, start_date = ?, end_date = ?, revision = revision + 1 WHERE company_name = ? AND book = ? AND name = ? AND revision = ?`,
		period.Label, nullText(period.FiscalYear), period.StartDate, period.EndDate, period.Company, period.Book, period.Name, expected)
	if err != nil {
		return Period{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Period{}, fmt.Errorf("%w: expected revision %d", ErrConflict, expected)
	}
	period.Status = current.Status
	period.Revision = expected + 1
	return period, nil
}

// checkOverlap refuses a period that shares a date with another period in
// its calendar.
func (s *Service) checkOverlap(ctx context.Context, period Period) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, start_date, end_date FROM athena_periods WHERE company_name = ? AND book = ? AND name <> ?`,
		period.Company, period.Book, period.Name)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var start, end time.Time
		if err := rows.Scan(&name, &start, &end); err != nil {
			return err
		}
		if Overlaps(period.StartDate, period.EndDate, scanDate(start), scanDate(end)) {
			return inputErrorf("period %q overlaps %q", period.Name, name)
		}
	}
	return rows.Err()
}

// Overlaps reports whether two inclusive date ranges share a date. Dates are
// YYYY-MM-DD, so string order is date order.
func Overlaps(start1, end1, start2, end2 string) bool {
	return start1 <= end2 && start2 <= end1
}

func (s *Service) GetPeriod(ctx context.Context, company, book, name string) (Period, error) {
	var period Period
	var fiscal sql.NullString
	var start, end time.Time
	err := s.DB.QueryRowContext(ctx, `SELECT company_name, book, name, label, fiscal_year_name, start_date, end_date, status, revision FROM athena_periods WHERE company_name = ? AND book = ? AND name = ?`, company, book, name).
		Scan(&period.Company, &period.Book, &period.Name, &period.Label, &fiscal, &start, &end, &period.Status, &period.Revision)
	if err != nil {
		return Period{}, notFound(err, "period", name)
	}
	period.FiscalYear = fiscal.String
	period.StartDate, period.EndDate = scanDate(start), scanDate(end)
	return period, nil
}

func (s *Service) ListPeriods(ctx context.Context, company, book string) ([]Period, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT company_name, book, name, label, fiscal_year_name, start_date, end_date, status, revision FROM athena_periods WHERE company_name = ? AND book = ? ORDER BY start_date`, company, book)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Period
	for rows.Next() {
		var period Period
		var fiscal sql.NullString
		var start, end time.Time
		if err := rows.Scan(&period.Company, &period.Book, &period.Name, &period.Label, &fiscal, &start, &end, &period.Status, &period.Revision); err != nil {
			return nil, err
		}
		period.FiscalYear = fiscal.String
		period.StartDate, period.EndDate = scanDate(start), scanDate(end)
		out = append(out, period)
	}
	return out, rows.Err()
}

// SetPeriodStatus moves a period between open, soft-closed and hard-closed.
// A hard close never reopens: there is no approved-reopen path yet, so a
// correction belongs in an open period.
func (s *Service) SetPeriodStatus(ctx context.Context, company, book, name, status, actor string, expected int64) (Period, error) {
	switch status {
	case PeriodOpen, PeriodSoftClosed, PeriodHardClosed:
	default:
		return Period{}, inputErrorf("status %q must be open, soft_closed or hard_closed", status)
	}
	current, err := s.GetPeriod(ctx, company, book, name)
	if err != nil {
		return Period{}, err
	}
	if current.Revision != expected {
		return Period{}, fmt.Errorf("%w: expected revision %d", ErrConflict, expected)
	}
	if current.Status == status {
		return current, nil
	}
	if current.Status == PeriodHardClosed {
		return Period{}, inputErrorf("period %q is hard-closed; reopening needs a separately approved reopen, so post a correction in an open period", name)
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_periods SET status = ?, status_actor = ?, status_at = CURRENT_TIMESTAMP, revision = revision + 1 WHERE company_name = ? AND book = ? AND name = ? AND revision = ?`,
		status, actor, company, book, name, expected)
	if err != nil {
		return Period{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Period{}, fmt.Errorf("%w: expected revision %d", ErrConflict, expected)
	}
	current.Status, current.Revision = status, expected+1
	return current, nil
}

// PeriodFor returns the period containing date. Non-overlap keeps it to one.
func (s *Service) PeriodFor(ctx context.Context, company, book, date string) (Period, error) {
	if !validDate(date) {
		return Period{}, inputErrorf("%q is not a date as YYYY-MM-DD", date)
	}
	periods, err := s.ListPeriods(ctx, company, book)
	if err != nil {
		return Period{}, err
	}
	for _, period := range periods {
		if period.StartDate <= date && date <= period.EndDate {
			return period, nil
		}
	}
	return Period{}, fmt.Errorf("%w: no period covers %s", ErrNotFound, date)
}
