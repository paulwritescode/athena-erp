package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"athena-erp/internal/auth"
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

// TestEventsJobsWorkflowsMigrationExtendsTheOutbox builds a database the way
// migration 14 leaves it, inserts a minimal posting outbox row, upgrades to
// migration 17 and checks the row survives with its id, event type and
// payload intact, the dispatch columns default to pending, and the new
// events, inbox, subscription, job, occurrence, workflow and task tables
// exist with no secret or disposal tables (Z4 still open at 17). It then
// upgrades fully and checks the secret, key-version, disposal and hold
// tables exist (Z4 decided 2026-11-24, migration 22).
func TestEventsJobsWorkflowsMigrationExtendsTheOutbox(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{Target: 14}); err != nil {
		t.Fatal(err)
	}
	const rowID = "01911111-2222-7333-8444-555555555555"
	if _, err := db.ExecContext(ctx, `INSERT INTO athena_outbox (id, event_type, payload) VALUES (?, 'posting.posted', '{"row_count":2}')`, rowID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{Target: 17}); err != nil {
		t.Fatal(err)
	}
	var id, eventType, payload, status string
	var eventID sql.NullString
	var attempts int
	var dispatchedAt sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT id, event_type, CAST(payload AS CHAR), status, event_id, attempts, dispatched_at FROM athena_outbox`).Scan(&id, &eventType, &payload, &status, &eventID, &attempts, &dispatchedAt); err != nil {
		t.Fatalf("legacy outbox row: %v", err)
	}
	if id != rowID || eventType != "posting.posted" || payload != `{"row_count":2}` {
		t.Errorf("legacy outbox row became %q %q %q, want id, posting.posted and payload intact", id, eventType, payload)
	}
	if status != "pending" || eventID.Valid || attempts != 0 || dispatchedAt.Valid {
		t.Errorf("legacy outbox dispatch columns are %q %v %d %v, want pending, NULL, 0, NULL", status, eventID, attempts, dispatchedAt)
	}
	for _, table := range []string{"athena_events", "athena_inbox", "athena_event_subscriptions", "athena_jobs", "athena_schedule_occurrences", "athena_workflow_instances", "athena_tasks"} {
		var name string
		if err := db.QueryRowContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?`, table).Scan(&name); err != nil {
			t.Errorf("table %s missing after migration 17: %v", table, err)
		}
	}
	for _, table := range []string{"athena_secret_refs", "athena_key_versions", "athena_disposal_register", "athena_holds"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("table %s exists at migration 17, want it deferred until Z4 rules on retention and custody", table)
		}
	}
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"athena_secret_refs", "athena_key_versions", "athena_disposal_register", "athena_holds"} {
		var name string
		if err := db.QueryRowContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?`, table).Scan(&name); err != nil {
			t.Errorf("table %s missing after migration 22: %v", table, err)
		}
	}
}

// TestAccessModelMigrationPreservesAccess builds a database the way the
// current build leaves it (migrations 1 to 11), adds users, a Role and a
// session, upgrades to migration 12 and checks that nobody gains or loses
// sign-in: enabled users stay active, the disabled user stays refused, Role
// links are copied to assignments, and the old `access` permission becomes
// the granular access permissions.
func TestAccessModelMigrationPreservesAccess(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{Target: 11}); err != nil {
		t.Fatal(err)
	}
	const password = "correct horse battery staple"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	exec(`INSERT INTO athena_users (id, username, display_name, password_hash, is_owner) VALUES
		('11111111-1111-1111-1111-111111111111', 'ada', 'Ada', ?, 1)`, hash)
	exec(`INSERT INTO athena_users (id, username, display_name, password_hash) VALUES
		('22222222-2222-2222-2222-222222222222', 'carol', 'Carol', ?)`, hash)
	exec(`INSERT INTO athena_users (id, username, display_name, password_hash, disabled) VALUES
		('33333333-3333-3333-3333-333333333333', 'bob', 'Bob', ?, 1)`, hash)
	exec(`INSERT INTO athena_roles (id, name, description, system_permissions) VALUES
		('44444444-4444-4444-4444-444444444444', 'clerk', '', '["access"]')`)
	exec(`INSERT INTO athena_grants (id, role_id, record_type, actions, permission_level) VALUES
		('55555555-5555-5555-5555-555555555555', '44444444-4444-4444-4444-444444444444', 'customer', '["read","write"]', 0)`)
	exec(`INSERT INTO athena_user_roles (user_id, role_id) VALUES
		('22222222-2222-2222-2222-222222222222', '44444444-4444-4444-4444-444444444444')`)
	exec(`INSERT INTO athena_sessions (id, token_hash, user_id, created_at, last_seen_at, expires_at) VALUES
		('66666666-6666-6666-6666-666666666666', '00', '22222222-2222-2222-2222-222222222222', UTC_TIMESTAMP(3), UTC_TIMESTAMP(3), UTC_TIMESTAMP(3) + INTERVAL 7 DAY)`)

	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatal(err)
	}

	states := map[string]string{}
	rows, err := db.QueryContext(ctx, `SELECT username, state, generation, invite_delivery FROM athena_users`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var username, state string
		var generation int
		var delivery string
		if err := rows.Scan(&username, &state, &generation, &delivery); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		states[username] = state
		if generation != 0 {
			t.Errorf("user %s has generation %d, want 0", username, generation)
		}
		if delivery != "manual" {
			t.Errorf("user %s has invite delivery %q, want manual", username, delivery)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if states["ada"] != "active" || states["carol"] != "active" {
		t.Errorf("enabled users became %v, want active", states)
	}
	if states["bob"] != "suspended" {
		t.Errorf("disabled user bob became %q, want suspended", states["bob"])
	}

	var userID, roleID, scope string
	var activeFrom, expiresAt sql.NullString
	var revision int64
	if err := db.QueryRowContext(ctx, `SELECT user_id, role_id, CAST(scope AS CHAR), active_from, expires_at, revision FROM athena_role_assignments`).Scan(&userID, &roleID, &scope, &activeFrom, &expiresAt, &revision); err != nil {
		t.Fatalf("copied assignment: %v", err)
	}
	if userID != "22222222-2222-2222-2222-222222222222" || roleID != "44444444-4444-4444-4444-444444444444" {
		t.Errorf("assignment copied for %s/%s, want carol/clerk", userID, roleID)
	}
	if scope != "{}" || activeFrom.Valid || expiresAt.Valid {
		t.Errorf("assignment has scope %q from %v to %v, want empty scope and no dates", scope, activeFrom, expiresAt)
	}
	var assignmentCount, linkCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_role_assignments`).Scan(&assignmentCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_user_roles`).Scan(&linkCount); err != nil {
		t.Fatal(err)
	}
	if assignmentCount != 1 || linkCount != 1 {
		t.Errorf("assignments %d and old links %d, want one of each", assignmentCount, linkCount)
	}

	var requiresScope string
	var system bool
	if err := db.QueryRowContext(ctx, "SELECT CAST(requires_scope AS CHAR), `system` FROM athena_roles").Scan(&requiresScope, &system); err != nil {
		t.Fatal(err)
	}
	if requiresScope != "[]" || system {
		t.Errorf("Role has scope requirement %q system %v, want [] and false", requiresScope, system)
	}
	var permissionsPayload string
	if err := db.QueryRowContext(ctx, `SELECT CAST(system_permissions AS CHAR) FROM athena_roles`).Scan(&permissionsPayload); err != nil {
		t.Fatal(err)
	}
	var permissions []string
	if err := json.Unmarshal([]byte(permissionsPayload), &permissions); err != nil {
		t.Fatal(err)
	}
	held := map[string]bool{}
	for _, p := range permissions {
		held[p] = true
	}
	for _, want := range []string{"access:assign", "access:explore", "access:recover", "access:roles", "access:sessions", "access:users"} {
		if !held[want] {
			t.Errorf("migrated Role lacks %s (has %v)", want, permissions)
		}
	}
	if held["access"] || held["access:ownership"] {
		t.Errorf("migrated Role holds %v, want no plain access and no ownership", permissions)
	}

	var nullable string
	if err := db.QueryRowContext(ctx, `SELECT IS_NULLABLE FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'athena_users' AND column_name = 'password_hash'`).Scan(&nullable); err != nil {
		t.Fatal(err)
	}
	if nullable != "YES" {
		t.Error("password_hash stayed NOT NULL, want nullable for invited users")
	}
	var challenges int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_challenges`).Scan(&challenges); err != nil {
		t.Fatalf("challenges table: %v", err)
	}
	var device, address sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT device, address_digest FROM athena_sessions`).Scan(&device, &address); err != nil {
		t.Fatalf("session device columns: %v", err)
	}
	if device.Valid || address.Valid {
		t.Errorf("old session gained device %v address %v, want NULL", device, address)
	}

	service := &auth.Service{DB: db}
	if _, err := service.Login(ctx, "ada", password, "test-request"); err != nil {
		t.Errorf("owner sign-in after upgrade: %v", err)
	}
	got, err := service.Login(ctx, "carol", password, "test-request")
	if err != nil {
		t.Fatalf("clerk sign-in after upgrade: %v", err)
	}
	if !got.Principal.Can("customer", "write") {
		t.Error("clerk lost the customer write Grant in the upgrade")
	}
	if _, err := service.Login(ctx, "bob", password, "test-request"); !errors.Is(err, auth.ErrDisabled) {
		t.Errorf("disabled user sign-in after upgrade: %v, want ErrDisabled", err)
	}
}

// TestTreesEffectiveUoMMigrationBackfillsPaths builds a database the way
// migration 17 leaves it, inserts an account tree, a branch tree and a
// determination, upgrades to migration 18 and checks the UoM tables exist
// empty, the tree paths and depths are backfilled, and the determination
// survives with an open-ended range.
func TestTreesEffectiveUoMMigrationBackfillsPaths(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{Target: 17}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO athena_account_types (name, label, normal_balance) VALUES ('asset', 'Asset', 'debit')`); err != nil {
		t.Fatal(err)
	}
	for _, account := range []struct {
		name, parent string
		group        bool
	}{
		{"assets", "", true},
		{"current-assets", "assets", true},
		{"cash", "current-assets", false},
	} {
		var parent any
		if account.parent != "" {
			parent = account.parent
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO athena_accounts (name, label, account_type, parent_name, is_group) VALUES (?, ?, 'asset', ?, ?)`,
			account.name, account.name, parent, account.group); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO athena_companies (name, label) VALUES ('acme', 'Acme')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO athena_branches (name, label, company_name, is_group) VALUES ('hq', 'HQ', 'acme', 1), ('hq-shop', 'Shop', 'acme', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE athena_branches SET parent_name = 'hq' WHERE name = 'hq-shop'`); err != nil {
		t.Fatal(err)
	}
	const determinationID = "01911111-2222-7333-8444-666666666666"
	if _, err := db.ExecContext(ctx, `INSERT INTO athena_account_determinations (id, label, account_name, match_json) VALUES (?, 'Cash sales', 'cash', '{"channel":"shop"}')`, determinationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"athena_uom_families", "athena_uom_conversions"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			t.Errorf("table %s missing after migration 18: %v", table, err)
			continue
		}
		if count != 0 {
			t.Errorf("table %s holds %d rows, want it unseeded", table, count)
		}
	}
	for _, want := range []struct {
		name, path string
		depth      int
	}{
		{"assets", "/assets/", 0},
		{"current-assets", "/assets/current-assets/", 1},
		{"cash", "/assets/current-assets/cash/", 2},
	} {
		var path string
		var depth int
		if err := db.QueryRowContext(ctx, `SELECT tree_path, tree_depth FROM athena_accounts WHERE name = ?`, want.name).Scan(&path, &depth); err != nil {
			t.Errorf("account %s path: %v", want.name, err)
			continue
		}
		if path != want.path || depth != want.depth {
			t.Errorf("account %s has path %q depth %d, want %q depth %d", want.name, path, depth, want.path, want.depth)
		}
	}
	var branchPath string
	var branchDepth int
	if err := db.QueryRowContext(ctx, `SELECT tree_path, tree_depth FROM athena_branches WHERE name = 'hq-shop'`).Scan(&branchPath, &branchDepth); err != nil {
		t.Fatalf("branch path: %v", err)
	}
	if branchPath != "/hq/hq-shop/" || branchDepth != 1 {
		t.Errorf("branch hq-shop has path %q depth %d, want /hq/hq-shop/ depth 1", branchPath, branchDepth)
	}
	var label, account string
	var from, to sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT label, account_name, effective_from, effective_to FROM athena_account_determinations WHERE id = ?`, determinationID).Scan(&label, &account, &from, &to); err != nil {
		t.Fatalf("legacy determination: %v", err)
	}
	if label != "Cash sales" || account != "cash" {
		t.Errorf("legacy determination became %q -> %q, want Cash sales -> cash", label, account)
	}
	if from.Valid || to.Valid {
		t.Errorf("legacy determination range is %v to %v, want open-ended NULLs", from, to)
	}
}

// TestPartyLedgerMigrationCreatesThePartyTables builds a database the way
// migration 19 leaves it, upgrades to migration 20 and checks the party
// ledger tables exist with their keys: append-only l_party facts with party
// and control indexes, mutable athena_party_items with one row per source
// instalment, and append-only athena_settlement_applications with one row
// per settlement line and item. The tables start empty: items and
// applications are Workspace data the settlement service writes.
func TestPartyLedgerMigrationCreatesThePartyTables(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{Target: 19}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatal(err)
	}
	tables := map[string][]string{
		"l_party": {
			"id", "company_name", "book", "party_type", "party_id",
			"control_account_name", "item_id", "application_id", "posting_date",
			"debit_base", "credit_base", "txn_currency", "txn_debit", "txn_credit",
			"voucher_type", "voucher_id", "voucher_revision", "event",
			"posting_group_id", "is_reversal", "reversed_row_id", "created_at",
		},
		"athena_party_items": {
			"id", "company_name", "book", "party_type", "party_id",
			"control_account_name", "source_voucher_type", "source_voucher_id",
			"source_voucher_revision", "instalment_no", "due_date", "currency",
			"side", "original_amount", "residual_amount", "status", "revision",
			"created_at", "updated_at",
		},
		"athena_settlement_applications": {
			"id", "company_name", "book", "event", "settlement_voucher_type",
			"settlement_voucher_id", "settlement_voucher_revision",
			"settlement_line_no", "item_id", "payment_currency", "payment_amount",
			"item_currency", "item_amount", "base_amount", "is_reversal",
			"reverses_application_id", "posting_group_id", "created_at",
		},
	}
	for table, columns := range tables {
		rows, err := db.QueryContext(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ?`, table)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for rows.Next() {
			var column string
			if err := rows.Scan(&column); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			got[column] = true
		}
		rows.Close()
		if len(got) == 0 {
			t.Errorf("table %s missing after migration 20", table)
			continue
		}
		for _, column := range columns {
			if !got[column] {
				t.Errorf("table %s misses column %s after migration 20", table, column)
			}
		}
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("table %s holds %d rows, want it empty: migration 20 seeds nothing", table, count)
		}
	}
	for table, key := range map[string]string{
		"athena_party_items":             "uq_party_items_source",
		"athena_settlement_applications": "uq_settlement_apps_line",
	} {
		var name string
		if err := db.QueryRowContext(ctx, `SELECT index_name FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ? LIMIT 1`, table, key).Scan(&name); err != nil {
			t.Errorf("unique key %s on %s missing after migration 20: %v", key, table, err)
		}
	}
}

