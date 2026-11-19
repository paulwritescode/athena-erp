package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"

	"athena-erp/internal/store"
)

// This file holds every name from before the vocabulary rename (ADR-0008).
// System migrations 1 to 5 are permanent and still need the old names, and a
// database from before the rename is identified by the old identity table.
// Nothing outside this file, the migrations and their tests may use them.

const (
	legacyRecordsTable    = "athena_records"
	legacyModulesTable    = "athena_modules"
	legacyOperationsTable = "athena_record_operations"
	legacyHistoryTable    = "athena_entry_history"
	legacyProjectTable    = "athena_project"

	legacyOperationsIndex = "idx_record_operations_status"
	legacyHistoryIndex    = "idx_entry_history_entry"

	legacyRecordNameColumn = "record_name"
	legacyEntryIDColumn    = "entry_id"
	legacyModuleColumn     = "module_name"

	// legacyTargetKey is the JSON key a Link field used for its target.
	legacyTargetKey = "target_record"

	// legacyIdentLimit is MariaDB's identifier limit, which the old table
	// names were shortened at.
	legacyIdentLimit = 64
)

// legacySystemTables are the old system tables. A legacy Record table can
// never have had one of these names.
var legacySystemTables = map[string]bool{
	legacyRecordsTable: true, legacyModulesTable: true, legacyOperationsTable: true,
	legacyHistoryTable: true, legacyProjectTable: true,
}

// legacyRecordTableName is the table name used before the rename. It is a
// frozen copy: migration 4 and the table rename in migration 5 both need it.
func legacyRecordTableName(name string) string {
	return store.ShortenIdentifier("athena_record_"+strings.ToLower(name), legacyIdentLimit)
}

// legacyForeignKeyName is the old constraint name of a Link field.
func legacyForeignKeyName(recordName, fieldName string) string {
	sum := sha256.Sum256([]byte(recordName + ":" + fieldName))
	return fmt.Sprintf("fk_record_%x", sum[:8])
}

// workspaceIdentity reads the Workspace ID. A database from before the
// rename has athena_project until migration 5 renames it, and the identity
// check runs before migrations.
func workspaceIdentity(ctx context.Context, db *sql.DB) (string, error) {
	table := "athena_workspace"
	if ok, err := dialect.TableExists(ctx, db, table); err == nil && !ok {
		if legacy, err := dialect.TableExists(ctx, db, legacyProjectTable); err == nil && legacy {
			table = legacyProjectTable
		}
	}
	var id string
	err := db.QueryRowContext(ctx, "SELECT id FROM "+dialect.QuoteIdent(table)+" LIMIT 1").Scan(&id)
	return id, err
}
