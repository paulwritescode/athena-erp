package testdb_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"athena-erp/internal/testdb"
)

// TestThrowawayNameRecordsPackageAndTime checks the names New hands out:
// this file's package (_test trimmed), a current timestamp and a random
// tail, so SweepStale can age leftovers.
func TestThrowawayNameRecordsPackageAndTime(t *testing.T) {
	_, cfg := testdb.NewWithConfig(t)
	name := cfg.DBName
	matched, err := regexp.MatchString(`^test_athena_testdb_[0-9]{10}_[0-9a-f]{12}$`, name)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatalf("database name = %q, want test_athena_testdb_<unixts>_<rand12>", name)
	}
	sec, err := strconv.ParseInt(strings.Split(name, "_")[3], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if born := time.Unix(sec, 0); time.Since(born) > 2*time.Minute || time.Since(born) < 0 {
		t.Fatalf("database timestamp = %v, want about now", born)
	}
	if len(name) > 64 {
		t.Fatalf("database name is %d characters, over the 64 limit", len(name))
	}
}
