package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"athena-erp/internal/testdb"
)

func TestMigrateCommand(t *testing.T) {
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir()) // no home here, so the DSN fallback applies

	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := migrateCommand(args, &out); err != nil {
			t.Fatalf("migrate %v: %v", args, err)
		}
		return out.String()
	}

	if out := run("status"); !strings.Contains(out, "pending") || strings.Contains(out, "applied") {
		t.Fatalf("fresh status = %q", out)
	}
	if out := run("up", "--dry-run"); !strings.Contains(out, "Would apply 1 core_tables") {
		t.Fatalf("dry run = %q", out)
	}
	if out := run("status"); strings.Contains(out, "applied") {
		t.Fatalf("dry run changed the database: %q", out)
	}
	if out := run("up", "--to", "2"); !strings.Contains(out, "Applied 2 record_operations") || strings.Contains(out, "entry_history") {
		t.Fatalf("up --to 2 = %q", out)
	}
	if out := run("up"); !strings.Contains(out, "Applied 5 vocabulary_rename") {
		t.Fatalf("up = %q", out)
	}
	if out := run("up"); !strings.Contains(out, "No pending migrations") {
		t.Fatalf("second up = %q", out)
	}

	var lines []migrationLine
	if err := json.Unmarshal([]byte(run("status", "--json")), &lines); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 8 || !lines[7].Applied || lines[7].AppliedAt == nil {
		t.Fatalf("json status = %+v", lines)
	}
	if strings.Contains(run("status", "--json"), cfg.Passwd) && cfg.Passwd != "" {
		t.Fatal("output contains the password")
	}

	if err := migrateCommand([]string{"sideways"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown subcommand accepted")
	}
	if err := migrateCommand([]string{"up", "--to", "99"}, &bytes.Buffer{}); err == nil {
		t.Fatal("out-of-range --to accepted")
	}
}
