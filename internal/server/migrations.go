package server

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/store"
	"connectrpc.com/connect/v2"
)

// Migration step kinds reported to clients.
const (
	stepAddField     = "add_field"
	stepRemoveField  = "remove_field"
	stepChangeType   = "change_type"
	stepChangeLink   = "change_link"
	stepRequireField = "require_field"
	stepCreateTable  = "create_table"
	stepAddUnique    = "add_unique"
	stepDropUnique   = "drop_unique"
	stepAddIndex     = "add_index"
	stepDropIndex    = "drop_index"
	stepRecalculate  = "recalculate"
)

// saveOptions are the client's answers to a migration plan. They are stored
// with the journaled operation so a resumed save behaves like the original.
type saveOptions struct {
	ConfirmDataLoss bool `json:"confirm_data_loss,omitempty"`
	// Backfills holds validated database arguments (string or bool) by field key.
	Backfills map[string]any `json:"backfills,omitempty"`
}

type migrationStep struct {
	Kind          string
	Field         recordTypeField
	Description   string
	Destructive   bool
	Affected      int64
	NeedsBackfill bool
}

func (s migrationStep) proto() *athenav1.MigrationStep {
	return &athenav1.MigrationStep{Kind: s.Kind, FieldKey: s.Field.Name, Description: s.Description, Destructive: s.Destructive, AffectedRecords: s.Affected, NeedsBackfill: s.NeedsBackfill}
}

func (s recordTypeService) PlanRecordTypeSave(ctx context.Context, req *athenav1.PlanRecordTypeSaveRequest) (*athenav1.PlanRecordTypeSaveResponse, error) {
	if s.db == nil {
		return nil, connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	if req.GetRecordType() == nil {
		return nil, invalid("record type is required")
	}
	recordType := recordTypeFromProto(req.GetRecordType())
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, internalError("open connection", err)
	}
	defer conn.Close()
	previous, revision, err := loadRecordType(ctx, conn, recordType.Name)
	if err != nil {
		return nil, internalError("load Record Type", err)
	}
	if revision != req.GetExpectedRevision() {
		return nil, connect.NewError(connect.CodeAborted, "this Record Type was saved somewhere else; reload it and try again")
	}
	if err := validateRecordType(ctx, conn, recordType, previous); err != nil {
		return nil, err
	}
	steps, err := planMigration(ctx, conn, recordType, previous)
	if err != nil {
		return nil, err
	}
	res := &athenav1.PlanRecordTypeSaveResponse{}
	for _, step := range steps {
		res.Steps = append(res.Steps, step.proto())
		res.RequiresConfirmation = res.RequiresConfirmation || step.Destructive || step.NeedsBackfill
	}
	return res, nil
}

