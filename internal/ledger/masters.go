package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Company tops the organisation tree. Postings name it on every row.
type Company struct {
	Name         string
	Label        string
	BaseCurrency string
	Disabled     bool
	Revision     int64
}

// Branch belongs to one company. Branches nest for grouping.
type Branch struct {
	Name     string
	Label    string
	Company  string
	Parent   string
	IsGroup  bool
	Disabled bool
	Revision int64
}

// Department belongs to one company and optionally one branch. Departments nest.
type Department struct {
	Name     string
	Label    string
	Company  string
	Branch   string
	Parent   string
	IsGroup  bool
	Disabled bool
	Revision int64
}

// CostCentre belongs to one company. Cost centres nest.
type CostCentre struct {
	Name     string
	Label    string
	Company  string
	Parent   string
	IsGroup  bool
	Disabled bool
	Revision int64
}

func (s *Service) SaveCompany(ctx context.Context, company Company, expected int64) (Company, error) {
	if err := checkName("company", company.Name); err != nil {
		return Company{}, err
	}
	if err := checkLabel(company.Label); err != nil {
		return Company{}, err
	}
	if company.BaseCurrency != "" {
		if _, err := s.GetCurrency(ctx, company.BaseCurrency); err != nil {
			return Company{}, err
		}
	}
	if expected == 0 {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_companies (name, label, base_currency, disabled) VALUES (?, ?, ?, ?)`,
			company.Name, company.Label, nullText(company.BaseCurrency), company.Disabled)
		if err != nil {
			return Company{}, s.conflict(err, "company", company.Name)
		}
		company.Revision = 1
		return company, nil
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_companies SET label = ?, base_currency = ?, disabled = ?, revision = revision + 1 WHERE name = ? AND revision = ?`,
		company.Label, nullText(company.BaseCurrency), company.Disabled, company.Name, expected)
	if err != nil {
		return Company{}, err
	}
	return company, s.cas(ctx, result, "athena_companies", "name", company.Name, expected, &company.Revision)
}

func (s *Service) GetCompany(ctx context.Context, name string) (Company, error) {
	var company Company
	var base sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT name, label, base_currency, disabled, revision FROM athena_companies WHERE name = ?`, name).
		Scan(&company.Name, &company.Label, &base, &company.Disabled, &company.Revision)
	if err != nil {
		return Company{}, notFound(err, "company", name)
	}
	company.BaseCurrency = base.String
	return company, nil
}

func (s *Service) ListCompanies(ctx context.Context) ([]Company, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, label, base_currency, disabled, revision FROM athena_companies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Company
	for rows.Next() {
		var company Company
		var base sql.NullString
		if err := rows.Scan(&company.Name, &company.Label, &base, &company.Disabled, &company.Revision); err != nil {
			return nil, err
		}
		company.BaseCurrency = base.String
		out = append(out, company)
	}
	return out, rows.Err()
}

// SaveBranch stores a branch after checking its company, parent chain and
// group flag. Expected revision 0 creates; anything else updates with a
// compare-and-swap.
func (s *Service) SaveBranch(ctx context.Context, branch Branch, expected int64) (Branch, error) {
	if err := checkName("branch", branch.Name); err != nil {
		return Branch{}, err
	}
	if err := checkLabel(branch.Label); err != nil {
		return Branch{}, err
	}
	if _, err := s.GetCompany(ctx, branch.Company); err != nil {
		return Branch{}, err
	}
	if err := s.checkNested(ctx, "athena_branches", "branch", branch.Name, branch.Company, branch.Parent, branch.IsGroup); err != nil {
		return Branch{}, err
	}
	if expected == 0 {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_branches (name, label, company_name, parent_name, is_group, disabled) VALUES (?, ?, ?, ?, ?, ?)`,
			branch.Name, branch.Label, branch.Company, nullText(branch.Parent), branch.IsGroup, branch.Disabled)
		if err != nil {
			return Branch{}, s.conflict(err, "branch", branch.Name)
		}
		branch.Revision = 1
		return branch, nil
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_branches SET label = ?, company_name = ?, parent_name = ?, is_group = ?, disabled = ?, revision = revision + 1 WHERE name = ? AND revision = ?`,
		branch.Label, branch.Company, nullText(branch.Parent), branch.IsGroup, branch.Disabled, branch.Name, expected)
	if err != nil {
		return Branch{}, err
	}
	return branch, s.cas(ctx, result, "athena_branches", "name", branch.Name, expected, &branch.Revision)
}

func (s *Service) GetBranch(ctx context.Context, name string) (Branch, error) {
	var branch Branch
	var parent sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT name, label, company_name, parent_name, is_group, disabled, revision FROM athena_branches WHERE name = ?`, name).
		Scan(&branch.Name, &branch.Label, &branch.Company, &parent, &branch.IsGroup, &branch.Disabled, &branch.Revision)
	if err != nil {
		return Branch{}, notFound(err, "branch", name)
	}
	branch.Parent = parent.String
	return branch, nil
}

