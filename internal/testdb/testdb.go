// Package testdb gives each test its own throwaway MariaDB database.
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

// EnvURL names the variable holding a MariaDB DSN whose user may create and
// drop databases named test_athena_*. The database name in the DSN is ignored.
const EnvURL = "ATHENA_TEST_DATABASE_URL"

// namePrefix starts every throwaway database name. MariaDB's default install
// grants all users CREATE and DROP on databases named test_*, so the prefix
// works without extra privileges; other servers need the same grant.
const namePrefix = "test_athena_"

// New creates an empty database, returns a connection to it and drops the
// database when the test ends. It skips the test when EnvURL is unset, so
// the suite passes on machines without MariaDB. The connection uses the same
// UTC session settings as the server.
func New(t testing.TB) *sql.DB {
	t.Helper()
	db, _ := NewWithConfig(t)
	return db
}

// NewWithConfig is New that also returns the driver config for the new
// database, for code that opens its own connection (the CLI tests).
func NewWithConfig(t testing.TB) (*sql.DB, *mysql.Config) {
	t.Helper()
	db, cfg, adminDB, name := create(t)
	t.Cleanup(func() {
		if _, err := adminDB.Exec("DROP DATABASE IF EXISTS `" + name + "`"); err != nil {
			t.Errorf("drop test database %s: %v", name, err)
		}
		adminDB.Close()
	})
	t.Cleanup(func() { db.Close() })
	return db, cfg
}

// NewScoped is New for tests that create many databases in one run, such as
// one per property-test iteration. It returns the connection and a cleanup
// func that drops the database and closes its connections immediately; call
// it when the iteration ends so connections do not pile up to the server
// limit. A backstop registered with t.Cleanup drops leftovers at test end,
// so iterations that fail midway still clean up.
func NewScoped(t testing.TB) (*sql.DB, func()) {
	t.Helper()
	db, _, adminDB, name := create(t)
	cleaned := false
	cleanup := func() {
		if cleaned {
			return
		}
		cleaned = true
		_, _ = adminDB.Exec("DROP DATABASE IF EXISTS `" + name + "`")
		db.Close()
		adminDB.Close()
	}
	t.Cleanup(cleanup)
	return db, cleanup
}

// create makes one throwaway database and returns a connection to it with
// the driver config, the admin connection and the database name. The caller
// registers the cleanup.
func create(t testing.TB) (db *sql.DB, cfg *mysql.Config, adminDB *sql.DB, name string) {
	t.Helper()
	dsn := os.Getenv(EnvURL)
	if dsn == "" {
		t.Skipf("%s is not set; skipping the MariaDB test", EnvURL)
	}
	db, cfg, adminDB, name, err := createDatabase(dsn)
	if err != nil {
		t.Fatal(err)
	}
	return db, cfg, adminDB, name
}

// createDatabase parses a DSN, creates one throwaway database and opens the
// admin and database connections. It returns errors instead of failing a
// test, so TestMain setup can use it before any test exists.
func createDatabase(dsn string) (db *sql.DB, cfg *mysql.Config, adminDB *sql.DB, name string, err error) {
	cfg, err = mysql.ParseDSN(dsn)
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("parse %s: %w", EnvURL, err)
	}
	name = namePrefix + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]

	admin := cfg.Clone()
	admin.DBName = ""
	if adminDB, err = openDatabase(admin); err != nil {
		return nil, nil, nil, "", err
	}
	if _, err = adminDB.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		adminDB.Close()
		return nil, nil, nil, "", fmt.Errorf("create test database: %w", err)
	}

	cfg = cfg.Clone()
	cfg.DBName = name
	if db, err = openDatabase(cfg); err != nil {
		_, _ = adminDB.Exec("DROP DATABASE IF EXISTS `" + name + "`")
		adminDB.Close()
		return nil, nil, nil, "", err
	}
	return db, cfg, adminDB, name, nil
}