// planMigration lists the table changes needed to move from previous to
// recordType, with the Records each step affects. It reads but never writes.
func planMigration(ctx context.Context, conn sqlConn, recordType recordTypeDefinition, previous *recordTypeDefinition) ([]migrationStep, error) {
	if previous == nil {
		return []migrationStep{{Kind: stepCreateTable, Description: fmt.Sprintf("Create the table for %s (%s) with %s", recordType.Label, recordType.kind(), plural(int64(len(recordType.Fields)), "field", "fields"))}}, nil
	}
	table := recordTypeTableName(recordType.Name)
	hasTable, err := dialect.TableExists(ctx, conn, table)
	if err != nil {
		return nil, internalError("check table", err)
	}
	count := func(where string) (int64, error) {
		if !hasTable {
			return 0, nil
		}
		var n int64
		err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+dialect.QuoteIdent(table)+" WHERE "+where).Scan(&n)
		return n, err
	}
	duplicates := func(column string) (int64, error) {
		if !hasTable {
			return 0, nil
		}
		var n int64
		err := conn.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM (SELECT %[1]s FROM %[2]s WHERE %[1]s IS NOT NULL GROUP BY %[1]s HAVING COUNT(*) > 1) d", column, dialect.QuoteIdent(table))).Scan(&n)
		return n, err
	}
	records, err := count("1=1")
	if err != nil {
		return nil, internalError("count records", err)
	}
	previousByKey := map[string]recordTypeField{}
	for _, field := range previous.Fields {
		previousByKey[field.Name] = field
	}
	current := map[string]bool{}
	var steps []migrationStep
	for _, field := range recordType.Fields {
		current[field.Name] = true
		old, existed := previousByKey[field.Name]
		column := dialect.QuoteIdent(field.Name)
		if !existed {
			step := migrationStep{Kind: stepAddField, Field: field, Description: fmt.Sprintf("Add %s", field.Label)}
			if field.Required && records > 0 {
				step.Kind, step.NeedsBackfill, step.Affected = stepRequireField, true, records
				step.Description = fmt.Sprintf("Add required %s and fill it in for %s", field.Label, plural(records, "existing record", "existing records"))
			}
			steps = append(steps, step)
			continue
		}
		var cleared int64
		predicate, reason, err := clearPredicate(old, field, column)
		if err != nil {
			return nil, invalid(err.Error())
		}
		if reason != "" {
			if cleared, err = count(column + " IS NOT NULL AND (" + predicate + ")"); err != nil {
				return nil, internalError("check values", err)
			}
			step := migrationStep{Kind: stepChangeType, Field: field, Affected: cleared, Destructive: cleared > 0}
			switch reason {
			case reasonType:
				step.Description = fmt.Sprintf("Change %s from %s to %s", field.Label, old.Type, field.Type)
			case reasonLink:
				step.Kind = stepChangeLink
				step.Description = fmt.Sprintf("Link %s to %s instead of %s", field.Label, field.TargetRecordType, old.TargetRecordType)
			case reasonMeaning:
				step.Description = fmt.Sprintf("Change the currency, scale or unit of %s", field.Label)
			default:
				step.Description = fmt.Sprintf("Change the choices of %s", field.Label)
			}
			if cleared > 0 {
				step.Description += fmt.Sprintf("; %s cannot convert and will be cleared", plural(cleared, "value", "values"))
			}
			steps = append(steps, step)
		}
		indexSteps, err := planIndexes(old, field, func() (int64, error) { return duplicates(column) })
		if err != nil {
			return nil, err
		}
		steps = append(steps, indexSteps...)
		if field.Required {
			empty, err := count(column + " IS NULL")
			if err != nil {
				return nil, internalError("check values", err)
			}
			if missing := empty + cleared; missing > 0 {
				steps = append(steps, migrationStep{Kind: stepRequireField, Field: field, NeedsBackfill: true, Affected: missing,
					Description: fmt.Sprintf("Make %s required and fill it in for %s without a value", field.Label, plural(missing, "record", "records"))})
			}
		}
	}
	for _, old := range previous.Fields {
		if current[old.Name] {
			continue
		}
		values, err := count(dialect.QuoteIdent(old.Name) + " IS NOT NULL")
		if err != nil {
			return nil, internalError("check values", err)
		}
		steps = append(steps, migrationStep{Kind: stepRemoveField, Field: old, Destructive: values > 0, Affected: values,
			Description: fmt.Sprintf("Remove %s and delete its %s", old.Label, plural(values, "stored value", "stored values"))})
	}
	if formulasChanged(recordType, previous) && records > 0 {
		steps = append(steps, migrationStep{Kind: stepRecalculate, Affected: records,
			Description: fmt.Sprintf("Recalculate the formulas of %s", plural(records, "record", "records"))})
	}
	return steps, nil
}

// Reasons clearPredicate gives for clearing stored values.
const (
	reasonType    = "type"
	reasonLink    = "link"
	reasonMeaning = "meaning"
	reasonValues  = "values"
)

