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

func TestNumberingLookupAndAuditCommands(t *testing.T) {
	db, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir())
	run := func(command func([]string, io.Writer) error, args ...string) (string, error) {
		var out bytes.Buffer
		err := command(args, &out)
		return out.String(), err
	}
	definition := filepath.Join(t.TempDir(), "invoice.json")
	if err := os.WriteFile(definition, []byte(`{"key":"invoice","label":"Invoice","kind":"document","class":"posting","submittable":true,
		"namingSeries":"CLI-{####}","gapFree":true,"allowManualName":true,"labelField":"title",
		"fields":[{"key":"title","label":"Title","type":"Text","required":true}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := run(recordTypesCommand, "save", "--file", definition); err != nil {
		t.Fatalf("save = %q, %v", out, err)
	}
	if out, err := run(recordTypesCommand, "show", "invoice"); err != nil || !strings.Contains(out, "CLI-{####}") || !regexp.MustCompile(`"gapFree":\s+true`).MatchString(out) {
		t.Fatalf("show = %q, %v", out, err)
	}
	created, err := run(recordsCommand, "create", "invoice", "--values", `{"title":"Alpha"}`, "--json")
	if err != nil {
		t.Fatal(err)
	}
	id := regexp.MustCompile(`"id":\s+"([0-9a-f-]{36})"`).FindStringSubmatch(created)[1]
	if out, err := run(recordsCommand, "submit", "invoice", id, "--revision", "1"); err != nil || !strings.Contains(out, "CLI-0001 is now submitted") {
		t.Fatalf("submit = %q, %v", out, err)
	}
	if out, err := run(recordsCommand, "create", "invoice", "--values", `{"title":"Old"}`, "--name", "LEGACY-7"); err != nil || !strings.Contains(out, "Created ") {
		t.Fatalf("manual name = %q, %v", out, err)
	}
	if _, err := run(recordsCommand, "create", "invoice", "--values", `{"title":"Dup"}`, "--name", "LEGACY-7"); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("a repeated manual name must be refused: %v", err)
	}
	// lookup: search text before the flags, labelled by the label field.
	if out, err := run(recordsCommand, "lookup", "invoice", "alp"); err != nil || !strings.Contains(out, id+"\tAlpha") || strings.Contains(out, "Old") {
		t.Fatalf("lookup = %q, %v", out, err)
	}
	if out, err := run(recordsCommand, "lookup", "invoice", "--json", "--limit", "1"); err != nil || !strings.Contains(out, `"nextPageToken"`) {
		t.Fatalf("lookup json = %q, %v", out, err)
	}
	if out, err := run(recordsCommand, "history", "invoice", id); err != nil || !strings.Contains(out, "submitted") || !strings.Contains(out, "name: ") || !strings.Contains(out, "-> CLI-0001") {
		t.Fatalf("history = %q, %v", out, err)
	}
	out, err := run(auditCommand, "verify")
	head := regexp.MustCompile(`head (\d+:[0-9a-f]{64})`).FindStringSubmatch(out)
	if err != nil || head == nil {
		t.Fatalf("verify = %q, %v", out, err)
	}
	if out, err := run(auditCommand, "verify", "--anchor", head[1], "--json"); err != nil || !regexp.MustCompile(`"ok":\s+true`).MatchString(out) {
		t.Fatalf("anchored verify = %q, %v", out, err)
	}
	if out, err := run(auditCommand, "anchor"); err != nil || strings.TrimSpace(out) != head[1] {
		t.Fatalf("anchor = %q, want %q, %v", out, head[1], err)
	}
	if _, err := run(auditCommand, "anchor", "--anchor", head[1]); err == nil {
		t.Fatal("audit anchor accepted --anchor")
	}
	if _, err := run(auditCommand, "verify", "--anchor", "nonsense"); err == nil {
		t.Fatal("a malformed anchor was accepted")
	}
	// Tampering makes verify fail with a non-zero result.
	if _, err := db.Exec("UPDATE athena_audit_log SET actor = 'x' WHERE seq = 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(auditCommand, "verify"); err == nil || !strings.Contains(err.Error(), "Audit Log is broken") {
		t.Fatalf("a tampered log must fail verification: %v", err)
	}
	if out, err := run(auditCommand, "anchor"); err == nil || strings.TrimSpace(out) != "" {
		t.Fatalf("a tampered log must not print an anchor: %q, %v", out, err)
	}
	if out, err := run(auditCommand, "help"); err != nil || !strings.Contains(out, "audit verify") {
		t.Fatalf("help = %q, %v", out, err)
	}
}

func TestExpressionsCommand(t *testing.T) {
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir())
	run := func(command func([]string, io.Writer) error, args ...string) (string, error) {
		var out bytes.Buffer
		err := command(args, &out)
		return out.String(), err
	}
	definition := filepath.Join(t.TempDir(), "calc.json")
	if err := os.WriteFile(definition, []byte(`{"key":"calc","label":"Calc","fields":[
		{"key":"qty","label":"Qty","type":"Number"},
		{"key":"double","label":"Double","type":"Number","formula":"doc.qty * 2"},
		{"key":"why","label":"Why","type":"Text","requiredIf":"doc.qty > 10"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := run(recordTypesCommand, "save", "--file", definition); err != nil {
		t.Fatalf("save = %q, %v", out, err)
	}
	if _, err := run(recordsCommand, "create", "calc", "--values", `{"qty":"11"}`); err == nil || !strings.Contains(err.Error(), "Why is required") {
		t.Fatalf("required_if must be enforced: %v", err)
	}
	created, err := run(recordsCommand, "create", "calc", "--values", `{"qty":"4"}`, "--json")
	if err != nil || !regexp.MustCompile(`"double":\s*\{\s*"stringValue":\s*"8"`).MatchString(created) {
		t.Fatalf("create = %q, %v", created, err)
	}
	id := regexp.MustCompile(`"id":\s+"([0-9a-f-]{36})"`).FindStringSubmatch(created)[1]
	if out, err := run(expressionsCommand, "check", "calc", "doc.qty > dec(1)"); err != nil || !strings.Contains(out, "type: bool") || !strings.Contains(out, "runs in the database") {
		t.Fatalf("check = %q, %v", out, err)
	}
	if _, err := run(expressionsCommand, "check", "calc", "doc.qty > 1.5"); err == nil {
		t.Fatal("a float literal must be refused")
	}
	if out, err := run(expressionsCommand, "eval", "calc", "doc.double + doc.qty", "--record", id); err != nil || strings.TrimSpace(out) != "12" {
		t.Fatalf("eval = %q, %v", out, err)
	}
	if out, err := run(expressionsCommand, "preview", "calc", "--values", `{"qty":"2.5"}`); err != nil || !strings.Contains(out, "double = 5") {
		t.Fatalf("preview = %q, %v", out, err)
	}
	if out, err := run(recordsCommand, "list", "calc", "--filter", "doc.double > dec(100)"); err != nil || strings.Contains(out, id) {
		t.Fatalf("filtered list = %q, %v", out, err)
	}
	if out, err := run(recordsCommand, "list", "calc", "--filter", "doc.double == dec(8)"); err != nil || !strings.Contains(out, id) {
		t.Fatalf("filtered list = %q, %v", out, err)
	}
	if out, err := run(expressionsCommand, "--help"); err != nil || !strings.Contains(out, "check|eval|preview") {
		t.Fatalf("help = %q, %v", out, err)
	}
}

func TestRecordTypesShowReportsLocalAdministrator(t *testing.T) {
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir()) // no home here, so the DSN fallback applies
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := recordTypesCommand(args, &out)
		return out.String(), err
	}
	definition := filepath.Join(t.TempDir(), "item.json")
	if err := os.WriteFile(definition, []byte(`{"key":"item","label":"Item","kind":"master","fields":[{"key":"code","label":"Code","type":"Text"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run("save", "--file", definition); err != nil {
		t.Fatal(err)
	}
	out, err := run("show", "item", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"updatedBy"`) || !strings.Contains(out, `"Local administrator"`) {
		t.Fatalf("show = %q, want the operator as the saver", out)
	}
	if !strings.Contains(out, `"updatedAt"`) {
		t.Fatalf("show = %q, want a last-save timestamp", out)
	}
}

func TestRecordTypesSaveLayoutJSON(t *testing.T) {
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir()) // no home here, so the DSN fallback applies
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := recordTypesCommand(args, &out)
		return out.String(), err
	}
	definition := filepath.Join(t.TempDir(), "page.json")
	if err := os.WriteFile(definition, []byte(`{"key":"page","label":"Page","fields":[
		{"key":"title","label":"Title","type":"Text"},
		{"key":"body","label":"Body","type":"Text"}],
		"layout":{"sections":[{"title":"Main","columns":2,"visibility":"doc.title != \"\"","items":[
		{"field":{"fieldKey":"title","width":"half","helpText":"The heading"}},
		{"text":{"text":"Hello"}}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := run("save", "--file", definition); err != nil || !strings.Contains(out, "Saved page at revision 1") {
		t.Fatalf("save = %q, %v", out, err)
	}
	out, err := run("show", "page", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"layout"`, `"fieldKey"`, `"width"`, `"helpText"`, `"The heading"`, `"Hello"`, `"visibility"`, `"body"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("show = %q, want %s", out, want)
		}
	}
}

func TestRecordsCreateImageValue(t *testing.T) {
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir()) // no home here, so the DSN fallback applies
	run := func(command func([]string, *bytes.Buffer) error, args ...string) (string, error) {
		var out bytes.Buffer
		err := command(args, &out)
		return out.String(), err
	}
	definition := filepath.Join(t.TempDir(), "item.json")
	if err := os.WriteFile(definition, []byte(`{"key":"item","label":"Item","fields":[{"key":"photo","label":"Photo","type":"Image"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(func(a []string, b *bytes.Buffer) error { return recordTypesCommand(a, b) }, "save", "--file", definition); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("ab", 32)
	out, err := run(func(a []string, b *bytes.Buffer) error { return recordsCommand(a, b) }, "create", "item", "--json",
		"--values", `{"photo":{"path":"images/logo.png","hash":"`+hash+`","alt_text":"The logo"}}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"imageValue"`, `"images/logo.png"`, `"The logo"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("create = %q, want %s", out, want)
		}
	}
	if _, err := run(func(a []string, b *bytes.Buffer) error { return recordsCommand(a, b) }, "create", "item",
		"--values", `{"photo":{"path":"images/logo.png","hash":"`+hash+`"}}`); err == nil {
		t.Fatal("a value without alt text was accepted")
	}
}
