package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"athena-erp/internal/types"
)

// ErrCrossFamily reports a conversion whose units sit in different UoM
// families. Families never convert into each other (R13.5).
var ErrCrossFamily = errors.New("ledger: units are in different UoM families")

// UoMFamily names a family of convertible units and its base unit, the unit
// every conversion runs through. Conversions hold base units per one of the
// unit, so the base unit itself converts at 1 and needs no row.
type UoMFamily struct {
	Name     string
	Label    string
	BaseUnit string
	Disabled bool
	Revision int64
}

// UoMConversion converts one Unit into base units at Factor. An empty Item
// is the family default every item without its own row uses; a filled Item
// overrides the default for that item only.
type UoMConversion struct {
	Family   string
	Unit     string
	Item     string
	Factor   types.Decimal
	Scale    int
	Disabled bool
	Revision int64
}

func (s *Service) SaveUoMFamily(ctx context.Context, family UoMFamily, expected int64) (UoMFamily, error) {
	if err := checkName("uom family", family.Name); err != nil {
		return UoMFamily{}, err
	}
	if err := checkLabel(family.Label); err != nil {
		return UoMFamily{}, err
	}
	if !types.ValidUoM(family.BaseUnit) {
		return UoMFamily{}, inputErrorf("base unit %q is not a unit of measure", family.BaseUnit)
	}
	if expected == 0 {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_uom_families (name, label, base_unit, disabled) VALUES (?, ?, ?, ?)`,
			family.Name, family.Label, family.BaseUnit, family.Disabled)
		if err != nil {
			return UoMFamily{}, s.conflict(err, "uom family", family.Name)
		}
		family.Revision = 1
		return family, nil
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_uom_families SET label = ?, base_unit = ?, disabled = ?, revision = revision + 1 WHERE name = ? AND revision = ?`,
		family.Label, family.BaseUnit, family.Disabled, family.Name, expected)
	if err != nil {
		return UoMFamily{}, err
	}
	return family, s.cas(ctx, result, "athena_uom_families", "name", family.Name, expected, &family.Revision)
}

func (s *Service) GetUoMFamily(ctx context.Context, name string) (UoMFamily, error) {
	var family UoMFamily
	err := s.DB.QueryRowContext(ctx, `SELECT name, label, base_unit, disabled, revision FROM athena_uom_families WHERE name = ?`, name).
		Scan(&family.Name, &family.Label, &family.BaseUnit, &family.Disabled, &family.Revision)
	if err != nil {
		return UoMFamily{}, notFound(err, "uom family", name)
	}
	return family, nil
}

func (s *Service) ListUoMFamilies(ctx context.Context) ([]UoMFamily, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, label, base_unit, disabled, revision FROM athena_uom_families ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UoMFamily
	for rows.Next() {
		var family UoMFamily
		if err := rows.Scan(&family.Name, &family.Label, &family.BaseUnit, &family.Disabled, &family.Revision); err != nil {
			return nil, err
		}
		out = append(out, family)
	}
	return out, rows.Err()
}

func (s *Service) SaveUoMConversion(ctx context.Context, conversion UoMConversion, expected int64) (UoMConversion, error) {
	family, err := s.GetUoMFamily(ctx, conversion.Family)
	if err != nil {
		return UoMConversion{}, err
	}
	if !types.ValidUoM(conversion.Unit) {
		return UoMConversion{}, inputErrorf("unit %q is not a unit of measure", conversion.Unit)
	}
	if conversion.Unit == family.BaseUnit {
		return UoMConversion{}, inputErrorf("unit %q is the base unit of family %q; it converts at 1 and needs no row", conversion.Unit, conversion.Family)
	}
	if conversion.Factor.Sign() <= 0 {
		return UoMConversion{}, inputErrorf("a conversion factor must be above zero")
	}
	if conversion.Scale < 0 || conversion.Scale > 12 {
		return UoMConversion{}, inputErrorf("the conversion scale is 0 to 12")
	}
	if conversion.Item != "" {
		if err := checkName("item", conversion.Item); err != nil {
			return UoMConversion{}, err
		}
	}
	if expected == 0 {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_uom_conversions (family_name, unit, item_key, factor, scale, disabled) VALUES (?, ?, ?, ?, ?, ?)`,
			conversion.Family, conversion.Unit, conversion.Item, conversion.Factor.String(), conversion.Scale, conversion.Disabled)
		if err != nil {
			return UoMConversion{}, s.conflict(err, "uom conversion", conversion.Family+"/"+conversion.Unit)
		}
		conversion.Revision = 1
		return conversion, nil
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_uom_conversions SET factor = ?, scale = ?, disabled = ?, revision = revision + 1 WHERE family_name = ? AND unit = ? AND item_key = ? AND revision = ?`,
		conversion.Factor.String(), conversion.Scale, conversion.Disabled, conversion.Family, conversion.Unit, conversion.Item, expected)
	if err != nil {
		return UoMConversion{}, err
	}
	where := "family_name = ? AND unit = ? AND item_key = ?"
	key := conversion.Family + "/" + conversion.Unit
	if conversion.Item != "" {
		key += " for " + conversion.Item
	}
	return conversion, s.casWhere(ctx, result, "athena_uom_conversions", where, []any{conversion.Family, conversion.Unit, conversion.Item}, key, expected, &conversion.Revision)
}

