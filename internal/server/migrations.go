package server

import (
	"context"
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
	Field         recordField
	Description   string
	Destructive   bool
	Affected      int64
	NeedsBackfill bool
}

func (s migrationStep) proto() *athenav1.MigrationStep {
	return &athenav1.MigrationStep{Kind: s.Kind, FieldKey: s.Field.Name, Description: s.Description, Destructive: s.Destructive, AffectedEntries: s.Affected, NeedsBackfill: s.NeedsBackfill}
}

func (s recordService) PlanRecordSave(ctx context.Context, req *athenav1.PlanRecordSaveRequest) (*athenav1.PlanRecordSaveResponse, error) {
	if s.db == nil {
		return nil, connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	if req.GetRecord() == nil {
		return nil, invalid("record is required")
	}
	record := recordFromProto(req.GetRecord())
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, internalError("open connection", err)
	}
	defer conn.Close()
	previous, revision, err := loadRecord(ctx, conn, record.Name)
	if err != nil {
		return nil, internalError("load Record", err)
	}
	if revision != req.GetExpectedRevision() {
		return nil, connect.NewError(connect.CodeAborted, "this Record was saved somewhere else; reload it and try again")
	}
	if err := validateRecord(ctx, conn, record); err != nil {
		return nil, err
	}
	steps, err := planMigration(ctx, conn, record, previous)
	if err != nil {
		return nil, err
	}
	res := &athenav1.PlanRecordSaveResponse{}
	for _, step := range steps {
		res.Steps = append(res.Steps, step.proto())
		res.RequiresConfirmation = res.RequiresConfirmation || step.Destructive || step.NeedsBackfill
	}
	return res, nil
}

// planMigration lists the table changes needed to move from previous to
// record, with the entries each step affects. It reads but never writes.
func planMigration(ctx context.Context, conn sqlConn, record recordDefinition, previous *recordDefinition) ([]migrationStep, error) {
	if previous == nil {
		return []migrationStep{{Kind: stepCreateTable, Description: fmt.Sprintf("Create the table for %s with %s", record.Label, plural(int64(len(record.Fields)), "field", "fields"))}}, nil
	}
	table := recordTableName(record.Name)
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
	entries, err := count("1=1")
	if err != nil {
		return nil, internalError("count entries", err)
	}
	previousByKey := map[string]recordField{}
	for _, field := range previous.Fields {
		previousByKey[field.Name] = field
	}
	current := map[string]bool{}
	var steps []migrationStep
	for _, field := range record.Fields {
		current[field.Name] = true
		old, existed := previousByKey[field.Name]
		column := dialect.QuoteIdent(field.Name)
		if !existed {
			step := migrationStep{Kind: stepAddField, Field: field, Description: fmt.Sprintf("Add %s", field.Label)}
			if field.Required && entries > 0 {
				step.Kind, step.NeedsBackfill, step.Affected = stepRequireField, true, entries
				step.Description = fmt.Sprintf("Add required %s and fill it in for %s", field.Label, plural(entries, "existing entry", "existing entries"))
			}
			steps = append(steps, step)
			continue
		}
		var cleared int64
		if old.Type != field.Type || (field.Type == "Link" && old.TargetRecord != field.TargetRecord) {
			predicate, err := dialect.IncompatibleValues(column, field.Type, recordTableName(field.TargetRecord))
			if err != nil {
				return nil, invalid(err.Error())
			}
			if cleared, err = count(column + " IS NOT NULL AND (" + predicate + ")"); err != nil {
				return nil, internalError("check values", err)
			}
			step := migrationStep{Kind: stepChangeType, Field: field, Affected: cleared, Destructive: cleared > 0}
			if old.Type != field.Type {
				step.Description = fmt.Sprintf("Change %s from %s to %s", field.Label, old.Type, field.Type)
			} else {
				step.Kind = stepChangeLink
				step.Description = fmt.Sprintf("Link %s to %s instead of %s", field.Label, field.TargetRecord, old.TargetRecord)
			}
			if cleared > 0 {
				step.Description += fmt.Sprintf("; %s cannot convert and will be cleared", plural(cleared, "value", "values"))
			}
			steps = append(steps, step)
		}
		if field.Required {
			empty, err := count(column + " IS NULL")
			if err != nil {
				return nil, internalError("check values", err)
			}
			if missing := empty + cleared; missing > 0 {
				steps = append(steps, migrationStep{Kind: stepRequireField, Field: field, NeedsBackfill: true, Affected: missing,
					Description: fmt.Sprintf("Make %s required and fill it in for %s without a value", field.Label, plural(missing, "entry", "entries"))})
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
			return opts, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s needs a value to fill in for %s", step.Field.Label, plural(step.Affected, "entry", "entries")))
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

// migrateFields applies field-level changes from previous to record. Every
// statement checks or tolerates the current state, so a resumed operation can
// run it again after a partial attempt.
func migrateFields(ctx context.Context, conn sqlConn, record recordDefinition, previous *recordDefinition, opts saveOptions) error {
	table := recordTableName(record.Name)
	previousByKey := map[string]recordField{}
	if previous != nil {
		for _, field := range previous.Fields {
			previousByKey[field.Name] = field
		}
	}
	current := map[string]bool{}
	for _, field := range record.Fields {
		current[field.Name] = true
	}
	for _, old := range previousByKey {
		if current[old.Name] {
			continue
		}
		if old.Type == "Link" {
			if _, err := conn.ExecContext(ctx, dialect.DropForeignKey(table, recordForeignKeyName(record.Name, old.Name))); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, dialect.DropColumn(table, old.Name)); err != nil {
			return err
		}
	}
	for _, field := range record.Fields {
		columnType, _ := dialect.ColumnType(field.Type)
		column := dialect.QuoteIdent(field.Name)
		spec := store.Column{Name: field.Name, Type: columnType}
		old, existed := previousByKey[field.Name]
		typeChanged := existed && old.Type != field.Type
		linkChanged := existed && field.Type == "Link" && (old.Type != "Link" || old.TargetRecord != field.TargetRecord)
		if existed && old.Type == "Link" && (field.Type != "Link" || old.TargetRecord != field.TargetRecord) {
			if _, err := conn.ExecContext(ctx, dialect.DropForeignKey(table, recordForeignKeyName(record.Name, field.Name))); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, dialect.AddColumn(table, spec)); err != nil {
			return err
		}
		if (typeChanged || linkChanged) && opts.ConfirmDataLoss {
			predicate, err := dialect.IncompatibleValues(column, field.Type, recordTableName(field.TargetRecord))
			if err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = NULL WHERE %s IS NOT NULL AND (%s)", dialect.QuoteIdent(table), column, column, predicate)); err != nil {
				return err
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
			if err := ensureLink(ctx, conn, record.Name, field); err != nil {
				return err
			}
		}
		if value, ok := opts.Backfills[field.Name]; ok {
			if _, err := conn.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s IS NULL", dialect.QuoteIdent(table), column, column), value); err != nil {
				return err
			}
		}
	}
	return nil
}

// describeSteps joins step descriptions for log lines.
func describeSteps(steps []migrationStep) string {
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		parts = append(parts, step.Description)
	}
	return strings.Join(parts, "; ")
}
