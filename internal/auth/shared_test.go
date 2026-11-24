// Package auth_test shares one migrated database across its tests; see the
// TestMain below.
package auth_test

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
// helper resets it before use, which is far cheaper than migrating a fresh
// database per test or per property iteration.
var sharedDB *testdb.Shared

func TestMain(m *testing.M) {
	// Cheap Argon2id cost for every test in this binary: hashing at the
	// production cost here starves the parallel packages in a full run.
	// The two password tests below pin the production cost explicitly for
	// themselves, so the shipped default stays tested.
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
