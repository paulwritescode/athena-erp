package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"athena-erp/internal/store"
)

// renameVocabulary is system migration 5. It moves a database from the
// pre-ADR-0008 names to the Record Type, Record, Pack and Workspace names.
// Every sub-step checks what exists first, so a crash anywhere is repaired by
// running it again. It never drops data and never merges two tables: when
// both the old and the new name exist it stops with an error naming both.
func renameVocabulary(ctx context.Context, x store.Execer) error {
	// Record tables go first: a legacy key such as "history" owns the table
	// athena_record_history, which a system table is about to take.
	if err := renameRecordTables(ctx, x); err != nil {
		return err
	}
	for _, pair := range [][2]string{
		{legacyRecordsTable, "athena_record_types"},
		{legacyOperationsTable, "athena_record_type_operations"},
		{legacyHistoryTable, "athena_record_history"},
		{legacyModulesTable, "athena_packs"},
		{legacyProjectTable, "athena_workspace"},
	} {
		if err := renameTable(ctx, x, pair[0], pair[1]); err != nil {
			return err
		}
	}
	for _, c := range []struct{ table, from, to string }{
		{"athena_record_type_operations", legacyRecordNameColumn, "record_type_name"},
		{"athena_record_history", legacyRecordNameColumn, "record_type_name"},
		{"athena_record_history", legacyEntryIDColumn, "record_id"},
		{"athena_rules", legacyModuleColumn, "pack_name"},
	} {
		if err := renameColumn(ctx, x, c.table, c.from, c.to); err != nil {
			return err
		}
	}
	if err := renameIndex(ctx, x, "athena_record_type_operations", legacyOperationsIndex, "idx_record_type_operations_status"); err != nil {
		return err
	}
	if err := renameIndex(ctx, x, "athena_record_history", legacyHistoryIndex, "idx_record_history_record"); err != nil {
		return err
	}
	if err := defaultPackName(ctx, x); err != nil {
		return err
	}
	for _, t := range []struct{ table, key string }{{"athena_record_types", "name"}, {"athena_record_type_operations", "id"}} {
		if err := rewriteTargetKeys(ctx, x, t.table, t.key); err != nil {
			return err
		}
	}
	return renameForeignKeys(ctx, x)
}

