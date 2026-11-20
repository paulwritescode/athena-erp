// Package testdb gives each test its own throwaway MariaDB database.
package testdb

import (
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
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse %s: %v", EnvURL, err)
	}
	name = namePrefix + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]

	admin := cfg.Clone()
	admin.DBName = ""
	adminDB = openDB(t, admin)
	if _, err := adminDB.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		adminDB.Close()
		t.Fatalf("create test database: %v", err)
	}

	cfg = cfg.Clone()
	cfg.DBName = name
	db = openDB(t, cfg)
	return db, cfg, adminDB, name
}

func openDB(t testing.TB, cfg *mysql.Config) *sql.DB {
	t.Helper()
	cfg = cfg.Clone()
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["time_zone"] = "'+00:00'"
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatalf("open MariaDB connection: %v", err)
	}
	db := sql.OpenDB(connector)
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(fmt.Errorf("connect to MariaDB: %w", err))
	}
	return db
}
