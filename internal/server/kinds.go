package server

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"athena-erp/internal/store"
	"athena-erp/internal/types"
)

// Record kinds (spec 2). Only these four can be created from the desk, the
// CLI or the API. Ledger, balance, log, snapshot and meta Record Types belong
// to the kernel and arrive with the phases that own them.
const (
	kindMaster   = "master"
	kindDocument = "document"
	kindChild    = "child"
	kindSetting  = "setting"
)

// refuseKernelKind rejects writes to kernel-owned kinds. Ledger rows are
// written by their owning service (LedgerService for ledger tables), never
// through RecordService. No such definition can exist yet: validateKind
// refuses these kinds from the API, and kernel-owned ledger masters live in
// athena_ tables outside Record Types (internal/ledger).
func refuseKernelKind(kind string) error {
	if kernelKinds[kind] {
		return invalid(fmt.Sprintf("the kind %q belongs to the kernel and cannot be written here", kind))
	}
	return nil
}

var (
	kernelKinds     = map[string]bool{"ledger": true, "balance": true, "log": true, "snapshot": true, "meta": true}
	documentClasses = map[string]bool{"posting": true, "non_posting": true, "work": true, "batch": true}
)

// normalizeKind reads an empty kind as master, the kind every Record Type had
// before kinds existed.
func normalizeKind(kind string) string {
	if kind == "" {
		return kindMaster
	}
	return kind
}

func (d recordTypeDefinition) kind() string { return normalizeKind(d.Kind) }

// systemColumns lists the columns the kernel adds to a Record Type table
// besides id, revision, created_at and updated_at (spec 24.1). Their values
// come from the kernel, never from a request. The spec's version field is the
// revision column.
func systemColumns(kind string) []store.Column {
	actor, _ := dialect.ColumnType("Text")
	reference, _ := dialect.ColumnType("Link")
	columns := []store.Column{
		{Name: "created_by", Type: actor},
		{Name: "modified_by", Type: actor},
		{Name: "def_version", Type: dialect.BigIntType()},
		{Name: "request_id", Type: reference},
	}
	switch kind {
	case kindMaster:
		columns = append(columns, store.Column{Name: "owner", Type: actor}, store.Column{Name: "deleted_at", Type: dialect.TimestampType()})
	case kindDocument:
		columns = append(columns, store.Column{Name: "owner", Type: actor})
	case kindChild:
		columns = append(columns, store.Column{Name: "parent_id", Type: reference}, store.Column{Name: "idx", Type: dialect.BigIntType()})
	}
	return columns
}

// ensureSystemColumns adds any system column the table lacks. It is safe to
// repeat, so create, resume and migration 6 all use it.
func ensureSystemColumns(ctx context.Context, x store.Execer, table string, columns []store.Column) error {
	for _, column := range columns {
		if _, err := x.ExecContext(ctx, dialect.AddColumn(table, column)); err != nil {
			return err
		}
	}
	return nil
}

// validateKind checks the kind, class and parent of a definition against the
// saved revision. The kind and the parent are fixed once saved.
func validateKind(ctx context.Context, conn sqlConn, recordType recordTypeDefinition, previous *recordTypeDefinition) error {
	kind := recordType.kind()
	switch {
	case kernelKinds[kind]:
		return invalid(fmt.Sprintf("the kind %q is assigned by the kernel and cannot be created here", kind))
	case kind != kindMaster && kind != kindDocument && kind != kindChild && kind != kindSetting:
		return invalid("the kind must be master, document, child or setting")
	}
	if previous != nil && previous.kind() != kind {
		return invalid(fmt.Sprintf("the kind is %s and cannot change after the first save", previous.kind()))
	}
	if kind == kindDocument {
		if !documentClasses[recordType.Class] {
			return invalid("a document needs a class: posting, non_posting, work or batch")
		}
	} else if recordType.Class != "" {
		return invalid("only documents have a class")
	}
	if kind != kindChild {
		if recordType.ParentRecordType != "" {
			return invalid("only child Record Types have a parent")
		}
		return nil
	}
	if recordType.ParentRecordType == "" {
		return invalid("a child Record Type needs the Record Type that owns its Records")
	}
	if previous != nil && previous.ParentRecordType != recordType.ParentRecordType {
		return invalid("the parent cannot change after the first save")
	}
	if recordType.ParentRecordType == recordType.Name {
		return invalid("a Record Type cannot be its own parent")
	}
	parent, _, err := loadRecordType(ctx, conn, recordType.ParentRecordType)
	if err != nil {
		return internalError("check parent", err)
	}
	if parent == nil {
		return invalid(fmt.Sprintf("the parent %q is not a saved Record Type", recordType.ParentRecordType))
	}
	if k := parent.kind(); k != kindMaster && k != kindDocument {
		return invalid("a child Record Type belongs to a master or a document")
	}
	return nil
}

// Limits of the stored text types, in characters.
var textLimits = map[string]int{"Text": 255, "Email": 320, "Phone": 40, "Select": 120, "Enum": 120, "Long text": 65535}