// clearPredicate returns the condition selecting stored values of old that
// cannot be kept under field, with why. Stored minor units and choices carry
// meaning that a new currency, scale, unit, list or type would silently
// change, so those values are cleared instead of reinterpreted. reason is ""
// when nothing changes.
func clearPredicate(old, field recordTypeField, column string) (predicate, reason string, err error) {
	typeChanged := old.Type != field.Type
	scale := func(f recordTypeField) int {
		if f.Scale == nil {
			return -1
		}
		return *f.Scale
	}
	switch {
	case old.Type == "Money" && field.Type == "Money" && (old.Currency != field.Currency || scale(old) != scale(field)):
		return "1 = 1", reasonMeaning, nil
	case old.Type == "Quantity" && field.Type == "Quantity" && old.UoM != field.UoM:
		return "1 = 1", reasonMeaning, nil
	case field.Type == "Enum" && (typeChanged || !sameEnumValues(old.Values, field.Values)):
		reason = reasonValues
		if typeChanged {
			reason = reasonType
		}
		return column + " NOT IN (" + enumLiteral(field.Values) + ")", reason, nil
	case old.Type == "Money" && typeChanged:
		return "1 = 1", reasonType, nil
	case typeChanged || (field.Type == "Link" && old.TargetRecordType != field.TargetRecordType):
		predicate, err = dialect.IncompatibleValues(column, field.Type, recordTypeTableName(field.TargetRecordType))
		if typeChanged {
			return predicate, reasonType, err
		}
		return predicate, reasonLink, err
	}
	return "", "", nil
}

// planIndexes lists the index changes between two revisions of a field. Making
// a field unique is refused while stored values repeat.
func planIndexes(old, field recordTypeField, duplicates func() (int64, error)) ([]migrationStep, error) {
	var steps []migrationStep
	if field.Unique && !old.Unique {
		repeated, err := duplicates()
		if err != nil {
			return nil, internalError("check values", err)
		}
		if repeated > 0 {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s cannot be unique yet: %s are used more than once. Make them different first", field.Label, plural(repeated, "value", "values")))
		}
		steps = append(steps, migrationStep{Kind: stepAddUnique, Field: field, Description: fmt.Sprintf("Make %s unique", field.Label)})
	}
	if !field.Unique && old.Unique {
		steps = append(steps, migrationStep{Kind: stepDropUnique, Field: field, Description: fmt.Sprintf("Allow repeated values in %s", field.Label)})
	}
	hadPlain, wantPlain := old.Index && !old.Unique, field.Index && !field.Unique
	if wantPlain && !hadPlain {
		steps = append(steps, migrationStep{Kind: stepAddIndex, Field: field, Description: fmt.Sprintf("Index %s", field.Label)})
	}
	if hadPlain && !wantPlain {
		steps = append(steps, migrationStep{Kind: stepDropIndex, Field: field, Description: fmt.Sprintf("Remove the index on %s", field.Label)})
	}
	return steps, nil
}

// checkPlan confirms the client answered every step that needs it and turns
// backfill values into validated database arguments.
func checkPlan(steps []migrationStep, confirm bool, backfills map[string]*athenav1.FieldValue) (saveOptions, error) {
	opts := saveOptions{ConfirmDataLoss: confirm}
	for _, step := range steps {
		if step.Destructive && !confirm {
			return opts, connect.NewError(connect.CodeFailedPrecondition, step.Description+". Confirm the data loss to continue")
		}
		if !step.NeedsBackfill {
			continue
		}
		value, ok := backfills[step.Field.Name]
		if !ok {
			return opts, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s needs a value to fill in for %s", step.Field.Label, plural(step.Affected, "record", "records")))
		}
		arg, err := fieldArgument(step.Field, value)
		if err != nil {
			return opts, err
		}
		if arg == nil {
			return opts, invalid(fmt.Sprintf("the fill-in value for %s cannot be empty", step.Field.Label))
		}
		if opts.Backfills == nil {
			opts.Backfills = map[string]any{}
		}
		opts.Backfills[step.Field.Name] = arg
	}
	return opts, nil
}

