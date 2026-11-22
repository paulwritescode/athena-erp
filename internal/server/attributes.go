package server

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/types"
)

// compilePattern anchors a field pattern so it must match the whole value.
// web/src/fieldRules.ts builds the same anchored expression.
func compilePattern(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile("^(?:" + pattern + ")$")
}

// moneyCurrency returns the field's currency at its scale. Rounding mode is
// irrelevant because values that need rounding are rejected, never rounded.
func moneyCurrency(field recordTypeField) (types.Currency, error) {
	if field.Scale == nil {
		return types.Currency{}, errors.New("money field has no scale")
	}
	return types.NewCurrency(field.Currency, *field.Scale, types.HalfUp)
}

// moneyMinor converts an amount written in major units ("12.30") to integer
// minor units. It fails when the amount has more decimal places than the
// scale or does not fit 64 bits.
func moneyMinor(field recordTypeField, text string) (int64, error) {
	if !moneyPattern.MatchString(text) {
		return 0, errors.New("not an amount")
	}
	d, err := types.ParseDecimal(text)
	if err != nil {
		return 0, err
	}
	cur, err := moneyCurrency(field)
	if err != nil {
		return 0, err
	}
	money, rounding, err := types.MoneyFromDecimal(d, cur)
	if err != nil {
		return 0, err
	}
	if rounding.Sign() != 0 {
		return 0, types.ErrRounding
	}
	return money.Minor, nil
}

// moneyText formats stored minor units in major units at the field's scale.
func moneyText(field recordTypeField, minor int64) string {
	cur, err := moneyCurrency(field)
	if err != nil {
		return fmt.Sprint(minor)
	}
	return types.NewMoney(minor, cur).Decimal().String()
}

// defaultArgument converts a field's literal default to a database argument.
func defaultArgument(field recordTypeField) (any, error) {
	if field.Type == "Checkbox" {
		switch field.Default {
		case "true", "false":
			return field.Default == "true", nil
		}
		return nil, invalid(fmt.Sprintf("%s: the default must be true or false", field.Label))
	}
	arg, err := fieldArgument(field, &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: field.Default}})
	if err != nil {
		return nil, invalid(fmt.Sprintf("%s: the default is not valid: %s", field.Label, strings.TrimPrefix(connectMessage(err), field.Label+" ")))
	}
	return arg, nil
}

// checkRange applies the field's minimum and maximum to a value that already
// passed its type check.
func checkRange(field recordTypeField, text string) error {
	if field.Min != "" && compareBound(field, text, field.Min) < 0 {
		return invalid(fmt.Sprintf("%s must be at least %s", field.Label, field.Min))
	}
	if field.Max != "" && compareBound(field, text, field.Max) > 0 {
		return invalid(fmt.Sprintf("%s must be at most %s", field.Label, field.Max))
	}
	return nil
}

// enumLiteral renders a list of choices as SQL string literals. Choices match
// enumValuePattern, so they hold no quote or backslash; the escaping is a
// second guard.
func enumLiteral(values []enumValue) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = "'" + strings.NewReplacer("'", "''", `\`, `\\`).Replace(v.Value) + "'"
	}
	return strings.Join(parts, ",")
}

func sameEnumValues(a, b []enumValue) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Value != b[i].Value {
			return false
		}
	}
	return true
}

// parseEffectiveRange validates an end-exclusive effective range written as
// YYYY-MM-DD dates and returns the database arguments, nil for an open side.
// An empty range is fully open. Equal bounds are empty, so from must sort
// strictly before to.
func parseEffectiveRange(from, to string) (any, any, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" && to == "" {
		return nil, nil, nil
	}
	if from != "" {
		if _, err := time.Parse(time.DateOnly, from); err != nil {
			return nil, nil, invalid("effective from must be a date as YYYY-MM-DD")
		}
	}
	if to != "" {
		if _, err := time.Parse(time.DateOnly, to); err != nil {
			return nil, nil, invalid("effective to must be a date as YYYY-MM-DD")
		}
	}
	if from != "" && to != "" && from >= to {
		return nil, nil, invalid("effective from must be before effective to")
	}
	var fromArg, toArg any
	if from != "" {
		fromArg = from
	}
	if to != "" {
		toArg = to
	}
	return fromArg, toArg, nil
}

// parseAsOf validates an "as of" date for effective-dated lists and lookups.
func parseAsOf(asOf string) (string, error) {
	asOf = strings.TrimSpace(asOf)
	if _, err := time.Parse(time.DateOnly, asOf); err != nil {
		return "", invalid("as of must be a date as YYYY-MM-DD")
	}
	return asOf, nil
}