func (s *Service) GetUoMConversion(ctx context.Context, family, unit, item string) (UoMConversion, error) {
	var conversion UoMConversion
	var factorText string
	err := s.DB.QueryRowContext(ctx, `SELECT family_name, unit, item_key, factor, scale, disabled, revision FROM athena_uom_conversions WHERE family_name = ? AND unit = ? AND item_key = ?`,
		family, unit, item).Scan(&conversion.Family, &conversion.Unit, &conversion.Item, &factorText, &conversion.Scale, &conversion.Disabled, &conversion.Revision)
	if err != nil {
		return UoMConversion{}, notFound(err, "uom conversion", family+"/"+unit)
	}
	factor, err := types.ParseDecimal(factorText)
	if err != nil {
		return UoMConversion{}, err
	}
	conversion.Factor = factor
	return conversion, nil
}

func (s *Service) ListUoMConversions(ctx context.Context, family string) ([]UoMConversion, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT family_name, unit, item_key, factor, scale, disabled, revision FROM athena_uom_conversions WHERE family_name = ? ORDER BY unit, item_key`, family)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UoMConversion
	for rows.Next() {
		var conversion UoMConversion
		var factorText string
		if err := rows.Scan(&conversion.Family, &conversion.Unit, &conversion.Item, &factorText, &conversion.Scale, &conversion.Disabled, &conversion.Revision); err != nil {
			return nil, err
		}
		factor, err := types.ParseDecimal(factorText)
		if err != nil {
			return nil, err
		}
		conversion.Factor = factor
		out = append(out, conversion)
	}
	return out, rows.Err()
}

// casWhere is cas for rows a single column does not identify: the update
// already ran, and where with args finds the row again to tell a miss from
// a stale revision.
func (s *Service) casWhere(ctx context.Context, result sql.Result, table, where string, args []any, key string, expected int64, revision *int64) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		*revision = expected + 1
		return nil
	}
	var exists bool
	if err := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE "+where+")", args...).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %q", ErrNotFound, key)
	}
	return fmt.Errorf("%w: expected revision %d", ErrConflict, expected)
}

// familyForUnit finds the one family a unit belongs to: a conversion row
// names it, or it is a family's base unit. A unit in no family is unknown;
// a unit in several is ambiguous and names each family.
func (s *Service) familyForUnit(ctx context.Context, unit string) (UoMFamily, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT family_name FROM athena_uom_conversions WHERE unit = ?
		UNION SELECT name FROM athena_uom_families WHERE base_unit = ?`, unit, unit)
	if err != nil {
		return UoMFamily{}, err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return UoMFamily{}, err
		}
		names = append(names, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return UoMFamily{}, err
	}
	if len(names) == 0 {
		return UoMFamily{}, fmt.Errorf("%w: unknown unit %q", ErrNotFound, unit)
	}
	if len(names) > 1 {
		sort.Strings(names)
		quoted := make([]string, len(names))
		for i, name := range names {
			quoted[i] = fmt.Sprintf("%q", name)
		}
		return UoMFamily{}, fmt.Errorf("%w: unit %q sits in families %s", ErrAmbiguous, unit, strings.Join(quoted, " and "))
	}
	family, err := s.GetUoMFamily(ctx, names[0])
	if err != nil {
		return UoMFamily{}, err
	}
	if family.Disabled {
		return UoMFamily{}, inputErrorf("uom family %q is disabled", family.Name)
	}
	return family, nil
}

// conversionRow resolves the factor row for a unit: the item override when
// one is enabled, else the family default. Disabled rows are skipped, so a
// disabled override falls back to the default; with no enabled row the unit
// has no conversion in the family.
func (s *Service) conversionRow(ctx context.Context, family, unit, item string) (UoMConversion, error) {
	if item != "" {
		row, err := s.GetUoMConversion(ctx, family, unit, item)
		if err == nil {
			if !row.Disabled {
				return row, nil
			}
		} else if !errors.Is(err, ErrNotFound) {
			return UoMConversion{}, err
		}
	}
	row, err := s.GetUoMConversion(ctx, family, unit, "")
	if err == nil {
		if !row.Disabled {
			return row, nil
		}
	} else if !errors.Is(err, ErrNotFound) {
		return UoMConversion{}, err
	}
	return UoMConversion{}, fmt.Errorf("%w: no enabled conversion for unit %q in family %q", ErrNotFound, unit, family)
}

