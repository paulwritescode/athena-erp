package server

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
)

const (
	renameWorkspaceID = "0190a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b"
	renameCustomerID  = "0190a1b2-0000-7000-8000-000000000001"
	renameInvoiceID   = "0190a1b2-0000-7000-8000-000000000002"
)

var renameCustomerType = `{"name":"customer","label":"Customer","description":"","fields":[{"name":"name","label":"Name","type":"Text","required":true}]}`

var renameInvoiceType = `{"name":"invoice","label":"Invoice","description":"","fields":[{"name":"customer","label":"Customer","type":"Link","required":false,"target_record":"customer"},{"name":"total","label":"Total","type":"Currency","required":false}]}`

// renameCustomerWithEmail is a pending save written before the rename. It
// still carries the old key and must finish against the new names.
var renameCustomerWithEmail = `{"name":"customer","label":"Customer","description":"","fields":[{"name":"name","label":"Name","type":"Text","required":true},{"name":"email","label":"Email","type":"Email","required":false}]}`

// preRenameDatabase builds a database as the build before ADR-0008 left it:
// system migrations 1 to 4 applied, two Record tables with rows and a Link
// foreign key, a project identity, a pending and a completed journal row,
// and history rows. Every name here is a pre-rename name.
func preRenameDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{Target: 4}); err != nil {
		t.Fatal(err)
	}
	customerTable, invoiceTable := legacyRecordTableName("customer"), legacyRecordTableName("invoice")
	statements := []string{
		dialect.CreateRecordTypeTable(customerTable, []store.Column{{Name: "name", Type: "VARCHAR(255)"}}),
		dialect.CreateRecordTypeTable(invoiceTable, []store.Column{{Name: "customer", Type: "CHAR(36)"}, {Name: "total", Type: "DECIMAL(18,6)"}}),
		dialect.AddForeignKey(invoiceTable, legacyForeignKeyName("invoice", "customer"), "customer", customerTable),
		"INSERT INTO athena_records (name, label, revision, definition) VALUES ('customer', 'Customer', 1, '" + renameCustomerType + "')",
		"INSERT INTO athena_records (name, label, revision, definition) VALUES ('invoice', 'Invoice', 3, '" + renameInvoiceType + "')",
		"INSERT INTO `" + customerTable + "` (id, revision, name) VALUES ('" + renameCustomerID + "', 2, 'Acme')",
		"INSERT INTO `" + invoiceTable + "` (id, revision, customer, total) VALUES ('" + renameInvoiceID + "', 5, '" + renameCustomerID + "', 12.500000)",
		"CREATE TABLE athena_project (id CHAR(36) NOT NULL PRIMARY KEY, slug VARCHAR(48) NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)",
		"INSERT INTO athena_project (id, slug) VALUES ('" + renameWorkspaceID + "', 'acme')",
		"INSERT INTO athena_rules (name, label, definition) VALUES ('r1', 'Rule', '{}')",
		"INSERT INTO athena_record_operations (id, record_name, action, expected_revision, target_revision, definition, status, last_step) VALUES ('0190a1b2-1111-7000-8000-000000000001', 'invoice', 'save', 2, 3, '" + renameInvoiceType + "', 'completed', 'activated')",
		"INSERT INTO athena_record_operations (id, record_name, action, expected_revision, definition, options, status, last_step) VALUES ('0190a1b2-1111-7000-8000-000000000002', 'customer', 'save', 1, '" + renameCustomerWithEmail + "', '{}', 'pending', 'planned')",
		"INSERT INTO athena_entry_history (id, record_name, entry_id, revision, action, changed_fields, actor) VALUES ('0190a1b2-2222-7000-8000-000000000001', 'customer', '" + renameCustomerID + "', 2, 'updated', '[\"name\"]', 'Local administrator')",
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("fixture %.60s: %v", statement, err)
		}
	}
	return db
}

// catalogue lists every column, index and foreign key in the database, so two
// databases can be compared by structure.
func catalogue(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var out []string
	for _, query := range []string{
		`SELECT CONCAT('column ', table_name, '.', column_name, ' ', column_type, ' ', is_nullable, ' ', IFNULL(column_default, '-')) FROM information_schema.columns WHERE table_schema = DATABASE() ORDER BY 1`,
		`SELECT DISTINCT CONCAT('index ', table_name, '.', index_name) FROM information_schema.statistics WHERE table_schema = DATABASE() ORDER BY 1`,
		`SELECT CONCAT('fk ', table_name, '.', constraint_name, ' -> ', referenced_table_name) FROM information_schema.referential_constraints WHERE constraint_schema = DATABASE() ORDER BY 1`,
	} {
		rows, err := db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			out = append(out, line)
		}
		rows.Close()
	}
	return out
}