// savedRecordTypeNames lists saved keys from whichever metadata table exists.
func savedRecordTypeNames(ctx context.Context, x store.Execer) ([]string, error) {
	source := ""
	for _, table := range []string{legacyRecordsTable, "athena_record_types"} {
		ok, err := dialect.TableExists(ctx, x, table)
		if err != nil {
			return nil, err
		}
		if ok {
			source = table
			break
		}
	}
	if source == "" {
		return nil, nil
	}
	rows, err := x.QueryContext(ctx, "SELECT name FROM "+dialect.QuoteIdent(source)+" ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// renameRecordTables moves athena_record_<key> to p_workspace__<key> for
// every saved key. All pairs are checked before the first rename.
func renameRecordTables(ctx context.Context, x store.Execer) error {
	names, err := savedRecordTypeNames(ctx, x)
	if err != nil {
		return err
	}
	type pair struct{ from, to, key string }
	var pairs []pair
	claimed := map[string]string{}
	for _, name := range names {
		if !legacyRecordKeyPattern.MatchString(name) {
			return fmt.Errorf("saved Record Type name %q is not a valid identifier", name)
		}
		p := pair{from: legacyRecordTableName(name), to: recordTypeTableName(name), key: name}
		if legacySystemTables[p.from] {
			return fmt.Errorf("Record Type %q would use the table %s, which is a system table; rename it before upgrading", name, p.from)
		}
		if other, ok := claimed[p.to]; ok {
			return fmt.Errorf("Record Types %q and %q both map to the table %s; keys that differ only by case cannot be upgraded", other, name, p.to)
		}
		claimed[p.to] = name
		hasOld, err := dialect.TableExists(ctx, x, p.from)
		if err != nil {
			return err
		}
		hasNew, err := dialect.TableExists(ctx, x, p.to)
		if err != nil {
			return err
		}
		if hasOld && hasNew {
			return fmt.Errorf("both %s and %s exist for Record Type %q; nothing was changed, resolve it by hand", p.from, p.to, name)
		}
		pairs = append(pairs, p)
	}
	for _, p := range pairs {
		if err := renameTable(ctx, x, p.from, p.to); err != nil {
			return err
		}
	}
	return nil
}

func renameTable(ctx context.Context, x store.Execer, from, to string) error {
	hasOld, err := dialect.TableExists(ctx, x, from)
	if err != nil {
		return err
	}
	hasNew, err := dialect.TableExists(ctx, x, to)
	if err != nil {
		return err
	}
	if hasOld && hasNew {
		return fmt.Errorf("both table %s and table %s exist; nothing was merged, resolve it by hand", from, to)
	}
	if !hasOld {
		return nil
	}
	_, err = x.ExecContext(ctx, dialect.RenameTable(from, to))
	return err
}

func renameColumn(ctx context.Context, x store.Execer, table, from, to string) error {
	if ok, err := dialect.TableExists(ctx, x, table); err != nil || !ok {
		return err
	}
	hasOld, err := dialect.ColumnExists(ctx, x, table, from)
	if err != nil {
		return err
	}
	hasNew, err := dialect.ColumnExists(ctx, x, table, to)
	if err != nil {
		return err
	}
	if hasOld && hasNew {
		return fmt.Errorf("both column %s and column %s exist in %s; nothing was merged", from, to, table)
	}
	if !hasOld {
		return nil
	}
	_, err = x.ExecContext(ctx, dialect.RenameColumn(table, from, to))
	return err
}

func renameIndex(ctx context.Context, x store.Execer, table, from, to string) error {
	if ok, err := dialect.TableExists(ctx, x, table); err != nil || !ok {
		return err
	}
	hasOld, err := dialect.IndexExists(ctx, x, table, from)
	if err != nil {
		return err
	}
	hasNew, err := dialect.IndexExists(ctx, x, table, to)
	if err != nil {
		return err
	}
	if hasOld && hasNew {
		return fmt.Errorf("both index %s and index %s exist on %s; nothing was merged", from, to, table)
	}
	if !hasOld {
		return nil
	}
	_, err = x.ExecContext(ctx, dialect.RenameIndex(table, from, to))
	return err
}

// defaultPackName makes 'core' (lowercase, like every other identifier) the
// default Pack of a Rule and converts the one value the old default wrote.
func defaultPackName(ctx context.Context, x store.Execer) error {
	if ok, err := dialect.ColumnExists(ctx, x, "athena_rules", "pack_name"); err != nil || !ok {
		return err
	}
	for _, statement := range []string{
		"ALTER TABLE athena_rules ALTER COLUMN pack_name SET DEFAULT 'core'",
		"UPDATE athena_rules SET pack_name = 'core', updated_at = updated_at WHERE pack_name = 'Core'",
	} {
		if _, err := x.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// rewriteTargetKeys renames target_record to target_record_type inside every
// stored definition, and writes back only the rows that change. It reads both
// keys, so a second run finds nothing to do.
func rewriteTargetKeys(ctx context.Context, x store.Execer, table, keyColumn string) error {
	if ok, err := dialect.TableExists(ctx, x, table); err != nil || !ok {
		return err
	}
	rows, err := x.QueryContext(ctx, "SELECT "+dialect.QuoteIdent(keyColumn)+", definition FROM "+dialect.QuoteIdent(table))
	if err != nil {
		return err
	}
	type update struct {
		key     string
		payload []byte
	}
	var updates []update
	for rows.Next() {
		var key string
		var payload []byte
		if err := rows.Scan(&key, &payload); err != nil {
			rows.Close()
			return err
		}
		rewritten, changed, err := rewriteDefinition(payload)
		if err != nil {
			rows.Close()
			return fmt.Errorf("%s %s: %w", table, key, err)
		}
		if changed {
			updates = append(updates, update{key, rewritten})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	// Setting updated_at to itself keeps ON UPDATE from stamping the row.
	statement := "UPDATE " + dialect.QuoteIdent(table) + " SET definition = ?, updated_at = updated_at WHERE " + dialect.QuoteIdent(keyColumn) + " = ?"
	for _, u := range updates {
		if _, err := x.ExecContext(ctx, statement, u.payload, u.key); err != nil {
			return err
		}
	}
	return nil
}

func rewriteDefinition(payload []byte) ([]byte, bool, error) {
	var definition map[string]any
	if err := json.Unmarshal(payload, &definition); err != nil {
		return nil, false, err
	}
	fields, _ := definition["fields"].([]any)
	changed := false
	for _, raw := range fields {
		field, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if target, has := field[legacyTargetKey]; has {
			if _, done := field["target_record_type"]; !done {
				field["target_record_type"] = target
			}
			delete(field, legacyTargetKey)
			changed = true
		}
	}
	if !changed {
		return payload, false, nil
	}
	out, err := json.Marshal(definition)
	return out, true, err
}

// renameForeignKeys replaces fk_record_<hash> with fk_rtype_<hash> on every
// Link column. RENAME TABLE already pointed the old constraints at the new
// tables but kept their names. Each constraint is dropped and added in turn,
// and a run that stopped between the two adds the missing one.
func renameForeignKeys(ctx context.Context, x store.Execer) error {
	names, err := savedRecordTypeNames(ctx, x)
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		var payload []byte
		if err := x.QueryRowContext(ctx, "SELECT definition FROM athena_record_types WHERE name = ?", name).Scan(&payload); err != nil {
			return err
		}
		var definition recordTypeDefinition
		if err := json.Unmarshal(payload, &definition); err != nil {
			return fmt.Errorf("record type %s: %w", name, err)
		}
		table := recordTypeTableName(name)
		if ok, err := dialect.TableExists(ctx, x, table); err != nil || !ok {
			if err != nil {
				return err
			}
			continue
		}
		for _, field := range definition.Fields {
			if field.Type != "Link" {
				continue
			}
			if err := renameForeignKey(ctx, x, table, name, field); err != nil {
				return err
			}
		}
	}
	return nil
}

func renameForeignKey(ctx context.Context, x store.Execer, table, name string, field recordTypeField) error {
	target := recordTypeTableName(field.TargetRecordType)
	oldName, newName := legacyForeignKeyName(name, field.Name), recordTypeForeignKeyName(name, field.Name)
	referenced, found, err := dialect.ForeignKeyTarget(ctx, x, table, newName)
	if err != nil {
		return err
	}
	if found && referenced != target {
		return fmt.Errorf("constraint %s on %s references %s, not %s", newName, table, referenced, target)
	}
	if _, err := x.ExecContext(ctx, dialect.DropForeignKey(table, oldName)); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = x.ExecContext(ctx, dialect.AddForeignKey(table, newName, field.Name, target))
	return err
}