// TestStockLedgerMigrationCreatesTheStockTables builds a database the way
// migration 20 leaves it, upgrades to migration 21 and checks the stock
// ledger tables exist with their keys: append-only l_stock facts with item,
// control, lot, serial, voucher and group indexes, append-only
// l_stock_value cost deltas referencing the original rows, receipt layers
// with one row per receipt, append-only item applications linking issues
// to receipts, bin balances keyed by company, book, owner, item, location,
// lot and serial, reservations with one hold per holder and bin, serials
// with one row per unit, batches with one row per lot, integrity-log
// entries and rebuild state. The tables start empty: layers, bins,
// reservations, serials and batches are Workspace data the stock service
// writes.
func TestStockLedgerMigrationCreatesTheStockTables(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{Target: 20}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatal(err)
	}
	tables := map[string][]string{
		"l_stock": {
			"id", "company_name", "book", "owner", "item_type", "item_id",
			"control_account_name", "location_name", "lot_no", "serial_no",
			"base_uom", "qty_base", "value_base", "base_currency",
			"movement_date", "posting_date", "dimensions",
			"voucher_type", "voucher_id", "voucher_revision", "source_line_no",
			"event", "posting_group_id", "is_reversal", "reversed_row_id",
			"created_at",
		},
		"l_stock_value": {
			"id", "company_name", "book", "stock_row_id", "item_type", "item_id",
			"control_account_name", "delta_value", "on_hand_delta", "sold_delta",
			"base_currency", "economic_date", "posting_date",
			"voucher_type", "voucher_id", "voucher_revision", "source_line_no",
			"event", "posting_group_id", "is_reversal", "reversed_row_id",
			"created_at",
		},
		"athena_stock_layers": {
			"id", "company_name", "book", "item_type", "item_id",
			"location_name", "lot_no", "receipt_row_id", "base_uom",
			"original_qty", "residual_qty", "unit_cost", "base_currency",
			"status", "revision", "created_at", "updated_at",
		},
		"athena_item_applications": {
			"id", "company_name", "book", "event", "kind", "issue_row_id",
			"receipt_layer_id", "applied_qty", "applied_value", "base_currency",
			"voucher_type", "voucher_id", "voucher_revision", "posting_group_id",
			"is_reversal", "reverses_application_id", "created_at",
		},
		"athena_stock_bins": {
			"company_name", "book", "owner", "item_type", "item_id",
			"location_name", "lot_no", "serial_no", "base_uom", "on_hand",
			"reserved_qty", "value_on_hand", "base_currency", "revision",
			"created_at", "updated_at",
		},
		"athena_reservations": {
			"id", "company_name", "book", "owner", "item_type", "item_id",
			"location_name", "lot_no", "serial_no", "base_uom", "qty",
			"status", "holder_type", "holder_id", "expires_at", "revision",
			"created_at", "updated_at",
		},
		"athena_serials": {
			"id", "company_name", "book", "item_type", "item_id", "serial_no",
			"status", "owner", "location_name", "lot_no", "receipt_row_id",
			"issue_row_id", "revision", "created_at", "updated_at",
		},
		"athena_batches": {
			"id", "company_name", "book", "item_type", "item_id", "lot_no",
			"expiry_date", "received_date", "status", "receipt_row_id",
			"revision", "created_at", "updated_at",
		},
		"athena_integrity_log": {
			"id", "company_name", "book", "check_name", "control_account_name",
			"cutoff_date", "expected_value", "observed_value", "base_currency",
			"owner", "status", "detail", "created_at",
		},
		"athena_stock_rebuilds": {
			"id", "company_name", "book", "state", "watermark_row_id",
			"started_at", "completed_at", "error",
		},
	}
	for table, columns := range tables {
		rows, err := db.QueryContext(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ?`, table)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for rows.Next() {
			var column string
			if err := rows.Scan(&column); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			got[column] = true
		}
		rows.Close()
		if len(got) == 0 {
			t.Errorf("table %s missing after migration 21", table)
			continue
		}
		for _, column := range columns {
			if !got[column] {
				t.Errorf("table %s misses column %s after migration 21", table, column)
			}
		}
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("table %s holds %d rows, want it empty: migration 21 seeds nothing", table, count)
		}
	}
	for table, key := range map[string]string{
		"athena_stock_layers": "uq_stock_layers_receipt",
		"athena_reservations": "uq_reservations_hold",
		"athena_serials":      "uq_serials_unit",
		"athena_batches":      "uq_batches_lot",
	} {
		var name string
		if err := db.QueryRowContext(ctx, `SELECT index_name FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ? LIMIT 1`, table, key).Scan(&name); err != nil {
			t.Errorf("unique key %s on %s missing after migration 21: %v", key, table, err)
		}
	}
}

// TestSecretsRetentionMigrationCreatesTheSecretsTables upgrades a database
// left at migration 21 to the latest and checks migration 22 creates the
// secret-reference, key-version, disposal-register and hold tables with
// their columns and unique keys, and seeds nothing.
func TestSecretsRetentionMigrationCreatesTheSecretsTables(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{Target: 21}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatal(err)
	}
	tables := map[string][]string{
		"athena_secret_refs": {
			"id", "name", "provider", "provider_ref",
			"created_at", "updated_at",
		},
		"athena_key_versions": {
			"id", "key_name", "version", "status", "created_at",
		},
		"athena_disposal_register": {
			"id", "record_type", "record_id", "action", "outcome",
			"detail", "actor", "created_at",
		},
		"athena_holds": {
			"id", "record_type", "record_id", "reason", "placed_by",
			"released_at", "created_at",
		},
	}
	for table, columns := range tables {
		rows, err := db.QueryContext(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ?`, table)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for rows.Next() {
			var column string
			if err := rows.Scan(&column); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			got[column] = true
		}
		rows.Close()
		if len(got) == 0 {
			t.Errorf("table %s missing after migration 22", table)
			continue
		}
		for _, column := range columns {
			if !got[column] {
				t.Errorf("table %s misses column %s after migration 22", table, column)
			}
		}
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("table %s holds %d rows, want it empty: migration 22 seeds nothing", table, count)
		}
	}
	for table, key := range map[string]string{
		"athena_secret_refs":  "uq_secret_refs_name",
		"athena_key_versions": "uq_key_versions_name_version",
	} {
		var name string
		if err := db.QueryRowContext(ctx, `SELECT index_name FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ? LIMIT 1`, table, key).Scan(&name); err != nil {
			t.Errorf("unique key %s on %s missing after migration 22: %v", key, table, err)
		}
	}
}