// dataOf returns the rows of the tables that hold business data and history.
func dataOf(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var out []string
	for _, query := range []string{
		"SELECT CONCAT_WS('|', id, revision, name) FROM p_workspace__customer ORDER BY id",
		"SELECT CONCAT_WS('|', id, revision, customer, total) FROM p_workspace__invoice ORDER BY id",
		"SELECT CONCAT_WS('|', record_type_name, record_id, revision, action, changed_fields, actor) FROM athena_record_history ORDER BY id",
		"SELECT CONCAT_WS('|', name, revision) FROM athena_record_types ORDER BY name",
		"SELECT CONCAT_WS('|', id, record_type_name, status, definition) FROM athena_record_type_operations ORDER BY id",
	} {
		rows, err := db.Query(query)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			out = append(out, line)
		}
		rows.Close()
	}
	return out
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	ok, err := dialect.TableExists(context.Background(), db, name)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestRenameKeepsDataAndRewritesNames(t *testing.T) {
	ctx := context.Background()
	db := preRenameDatabase(t)
	before := Workspace{ID: renameWorkspaceID, Slug: "acme"}
	// The identity check runs before migrations, against the old table.
	if err := checkWorkspace(ctx, db, before); err != nil {
		t.Fatalf("identity on a pre-rename database: %v", err)
	}
	applied, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{})
	if err != nil || len(applied) != 2 || applied[0].Name != "vocabulary_rename" || applied[1].Name != "record_kinds_and_system_fields" {
		t.Fatalf("up = %v, %v", applied, err)
	}
	for _, old := range []string{"athena_records", "athena_modules", "athena_record_operations", "athena_entry_history", "athena_project", "athena_record_customer", "athena_record_invoice"} {
		if tableExists(t, db, old) {
			t.Errorf("old table %s still exists", old)
		}
	}
	for _, name := range []string{"athena_record_types", "athena_packs", "athena_record_type_operations", "athena_record_history", "athena_workspace", "p_workspace__customer", "p_workspace__invoice"} {
		if !tableExists(t, db, name) {
			t.Errorf("table %s is missing", name)
		}
	}
	if err := checkWorkspace(ctx, db, before); err != nil {
		t.Fatalf("identity after the rename: %v", err)
	}
	wantData := []string{
		"0190a1b2-0000-7000-8000-000000000001|2|Acme",
		"0190a1b2-0000-7000-8000-000000000002|5|0190a1b2-0000-7000-8000-000000000001|12.500000",
		"customer|0190a1b2-0000-7000-8000-000000000001|2|updated|[\"name\"]|Local administrator",
		"customer|1", "invoice|3",
	}
	got := dataOf(t, db)
	if !reflect.DeepEqual(got[:5], wantData) {
		t.Fatalf("data after rename:\n got %q\nwant %q", got[:5], wantData)
	}
	for _, line := range got[5:] {
		if strings.Contains(line, `"target_record":`) {
			t.Errorf("journal definition not rewritten: %s", line)
		}
	}
	if !strings.Contains(strings.Join(got[5:], "\n"), `"target_record_type":"customer"`) {
		t.Error("the journaled invoice definition lost its link target")
	}
	var definition string
	if err := db.QueryRow("SELECT definition FROM athena_record_types WHERE name = 'invoice'").Scan(&definition); err != nil || !strings.Contains(definition, `"target_record_type":"customer"`) || strings.Contains(definition, `"target_record":`) {
		t.Fatalf("definition = %s, %v", definition, err)
	}
	cat := strings.Join(catalogue(t, db), "\n")
	for _, want := range []string{
		"fk p_workspace__invoice.fk_rtype_" + strings.TrimPrefix(recordTypeForeignKeyName("invoice", "customer"), "fk_rtype_") + " -> p_workspace__customer",
		"index athena_record_type_operations.idx_record_type_operations_status",
		"index athena_record_history.idx_record_history_record",
		"column athena_rules.pack_name varchar(140) NO 'core'",
		"column athena_record_history.record_id char(36) NO -",
	} {
		if !strings.Contains(cat, want) {
			t.Errorf("catalogue lacks %q", want)
		}
	}
	for _, gone := range []string{"fk_record_", "idx_record_operations_status", "idx_entry_history_entry", "module_name", "entry_id", "record_name"} {
		if strings.Contains(cat, gone) {
			t.Errorf("catalogue still has %q", gone)
		}
	}
	var pack string
	if err := db.QueryRow("SELECT pack_name FROM athena_rules WHERE name = 'r1'").Scan(&pack); err != nil || pack != "core" {
		t.Fatalf("pack_name = %q, %v", pack, err)
	}
	// History stays readable through the new service.
	records := recordService{db: db}
	history, err := records.ListRecordHistory(ctx, &athenav1.ListRecordHistoryRequest{RecordTypeKey: "customer", Id: renameCustomerID})
	if err != nil || len(history.GetEvents()) != 1 || history.GetEvents()[0].GetAction() != "updated" {
		t.Fatalf("history = %v, %v", history, err)
	}
	// The pending journal row finishes against the new names.
	if err := resumeOperations(ctx, db); err != nil {
		t.Fatal(err)
	}
	var status string
	var revision int64
	if err := db.QueryRow("SELECT status FROM athena_record_type_operations WHERE id = '0190a1b2-1111-7000-8000-000000000002'").Scan(&status); err != nil || status != "completed" {
		t.Fatalf("pending operation = %q, %v", status, err)
	}
	if err := db.QueryRow("SELECT revision FROM athena_record_types WHERE name = 'customer'").Scan(&revision); err != nil || revision != 2 {
		t.Fatalf("customer revision = %d, %v", revision, err)
	}
	if ok, err := dialect.ColumnExists(ctx, db, "p_workspace__customer", "email"); err != nil || !ok {
		t.Fatalf("resumed save did not add the email column: %v", err)
	}
}

func TestRenameIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := preRenameDatabase(t)
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatal(err)
	}
	structure, data := catalogue(t, db), dataOf(t, db)
	applied, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{})
	if err != nil || len(applied) != 0 {
		t.Fatalf("second up applied %v, %v", applied, err)
	}
	// Running the step itself again, as after a crash before it was recorded, changes nothing.
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := renameVocabulary(ctx, conn); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if !reflect.DeepEqual(structure, catalogue(t, db)) || !reflect.DeepEqual(data, dataOf(t, db)) {
		t.Fatal("repeating the rename changed the database")
	}
}

// failingExecer stops after a fixed number of statements, as a crash would.
type failingExecer struct {
	store.Execer
	remaining int
	executed  int
}

func (f *failingExecer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if f.remaining == 0 {
		return nil, fmt.Errorf("simulated crash before statement %d", f.executed+1)
	}
	f.remaining--
	f.executed++
	return f.Execer.ExecContext(ctx, query, args...)
}

func TestRenameResumesAfterACrashAtEveryStatement(t *testing.T) {
	ctx := context.Background()
	reference := preRenameDatabase(t)
	counter := &failingExecer{remaining: 1 << 20}
	var statements int
	counting := SystemMigrations()
	counting[4].Up = func(ctx context.Context, x store.Execer) error {
		counter.Execer = x
		err := renameVocabulary(ctx, counter)
		statements = counter.executed
		return err
	}
	if _, err := store.Up(ctx, reference, dialect, counting, store.UpOptions{}); err != nil {
		t.Fatal(err)
	}
	wantStructure, wantData := catalogue(t, reference), dataOf(t, reference)
	if statements < 10 {
		t.Fatalf("the rename ran only %d statements", statements)
	}
	for k := 0; k < statements; k++ {
		db := preRenameDatabase(t)
		crashing := SystemMigrations()
		crashing[4].Up = func(ctx context.Context, x store.Execer) error {
			return renameVocabulary(ctx, &failingExecer{Execer: x, remaining: k})
		}
		if _, err := store.Up(ctx, db, dialect, crashing, store.UpOptions{}); err == nil {
			t.Fatalf("k=%d: the simulated crash did not stop the migration", k)
		}
		if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
			t.Fatalf("k=%d: resume: %v", k, err)
		}
		if !reflect.DeepEqual(wantStructure, catalogue(t, db)) || !reflect.DeepEqual(wantData, dataOf(t, db)) {
			t.Fatalf("k=%d: resumed database differs from an uninterrupted one", k)
		}
	}
}

