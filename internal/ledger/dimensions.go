package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
)

// Dimension names one analysis axis, such as a project or a region. Members
// are the permitted values; MandatoryFor names the accounts that require a
// value on every posting line. Validity bounds the dates the definition
// applies to, empty for always.
type Dimension struct {
	Name         string
	Label        string
	Members      []string
	MandatoryFor []string
	ValidFrom    string
	ValidTo      string
	Disabled     bool
	Revision     int64
}

func (s *Service) SaveDimension(ctx context.Context, dimension Dimension, expected int64) (Dimension, error) {
	if err := checkName("dimension", dimension.Name); err != nil {
		return Dimension{}, err
	}
	if err := checkLabel(dimension.Label); err != nil {
		return Dimension{}, err
	}
	for _, member := range dimension.Members {
		if member == "" {
			return Dimension{}, inputErrorf("dimension members must not be empty")
		}
	}
	for _, account := range dimension.MandatoryFor {
		if _, err := s.GetAccount(ctx, account); err != nil {
			return Dimension{}, err
		}
	}
	if err := checkDateRange(dimension.ValidFrom, dimension.ValidTo); err != nil {
		return Dimension{}, err
	}
	members, err := json.Marshal(nonNilStrings(dimension.Members))
	if err != nil {
		return Dimension{}, err
	}
	mandatory, err := json.Marshal(nonNilStrings(dimension.MandatoryFor))
	if err != nil {
		return Dimension{}, err
	}
	if expected == 0 {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_dimensions (name, label, members, mandatory_for, valid_from, valid_to, disabled) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			dimension.Name, dimension.Label, members, mandatory, nullText(dimension.ValidFrom), nullText(dimension.ValidTo), dimension.Disabled)
		if err != nil {
			return Dimension{}, s.conflict(err, "dimension", dimension.Name)
		}
		dimension.Revision = 1
		return dimension, nil
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_dimensions SET label = ?, members = ?, mandatory_for = ?, valid_from = ?, valid_to = ?, disabled = ?, revision = revision + 1 WHERE name = ? AND revision = ?`,
		dimension.Label, members, mandatory, nullText(dimension.ValidFrom), nullText(dimension.ValidTo), dimension.Disabled, dimension.Name, expected)
	if err != nil {
		return Dimension{}, err
	}
	return dimension, s.cas(ctx, result, "athena_dimensions", "name", dimension.Name, expected, &dimension.Revision)
}

func (s *Service) GetDimension(ctx context.Context, name string) (Dimension, error) {
	var dimension Dimension
	var members, mandatory []byte
	var from, to sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT name, label, members, mandatory_for, valid_from, valid_to, disabled, revision FROM athena_dimensions WHERE name = ?`, name).
		Scan(&dimension.Name, &dimension.Label, &members, &mandatory, &from, &to, &dimension.Disabled, &dimension.Revision)
	if err != nil {
		return Dimension{}, notFound(err, "dimension", name)
	}
	if err := json.Unmarshal(members, &dimension.Members); err != nil {
		return Dimension{}, err
	}
	if err := json.Unmarshal(mandatory, &dimension.MandatoryFor); err != nil {
		return Dimension{}, err
	}
	dimension.ValidFrom, dimension.ValidTo = scanNullDate(from), scanNullDate(to)
	return dimension, nil
}

func (s *Service) ListDimensions(ctx context.Context) ([]Dimension, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, label, members, mandatory_for, valid_from, valid_to, disabled, revision FROM athena_dimensions ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dimension
	for rows.Next() {
		var dimension Dimension
		var members, mandatory []byte
		var from, to sql.NullTime
		if err := rows.Scan(&dimension.Name, &dimension.Label, &members, &mandatory, &from, &to, &dimension.Disabled, &dimension.Revision); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(members, &dimension.Members); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(mandatory, &dimension.MandatoryFor); err != nil {
			return nil, err
		}
		dimension.ValidFrom, dimension.ValidTo = scanNullDate(from), scanNullDate(to)
		out = append(out, dimension)
	}
	return out, rows.Err()
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
