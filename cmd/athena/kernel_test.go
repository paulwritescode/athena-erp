package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"athena-erp/internal/testdb"
)

func TestRecordTypesAndRecordsCommands(t *testing.T) {
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir()) // no home here, so the DSN fallback applies
	run := func(command func([]string, *bytes.Buffer) error, args ...string) (string, error) {
		var out bytes.Buffer
		err := command(args, &out)
		return out.String(), err
	}
	types := func(args ...string) (string, error) {
		return run(func(a []string, b *bytes.Buffer) error { return recordTypesCommand(a, b) }, args...)
	}
	records := func(args ...string) (string, error) {
		return run(func(a []string, b *bytes.Buffer) error { return recordsCommand(a, b) }, args...)
	}

	definition := filepath.Join(t.TempDir(), "item.json")
	if err := os.WriteFile(definition, []byte(`{"key":"item","label":"Item","kind":"master","fields":[
		{"key":"code","label":"Code","type":"Text","required":true,"unique":true},
		{"key":"price","label":"Price","type":"Money","currency":"USD","scale":2,"min":"0"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := types("save", "--file", definition, "--dry-run"); err != nil || !strings.Contains(out, "Create the table for Item (master)") {
		t.Fatalf("dry run = %q, %v", out, err)
	}
	if out, _ := types("list"); strings.Contains(out, "item") {
		t.Fatalf("a dry run saved the Record Type: %q", out)
	}
	if out, err := types("save", "--file", definition); err != nil || !strings.Contains(out, "Saved item at revision 1") {
		t.Fatalf("save = %q, %v", out, err)
	}
	if out, err := types("save", "--file", definition); err != nil || !strings.Contains(out, "revision 1") && !strings.Contains(out, "revision 2") {
		t.Fatalf("saving the same file again updates it: %q, %v", out, err)
	}
	if out, err := types("list"); err != nil || !strings.Contains(out, "item") || !strings.Contains(out, "master") {
		t.Fatalf("list = %q, %v", out, err)
	}
	if out, err := types("show", "item", "--json"); err != nil || !strings.Contains(out, `"USD"`) {
		t.Fatalf("show = %q, %v", out, err)
	}

	if out, err := records("create", "item", "--values", `{"code":"A1","price":"9.5"}`); err != nil || !strings.Contains(out, "Created ") {
		t.Fatalf("create = %q, %v", out, err)
	}
	if _, err := records("create", "item", "--values", `{"code":"A1"}`); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("a duplicate unique value must be refused, got %v", err)
	}
	if _, err := records("create", "item", "--values", `{"code":"B","price":"-1"}`); err == nil {
		t.Fatal("a value below the minimum was accepted")
	}
	listed, err := records("list", "item", "--json")
	if err != nil || !strings.Contains(listed, `"9.50"`) {
		t.Fatalf("list = %q, %v", listed, err)
	}
	id := regexp.MustCompile(`"id":\s+"([0-9a-f-]{36})"`).FindStringSubmatch(listed)[1]
	if _, err := records("delete", "item", id); err == nil {
		t.Fatal("delete without --revision was accepted")
	}
	if out, err := records("delete", "item", id, "--revision", "1"); err != nil || !strings.Contains(out, "Deleted") {
		t.Fatalf("delete = %q, %v", out, err)
	}
	if out, _ := records("list", "item"); strings.TrimSpace(out) != "" {
		t.Fatalf("a soft-deleted Record is still listed: %q", out)
	}

	if _, err := types("delete", "item"); err == nil {
		t.Fatal("delete without --yes was accepted")
	}
	if out, err := types("delete", "item", "--yes"); err != nil || !strings.Contains(out, "Deleted item") {
		t.Fatalf("delete = %q, %v", out, err)
	}
	for _, bad := range [][]string{{"sideways"}, {"show"}} {
		if _, err := types(bad...); err == nil {
			t.Fatalf("record-types %v accepted", bad)
		}
	}
	if out, err := types("help"); err != nil || !strings.Contains(out, "record-types list|show|save|delete") {
		t.Fatalf("help = %q, %v", out, err)
	}
}

func TestRecordsLifecycleCommands(t *testing.T) {
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir())
	run := func(command func([]string, io.Writer) error, args ...string) (string, error) {
		var out bytes.Buffer
		err := command(args, &out)
		return out.String(), err
	}
	definition := filepath.Join(t.TempDir(), "order.json")
	if err := os.WriteFile(definition, []byte(`{"key":"order","label":"Order","kind":"document","class":"non_posting","submittable":true,"amendable":true,"fields":[
		{"key":"title","label":"Title","type":"Text","required":true}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(recordTypesCommand, "save", "--file", definition); err != nil {
		t.Fatal(err)
	}
	created, err := run(recordsCommand, "create", "order", "--values", `{"title":"CLI"}`, "--json")
	if err != nil {
		t.Fatal(err)
	}
	id := regexp.MustCompile(`"id":\s+"([0-9a-f-]{36})"`).FindStringSubmatch(created)[1]
	if _, err := run(recordsCommand, "submit", "order", id); err == nil {
		t.Fatal("submit without --revision was accepted")
	}
	if out, err := run(recordsCommand, "submit", "order", id, "--revision", "1"); err != nil || !strings.Contains(out, "is now submitted (revision 2)") {
		t.Fatalf("submit = %q, %v", out, err)
	}
	if _, err := run(recordsCommand, "submit", "order", id, "--revision", "2"); err == nil || !strings.Contains(err.Error(), "submitted document cannot be submitted") {
		t.Fatalf("a second submit must be refused: %v", err)
	}
	if out, err := run(recordsCommand, "cancel", "order", id, "--revision", "2"); err != nil || !strings.Contains(out, "is now cancelled") {
		t.Fatalf("cancel = %q, %v", out, err)
	}
	if out, err := run(recordsCommand, "amend", "order", id, "--revision", "3"); err != nil || !strings.Contains(out, "-1 is now draft") {
		t.Fatalf("amend = %q, %v", out, err)
	}
	if out, err := run(recordsCommand, "versions", "order", id); err != nil || !strings.Contains(out, "version 2") || !strings.Contains(out, "cancel") {
		t.Fatalf("versions = %q, %v", out, err)
	}
	if out, err := run(recordsCommand, "activity", "order", id, "--json"); err != nil || !strings.Contains(out, `"amended"`) {
		t.Fatalf("activity = %q, %v", out, err)
	}
}