func openDB(t testing.TB, cfg *mysql.Config) *sql.DB {
	t.Helper()
	db, err := openDatabase(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func openDatabase(cfg *mysql.Config) (*sql.DB, error) {
	cfg = cfg.Clone()
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["time_zone"] = "'+00:00'"
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("open MariaDB connection: %w", err)
	}
	db := sql.OpenDB(connector)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to MariaDB: %w", err)
	}
	return db, nil
}

// migrationsTable is the schema-migrations ledger. Reset keeps its rows so
// the shared database stays migrated; the name is a durable contract of the
// migration system (ADR-0006).
const migrationsTable = "athena_migrations"

// Shared is one migrated database reused across many tests in a package.
// Open it in TestMain, migrate it once, Freeze the migrated schema and seed
// rows, then Reset before each use. Sequential tests only: concurrent users
// would see each other's rows. Tests that need their own migration state
// keep New.
type Shared struct {
	db    *sql.DB
	admin *sql.DB
	name  string
	base  map[string]struct{}
	// counters holds the post-migration AUTO_INCREMENT value of tables that
	// have one; Reset restores them so counters restart like TRUNCATE would.
	counters map[string]int64
	seeds    map[string]seedTable
}

// seedTable is one migrated table's post-migration content: the rows the
// migrations themselves seeded, such as the audit chain head. Reset
// restores exactly these rows after truncating.
type seedTable struct {
	columns []string
	rows    [][]any
}

// OpenShared creates the package's shared database. It returns (nil, nil)
// when EnvURL is unset, so TestMain can still run the package's pure tests
// while database fixtures skip.
func OpenShared() (*Shared, error) {
	dsn := os.Getenv(EnvURL)
	if dsn == "" {
		return nil, nil
	}
	db, _, adminDB, name, err := createDatabase(dsn)
	if err != nil {
		return nil, err
	}
	return &Shared{db: db, admin: adminDB, name: name}, nil
}

// DB returns the shared connection.
func (s *Shared) DB() *sql.DB {
	return s.db
}

// Freeze snapshots the migrated schema, its counters and its seed rows; call
// it once, after migrating, before any test writes. Reset drops every table
// created after this point, empties the rest with counters restarted and
// restores the seed rows, so each user starts from exactly what the
// migrations built.
func (s *Shared) Freeze() error {
	tables, err := s.tables()
	if err != nil {
		return err
	}
	s.base = make(map[string]struct{}, len(tables))
	s.seeds = make(map[string]seedTable, len(tables))
	counters, err := s.autoIncrements()
	if err != nil {
		return err
	}
	s.counters = counters
	for _, table := range tables {
		s.base[table] = struct{}{}
		if table == migrationsTable {
			continue
		}
		seed, err := s.snapshot(table)
		if err != nil {
			return err
		}
		if len(seed.rows) > 0 {
			s.seeds[table] = seed
		}
	}
	return nil
}

// Reset returns the shared database to its freshly migrated state: tables
// created after Freeze (Record Type tables and other test artifacts) are
// dropped, migrated tables are emptied with their counters restarted, the
// migrations ledger is kept, and the migrations' seed rows are restored.
// Rows go through DELETE rather than TRUNCATE: identical results for tests,
// far fewer dictionary locks when other suites run beside this one. Foreign
// keys are disabled on the reset connection only, for the duration of the
// reset.
func (s *Shared) Reset() error {
	if s.base == nil {
		return fmt.Errorf("testdb: call Freeze after migrating before Reset")
	}
	tables, err := s.tables()
	if err != nil {
		return err
	}
	present := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		present[table] = struct{}{}
	}
	var missing []string
	for table := range s.base {
		if _, ok := present[table]; !ok {
			missing = append(missing, table)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("testdb: previous test dropped platform tables %v; give it an isolated database", missing)
	}
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reset test database: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		return fmt.Errorf("reset test database: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS=1") }()
	for _, table := range tables {
		if table == migrationsTable {
			continue
		}
		statement := "DELETE FROM `" + table + "`"
		if _, ok := s.base[table]; !ok {
			statement = "DROP TABLE `" + table + "`"
		}
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("reset test database: %w", err)
		}
	}
	current, err := s.autoIncrements()
	if err != nil {
		return err
	}
	for table, frozen := range s.counters {
		// The value interpolates directly: DDL takes no placeholders, and
		// it is our own counter reading, not test input.
		if now, ok := current[table]; ok && now != frozen {
			if _, err := conn.ExecContext(ctx, fmt.Sprintf("ALTER TABLE `%s` AUTO_INCREMENT = %d", table, frozen)); err != nil {
				return fmt.Errorf("reset test database: %w", err)
			}
		}
	}
	for table, seed := range s.seeds {
		if err := s.restore(ctx, conn, table, seed); err != nil {
			return err
		}
	}
	return nil
}