var (
	enumValuePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{0,59}$`)
	moneyPattern     = regexp.MustCompile(`^-?[0-9]{1,40}(\.[0-9]{1,40})?$`)
	// maxQuantityScale is the fraction digits the Quantity column keeps.
	maxQuantityScale = 6
)

// maxPatternLength bounds a stored regular expression.
const maxPatternLength = 500

// validateAttributes checks one field's attributes on their own, without the
// database. Records are validated by fieldArgument against the same attributes.
func validateAttributes(field recordTypeField) error {
	label := field.Label
	typed := func(allowed string, set bool, name string) error {
		if set && !strings.Contains(allowed, "|"+field.Type+"|") {
			return invalid(fmt.Sprintf("%s: %s does not apply to %s fields", label, name, field.Type))
		}
		return nil
	}
	checks := []error{
		typed("|Enum|", len(field.Values) > 0, "choices"),
		typed("|Money|", field.Currency != "", "a currency"),
		typed("|Quantity|", field.UoM != "", "a unit of measure"),
		typed("|Money|Quantity|", field.Scale != nil, "a scale"),
		typed("|Text|Long text|Phone|", field.Pattern != "", "a pattern"),
		typed("|Text|Long text|Email|Phone|", field.MaxLength != 0, "a maximum length"),
		typed("|Number|Currency|Money|Quantity|Date|", field.Min != "" || field.Max != "", "a minimum or maximum"),
		typed("|Text|Email|Phone|Number|Currency|Money|Quantity|Date|Select|Enum|Link|", field.Unique || field.Index, "an index"),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	switch field.Type {
	case "Enum":
		if len(field.Values) == 0 {
			return invalid(fmt.Sprintf("%s needs at least one choice", label))
		}
		seen := map[string]bool{}
		for _, v := range field.Values {
			if !enumValuePattern.MatchString(v.Value) {
				return invalid(fmt.Sprintf("%s: the choice %q must start with a lowercase letter or digit and use up to 60 lowercase letters, digits or underscores", label, v.Value))
			}
			if seen[v.Value] {
				return invalid(fmt.Sprintf("%s: the choice %q is used twice", label, v.Value))
			}
			seen[v.Value] = true
			if v.Label == "" || utf8.RuneCountInString(v.Label) > 255 {
				return invalid(fmt.Sprintf("%s: every choice needs a name of at most 255 characters", label))
			}
		}
	case "Money":
		if _, err := types.NewCurrency(field.Currency, 0, types.HalfUp); err != nil {
			return invalid(fmt.Sprintf("%s needs a currency code of three capital letters, like USD", label))
		}
		if field.Scale == nil || *field.Scale < 0 || *field.Scale > types.MaxExponent {
			return invalid(fmt.Sprintf("%s needs a scale from 0 to %d, the number of digits after the decimal point", label, types.MaxExponent))
		}
	case "Quantity":
		if _, err := types.NewQuantity(types.Decimal{}, field.UoM); err != nil {
			return invalid(fmt.Sprintf("%s needs a unit of measure key, like kg", label))
		}
		if field.Scale != nil && (*field.Scale < 0 || *field.Scale > maxQuantityScale) {
			return invalid(fmt.Sprintf("%s needs a scale from 0 to %d", label, maxQuantityScale))
		}
	}
	if field.Pattern != "" {
		if utf8.RuneCountInString(field.Pattern) > maxPatternLength {
			return invalid(fmt.Sprintf("%s: the pattern is longer than %d characters", label, maxPatternLength))
		}
		if _, err := compilePattern(field.Pattern); err != nil {
			return invalid(fmt.Sprintf("%s: the pattern is not a valid regular expression", label))
		}
	}
	if field.MaxLength < 0 || (field.MaxLength > 0 && field.MaxLength > textLimits[field.Type]) {
		return invalid(fmt.Sprintf("%s: the maximum length must be between 1 and %d", label, textLimits[field.Type]))
	}
	if (field.Unique || field.Index) && (field.Type == "Long text" || field.Type == "Checkbox") {
		return invalid(fmt.Sprintf("%s: %s fields cannot be indexed", label, field.Type))
	}
	for _, bound := range []struct{ name, value string }{{"minimum", field.Min}, {"maximum", field.Max}} {
		if bound.value == "" {
			continue
		}
		if err := checkBound(field, bound.value); err != nil {
			return invalid(fmt.Sprintf("%s: the %s %v", label, bound.name, err))
		}
	}
	if field.Min != "" && field.Max != "" && compareBound(field, field.Min, field.Max) > 0 {
		return invalid(fmt.Sprintf("%s: the minimum is above the maximum", label))
	}
	if field.ReadOnly && field.Required && field.Default == "" {
		return invalid(fmt.Sprintf("%s is required and read only, so it needs a default value", label))
	}
	if field.Default != "" {
		if field.Type == "Link" {
			return invalid(fmt.Sprintf("%s: a Link field cannot have a default value", label))
		}
		if field.Unique {
			return invalid(fmt.Sprintf("%s: a unique field cannot have a default value", label))
		}
		if _, err := defaultArgument(field); err != nil {
			return err
		}
	}
	return nil
}

// checkBound reports why a minimum or maximum is not a value of the field's type.
func checkBound(field recordTypeField, bound string) error {
	switch field.Type {
	case "Date":
		if _, err := types.ParseDate(bound); err != nil {
			return fmt.Errorf("must be a date as YYYY-MM-DD")
		}
	case "Money":
		if _, err := moneyMinor(field, bound); err != nil {
			return fmt.Errorf("must be an amount with at most %d decimal places", *field.Scale)
		}
	default:
		if !decimalPattern.MatchString(bound) {
			return fmt.Errorf("must be a number with up to 12 digits before and 6 after the decimal point")
		}
	}
	return nil
}

// compareBound orders two bounds of the field's type. Both must have passed checkBound.
func compareBound(field recordTypeField, a, b string) int {
	if field.Type == "Date" {
		return strings.Compare(a, b)
	}
	x, _ := types.ParseDecimal(a)
	y, _ := types.ParseDecimal(b)
	return x.Cmp(y)
}
