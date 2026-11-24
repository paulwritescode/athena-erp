package server

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"

	"athena-erp/internal/store"
	"athena-erp/internal/workspace"
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
		// Starlark Rules, their versions and the Rule Run log (ADR-0016). The
		// unused first athena_rules table is kept as athena_rules_legacy.
		{Version: 11, Name: "starlark_rules", Up: createRules},
		// Access model v2 (ADR-0017): single-use challenges, Role
		// assignments with Scope and dates, user state, and the granular
		// access permissions. Existing Role links are copied to
		// assignments, enabled users become active, disabled users become
		// suspended, and the old `access` permission becomes the granular
		// ones except ownership, so nobody gains or loses sign-in.
		{Version: 12, Name: "access_model", Up: accessModel},
		// Ledger foundation (ADR-0022): kernel-owned masters for companies,
		// dimensions, periods, currencies, accounts, determination and
		// Settings. No rows are seeded: jurisdiction, chart and tax stay
		// open for the sponsor.
		{Version: 13, Name: "ledger_foundation", Up: execAll(
			`CREATE TABLE IF NOT EXISTS athena_companies (
				name VARCHAR(140) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				base_currency VARCHAR(10) NULL,
				disabled TINYINT(1) NOT NULL DEFAULT 0,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_branches (
				name VARCHAR(140) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				company_name VARCHAR(140) NOT NULL,
				parent_name VARCHAR(140) NULL,
				is_group TINYINT(1) NOT NULL DEFAULT 0,
				disabled TINYINT(1) NOT NULL DEFAULT 0,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				KEY idx_branches_company (company_name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_departments (
				name VARCHAR(140) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				company_name VARCHAR(140) NOT NULL,
				branch_name VARCHAR(140) NULL,
				parent_name VARCHAR(140) NULL,
				is_group TINYINT(1) NOT NULL DEFAULT 0,
				disabled TINYINT(1) NOT NULL DEFAULT 0,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				KEY idx_departments_company (company_name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_cost_centres (
				name VARCHAR(140) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				company_name VARCHAR(140) NOT NULL,
				parent_name VARCHAR(140) NULL,
				is_group TINYINT(1) NOT NULL DEFAULT 0,
				disabled TINYINT(1) NOT NULL DEFAULT 0,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				KEY idx_cost_centres_company (company_name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_dimensions (
				name VARCHAR(140) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				members JSON NOT NULL,
				mandatory_for JSON NOT NULL,
				valid_from DATE NULL,
				valid_to DATE NULL,
				disabled TINYINT(1) NOT NULL DEFAULT 0,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_fiscal_years (
				name VARCHAR(140) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				company_name VARCHAR(140) NOT NULL,
				start_date DATE NOT NULL,
				end_date DATE NOT NULL,
				disabled TINYINT(1) NOT NULL DEFAULT 0,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				KEY idx_fiscal_years_company (company_name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_periods (
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL DEFAULT 'main',
				name VARCHAR(140) NOT NULL,
				label VARCHAR(255) NOT NULL,
				fiscal_year_name VARCHAR(140) NULL,
				start_date DATE NOT NULL,
				end_date DATE NOT NULL,
				status VARCHAR(20) NOT NULL DEFAULT 'open',
				status_actor VARCHAR(255) NULL,
				status_at TIMESTAMP NULL,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				PRIMARY KEY (company_name, book, name),
				KEY idx_periods_dates (company_name, book, start_date)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_currencies (
				code VARCHAR(10) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				minor_exponent INT NOT NULL DEFAULT 2,
				disabled TINYINT(1) NOT NULL DEFAULT 0,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_exchange_rates (
				from_code VARCHAR(10) NOT NULL,
				to_code VARCHAR(10) NOT NULL,
				rate_type VARCHAR(40) NOT NULL,
				source VARCHAR(40) NOT NULL,
				effective_at DATETIME(3) NOT NULL,
				rate DECIMAL(30,12) NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (from_code, to_code, rate_type, source, effective_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_account_types (
				name VARCHAR(140) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				normal_balance VARCHAR(10) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_accounts (
				name VARCHAR(140) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				account_type VARCHAR(140) NOT NULL,
				parent_name VARCHAR(140) NULL,
				is_group TINYINT(1) NOT NULL DEFAULT 0,
				class VARCHAR(40) NULL,
				control_role VARCHAR(40) NULL,
				company_name VARCHAR(140) NULL,
				disabled TINYINT(1) NOT NULL DEFAULT 0,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				KEY idx_accounts_parent (parent_name),
				KEY idx_accounts_company (company_name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_account_determinations (
				id CHAR(36) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				account_name VARCHAR(140) NOT NULL,
				match_json JSON NOT NULL,
				disabled TINYINT(1) NOT NULL DEFAULT 0,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				KEY idx_determinations_account (account_name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_settings (
				owner_pack VARCHAR(140) NOT NULL,
				setting_type VARCHAR(140) NOT NULL,
				label VARCHAR(255) NOT NULL,
				value_kind VARCHAR(10) NOT NULL,
				list_rule VARCHAR(10) NULL,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				PRIMARY KEY (owner_pack, setting_type)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_setting_values (
				owner_pack VARCHAR(140) NOT NULL,
				setting_type VARCHAR(140) NOT NULL,
				scope_chain VARCHAR(255) NOT NULL DEFAULT '',
				effective_from DATETIME(3) NOT NULL,
				revision BIGINT NOT NULL,
				value_json JSON NOT NULL,
				status VARCHAR(10) NOT NULL DEFAULT 'draft',
				published_by VARCHAR(255) NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (owner_pack, setting_type, scope_chain, effective_from, revision)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		)},
		// Posting engine (ADR-0023): idempotency operations, posting groups
		// with one row per voucher revision, event and book, the posting log
		// (failed attempts included), the general ledger, running account
		// balances and a minimal outbox the event bus (G8) extends.
		{Version: 14, Name: "posting_engine", Up: execAll(
			`CREATE TABLE IF NOT EXISTS athena_operations (
				id CHAR(36) NOT NULL PRIMARY KEY,
				workspace VARCHAR(140) NULL,
				actor VARCHAR(255) NOT NULL,
				command VARCHAR(40) NOT NULL,
				operation_id VARCHAR(140) NOT NULL,
				payload_hash CHAR(64) NOT NULL,
				result_json JSON NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE KEY uq_operations_replay (actor, command, operation_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_posting_groups (
				id CHAR(36) NOT NULL PRIMARY KEY,
				voucher_type VARCHAR(140) NOT NULL,
				voucher_id CHAR(36) NOT NULL,
				voucher_revision BIGINT NOT NULL,
				event VARCHAR(40) NOT NULL,
				book VARCHAR(40) NOT NULL,
				operation_id VARCHAR(140) NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE KEY uq_posting_groups_once (voucher_type, voucher_id, voucher_revision, event, book)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_posting_log (
				id CHAR(36) NOT NULL PRIMARY KEY,
				voucher_type VARCHAR(140) NOT NULL,
				voucher_id CHAR(36) NOT NULL,
				voucher_revision BIGINT NOT NULL,
				event VARCHAR(40) NOT NULL,
				book VARCHAR(40) NOT NULL,
				operation_id VARCHAR(140) NOT NULL,
				outcome VARCHAR(20) NOT NULL,
				code VARCHAR(40) NOT NULL DEFAULT '',
				message VARCHAR(600) NOT NULL DEFAULT '',
				plan_hash CHAR(64) NOT NULL DEFAULT '',
				actor VARCHAR(255) NOT NULL,
				request_id CHAR(36) NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				KEY idx_posting_log_voucher (voucher_type, voucher_id),
				KEY idx_posting_log_operation (operation_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS l_gl (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				account_name VARCHAR(140) NOT NULL,
				period_name VARCHAR(140) NOT NULL,
				posting_date DATE NOT NULL,
				debit_base DECIMAL(28,6) NOT NULL DEFAULT 0,
				credit_base DECIMAL(28,6) NOT NULL DEFAULT 0,
				txn_currency VARCHAR(10) NULL,
				txn_debit DECIMAL(28,6) NOT NULL DEFAULT 0,
				txn_credit DECIMAL(28,6) NOT NULL DEFAULT 0,
				quantity DECIMAL(28,6) NULL,
				quantity_uom VARCHAR(20) NULL,
				dimensions JSON NOT NULL,
				voucher_type VARCHAR(140) NOT NULL,
				voucher_id CHAR(36) NOT NULL,
				voucher_revision BIGINT NOT NULL,
				event VARCHAR(40) NOT NULL,
				posting_group_id CHAR(36) NOT NULL,
				is_reversal TINYINT(1) NOT NULL DEFAULT 0,
				reversed_row_id CHAR(36) NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				KEY idx_gl_account (company_name, book, account_name),
				KEY idx_gl_voucher (voucher_type, voucher_id),
				KEY idx_gl_group (posting_group_id),
				KEY idx_gl_period (company_name, book, period_name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_balance_account (
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				account_name VARCHAR(140) NOT NULL,
				base_currency VARCHAR(10) NOT NULL,
				debit_total DECIMAL(28,6) NOT NULL DEFAULT 0,
				credit_total DECIMAL(28,6) NOT NULL DEFAULT 0,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				PRIMARY KEY (company_name, book, account_name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_outbox (
				id CHAR(36) NOT NULL PRIMARY KEY,
				event_type VARCHAR(140) NOT NULL,
				payload JSON NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				KEY idx_outbox_type (event_type)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		)},
		{Version: 15, Name: "notifications", Up: execAll(
			`ALTER TABLE athena_challenges ADD COLUMN IF NOT EXISTS issuer_user_id CHAR(36) NULL`,
			`CREATE TABLE IF NOT EXISTS athena_notifications (
            id CHAR(36) NOT NULL PRIMARY KEY,
            recipient_user_id CHAR(36) NOT NULL,
            kind VARCHAR(40) NOT NULL,
            subject_type VARCHAR(64) NOT NULL,
            subject_id VARCHAR(512) NOT NULL,
            subject_label VARCHAR(255) NOT NULL,
            actor_user_id CHAR(36) NULL,
            params JSON NOT NULL,
            created_at DATETIME(6) NOT NULL,
            read_at DATETIME(6) NULL,
            KEY idx_inbox_unread (recipient_user_id, read_at, created_at, id),
            KEY idx_inbox_created (recipient_user_id, created_at, id)
          ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		)},
		{Version: 16, Name: "session_policy", Up: execAll(
			`CREATE TABLE IF NOT EXISTS athena_security_settings (
                setting_key VARCHAR(64) PRIMARY KEY, setting_value INT NOT NULL
            ) ENGINE=InnoDB`,
			`ALTER TABLE athena_sessions
                ADD COLUMN IF NOT EXISTS idle_minutes INT NOT NULL DEFAULT 15,
                ADD COLUMN IF NOT EXISTS step_up_at TIMESTAMP(3) NULL,
                ADD COLUMN IF NOT EXISTS permissions_until DATETIME NULL,
                ADD COLUMN IF NOT EXISTS remembered TINYINT(1) NOT NULL DEFAULT 0,
                ADD COLUMN IF NOT EXISTS rate_id CHAR(36) NULL`,
			// Earlier sessions have no policy snapshot. Require a fresh sign-in.
			`UPDATE athena_sessions SET revoked_at = CURRENT_TIMESTAMP(3) WHERE step_up_at IS NULL AND revoked_at IS NULL`,
		)},
		// Durable events, jobs and workflows (ADR-0028). Extends the minimal
		// athena_outbox from migration 14 with dispatch columns; the id, event
		// type and payload columns stay untouched. Secrets and Record lifetimes
		// are deferred: retention periods and key custody are undecided (Z4).
		{Version: 17, Name: "events_jobs_workflows", Up: execAll(
			`CREATE TABLE IF NOT EXISTS athena_events (
				id CHAR(36) NOT NULL PRIMARY KEY,
				sequence BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
				event_type VARCHAR(140) NOT NULL,
				schema_version INT NOT NULL DEFAULT 1,
				workspace VARCHAR(140) NOT NULL DEFAULT '',
				producer VARCHAR(140) NOT NULL,
				subject_type VARCHAR(140) NOT NULL,
				subject_id VARCHAR(512) NOT NULL,
				subject_revision BIGINT NOT NULL DEFAULT 0,
				actor VARCHAR(255) NOT NULL DEFAULT '',
				operation_id VARCHAR(140) NOT NULL DEFAULT '',
				correlation_id CHAR(36) NOT NULL,
				causation_id CHAR(36) NULL,
				payload JSON NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE KEY uq_events_sequence (sequence),
				KEY idx_events_type (event_type),
				KEY idx_events_subject (subject_type, subject_id(191))
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`ALTER TABLE athena_outbox
				ADD COLUMN IF NOT EXISTS event_id CHAR(36) NULL AFTER id,
				ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'pending' AFTER payload,
				ADD COLUMN IF NOT EXISTS attempts INT NOT NULL DEFAULT 0 AFTER status,
				ADD COLUMN IF NOT EXISTS available_at DATETIME(6) NULL AFTER attempts,
				ADD COLUMN IF NOT EXISTS leased_by VARCHAR(255) NULL AFTER available_at,
				ADD COLUMN IF NOT EXISTS leased_until DATETIME(6) NULL AFTER leased_by,
				ADD COLUMN IF NOT EXISTS last_error TEXT NULL AFTER leased_until,
				ADD COLUMN IF NOT EXISTS dispatched_at DATETIME(6) NULL AFTER last_error,
				ADD INDEX IF NOT EXISTS idx_outbox_dispatch (status, available_at)`,
			`CREATE TABLE IF NOT EXISTS athena_inbox (
				consumer VARCHAR(140) NOT NULL,
				event_id CHAR(36) NOT NULL,
				effect_key VARCHAR(200) NOT NULL DEFAULT '',
				result VARCHAR(20) NOT NULL DEFAULT 'applied',
				detail JSON NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (consumer, event_id, effect_key)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_event_subscriptions (
				consumer VARCHAR(140) NOT NULL,
				event_type VARCHAR(140) NOT NULL,
				mode VARCHAR(20) NOT NULL DEFAULT 'async',
				action VARCHAR(140) NOT NULL,
				effect_key VARCHAR(200) NOT NULL DEFAULT '',
				exhaust_behavior VARCHAR(40) NOT NULL DEFAULT 'dead',
				pack VARCHAR(140) NULL,
				enabled TINYINT(1) NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				PRIMARY KEY (consumer, event_type)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_jobs (
				id CHAR(36) NOT NULL PRIMARY KEY,
				kind VARCHAR(140) NOT NULL,
				payload JSON NOT NULL,
				status VARCHAR(20) NOT NULL DEFAULT 'queued',
				attempts INT NOT NULL DEFAULT 0,
				max_attempts INT NOT NULL DEFAULT 5,
				next_run_at DATETIME(6) NULL,
				lease_owner VARCHAR(255) NULL,
				lease_expires_at DATETIME(6) NULL,
				fencing BIGINT NOT NULL DEFAULT 0,
				occurrence_key VARCHAR(280) NULL,
				owner_user_id CHAR(36) NULL,
				owner_role VARCHAR(140) NULL,
				last_error TEXT NULL,
				result JSON NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				KEY idx_jobs_claim (status, next_run_at),
				KEY idx_jobs_occurrence (occurrence_key)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_schedule_occurrences (
				workspace VARCHAR(140) NOT NULL,
				schedule_name VARCHAR(140) NOT NULL,
				scope VARCHAR(255) NOT NULL DEFAULT '',
				period_key VARCHAR(64) NOT NULL,
				timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
				catchup_policy VARCHAR(20) NOT NULL DEFAULT 'skip',
				status VARCHAR(20) NOT NULL DEFAULT 'pending',
				lease_owner VARCHAR(255) NULL,
				lease_expires_at DATETIME(6) NULL,
				watermark DATETIME(6) NULL,
				result JSON NULL,
				job_id CHAR(36) NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				PRIMARY KEY (workspace, schedule_name, scope, period_key)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_workflow_instances (
				id CHAR(36) NOT NULL PRIMARY KEY,
				workflow_name VARCHAR(140) NOT NULL,
				definition_version INT NOT NULL DEFAULT 1,
				definition JSON NOT NULL,
				phase VARCHAR(40) NOT NULL DEFAULT 'running',
				status VARCHAR(20) NOT NULL DEFAULT 'open',
				expected_facts JSON NOT NULL,
				received_facts JSON NOT NULL,
				wait_event VARCHAR(140) NULL,
				wait_until DATETIME(6) NULL,
				due_at DATETIME(6) NULL,
				assigned_role VARCHAR(140) NULL,
				assigned_scope VARCHAR(255) NULL,
				cycle_count INT NOT NULL DEFAULT 0,
				cycle_budget INT NOT NULL DEFAULT 25,
				evidence JSON NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				KEY idx_workflow_instances_status (status, wait_until),
				KEY idx_workflow_instances_name (workflow_name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_tasks (
				id CHAR(36) NOT NULL PRIMARY KEY,
				instance_id CHAR(36) NOT NULL,
				task_key VARCHAR(200) NOT NULL,
				title VARCHAR(255) NOT NULL,
				status VARCHAR(20) NOT NULL DEFAULT 'open',
				assigned_role VARCHAR(140) NULL,
				assigned_scope VARCHAR(255) NULL,
				due_at DATETIME(6) NULL,
				completed_at DATETIME(6) NULL,
				evidence JSON NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				UNIQUE KEY uq_tasks_gate (instance_id, task_key),
				KEY idx_tasks_instance (instance_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		)},
		// Trees, effective dating and UoM (ADR-0029): UoM families and
		// conversions, materialised path and depth on the kernel tree
		// masters, and effective ranges on account determinations. Record
		// Type tree and effective-date columns are added by the publisher
		// when the flag is set, so no per-Record tables change here.
		{Version: 18, Name: "trees_effective_uom", Up: treesEffectiveUoM},
		// Version 19 widens athena_jobs.occurrence_key to the 602-char
		// maximum an occurrence key can reach (140+140+255+64 part chars
		// plus separators), with headroom; it stays well under the
		// utf8mb4 index limit. Corrective widen under ADR-0028 (C2
		// slice-3 review F3).
		{Version: 19, Name: "widen_job_occurrence_key", Up: execAll(
			`ALTER TABLE athena_jobs MODIFY COLUMN occurrence_key VARCHAR(700) NULL`,
		)},
		// Party ledger (ADR-0031): append-only l_party facts keyed by
		// party and item, mutable open items per instalment, and
		// append-only settlement application lines. Nothing is seeded:
		// items and applications are Workspace data written by the
		// settlement service (G7.2) only.
		{Version: 20, Name: "party_ledger", Up: execAll(
			`CREATE TABLE IF NOT EXISTS l_party (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				party_type VARCHAR(140) NOT NULL,
				party_id CHAR(36) NOT NULL,
				control_account_name VARCHAR(140) NOT NULL,
				item_id CHAR(36) NULL,
				application_id CHAR(36) NULL,
				posting_date DATE NOT NULL,
				debit_base DECIMAL(28,6) NOT NULL DEFAULT 0,
				credit_base DECIMAL(28,6) NOT NULL DEFAULT 0,
				txn_currency VARCHAR(10) NULL,
				txn_debit DECIMAL(28,6) NOT NULL DEFAULT 0,
				txn_credit DECIMAL(28,6) NOT NULL DEFAULT 0,
				voucher_type VARCHAR(140) NOT NULL,
				voucher_id CHAR(36) NOT NULL,
				voucher_revision BIGINT NOT NULL,
				event VARCHAR(40) NOT NULL,
				posting_group_id CHAR(36) NOT NULL,
				is_reversal TINYINT(1) NOT NULL DEFAULT 0,
				reversed_row_id CHAR(36) NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				KEY idx_party_ledger_party (company_name, book, party_type, party_id, posting_date),
				KEY idx_party_ledger_control (company_name, book, control_account_name, posting_date),
				KEY idx_party_ledger_item (item_id),
				KEY idx_party_ledger_voucher (voucher_type, voucher_id),
				KEY idx_party_ledger_group (posting_group_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_party_items (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				party_type VARCHAR(140) NOT NULL,
				party_id CHAR(36) NOT NULL,
				control_account_name VARCHAR(140) NOT NULL,
				source_voucher_type VARCHAR(140) NOT NULL,
				source_voucher_id CHAR(36) NOT NULL,
				source_voucher_revision BIGINT NOT NULL,
				instalment_no INT NOT NULL,
				due_date DATE NULL,
				currency VARCHAR(10) NOT NULL,
				side VARCHAR(6) NOT NULL,
				original_amount DECIMAL(28,6) NOT NULL,
				residual_amount DECIMAL(28,6) NOT NULL,
				status VARCHAR(20) NOT NULL DEFAULT 'open',
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				UNIQUE KEY uq_party_items_source (source_voucher_type, source_voucher_id, source_voucher_revision, instalment_no),
				KEY idx_party_items_party (company_name, book, party_type, party_id, status),
				KEY idx_party_items_open (company_name, book, status, due_date)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_settlement_applications (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				event VARCHAR(40) NOT NULL,
				settlement_voucher_type VARCHAR(140) NOT NULL,
				settlement_voucher_id CHAR(36) NOT NULL,
				settlement_voucher_revision BIGINT NOT NULL,
				settlement_line_no INT NOT NULL,
				item_id CHAR(36) NOT NULL,
				payment_currency VARCHAR(10) NOT NULL,
				payment_amount DECIMAL(28,6) NOT NULL,
				item_currency VARCHAR(10) NOT NULL,
				item_amount DECIMAL(28,6) NOT NULL,
				base_amount DECIMAL(28,6) NOT NULL,
				is_reversal TINYINT(1) NOT NULL DEFAULT 0,
				reverses_application_id CHAR(36) NULL,
				posting_group_id CHAR(36) NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE KEY uq_settlement_apps_line (settlement_voucher_type, settlement_voucher_id, settlement_voucher_revision, event, settlement_line_no, item_id),
				KEY idx_settlement_apps_item (item_id),
				KEY idx_settlement_apps_group (posting_group_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		)},
		// Stock ledger (ADR-0032): append-only l_stock quantity/value
		// facts and l_stock_value cost deltas, receipt layers with
		// append-only item applications, rebuildable bin balances,
		// reservations, serial and batch identity, integrity-log
		// entries and rebuild state. Nothing is seeded: layers, bins,
		// reservations, serials and batches are Workspace data written
		// by the stock service (G11.2) only.
		{Version: 21, Name: "stock_ledger", Up: execAll(
			`CREATE TABLE IF NOT EXISTS l_stock (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				owner VARCHAR(60) NOT NULL,
				item_type VARCHAR(140) NOT NULL,
				item_id CHAR(36) NOT NULL,
				control_account_name VARCHAR(140) NOT NULL,
				location_name VARCHAR(140) NOT NULL,
				lot_no VARCHAR(140) NULL,
				serial_no VARCHAR(140) NULL,
				base_uom VARCHAR(32) NOT NULL,
				qty_base DECIMAL(28,6) NOT NULL,
				value_base DECIMAL(28,6) NOT NULL DEFAULT 0,
				base_currency VARCHAR(10) NOT NULL,
				movement_date DATE NOT NULL,
				posting_date DATE NOT NULL,
				dimensions JSON NOT NULL,
				voucher_type VARCHAR(140) NOT NULL,
				voucher_id CHAR(36) NOT NULL,
				voucher_revision BIGINT NOT NULL,
				source_line_no INT NOT NULL,
				event VARCHAR(40) NOT NULL,
				posting_group_id CHAR(36) NOT NULL,
				is_reversal TINYINT(1) NOT NULL DEFAULT 0,
				reversed_row_id CHAR(36) NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				KEY idx_stock_item (company_name, book, item_type, item_id, location_name, movement_date),
				KEY idx_stock_control (company_name, book, control_account_name, posting_date),
				KEY idx_stock_lot (item_type, item_id, lot_no),
				KEY idx_stock_serial (company_name, book, serial_no),
				KEY idx_stock_voucher (voucher_type, voucher_id),
				KEY idx_stock_group (posting_group_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS l_stock_value (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				stock_row_id CHAR(36) NOT NULL,
				item_type VARCHAR(140) NOT NULL,
				item_id CHAR(36) NOT NULL,
				control_account_name VARCHAR(140) NOT NULL,
				delta_value DECIMAL(28,6) NOT NULL,
				on_hand_delta DECIMAL(28,6) NOT NULL DEFAULT 0,
				sold_delta DECIMAL(28,6) NOT NULL DEFAULT 0,
				base_currency VARCHAR(10) NOT NULL,
				economic_date DATE NOT NULL,
				posting_date DATE NOT NULL,
				voucher_type VARCHAR(140) NOT NULL,
				voucher_id CHAR(36) NOT NULL,
				voucher_revision BIGINT NOT NULL,
				source_line_no INT NOT NULL,
				event VARCHAR(40) NOT NULL,
				posting_group_id CHAR(36) NOT NULL,
				is_reversal TINYINT(1) NOT NULL DEFAULT 0,
				reversed_row_id CHAR(36) NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				KEY idx_stock_value_row (stock_row_id),
				KEY idx_stock_value_item (company_name, book, item_type, item_id, posting_date),
				KEY idx_stock_value_control (company_name, book, control_account_name, posting_date),
				KEY idx_stock_value_group (posting_group_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_stock_layers (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				item_type VARCHAR(140) NOT NULL,
				item_id CHAR(36) NOT NULL,
				location_name VARCHAR(140) NOT NULL,
				lot_no VARCHAR(140) NULL,
				receipt_row_id CHAR(36) NOT NULL,
				base_uom VARCHAR(32) NOT NULL,
				original_qty DECIMAL(28,6) NOT NULL,
				residual_qty DECIMAL(28,6) NOT NULL,
				unit_cost DECIMAL(28,6) NOT NULL,
				base_currency VARCHAR(10) NOT NULL,
				status VARCHAR(20) NOT NULL DEFAULT 'open',
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				UNIQUE KEY uq_stock_layers_receipt (receipt_row_id),
				KEY idx_stock_layers_item (company_name, book, item_id, location_name, status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_item_applications (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				event VARCHAR(40) NOT NULL,
				kind VARCHAR(20) NOT NULL,
				issue_row_id CHAR(36) NOT NULL,
				receipt_layer_id CHAR(36) NOT NULL,
				applied_qty DECIMAL(28,6) NOT NULL,
				applied_value DECIMAL(28,6) NOT NULL,
				base_currency VARCHAR(10) NOT NULL,
				voucher_type VARCHAR(140) NOT NULL,
				voucher_id CHAR(36) NOT NULL,
				voucher_revision BIGINT NOT NULL,
				posting_group_id CHAR(36) NOT NULL,
				is_reversal TINYINT(1) NOT NULL DEFAULT 0,
				reverses_application_id CHAR(36) NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				KEY idx_item_apps_issue (issue_row_id),
				KEY idx_item_apps_layer (receipt_layer_id),
				KEY idx_item_apps_group (posting_group_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_stock_bins (
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				owner VARCHAR(60) NOT NULL,
				item_type VARCHAR(140) NOT NULL,
				item_id CHAR(36) NOT NULL,
				location_name VARCHAR(140) NOT NULL,
				lot_no VARCHAR(140) NOT NULL DEFAULT '',
				serial_no VARCHAR(140) NOT NULL DEFAULT '',
				base_uom VARCHAR(32) NOT NULL,
				on_hand DECIMAL(28,6) NOT NULL DEFAULT 0,
				reserved_qty DECIMAL(28,6) NOT NULL DEFAULT 0,
				value_on_hand DECIMAL(28,6) NOT NULL DEFAULT 0,
				base_currency VARCHAR(10) NOT NULL,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				PRIMARY KEY (company_name, book, owner, item_id, location_name, lot_no, serial_no),
				KEY idx_stock_bins_item (company_name, book, item_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_reservations (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				owner VARCHAR(60) NOT NULL,
				item_type VARCHAR(140) NOT NULL,
				item_id CHAR(36) NOT NULL,
				location_name VARCHAR(140) NOT NULL,
				lot_no VARCHAR(140) NOT NULL DEFAULT '',
				serial_no VARCHAR(140) NOT NULL DEFAULT '',
				base_uom VARCHAR(32) NOT NULL,
				qty DECIMAL(28,6) NOT NULL,
				status VARCHAR(20) NOT NULL DEFAULT 'active',
				holder_type VARCHAR(140) NOT NULL,
				holder_id CHAR(36) NOT NULL,
				expires_at TIMESTAMP NULL,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				UNIQUE KEY uq_reservations_hold (holder_type, holder_id, item_id, location_name, lot_no, serial_no),
				KEY idx_reservations_item (company_name, book, item_id, location_name, status),
				KEY idx_reservations_holder (holder_type, holder_id, status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_serials (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				item_type VARCHAR(140) NOT NULL,
				item_id CHAR(36) NOT NULL,
				serial_no VARCHAR(140) NOT NULL,
				status VARCHAR(20) NOT NULL DEFAULT 'on_hand',
				owner VARCHAR(60) NOT NULL,
				location_name VARCHAR(140) NOT NULL,
				lot_no VARCHAR(140) NOT NULL DEFAULT '',
				receipt_row_id CHAR(36) NULL,
				issue_row_id CHAR(36) NULL,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				UNIQUE KEY uq_serials_unit (company_name, book, item_id, serial_no),
				KEY idx_serials_status (company_name, book, item_id, status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_batches (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				item_type VARCHAR(140) NOT NULL,
				item_id CHAR(36) NOT NULL,
				lot_no VARCHAR(140) NOT NULL,
				expiry_date DATE NULL,
				received_date DATE NULL,
				status VARCHAR(20) NOT NULL DEFAULT 'on_hand',
				receipt_row_id CHAR(36) NULL,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				UNIQUE KEY uq_batches_lot (company_name, book, item_id, lot_no),
				KEY idx_batches_expiry (company_name, book, item_id, expiry_date)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_integrity_log (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				check_name VARCHAR(140) NOT NULL,
				control_account_name VARCHAR(140) NOT NULL,
				cutoff_date DATE NOT NULL,
				expected_value DECIMAL(28,6) NOT NULL,
				observed_value DECIMAL(28,6) NOT NULL,
				base_currency VARCHAR(10) NOT NULL,
				owner VARCHAR(255) NOT NULL,
				status VARCHAR(20) NOT NULL DEFAULT 'open',
				detail JSON NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				KEY idx_integrity_check (company_name, book, check_name, control_account_name, cutoff_date),
				KEY idx_integrity_status (status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_stock_rebuilds (
				id CHAR(36) NOT NULL PRIMARY KEY,
				company_name VARCHAR(140) NOT NULL,
				book VARCHAR(40) NOT NULL,
				state VARCHAR(20) NOT NULL DEFAULT 'running',
				watermark_row_id CHAR(36) NULL,
				started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				completed_at TIMESTAMP NULL,
				error VARCHAR(600) NULL,
				KEY idx_stock_rebuilds_state (company_name, book, state)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		)},
		// Secrets and retention (ADR-0033, Z4 decided 2026-11-24):
		// references (never values), key versions for rotation, the
		// disposal register and holds. Nothing is seeded: references,
		// versions, register rows and holds are Workspace data written
		// by the secrets service (8.8/8.9) and the disposal job (8.10)
		// only. Key material never lands in these tables.
		{Version: 22, Name: "secrets_retention", Up: execAll(
			`CREATE TABLE IF NOT EXISTS athena_secret_refs (
				id CHAR(36) NOT NULL PRIMARY KEY,
				name VARCHAR(140) NOT NULL,
				provider VARCHAR(40) NOT NULL,
				provider_ref VARCHAR(600) NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				UNIQUE KEY uq_secret_refs_name (name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_key_versions (
				id CHAR(36) NOT NULL PRIMARY KEY,
				key_name VARCHAR(140) NOT NULL,
				version BIGINT NOT NULL,
				status VARCHAR(20) NOT NULL DEFAULT 'active',
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE KEY uq_key_versions_name_version (key_name, version)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_disposal_register (
				id CHAR(36) NOT NULL PRIMARY KEY,
				record_type VARCHAR(140) NOT NULL,
				record_id CHAR(36) NOT NULL,
				action VARCHAR(40) NOT NULL,
				outcome VARCHAR(20) NOT NULL,
				detail VARCHAR(600) NOT NULL DEFAULT '',
				actor VARCHAR(255) NOT NULL DEFAULT '',
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				KEY idx_disposal_register_record (record_type, record_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS athena_holds (
				id CHAR(36) NOT NULL PRIMARY KEY,
				record_type VARCHAR(140) NOT NULL,
				record_id CHAR(36) NOT NULL,
				reason VARCHAR(600) NOT NULL DEFAULT '',
				placed_by VARCHAR(255) NOT NULL DEFAULT '',
				released_at TIMESTAMP NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				KEY idx_holds_record (record_type, record_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		)},
	}
}

// treesEffectiveUoM creates the UoM masters, adds tree and effective-date
// columns to the kernel ledger tables, and backfills path and depth from
// the stored parent links. Nothing is seeded: families, units and ranges
// are Workspace data.
func treesEffectiveUoM(ctx context.Context, x store.Execer) error {
	err := execAll(
		`CREATE TABLE IF NOT EXISTS athena_uom_families (
				name VARCHAR(140) NOT NULL PRIMARY KEY,
				label VARCHAR(255) NOT NULL,
				base_unit VARCHAR(32) NOT NULL,
				disabled TINYINT(1) NOT NULL DEFAULT 0,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS athena_uom_conversions (
				family_name VARCHAR(140) NOT NULL,
				unit VARCHAR(32) NOT NULL,
				item_key VARCHAR(140) NOT NULL DEFAULT '',
				factor DECIMAL(30,12) NOT NULL,
				scale INT NOT NULL DEFAULT 6,
				disabled TINYINT(1) NOT NULL DEFAULT 0,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				PRIMARY KEY (family_name, unit, item_key),
				KEY idx_uom_conversions_family (family_name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`ALTER TABLE athena_accounts
				ADD COLUMN IF NOT EXISTS tree_path VARCHAR(2048) NULL AFTER parent_name,
				ADD COLUMN IF NOT EXISTS tree_depth INT NULL AFTER tree_path,
				ADD INDEX IF NOT EXISTS idx_accounts_tree (tree_path(191))`,
		`ALTER TABLE athena_branches
				ADD COLUMN IF NOT EXISTS tree_path VARCHAR(2048) NULL AFTER parent_name,
				ADD COLUMN IF NOT EXISTS tree_depth INT NULL AFTER tree_path,
				ADD INDEX IF NOT EXISTS idx_branches_tree (tree_path(191))`,
		`ALTER TABLE athena_departments
				ADD COLUMN IF NOT EXISTS tree_path VARCHAR(2048) NULL AFTER parent_name,
				ADD COLUMN IF NOT EXISTS tree_depth INT NULL AFTER tree_path,
				ADD INDEX IF NOT EXISTS idx_departments_tree (tree_path(191))`,
		`ALTER TABLE athena_cost_centres
				ADD COLUMN IF NOT EXISTS tree_path VARCHAR(2048) NULL AFTER parent_name,
				ADD COLUMN IF NOT EXISTS tree_depth INT NULL AFTER tree_path,
				ADD INDEX IF NOT EXISTS idx_cost_centres_tree (tree_path(191))`,
		`ALTER TABLE athena_account_determinations
				ADD COLUMN IF NOT EXISTS effective_from DATE NULL AFTER match_json,
				ADD COLUMN IF NOT EXISTS effective_to DATE NULL AFTER effective_from,
				ADD INDEX IF NOT EXISTS idx_determinations_effective (effective_from, effective_to)`,
	)(ctx, x)
	if err != nil {
		return err
	}
	for _, table := range []string{"athena_accounts", "athena_branches", "athena_departments", "athena_cost_centres"} {
		if err := backfillTreePaths(ctx, x, table); err != nil {
			return err
		}
	}
	return nil
}

// backfillTreePaths materialises slash-joined paths (`/root/child/`, root
// depth 0) from parent_name links. Names hold no slashes, so the join is
// unambiguous. Roots are written first, then each pass resolves children
// of resolved parents; a bounded pass count keeps a corrupt chain (cycle
// or dangling parent) from looping forever, and any row left without a
// path fails the migration naming the table. Saved masters already refuse
// cycles and missing parents, so a failure means hand-edited rows.
func backfillTreePaths(ctx context.Context, x store.Execer, table string) error {
	if _, err := x.ExecContext(ctx, `UPDATE `+table+` SET tree_path = CONCAT('/', name, '/'), tree_depth = 0 WHERE parent_name IS NULL AND tree_path IS NULL`); err != nil {
		return err
	}
	for range 100 {
		result, err := x.ExecContext(ctx, `UPDATE `+table+` AS child JOIN `+table+` AS parent ON child.parent_name = parent.name SET child.tree_path = CONCAT(parent.tree_path, child.name, '/'), child.tree_depth = parent.tree_depth + 1 WHERE child.tree_path IS NULL AND parent.tree_path IS NOT NULL`)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			break
		}
	}
	var left int
	if err := x.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE tree_path IS NULL`).Scan(&left); err != nil {
		return err
	}
	if left > 0 {
		return fmt.Errorf("migration 18: %d rows in %s have no tree path (cycle or missing parent)", left, table)
	}
	return nil
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
func MigrationStatus(ctx context.Context, endpoint *workspace.DBEndpoint, svc *Workspace) ([]store.MigrationState, error) {
	db, err := openMigrationDB(ctx, endpoint, svc)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return store.MigrationStatus(ctx, db, dialect, SystemMigrations())
}

// Migrate applies pending system migrations the way the server does at
// start-up and returns the ones it applied (or would apply with DryRun).
func Migrate(ctx context.Context, endpoint *workspace.DBEndpoint, svc *Workspace, opts MigrationOptions) ([]store.Migration, error) {
	db, err := openMigrationDB(ctx, endpoint, svc)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{Target: opts.Target, DryRun: opts.DryRun})
}

func openMigrationDB(ctx context.Context, endpoint *workspace.DBEndpoint, svc *Workspace) (*sql.DB, error) {
	dialect = dialectFor(endpoint.Driver)
	driverName := "MariaDB"
	if endpoint.Driver == workspace.DriverPostgres {
		driverName = "PostgreSQL"
	}
	connector, err := EndpointConnector(endpoint)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to %s: %w", driverName, err)
	}
	if svc != nil {
		if err := checkWorkspace(ctx, db, *svc); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

// accessModel moves the access tables to model v2. New columns arrive
// nullable where existing rows need a backfill, then turn NOT NULL. The
// old athena_user_roles links stay: code reads them until G1.5 switches to
// assignments, and they remain the rollback source.
func accessModel(ctx context.Context, x store.Execer) error {
	return execAll(
		`CREATE TABLE IF NOT EXISTS athena_challenges (
			id CHAR(36) NOT NULL PRIMARY KEY,
			user_id CHAR(36) NOT NULL,
			purpose VARCHAR(20) NOT NULL,
			digest CHAR(64) NOT NULL,
			generation INT NOT NULL,
			expires_at TIMESTAMP(3) NOT NULL,
			consumed_at TIMESTAMP(3) NULL,
			attempts INT NOT NULL DEFAULT 0,
			created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			UNIQUE KEY uq_challenges_digest (digest),
			KEY idx_challenges_user (user_id),
			CONSTRAINT fk_challenges_user FOREIGN KEY (user_id) REFERENCES athena_users (id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS athena_role_assignments (
			id CHAR(36) NOT NULL PRIMARY KEY,
			user_id CHAR(36) NOT NULL,
			role_id CHAR(36) NOT NULL,
			scope JSON NOT NULL,
			active_from DATE NULL,
			expires_at DATE NULL,
			revision BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
			KEY idx_role_assignments_user (user_id),
			KEY idx_role_assignments_role (role_id),
			CONSTRAINT fk_role_assignments_user FOREIGN KEY (user_id) REFERENCES athena_users (id) ON DELETE CASCADE,
			CONSTRAINT fk_role_assignments_role FOREIGN KEY (role_id) REFERENCES athena_roles (id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`ALTER TABLE athena_users ADD COLUMN IF NOT EXISTS state VARCHAR(20) NULL`,
		`UPDATE athena_users SET state = IF(disabled = 1, 'suspended', 'active') WHERE state IS NULL`,
		`ALTER TABLE athena_users MODIFY COLUMN state VARCHAR(20) NOT NULL DEFAULT 'active'`,
		`ALTER TABLE athena_users ADD COLUMN IF NOT EXISTS generation INT NOT NULL DEFAULT 0`,
		`ALTER TABLE athena_users ADD COLUMN IF NOT EXISTS invite_delivery VARCHAR(20) NOT NULL DEFAULT 'manual'`,
		`ALTER TABLE athena_users MODIFY COLUMN password_hash VARCHAR(255) NULL`,
		`ALTER TABLE athena_roles ADD COLUMN IF NOT EXISTS requires_scope JSON NULL`,
		`UPDATE athena_roles SET requires_scope = '[]' WHERE requires_scope IS NULL`,
		`ALTER TABLE athena_roles MODIFY COLUMN requires_scope JSON NOT NULL`,
		"ALTER TABLE athena_roles ADD COLUMN IF NOT EXISTS `system` TINYINT(1) NOT NULL DEFAULT 0",
		`UPDATE athena_roles SET system_permissions = REPLACE(system_permissions, '"access"', '"access:assign", "access:explore", "access:recover", "access:roles", "access:sessions", "access:users"')
			WHERE system_permissions LIKE '%"access"%'`,
		`ALTER TABLE athena_sessions ADD COLUMN IF NOT EXISTS device VARCHAR(128) NULL`,
		`ALTER TABLE athena_sessions ADD COLUMN IF NOT EXISTS address_digest CHAR(64) NULL`,
		`INSERT INTO athena_role_assignments (id, user_id, role_id, scope, active_from, expires_at, revision)
			SELECT UUID(), ur.user_id, ur.role_id, '{}', NULL, NULL, 1 FROM athena_user_roles ur
			WHERE NOT EXISTS (SELECT 1 FROM athena_role_assignments a
				WHERE a.user_id = ur.user_id AND a.role_id = ur.role_id AND a.active_from IS NULL AND a.expires_at IS NULL)`,
	)(ctx, x)
}

// createRules gives Starlark Rules their tables. The first athena_rules table
// (name, label, pack_name, definition JSON) was never read or written by any
// code; it is renamed athena_rules_legacy with its rows, not dropped. Rule Run rows hold hashes and counts, never Record values.
func createRules(ctx context.Context, x store.Execer) error {
	// The old table has a definition column; the new one does not, which
	// also makes a repeated run harmless.
	legacy, err := dialect.ColumnExists(ctx, x, "athena_rules", "definition")
	if err != nil {
		return err
	}
	if legacy {
		if _, err := x.ExecContext(ctx, "RENAME TABLE athena_rules TO athena_rules_legacy"); err != nil {
			return err
		}
	}
	return execAll(
		`CREATE TABLE IF NOT EXISTS athena_rules (
			name VARCHAR(63) NOT NULL PRIMARY KEY,
			label VARCHAR(255) NOT NULL,
			rule_type VARCHAR(20) NOT NULL,
			record_type VARCHAR(140) NULL,
			event VARCHAR(40) NULL,
			priority INT NOT NULL DEFAULT 100,
			enabled TINYINT(1) NOT NULL DEFAULT 1,
			version BIGINT NOT NULL,
			source MEDIUMTEXT NOT NULL,
			source_hash CHAR(64) NOT NULL,
			modified_by VARCHAR(255) NOT NULL,
			created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			KEY idx_rules_event (record_type, event, enabled)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS athena_rule_versions (
			rule_name VARCHAR(63) NOT NULL,
			version BIGINT NOT NULL,
			label VARCHAR(255) NOT NULL,
			rule_type VARCHAR(20) NOT NULL,
			record_type VARCHAR(140) NULL,
			event VARCHAR(40) NULL,
			priority INT NOT NULL,
			enabled TINYINT(1) NOT NULL,
			source MEDIUMTEXT NOT NULL,
			source_hash CHAR(64) NOT NULL,
			actor VARCHAR(255) NOT NULL,
			request_id CHAR(36) NULL,
			created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			PRIMARY KEY (rule_name, version)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS athena_rule_runs (
			id CHAR(36) NOT NULL PRIMARY KEY,
			rule_name VARCHAR(63) NOT NULL,
			rule_version BIGINT NOT NULL,
			rule_type VARCHAR(20) NOT NULL,
			record_type VARCHAR(140) NULL,
			record_id CHAR(36) NULL,
			event VARCHAR(40) NULL,
			input_hash CHAR(64) NOT NULL,
			outcome VARCHAR(20) NOT NULL,
			code VARCHAR(40) NOT NULL DEFAULT '',
			message VARCHAR(600) NOT NULL DEFAULT '',
			steps BIGINT NOT NULL,
			rows_read INT NOT NULL,
			rows_written INT NOT NULL,
			duration_micros BIGINT NOT NULL,
			dry_run TINYINT(1) NOT NULL,
			actor VARCHAR(255) NOT NULL,
			request_id CHAR(36) NULL,
			messages JSON NOT NULL,
			created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			KEY idx_rule_runs_rule (rule_name, id),
			KEY idx_rule_runs_record (record_type, record_id),
			KEY idx_rule_runs_request (request_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	)(ctx, x)
}