// autoIncrements reports the next AUTO_INCREMENT value of tables that have
// one. Tables without a counter are absent from the map.
func (s *Shared) autoIncrements() (map[string]int64, error) {
	rows, err := s.db.Query(`SELECT TABLE_NAME, AUTO_INCREMENT FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'`, s.name)
	if err != nil {
		return nil, fmt.Errorf("list test counters: %w", err)
	}
	defer rows.Close()
	counters := map[string]int64{}
	for rows.Next() {
		var table string
		var value sql.NullInt64
		if err := rows.Scan(&table, &value); err != nil {
			return nil, fmt.Errorf("list test counters: %w", err)
		}
		if value.Valid {
			counters[table] = value.Int64
		}
	}
	return counters, rows.Err()
}

// snapshot reads one table's rows for Freeze. Byte values are copied: the
// driver reuses its buffers across rows.
func (s *Shared) snapshot(table string) (seedTable, error) {
	rows, err := s.db.Query("SELECT * FROM `" + table + "`")
	if err != nil {
		return seedTable{}, fmt.Errorf("snapshot test table %s: %w", table, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return seedTable{}, fmt.Errorf("snapshot test table %s: %w", table, err)
	}
	seed := seedTable{columns: columns}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return seedTable{}, fmt.Errorf("snapshot test table %s: %w", table, err)
		}
		for i, value := range values {
			if raw, ok := value.([]byte); ok && raw != nil {
				// Copy into a non-nil slice: the driver reuses its
				// buffers, and an empty value must stay "" rather
				// than become NULL.
				restored := make([]byte, len(raw))
				copy(restored, raw)
				values[i] = restored
			}
		}
		seed.rows = append(seed.rows, values)
	}
	return seed, rows.Err()
}

// restore re-inserts one table's seed rows after truncation.
func (s *Shared) restore(ctx context.Context, conn *sql.Conn, table string, seed seedTable) error {
	quoted := make([]string, len(seed.columns))
	for i, column := range seed.columns {
		quoted[i] = "`" + column + "`"
	}
	statement := "INSERT INTO `" + table + "` (" + strings.Join(quoted, ",") + ") VALUES (?" + strings.Repeat(",?", len(quoted)-1) + ")"
	for _, row := range seed.rows {
		if _, err := conn.ExecContext(ctx, statement, row...); err != nil {
			return fmt.Errorf("restore test table %s: %w", table, err)
		}
	}
	return nil
}

// Close drops the shared database and closes its connections. A nil Shared
// (EnvURL unset) closes to nothing, to keep TestMain short.
func (s *Shared) Close() {
	if s == nil {
		return
	}
	_, _ = s.admin.Exec("DROP DATABASE IF EXISTS `" + s.name + "`")
	s.db.Close()
	s.admin.Close()
}

// tables lists the shared database's real tables. Views are never test
// artifacts and stay untouched.
func (s *Shared) tables() ([]string, error) {
	rows, err := s.db.Query(`SELECT TABLE_NAME FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME`, s.name)
	if err != nil {
		return nil, fmt.Errorf("list test tables: %w", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("list test tables: %w", err)
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}
