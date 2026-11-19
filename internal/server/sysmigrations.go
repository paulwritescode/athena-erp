package server

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"

	"athena-erp/internal/store"

	"github.com/go-sql-driver/mysql"
)

// SystemMigrations is the ordered list of system migrations. Versions are
// permanent: add new entries at the end and never edit an applied one. Each
// step is idempotent, so a pre-versioning database and a fresh one reach the
// same schema. The statements use MariaDB syntax; MariaDB is the only
// tested engine.
func SystemMigrations() []store.Migration {
	return []store.Migration{
		{Version: 1, Name: "core_tables", Up: execAll(
			`CREATE TABLE IF NOT EXISTS athena_records (
				name VARCHAR(140) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				definition JSON NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_modules (
				name VARCHAR(140) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				definition JSON NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_rules (
				name VARCHAR(140) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				module_name VARCHAR(140) NOT NULL DEFAULT 'Core',
				definition JSON NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`ALTER TABLE athena_records ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1 AFTER label`,
		)},
		// Journal of Record saves. A pending row is resumed at startup.
		{Version: 2, Name: "record_operations", Up: execAll(
			`CREATE TABLE IF NOT EXISTS athena_record_operations (
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
			`ALTER TABLE athena_record_operations ADD COLUMN IF NOT EXISTS action VARCHAR(20) NOT NULL DEFAULT 'save' AFTER record_name`,
			`ALTER TABLE athena_record_operations ADD COLUMN IF NOT EXISTS options JSON NULL AFTER definition`,
		)},
		// Audit history of entry changes, written in the same transaction as the change.
		{Version: 3, Name: "entry_history", Up: execAll(
			`CREATE TABLE IF NOT EXISTS athena_entry_history (
				id CHAR(36) NOT NULL PRIMARY KEY,
				record_name VARCHAR(140) NOT NULL,
				entry_id CHAR(36) NOT NULL,
				revision BIGINT NOT NULL,
				action VARCHAR(20) NOT NULL,
				changed_fields JSON NOT NULL,
				actor VARCHAR(255) NOT NULL,
				created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
				KEY idx_entry_history_entry (record_name, entry_id, created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		)},
		{Version: 4, Name: "entry_revisions", Up: addEntryRevisions},
		// Renames every table, column, index, constraint and stored key to the
		// Record Type, Record, Pack and Workspace vocabulary (ADR-0008).
		{Version: 5, Name: "vocabulary_rename", Up: renameVocabulary},
		// Gives every existing Record Type the master kind and its system
		// columns (ADR-0010).
		{Version: 6, Name: "record_kinds_and_system_fields", Up: addKindsAndSystemFields},
		// Document Versions and the Activity Timeline of submittable
		// documents (ADR-0011). Rows are written by the kernel only.
		{Version: 7, Name: "document_lifecycle", Up: execAll(
			`CREATE TABLE IF NOT EXISTS athena_document_versions (
				id CHAR(36) NOT NULL PRIMARY KEY,
				record_type_name VARCHAR(140) NOT NULL,
				record_id CHAR(36) NOT NULL,
				version BIGINT NOT NULL,
				revision BIGINT NOT NULL,
				event VARCHAR(20) NOT NULL,
				docstate BIGINT NOT NULL,
				snapshot JSON NOT NULL,
				diff JSON NOT NULL,
				actor VARCHAR(255) NOT NULL,
				request_id CHAR(36) NULL,
				created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
				UNIQUE KEY uq_document_versions (record_type_name, record_id, version)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_activity (
				id CHAR(36) NOT NULL PRIMARY KEY,
				record_type_name VARCHAR(140) NOT NULL,
				record_id CHAR(36) NOT NULL,
				event VARCHAR(40) NOT NULL,
				actor VARCHAR(255) NOT NULL,
				from_state BIGINT NULL,
				to_state BIGINT NULL,
				changed_fields JSON NOT NULL,
				related_id CHAR(36) NULL,
				request_id CHAR(36) NULL,
				created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
				KEY idx_activity_record (record_type_name, record_id, created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		)},
		// The hash-chained Audit Log replaces the Record history, and number
		// counters back naming series (ADR-0012). Old history rows are kept
		// as imported Audit Log rows.
		{Version: 8, Name: "audit_log_and_numbering", Up: createAuditAndNumbering},
		// Users, sessions, Roles, Grants and the Access Log (ADR-0014).
		{Version: 9, Name: "users_sessions_and_grants", Up: execAll(
			`CREATE TABLE IF NOT EXISTS athena_users (
				id CHAR(36) NOT NULL PRIMARY KEY,
				username VARCHAR(64) NOT NULL,
				display_name VARCHAR(255) NOT NULL,
				password_hash VARCHAR(255) NOT NULL,
				is_owner TINYINT(1) NOT NULL DEFAULT 0,
				disabled TINYINT(1) NOT NULL DEFAULT 0,
				failed_attempts INT NOT NULL DEFAULT 0,
				locked_until TIMESTAMP(3) NULL,
				last_login_at TIMESTAMP(3) NULL,
				created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
				updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
				UNIQUE KEY uq_users_username (username)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_roles (
				id CHAR(36) NOT NULL PRIMARY KEY,
				name VARCHAR(100) NOT NULL,
				description VARCHAR(255) NOT NULL DEFAULT '',
				system_permissions JSON NOT NULL,
				created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
				UNIQUE KEY uq_roles_name (name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_grants (
				id CHAR(36) NOT NULL PRIMARY KEY,
				role_id CHAR(36) NOT NULL,
				record_type VARCHAR(140) NOT NULL,
				actions JSON NOT NULL,
				permission_level INT NOT NULL DEFAULT 0,
				row_condition TEXT NULL,
				separate_duties TINYINT(1) NOT NULL DEFAULT 0,
				KEY idx_grants_role (role_id),
				CONSTRAINT fk_grants_role FOREIGN KEY (role_id) REFERENCES athena_roles (id) ON DELETE CASCADE
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_user_roles (
				user_id CHAR(36) NOT NULL,
				role_id CHAR(36) NOT NULL,
				PRIMARY KEY (user_id, role_id),
				CONSTRAINT fk_user_roles_user FOREIGN KEY (user_id) REFERENCES athena_users (id) ON DELETE CASCADE,
				CONSTRAINT fk_user_roles_role FOREIGN KEY (role_id) REFERENCES athena_roles (id) ON DELETE CASCADE
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_sessions (
				id CHAR(36) NOT NULL PRIMARY KEY,
				token_hash CHAR(64) NOT NULL,
				user_id CHAR(36) NOT NULL,
				created_at TIMESTAMP(3) NOT NULL,
				last_seen_at TIMESTAMP(3) NOT NULL,
				expires_at TIMESTAMP(3) NOT NULL,
				revoked_at TIMESTAMP(3) NULL,
				UNIQUE KEY uq_sessions_token (token_hash),
				KEY idx_sessions_user (user_id),
				CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES athena_users (id) ON DELETE CASCADE
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_access_log (
				id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
				at TIMESTAMP(3) NOT NULL,
				event VARCHAR(40) NOT NULL,
				outcome VARCHAR(20) NOT NULL,
				user_id CHAR(36) NULL,
				username VARCHAR(64) NULL,
				procedure_name VARCHAR(200) NULL,
				record_type VARCHAR(140) NULL,
				action VARCHAR(40) NULL,
				request_id CHAR(36) NULL,
				detail VARCHAR(255) NULL,
				KEY idx_access_log_event (event, id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		)},
		// Workspace files: one row per file lineage and one row per revision,
		// with the content of every revision (ADR-0015). A row has no path
		// after the file is deleted or moved away, and the UNIQUE key allows
		// many NULLs, so a deleted path can be created again.
		{Version: 10, Name: "workspace_files", Up: execAll(
			`CREATE TABLE IF NOT EXISTS athena_files (
				id CHAR(36) NOT NULL PRIMARY KEY,
				path VARCHAR(240) CHARACTER SET ascii COLLATE ascii_bin NULL,
				revision BIGINT NOT NULL,
				hash CHAR(64) NOT NULL,
				size BIGINT NOT NULL,
				modified_by VARCHAR(255) NOT NULL,
				created_at TIMESTAMP(3) NOT NULL,
				modified_at TIMESTAMP(3) NOT NULL,
				UNIQUE KEY uq_files_path (path)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_file_versions (
				seq BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
				file_id CHAR(36) NOT NULL,
				revision BIGINT NOT NULL,
				path VARCHAR(240) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
				action VARCHAR(20) NOT NULL,
				hash CHAR(64) NOT NULL,
				size BIGINT NOT NULL,
				content MEDIUMBLOB NULL,
				actor VARCHAR(255) NOT NULL,
				source VARCHAR(20) NOT NULL,
				request_id CHAR(36) NULL,
				created_at TIMESTAMP(3) NOT NULL,
				UNIQUE KEY uq_file_versions_revision (file_id, revision),
				KEY idx_file_versions_path (path, seq),
				CONSTRAINT fk_file_versions_file FOREIGN KEY (file_id) REFERENCES athena_files (id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		)},
	}
}

func execAll(statements ...string) func(context.Context, store.Execer) error {
	return func(ctx context.Context, x store.Execer) error {
		for _, statement := range statements {
			if _, err := x.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}
}

// legacyRecordKeyPattern accepts keys saved before keys were lowercase-only.
var legacyRecordKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

// addEntryRevisions adds the revision column to Record Type tables saved before
// optimistic concurrency existed. Names come from saved definitions only.
func addEntryRevisions(ctx context.Context, x store.Execer) error {
	rows, err := x.QueryContext(ctx, `SELECT name FROM athena_records`)
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		names = append(names, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, name := range names {
		if !legacyRecordKeyPattern.MatchString(name) {
			return fmt.Errorf("saved Record Type name %q is not a valid identifier", name)
		}
		table := legacyRecordTableName(name)
		exists, err := dialect.TableExists(ctx, x, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if _, err := x.ExecContext(ctx, dialect.AddRevisionColumn(table)); err != nil {
			return err
		}
	}
	return nil
}

// MigrationOptions selects what Migrate does. Target 0 means every pending
// migration.
type MigrationOptions struct {
	Target int
	DryRun bool
}

// MigrationStatus reports each system migration and whether the database has
// applied it. It writes nothing. When workspace is set, the database must
// belong to it.
func MigrationStatus(ctx context.Context, cfg *mysql.Config, workspace *Workspace) ([]store.MigrationState, error) {
	db, err := openMigrationDB(ctx, cfg, workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return store.MigrationStatus(ctx, db, dialect, SystemMigrations())
}

// Migrate applies pending system migrations the way the server does at
// start-up and returns the ones it applied (or would apply with DryRun).
func Migrate(ctx context.Context, cfg *mysql.Config, workspace *Workspace, opts MigrationOptions) ([]store.Migration, error) {
	db, err := openMigrationDB(ctx, cfg, workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{Target: opts.Target, DryRun: opts.DryRun})
}

func openMigrationDB(ctx context.Context, cfg *mysql.Config, workspace *Workspace) (*sql.DB, error) {
	connector, err := mysql.NewConnector(utcConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("open MariaDB connection: %w", err)
	}
	db := sql.OpenDB(connector)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to MariaDB: %w", err)
	}
	if workspace != nil {
		if err := checkWorkspace(ctx, db, *workspace); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}