func (s *Service) ListBranches(ctx context.Context, company string) ([]Branch, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, label, company_name, parent_name, is_group, disabled, revision FROM athena_branches WHERE company_name = ? ORDER BY name`, company)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Branch
	for rows.Next() {
		var branch Branch
		var parent sql.NullString
		if err := rows.Scan(&branch.Name, &branch.Label, &branch.Company, &parent, &branch.IsGroup, &branch.Disabled, &branch.Revision); err != nil {
			return nil, err
		}
		branch.Parent = parent.String
		out = append(out, branch)
	}
	return out, rows.Err()
}

func (s *Service) SaveDepartment(ctx context.Context, department Department, expected int64) (Department, error) {
	if err := checkName("department", department.Name); err != nil {
		return Department{}, err
	}
	if err := checkLabel(department.Label); err != nil {
		return Department{}, err
	}
	if _, err := s.GetCompany(ctx, department.Company); err != nil {
		return Department{}, err
	}
	if department.Branch != "" {
		branch, err := s.GetBranch(ctx, department.Branch)
		if err != nil {
			return Department{}, err
		}
		if branch.Company != department.Company {
			return Department{}, inputErrorf("branch %q belongs to another company", department.Branch)
		}
	}
	if err := s.checkNested(ctx, "athena_departments", "department", department.Name, department.Company, department.Parent, department.IsGroup); err != nil {
		return Department{}, err
	}
	if expected == 0 {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_departments (name, label, company_name, branch_name, parent_name, is_group, disabled) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			department.Name, department.Label, department.Company, nullText(department.Branch), nullText(department.Parent), department.IsGroup, department.Disabled)
		if err != nil {
			return Department{}, s.conflict(err, "department", department.Name)
		}
		department.Revision = 1
		return department, nil
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_departments SET label = ?, company_name = ?, branch_name = ?, parent_name = ?, is_group = ?, disabled = ?, revision = revision + 1 WHERE name = ? AND revision = ?`,
		department.Label, department.Company, nullText(department.Branch), nullText(department.Parent), department.IsGroup, department.Disabled, department.Name, expected)
	if err != nil {
		return Department{}, err
	}
	return department, s.cas(ctx, result, "athena_departments", "name", department.Name, expected, &department.Revision)
}

func (s *Service) GetDepartment(ctx context.Context, name string) (Department, error) {
	var department Department
	var branch, parent sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT name, label, company_name, branch_name, parent_name, is_group, disabled, revision FROM athena_departments WHERE name = ?`, name).
		Scan(&department.Name, &department.Label, &department.Company, &branch, &parent, &department.IsGroup, &department.Disabled, &department.Revision)
	if err != nil {
		return Department{}, notFound(err, "department", name)
	}
	department.Branch, department.Parent = branch.String, parent.String
	return department, nil
}

func (s *Service) ListDepartments(ctx context.Context, company string) ([]Department, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, label, company_name, branch_name, parent_name, is_group, disabled, revision FROM athena_departments WHERE company_name = ? ORDER BY name`, company)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Department
	for rows.Next() {
		var department Department
		var branch, parent sql.NullString
		if err := rows.Scan(&department.Name, &department.Label, &department.Company, &branch, &parent, &department.IsGroup, &department.Disabled, &department.Revision); err != nil {
			return nil, err
		}
		department.Branch, department.Parent = branch.String, parent.String
		out = append(out, department)
	}
	return out, rows.Err()
}