func checkItem(item string) error {
	if item == "" {
		return nil
	}
	return checkName("item", item)
}

// ToBaseQuantity converts an entered quantity into its family's base units
// and returns the base quantity with the factor used, so a line stores the
// entered unit, the factor and the base quantity together (R13.4). A
// quantity already in base units comes back unchanged with factor 1.
func (s *Service) ToBaseQuantity(ctx context.Context, qty types.Quantity, item string, mode types.RoundingMode) (types.Quantity, types.Decimal, error) {
	if !mode.Valid() {
		return types.Quantity{}, types.Decimal{}, types.ErrRoundingMode
	}
	if err := checkItem(item); err != nil {
		return types.Quantity{}, types.Decimal{}, err
	}
	family, err := s.familyForUnit(ctx, qty.UoM)
	if err != nil {
		return types.Quantity{}, types.Decimal{}, err
	}
	if qty.UoM == family.BaseUnit {
		one, err := types.ParseDecimal("1")
		if err != nil {
			return types.Quantity{}, types.Decimal{}, err
		}
		return qty, one, nil
	}
	row, err := s.conversionRow(ctx, family.Name, qty.UoM, item)
	if err != nil {
		return types.Quantity{}, types.Decimal{}, err
	}
	base, err := qty.Convert(row.Factor, family.BaseUnit, row.Scale, mode)
	if err != nil {
		return types.Quantity{}, types.Decimal{}, err
	}
	return base, row.Factor, nil
}

// FromBaseQuantity converts base units back into a family unit. The input
// must be in base units: anything else is a caller bug when both units
// share a family, and a cross-family conversion otherwise.
func (s *Service) FromBaseQuantity(ctx context.Context, base types.Quantity, toUnit, item string, mode types.RoundingMode) (types.Quantity, error) {
	if !mode.Valid() {
		return types.Quantity{}, types.ErrRoundingMode
	}
	if err := checkItem(item); err != nil {
		return types.Quantity{}, err
	}
	if !types.ValidUoM(toUnit) {
		return types.Quantity{}, types.ErrUoM
	}
	family, err := s.familyForUnit(ctx, toUnit)
	if err != nil {
		return types.Quantity{}, err
	}
	if base.UoM != family.BaseUnit {
		baseFamily, err := s.familyForUnit(ctx, base.UoM)
		if err != nil {
			return types.Quantity{}, err
		}
		if baseFamily.Name != family.Name {
			return types.Quantity{}, fmt.Errorf("%w: %s (%s) and %s (%s)", ErrCrossFamily, base.UoM, baseFamily.Name, toUnit, family.Name)
		}
		return types.Quantity{}, inputErrorf("want base units (%s), got %s", family.BaseUnit, base.UoM)
	}
	if toUnit == family.BaseUnit {
		return base, nil
	}
	row, err := s.conversionRow(ctx, family.Name, toUnit, item)
	if err != nil {
		return types.Quantity{}, err
	}
	return base.FromBase(row.Factor, toUnit, row.Scale, mode)
}

// ConvertQuantity converts between two units of one family through base
// units, and rejects units of different families (R13.5). Converting a
// unit into itself returns the quantity unchanged.
func (s *Service) ConvertQuantity(ctx context.Context, qty types.Quantity, toUnit, item string, mode types.RoundingMode) (types.Quantity, error) {
	if !mode.Valid() {
		return types.Quantity{}, types.ErrRoundingMode
	}
	if err := checkItem(item); err != nil {
		return types.Quantity{}, err
	}
	if !types.ValidUoM(toUnit) {
		return types.Quantity{}, types.ErrUoM
	}
	fromFamily, err := s.familyForUnit(ctx, qty.UoM)
	if err != nil {
		return types.Quantity{}, err
	}
	toFamily, err := s.familyForUnit(ctx, toUnit)
	if err != nil {
		return types.Quantity{}, err
	}
	if fromFamily.Name != toFamily.Name {
		return types.Quantity{}, fmt.Errorf("%w: %s (%s) and %s (%s)", ErrCrossFamily, qty.UoM, fromFamily.Name, toUnit, toFamily.Name)
	}
	if qty.UoM == toUnit {
		return qty, nil
	}
	base, _, err := s.ToBaseQuantity(ctx, qty, item, mode)
	if err != nil {
		return types.Quantity{}, err
	}
	return s.FromBaseQuantity(ctx, base, toUnit, item, mode)
}
