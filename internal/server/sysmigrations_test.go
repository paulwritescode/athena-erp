package server

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
)

// preP2Schema is the system schema the build before versioned migrations
// created, including a Record table saved before revisions existed.
var preP2Schema = []string{
	`CREATE TABLE athena_records (
		name VARCHAR(140) NOT NULL PRIMARY KEY,
		label VARCHAR(255) NOT NULL,
		definition JSON NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE athena_modules (
		name VARCHAR(140) NOT NULL PRIMARY KEY,
		label VARCHAR(255) NOT NULL,
		definition JSON NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE athena_rules (
		name VARCHAR(140) NOT NULL PRIMARY KEY,
		label VARCHAR(255) NOT NULL,
		module_name VARCHAR(140) NOT NULL DEFAULT 'Core',
		definition JSON NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE athena_record_operations (
		id CHAR(36) NOT NULL PRIMARY KEY,
		record_name VARCHAR(140) NOT NULL,
		expected_revision BIGINT NOT NULL,
		target_revision BIGINT NULL,
		definition JSON NOT NULL,
		status VARCHAR(20) NOT NULL,
		last_step VARCHAR(40) NOT NULL,
		error TEXT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		KEY idx_record_operations_status (status)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	// A Record table from before the revision column existed.
	"CREATE TABLE athena_record_legacy (id CHAR(36) NOT NULL PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)",
	`INSERT INTO athena_records (name, label, definition) VALUES ('legacy', 'Legacy', '{}')`,
}

// schemaOf lists table, column, type, nullability and default for the system
// tables and the legacy Record table.
func schemaOf(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT table_name, column_name, column_type, is_nullable, IFNULL(column_default, '-')
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND (table_name LIKE 'athena\_%' OR table_name LIKE 'p\_%') AND table_name <> 'athena_migrations'
		ORDER BY table_name, column_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a, b, c, d, e string
		if err := rows.Scan(&a, &b, &c, &d, &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, a+"."+b+" "+c+" "+d+" "+e)
	}
	return out
}

func TestFreshAndPreVersionedDatabasesReachTheSameSchema(t *testing.T) {
	ctx := context.Background()
	fresh := testdb.New(t)
	old := testdb.New(t)
	for _, statement := range preP2Schema {
		if _, err := old.Exec(statement); err != nil {
			t.Fatalf("pre-P2 fixture: %v", err)
		}
	}
	// The fresh database gets the same legacy Record table after migration 3,
	// so the revision step has the same work on both sides.
	if _, err := store.Up(ctx, fresh, dialect, SystemMigrations(), store.UpOptions{Target: 3}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range preP2Schema[4:] {
		if _, err := fresh.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for name, db := range map[string]*sql.DB{"fresh": fresh, "old": old} {
		if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
			t.Fatalf("%s up: %v", name, err)
		}
	}
	got, want := schemaOf(t, old), schemaOf(t, fresh)
	if len(got) != len(want) {
		t.Fatalf("schemas differ in size: old %d, fresh %d", len(got), len(want))
	}
	hasRevision := false
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("schema differs at %d: old %q, fresh %q", i, got[i], want[i])
		}
		hasRevision = hasRevision || strings.HasPrefix(got[i], "p_workspace__legacy.revision ")
	}
	if !hasRevision {
		t.Fatal("legacy Record table did not gain its revision column")
	}
	var applied int
	if err := old.QueryRow(`SELECT COUNT(*) FROM athena_migrations`).Scan(&applied); err != nil || applied != len(SystemMigrations()) {
		t.Fatalf("recorded %d migrations, %v", applied, err)
	}
}

func TestSystemMigrationsAreValid(t *testing.T) {
	if err := store.ValidateMigrations(SystemMigrations()); err != nil {
		t.Fatal(err)
	}
}