// migrateFields applies field-level changes from previous to recordType. Every
// statement checks or tolerates the current state, so a resumed operation can
// run it again after a partial attempt.
func migrateFields(ctx context.Context, conn sqlConn, recordType recordTypeDefinition, previous *recordTypeDefinition, opts saveOptions) error {
	table := recordTypeTableName(recordType.Name)
	previousByKey := map[string]recordTypeField{}
	if previous != nil {
		for _, field := range previous.Fields {
			previousByKey[field.Name] = field
		}
	}
	current := map[string]bool{}
	for _, field := range recordType.Fields {
		current[field.Name] = true
	}
	for _, old := range previousByKey {
		if current[old.Name] {
			continue
		}
		if old.Type == "Link" {
			if _, err := conn.ExecContext(ctx, dialect.DropForeignKey(table, recordTypeForeignKeyName(recordType.Name, old.Name))); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, dialect.DropColumn(table, old.Name)); err != nil {
			return err
		}
	}
	for _, field := range recordType.Fields {
		columnType, _ := dialect.ColumnType(field.Type)
		column := dialect.QuoteIdent(field.Name)
		spec := store.Column{Name: field.Name, Type: columnType}
		old, existed := previousByKey[field.Name]
		typeChanged := existed && old.Type != field.Type
		if existed && old.Type == "Link" && (field.Type != "Link" || old.TargetRecordType != field.TargetRecordType) {
			if _, err := conn.ExecContext(ctx, dialect.DropForeignKey(table, recordTypeForeignKeyName(recordType.Name, field.Name))); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, dialect.AddColumn(table, spec)); err != nil {
			return err
		}
		if existed && opts.ConfirmDataLoss {
			predicate, reason, err := clearPredicate(old, field, column)
			if err != nil {
				return err
			}
			if reason != "" {
				if _, err := conn.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = NULL WHERE %s IS NOT NULL AND (%s)", dialect.QuoteIdent(table), column, column, predicate)); err != nil {
					return err
				}
			}
		}
		// MODIFY also makes columns from earlier versions nullable; Go enforces required.
		nullable, err := dialect.ColumnIsNullable(ctx, conn, table, field.Name)
		if err != nil {
			return err
		}
		if typeChanged || !nullable {
			if _, err := conn.ExecContext(ctx, dialect.MakeNullable(table, spec)); err != nil {
				return err
			}
		}
		if field.Type == "Link" {
			if err := ensureLink(ctx, conn, recordType.Name, field); err != nil {
				return err
			}
		}
		if err := ensureIndexes(ctx, conn, recordType.Name, field); err != nil {
			return err
		}
		if value, ok := opts.Backfills[field.Name]; ok {
			if _, err := conn.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s IS NULL", dialect.QuoteIdent(table), column, column), value); err != nil {
				return err
			}
		}
	}
	return recalculateAfterSave(ctx, conn, recordType, previous)
}

// describeSteps joins step descriptions for log lines.
func describeSteps(steps []migrationStep) string {
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		parts = append(parts, step.Description)
	}
	return strings.Join(parts, "; ")
}

// fieldIndexName names a field's index deterministically and within the
// identifier limit; prefix is "uq" or "ix".
func fieldIndexName(prefix, recordTypeName, fieldName string) string {
	sum := sha256.Sum256([]byte(recordTypeName + ":" + fieldName))
	return fmt.Sprintf("%s_rtype_%x", prefix, sum[:8])
}

// ensureIndexes makes the field's unique and plain indexes match its
// attributes. A unique index serves lookups, so a unique field has no plain
// one. Each statement checks the catalogue first, so a resume can repeat it.
func ensureIndexes(ctx context.Context, conn sqlConn, recordTypeName string, field recordTypeField) error {
	table := recordTypeTableName(recordTypeName)
	for _, index := range []struct {
		prefix string
		want   bool
		unique bool
	}{{"uq", field.Unique, true}, {"ix", field.Index && !field.Unique, false}} {
		name := fieldIndexName(index.prefix, recordTypeName, field.Name)
		exists, err := dialect.IndexExists(ctx, conn, table, name)
		if err != nil {
			return err
		}
		switch {
		case index.want && !exists:
			_, err = conn.ExecContext(ctx, dialect.AddIndex(table, name, field.Name, index.unique))
		case !index.want && exists:
			_, err = conn.ExecContext(ctx, dialect.DropIndex(table, name))
		}
		if err != nil {
			return err
		}
	}
	return nil
}