func (s *Service) SaveCostCentre(ctx context.Context, centre CostCentre, expected int64) (CostCentre, error) {
	if err := checkName("cost centre", centre.Name); err != nil {
		return CostCentre{}, err
	}
	if err := checkLabel(centre.Label); err != nil {
		return CostCentre{}, err
	}
	if _, err := s.GetCompany(ctx, centre.Company); err != nil {
		return CostCentre{}, err
	}
	if err := s.checkNested(ctx, "athena_cost_centres", "cost centre", centre.Name, centre.Company, centre.Parent, centre.IsGroup); err != nil {
		return CostCentre{}, err
	}
	if expected == 0 {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_cost_centres (name, label, company_name, parent_name, is_group, disabled) VALUES (?, ?, ?, ?, ?, ?)`,
			centre.Name, centre.Label, centre.Company, nullText(centre.Parent), centre.IsGroup, centre.Disabled)
		if err != nil {
			return CostCentre{}, s.conflict(err, "cost centre", centre.Name)
		}
		centre.Revision = 1
		return centre, nil
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_cost_centres SET label = ?, company_name = ?, parent_name = ?, is_group = ?, disabled = ?, revision = revision + 1 WHERE name = ? AND revision = ?`,
		centre.Label, centre.Company, nullText(centre.Parent), centre.IsGroup, centre.Disabled, centre.Name, expected)
	if err != nil {
		return CostCentre{}, err
	}
	return centre, s.cas(ctx, result, "athena_cost_centres", "name", centre.Name, expected, &centre.Revision)
}

func (s *Service) GetCostCentre(ctx context.Context, name string) (CostCentre, error) {
	var centre CostCentre
	var parent sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT name, label, company_name, parent_name, is_group, disabled, revision FROM athena_cost_centres WHERE name = ?`, name).
		Scan(&centre.Name, &centre.Label, &centre.Company, &parent, &centre.IsGroup, &centre.Disabled, &centre.Revision)
	if err != nil {
		return CostCentre{}, notFound(err, "cost centre", name)
	}
	centre.Parent = parent.String
	return centre, nil
}

func (s *Service) ListCostCentres(ctx context.Context, company string) ([]CostCentre, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, label, company_name, parent_name, is_group, disabled, revision FROM athena_cost_centres WHERE company_name = ? ORDER BY name`, company)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CostCentre
	for rows.Next() {
		var centre CostCentre
		var parent sql.NullString
		if err := rows.Scan(&centre.Name, &centre.Label, &centre.Company, &parent, &centre.IsGroup, &centre.Disabled, &centre.Revision); err != nil {
			return nil, err
		}
		centre.Parent = parent.String
		out = append(out, centre)
	}
	return out, rows.Err()
}

// checkNested validates a parent link inside one master table: the parent
// exists in the same company, is a group, and does not lead back to the node.
// A node with children must itself be a group.
func (s *Service) checkNested(ctx context.Context, table, what, name, company, parent string, isGroup bool) error {
	if parent != "" {
		var companyName string
		var parentIsGroup bool
		var grandparent sql.NullString
		err := s.DB.QueryRowContext(ctx, "SELECT company_name, is_group, parent_name FROM "+table+" WHERE name = ?", parent).
			Scan(&companyName, &parentIsGroup, &grandparent)
		if err != nil {
			return notFound(err, what, parent)
		}
		if companyName != company {
			return inputErrorf("%s %q belongs to another company", what, parent)
		}
		if !parentIsGroup {
			return inputErrorf("%s %q is not a group", what, parent)
		}
		seen := map[string]bool{name: true, parent: true}
		for next := grandparent.String; next != ""; {
			if seen[next] {
				return inputErrorf("%s %q would close a cycle", what, name)
			}
			seen[next] = true
			var up sql.NullString
			if err := s.DB.QueryRowContext(ctx, "SELECT parent_name FROM "+table+" WHERE name = ?", next).Scan(&up); err != nil {
				return err
			}
			next = up.String
		}
	}
	if !isGroup {
		var children int
		if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE parent_name = ?", name).Scan(&children); err != nil {
			return err
		}
		if children > 0 {
			return inputErrorf("%s %q has children and must be a group", what, name)
		}
	}
	return nil
}

func nullText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func notFound(err error, what, name string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s %q", ErrNotFound, what, name)
	}
	return err
}

// cas turns an update result into a conflict or a new revision.
func (s *Service) cas(ctx context.Context, result sql.Result, table, keyColumn, key string, expected int64, revision *int64) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		*revision = expected + 1
		return nil
	}
	var exists bool
	if err := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE "+keyColumn+" = ?)", key).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %q", ErrNotFound, key)
	}
	return fmt.Errorf("%w: expected revision %d", ErrConflict, expected)
}
