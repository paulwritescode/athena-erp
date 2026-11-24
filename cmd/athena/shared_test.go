// The CLI tests in package main share one migrated database across the
// tests that address it by DSN; see the TestMain below.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"athena-erp/internal/auth"
	"athena-erp/internal/server"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
)

// sharedDB is the package's migrated database, reused across tests. Each
// fixture resets it before use, which is far cheaper than migrating a fresh
// database per test.
var sharedDB *testdb.Shared

func TestMain(m *testing.M) {
	// These tests exercise CLI behavior, not hashing cost (covered by the
	// password tests in internal/auth), so they hash at the same reduced
	// cost the property tests already use.
	auth.DefaultParams = auth.Params{MemoryKiB: 8 * 1024, Time: 1, Threads: 1}
	// Killed runs never drop their databases; sweep them before opening
	// ours. A failed sweep warns, never fails the suite over hygiene.
	if _, err := testdb.SweepStale(time.Hour); err != nil {
		fmt.Fprintln(os.Stderr, "sweep stale test databases:", err)
	}
	shared, err := testdb.OpenShared()
	if err != nil {
		fmt.Fprintln(os.Stderr, "shared test database:", err)
		os.Exit(1)
	}
	if shared != nil {
		if _, err := store.Up(context.Background(), shared.DB(), store.MariaDB{}, server.SystemMigrations(), store.UpOptions{}); err != nil {
			fmt.Fprintln(os.Stderr, "migrate shared test database:", err)
			os.Exit(1)
		}
		if err := shared.Freeze(); err != nil {
			fmt.Fprintln(os.Stderr, "snapshot shared test database:", err)
			os.Exit(1)
		}
	}
	sharedDB = shared
	code := m.Run()
	sharedDB.Close()
	os.Exit(code)
}

// sharedDatabase returns the shared migrated database after resetting it to
// its freshly migrated state. It skips the test when MariaDB is unavailable,
// like testdb.New.
func sharedDatabase(t *testing.T) *sql.DB {
	t.Helper()
	if sharedDB == nil {
		t.Skipf("%s is not set; skipping the MariaDB test", testdb.EnvURL)
	}
	if err := sharedDB.Reset(); err != nil {
		t.Fatalf("reset shared test database: %v", err)
	}
	return sharedDB.DB()
}

// sharedDSNEnv resets the shared database, points ATHENA_DATABASE_URL at it
// and moves to a fresh temp dir (no home, so the DSN fallback applies),
// replacing the per-test NewWithConfig + Setenv + Chdir. It returns the
// shared connection for tests that also use the database directly.
func sharedDSNEnv(t *testing.T) *sql.DB {
	t.Helper()
	db := sharedDatabase(t)
	cfg := sharedDB.Config()
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir()) // no home here, so the DSN fallback applies
	return db
}
