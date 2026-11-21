package server

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"athena-erp/internal/auth"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
)

// sharedDB is the package's migrated database, reused across tests. Each
// fixture resets it before use, which is far cheaper than migrating a fresh
// database per test. Tests that need their own migration state (targeted or
// legacy migrations) keep testdb.New.
var sharedDB *testdb.Shared

func TestMain(m *testing.M) {
	// These tests exercise access behavior, not hashing cost (covered by the
	// password tests in internal/auth), so they hash at the same reduced
	// cost the property tests already use.
	auth.DefaultParams = auth.Params{MemoryKiB: 8 * 1024, Time: 1, Threads: 1}
	shared, err := testdb.OpenShared()
	if err != nil {
		fmt.Fprintln(os.Stderr, "shared test database:", err)
		os.Exit(1)
	}
	if shared != nil {
		if _, err := store.Up(context.Background(), shared.DB(), dialect, SystemMigrations(), store.UpOptions{}); err != nil {
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
