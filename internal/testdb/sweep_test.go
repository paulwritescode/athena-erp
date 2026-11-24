package testdb

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// openAdmin opens the server connection the sweep tests use to stage
// databases. It skips when EnvURL is unset, like New.
func openAdmin(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv(EnvURL)
	if dsn == "" {
		t.Skipf("%s is not set; skipping the MariaDB test", EnvURL)
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = ""
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// stage creates an empty database with the given literal name for the sweep
// to judge. A backstop drops it after the test unless the sweep already did.
func stage(t *testing.T, admin *sql.DB, name string) {
	t.Helper()
	if _, err := admin.Exec("CREATE DATABASE `" + name + "`"); err != nil {
		t.Fatalf("stage %s: %v", name, err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })
}

func schemaExists(t *testing.T, admin *sql.DB, name string) bool {
	t.Helper()
	var found string
	err := admin.QueryRow("SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", name).Scan(&found)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true
}

func randHex(t *testing.T, n int) string {
	t.Helper()
	raw := make([]byte, (n+1)/2)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", raw)[:n]
}

// runScope is this process's staging prefix: pid-scoped so a retry after a
// kill never collides with stranded names, and so extreme-age sweeps touch
// nothing but staged databases.
func runScope() string {
	return namePrefix + "sw" + strconv.Itoa(os.Getpid()) + "_"
}

func droppedListed(dropped []string, name string) bool {
	for _, d := range dropped {
		if d == name {
			return true
		}
	}
	return false
}

func TestSweepStaleDropsOnlyProvablyOldDatabases(t *testing.T) {
	admin := openAdmin(t)
	scope := runScope()
	pkg := scope[len(namePrefix) : len(scope)-1]
	stamp := time.Now().Unix()
	old := fmt.Sprintf("%s%s_%d_%s", namePrefix, pkg, stamp-7200, randHex(t, 12))
	fresh := fmt.Sprintf("%s%s_%d_%s", namePrefix, pkg, stamp, randHex(t, 12))
	legacy := scope + randHex(t, 16) // Pre-timestamp uuid shape, tableless.
	for _, name := range []string{old, fresh, legacy} {
		stage(t, admin, name)
	}
	// The full production sweep: an hour age is safe for every live
	// database (no suite run outlives it), so this proves the real path.
	dropped, err := SweepStale(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !droppedListed(dropped, old) || schemaExists(t, admin, old) {
		t.Errorf("two-hour-old timestamped database %s survived the sweep (dropped=%v)", old, dropped)
	}
	if !schemaExists(t, admin, fresh) {
		t.Errorf("fresh timestamped database %s was swept", fresh)
	}
	if !schemaExists(t, admin, legacy) {
		t.Errorf("tableless pre-timestamp database %s was swept without proof of age", legacy)
	}
}

func TestSweepStaleHonorsMaxAgeBothWays(t *testing.T) {
	admin := openAdmin(t)
	scope := runScope()
	pkg := scope[len(namePrefix) : len(scope)-1]
	stamp := time.Now().Unix()
	young := fmt.Sprintf("%s%s_%d_%s", namePrefix, pkg, stamp, randHex(t, 12))
	stage(t, admin, young)
	// A tiny age drops even a fresh database: the limit is honored, not
	// hardcoded. Scoped, so nothing outside this run is touched.
	if _, err := sweepUnder(time.Nanosecond, scope); err != nil {
		t.Fatal(err)
	}
	if schemaExists(t, admin, young) {
		t.Fatalf("fresh database %s survived a one-nanosecond sweep", young)
	}
	aged := fmt.Sprintf("%s%s_%d_%s", namePrefix, pkg, stamp-7200, randHex(t, 12))
	stage(t, admin, aged)
	if _, err := sweepUnder(10000*time.Hour, scope); err != nil {
		t.Fatal(err)
	}
	if !schemaExists(t, admin, aged) {
		t.Fatalf("old database %s was swept under a huge max age", aged)
	}
}

func TestSweepStaleAgesPreTimestampDatabasesByTheirTables(t *testing.T) {
	admin := openAdmin(t)
	scope := runScope()
	legacy := scope + randHex(t, 16)
	stage(t, admin, legacy)
	if _, err := admin.Exec("CREATE TABLE `" + legacy + "`.`proof` (id INT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	// The table is brand new, so an hour sweep keeps it; a nanosecond sweep
	// drops it through the table-age fallback for pre-timestamp names.
	if _, err := sweepUnder(time.Hour, scope); err != nil {
		t.Fatal(err)
	}
	if !schemaExists(t, admin, legacy) {
		t.Fatal("pre-timestamp database with fresh tables was swept")
	}
	if _, err := sweepUnder(time.Nanosecond, scope); err != nil {
		t.Fatal(err)
	}
	if schemaExists(t, admin, legacy) {
		t.Fatal("pre-timestamp database with tables survived a one-nanosecond sweep")
	}
}

func TestSweepStaleWithoutDatabaseURLIsNothing(t *testing.T) {
	t.Setenv(EnvURL, "")
	dropped, err := SweepStale(time.Hour)
	if err != nil || dropped != nil {
		t.Fatalf("SweepStale without %s = %v, %v", EnvURL, dropped, err)
	}
}

func TestThrowawayBirth(t *testing.T) {
	now := time.Now().Unix()
	for _, tc := range []struct {
		name string
		want int64
		ok   bool
	}{
		{"test_athena_auth_1763980000_deadbeefcafe", 1763980000, true},
		{fmt.Sprintf("test_athena_server_%d_0123456789ab", now), now, true},
		{"test_athena_0123456789abcdef", 0, false}, // Pre-timestamp uuid shape.
		{"test_athena_browser_3055", 0, false},     // Browser harness owns its names.
		{"test_athena_auth_notats_0123456789ab", 0, false},
		{"test_athena_auth_0_0123456789ab", 0, false},
		{"test_athena_auth_-5_0123456789ab", 0, false},
		{"other_db", 0, false},
		{"", 0, false},
	} {
		got, ok := throwawayBirth(tc.name)
		if ok != tc.ok || (ok && got.Unix() != tc.want) {
			t.Errorf("throwawayBirth(%q) = (%v, %v), want (%d, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestValidThrowawayName(t *testing.T) {
	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{"test_athena_auth_1763980000_deadbeefcafe", true},
		{"test_athena_0123456789abcdef", true},
		{"test_athena_browser_3055", true},
		{"test_athena_UPPER", false},
		{"test_athena_a`b", false},
		{"test_athena_a-b", false},
		{"other_db", false},
		{"", false},
	} {
		if got := validThrowawayName(tc.name); got != tc.ok {
			t.Errorf("validThrowawayName(%q) = %v, want %v", tc.name, got, tc.ok)
		}
	}
}