func TestRenameRefusesToMergeOrGuess(t *testing.T) {
	ctx := context.Background()
	t.Run("old and new table both exist", func(t *testing.T) {
		db := preRenameDatabase(t)
		if _, err := db.Exec("CREATE TABLE p_workspace__customer (id CHAR(36) NOT NULL PRIMARY KEY)"); err != nil {
			t.Fatal(err)
		}
		before := catalogue(t, db)
		_, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{})
		if err == nil || !strings.Contains(err.Error(), "athena_record_customer") || !strings.Contains(err.Error(), "p_workspace__customer") {
			t.Fatalf("error = %v", err)
		}
		if !reflect.DeepEqual(before, catalogue(t, db)) {
			t.Fatal("a refused rename changed the database")
		}
	})
	t.Run("a key that names a legacy system table", func(t *testing.T) {
		db := preRenameDatabase(t)
		if _, err := db.Exec("INSERT INTO athena_records (name, label, revision, definition) VALUES ('operations', 'Operations', 1, '" + renameCustomerType + "')"); err != nil {
			t.Fatal(err)
		}
		before := catalogue(t, db)
		if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err == nil || !strings.Contains(err.Error(), "is a system table") {
			t.Fatalf("error = %v", err)
		}
		if !reflect.DeepEqual(before, catalogue(t, db)) {
			t.Fatal("a refused rename changed the database")
		}
	})
}

// A legacy key such as "history" owns athena_record_history before the rename;
// the Record table must move out of the way of the system table.
func TestRenameHandlesALegacyKeyThatNamesANewSystemTable(t *testing.T) {
	ctx := context.Background()
	db := preRenameDatabase(t)
	for _, statement := range []string{
		dialect.CreateRecordTypeTable("athena_record_history", []store.Column{{Name: "note", Type: "VARCHAR(255)"}}),
		"INSERT INTO athena_record_history (id, note) VALUES ('0190a1b2-3333-7000-8000-000000000001', 'kept')",
		`INSERT INTO athena_records (name, label, revision, definition) VALUES ('history', 'History', 1, '{"name":"history","label":"History","description":"","fields":[{"name":"note","label":"Note","type":"Text","required":false}]}')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatal(err)
	}
	var note string
	if err := db.QueryRow("SELECT note FROM p_workspace__history").Scan(&note); err != nil || note != "kept" {
		t.Fatalf("note = %q, %v", note, err)
	}
	if ok, err := dialect.ColumnExists(ctx, db, "athena_record_history", "record_type_name"); err != nil || !ok {
		t.Fatalf("system history table not in place: %v", err)
	}
}

func TestGeneratedIdentifiersAreDeterministicLowercaseAndWithinTheLimit(t *testing.T) {
	asciiLower := regexp.MustCompile(`^[a-z0-9_]+$`)
	fifty := "a" + strings.Repeat("b", 49)
	if !recordTypeKeyPattern.MatchString(fifty) {
		t.Fatal("a 50-character key must be valid")
	}
	if got := recordTypeTableName(fifty); got != "p_workspace__"+fifty || len(got) != store.GeneratedIdentLimit {
		t.Fatalf("50-character key table = %q (%d bytes)", got, len(got))
	}
	long := "A" + strings.Repeat("b", 62)
	a, b := recordTypeTableName(long), recordTypeTableName(long)
	if a != b || len(a) != store.GeneratedIdentLimit || !asciiLower.MatchString(a) {
		t.Fatalf("63-character legacy key table = %q (%d bytes)", a, len(a))
	}
	if other := recordTypeTableName("A" + strings.Repeat("b", 61) + "c"); other == a {
		t.Fatal("different long keys must not share a table name")
	}
	if got := recordTypeTableName("Customer"); got != "p_workspace__customer" {
		t.Fatalf("mixed-case key table = %q", got)
	}
	if fk := recordTypeForeignKeyName(long, strings.Repeat("f", 63)); len(fk) > store.GeneratedIdentLimit || !asciiLower.MatchString(fk) {
		t.Fatalf("foreign key name %q", fk)
	}
}
