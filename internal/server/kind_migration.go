package server

import (
	"context"
	"encoding/json"
	"fmt"

	"athena-erp/internal/store"
)

// addKindsAndSystemFields is migration 6. Record Types saved before kinds
// existed become masters: their definition gains the kind, their table gains
// the master system columns, and existing Records get the local actor as
// creator and owner and the Record Type's revision as def_version. It is
// idempotent, so an interrupted run resumes from the start.
func addKindsAndSystemFields(ctx context.Context, x store.Execer) error {
	rows, err := x.QueryContext(ctx, `SELECT name, definition, revision FROM athena_record_types`)
	if err != nil {
		return err
	}
	type saved struct {
		definition recordTypeDefinition
		revision   int64
	}
	var all []saved
	for rows.Next() {
		var payload []byte
		var item saved
		if err := rows.Scan(&item.definition.Name, &payload, &item.revision); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal(payload, &item.definition); err != nil {
			rows.Close()
			return fmt.Errorf("decode Record Type %q: %w", item.definition.Name, err)
		}
		all = append(all, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range all {
		definition := item.definition
		for _, field := range definition.Fields {
			if reservedColumns[field.Name] {
				return fmt.Errorf("Record Type %q has a field %q, which is now a system field; field keys cannot be renamed yet, so remove that field before migrating", definition.Name, field.Name)
			}
		}
		table := recordTypeTableName(definition.Name)
		exists, err := dialect.TableExists(ctx, x, table)
		if err != nil {
			return err
		}
		if exists {
			if err := ensureSystemColumns(ctx, x, table, kindMaster); err != nil {
				return err
			}
			if _, err := x.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET `created_by` = ?, `modified_by` = ?, `owner` = ?, `def_version` = ? WHERE `created_by` IS NULL", dialect.QuoteIdent(table)), localActor, localActor, localActor, item.revision); err != nil {
				return err
			}
		}
		if definition.Kind == "" {
			definition.Kind = kindMaster
			payload, err := json.Marshal(definition)
			if err != nil {
				return err
			}
			if _, err := x.ExecContext(ctx, `UPDATE athena_record_types SET definition = ? WHERE name = ?`, payload, definition.Name); err != nil {
				return err
			}
		}
	}
	return nil
}
