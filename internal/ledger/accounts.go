package ledger

import (
	"context"
	"database/sql"
)

// AccountType names one account family, such as asset or income, with the
// side its balances normally sit on.
type AccountType struct {
	Name          string
	Label         string
	NormalBalance string
}

// Account is one chart node. Group nodes organize the tree and never take
// postings; leaves do. A NULL company means every company.
type Account struct {
	Name        string
	Label       string
	Type        string
	Parent      string
	IsGroup     bool
	Class       string
	ControlRole string
	Company     string
	Disabled    bool
	Revision    int64
}

func (s *Service) SaveAccountType(ctx context.Context, accountType AccountType) (AccountType, error) {
	if err := checkName("account type", accountType.Name); err != nil {
		return AccountType{}, err
	}
	if err := checkLabel(accountType.Label); err != nil {
		return AccountType{}, err
	}
	if accountType.NormalBalance != "debit" && accountType.NormalBalance != "credit" {
		return AccountType{}, inputErrorf("normal balance %q must be debit or credit", accountType.NormalBalance)
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_account_types (name, label, normal_balance) VALUES (?, ?, ?)`,
		accountType.Name, accountType.Label, accountType.NormalBalance)
	if err != nil {
		return AccountType{}, s.conflict(err, "account type", accountType.Name)
	}
	return accountType, nil
}

func (s *Service) GetAccountType(ctx context.Context, name string) (AccountType, error) {
	var accountType AccountType
	err := s.DB.QueryRowContext(ctx, `SELECT name, label, normal_balance FROM athena_account_types WHERE name = ?`, name).
		Scan(&accountType.Name, &accountType.Label, &accountType.NormalBalance)
	if err != nil {
		return AccountType{}, notFound(err, "account type", name)
	}
	return accountType, nil
}

func (s *Service) ListAccountTypes(ctx context.Context) ([]AccountType, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, label, normal_balance FROM athena_account_types ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountType
	for rows.Next() {
		var accountType AccountType
		if err := rows.Scan(&accountType.Name, &accountType.Label, &accountType.NormalBalance); err != nil {
			return nil, err
		}
		out = append(out, accountType)
	}
	return out, rows.Err()
}

func (s *Service) SaveAccount(ctx context.Context, account Account, expected int64) (Account, error) {
	if err := checkName("account", account.Name); err != nil {
		return Account{}, err
	}
	if err := checkLabel(account.Label); err != nil {
		return Account{}, err
	}
	if _, err := s.GetAccountType(ctx, account.Type); err != nil {
		return Account{}, err
	}
	if account.Company != "" {
		if _, err := s.GetCompany(ctx, account.Company); err != nil {
			return Account{}, err
		}
	}
	if account.Parent != "" {
		parent, err := s.GetAccount(ctx, account.Parent)
		if err != nil {
			return Account{}, err
		}
		if parent.Company != "" && parent.Company != account.Company {
			return Account{}, inputErrorf("parent %q belongs to another company", account.Parent)
		}
		if account.Company == "" && parent.Company != "" {
			return Account{}, inputErrorf("a global account cannot sit under company account %q", account.Parent)
		}
		if !parent.IsGroup {
			return Account{}, inputErrorf("parent %q is not a group", account.Parent)
		}
		if err := s.checkAccountCycle(ctx, account.Name, account.Parent); err != nil {
			return Account{}, err
		}
	}
	if !account.IsGroup {
		var children int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_accounts WHERE parent_name = ?`, account.Name).Scan(&children); err != nil {
			return Account{}, err
		}
		if children > 0 {
			return Account{}, inputErrorf("account %q has children and must be a group", account.Name)
		}
	}
	var out Account
	err := s.withTransaction(ctx, func(tx *sql.Tx) error {
		svc := &Service{DB: tx, Dialect: s.Dialect}
		if expected == 0 {
			path, depth, err := treePathFor(ctx, tx, "athena_accounts", account.Name, account.Parent)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO athena_accounts (name, label, account_type, parent_name, tree_path, tree_depth, is_group, class, control_role, company_name, disabled) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				account.Name, account.Label, account.Type, nullText(account.Parent), path, depth, account.IsGroup, nullText(account.Class), nullText(account.ControlRole), nullText(account.Company), account.Disabled)
			if err != nil {
				return svc.conflict(err, "account", account.Name)
			}
			account.Revision = 1
			out = account
			return nil
		}
		var oldParent sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT parent_name FROM athena_accounts WHERE name = ?`, account.Name).Scan(&oldParent); err != nil {
			return notFound(err, "account", account.Name)
		}
		result, err := tx.ExecContext(ctx, `UPDATE athena_accounts SET label = ?, account_type = ?, parent_name = ?, is_group = ?, class = ?, control_role = ?, company_name = ?, disabled = ?, revision = revision + 1 WHERE name = ? AND revision = ?`,
			account.Label, account.Type, nullText(account.Parent), account.IsGroup, nullText(account.Class), nullText(account.ControlRole), nullText(account.Company), account.Disabled, account.Name, expected)
		if err != nil {
			return err
		}
		if err := svc.cas(ctx, result, "athena_accounts", "name", account.Name, expected, &account.Revision); err != nil {
			return err
		}
		if oldParent.String != account.Parent {
			if err := refreshSubtreePaths(ctx, tx, "athena_accounts", account.Name); err != nil {
				return err
			}
		}
		out = account
		return nil
	})
	return out, err
}

func (s *Service) checkAccountCycle(ctx context.Context, name, parent string) error {
	seen := map[string]bool{name: true, parent: true}
	next := parent
	for {
		var up sql.NullString
		if err := s.DB.QueryRowContext(ctx, `SELECT parent_name FROM athena_accounts WHERE name = ?`, next).Scan(&up); err != nil {
			return err
		}
		if up.String == "" {
			return nil
		}
		if seen[up.String] {
			return inputErrorf("account %q would close a cycle", name)
		}
		seen[up.String] = true
		next = up.String
	}
}

func (s *Service) GetAccount(ctx context.Context, name string) (Account, error) {
	var account Account
	var parent, class, role, company sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT name, label, account_type, parent_name, is_group, class, control_role, company_name, disabled, revision FROM athena_accounts WHERE name = ?`, name).
		Scan(&account.Name, &account.Label, &account.Type, &parent, &account.IsGroup, &class, &role, &company, &account.Disabled, &account.Revision)
	if err != nil {
		return Account{}, notFound(err, "account", name)
	}
	account.Parent, account.Class, account.ControlRole, account.Company = parent.String, class.String, role.String, company.String
	return account, nil
}

func (s *Service) ListAccounts(ctx context.Context, company string) ([]Account, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, label, account_type, parent_name, is_group, class, control_role, company_name, disabled, revision FROM athena_accounts WHERE company_name IS NULL OR company_name = ? ORDER BY name`, company)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var account Account
		var parent, class, role, companyName sql.NullString
		if err := rows.Scan(&account.Name, &account.Label, &account.Type, &parent, &account.IsGroup, &class, &role, &companyName, &account.Disabled, &account.Revision); err != nil {
			return nil, err
		}
		account.Parent, account.Class, account.ControlRole, account.Company = parent.String, class.String, role.String, companyName.String
		out = append(out, account)
	}
	return out, rows.Err()
}
