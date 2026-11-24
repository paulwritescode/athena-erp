package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRulesCommands(t *testing.T) {
	sharedDSNEnv(t)
	run := func(command func([]string, io.Writer) error, args ...string) (string, error) {
		var out bytes.Buffer
		err := command(args, &out)
		return out.String(), err
	}
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	definition := write("invoice.json", `{"key":"invoice","label":"Invoice","kind":"document","class":"non_posting","submittable":true,"fields":[
		{"key":"qty","label":"Quantity","type":"Number"},{"key":"total","label":"Total","type":"Number"}]}`)
	if _, err := run(recordTypesCommand, "save", "--file", definition); err != nil {
		t.Fatal(err)
	}
	if help, err := run(rulesCommand, "help"); err != nil || !strings.Contains(help, "--workspace") || !strings.Contains(help, "--json") || !strings.Contains(help, "not production-ready") {
		t.Fatalf("help = %q, %v", help, err)
	}
	validation := write("positive.star", "def validate(doc, ctx):\n    require(doc.qty > decimal(\"0\"), \"Quantity must be above zero\")\n")
	if out, err := run(rulesCommand, "save", "positive", "--file", validation, "--type", "validation", "--record-type", "invoice", "--event", "validate"); err != nil || !strings.Contains(out, "Saved positive (version 1") {
		t.Fatalf("save = %q, %v", out, err)
	}
	if _, err := run(rulesCommand, "save", "bad", "--file", write("float.star", "def validate(doc, ctx):\n    x = 1.5\n"), "--type", "validation", "--record-type", "invoice", "--event", "validate"); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("a float Rule saved: %v", err)
	}
	// An existing Rule keeps the attributes the flags leave out.
	if out, err := run(rulesCommand, "save", "positive", "--file", write("positive2.star", "def validate(doc, ctx):\n    pass\n"), "--priority", "5"); err != nil || !strings.Contains(out, "version 2") {
		t.Fatalf("update = %q, %v", out, err)
	}
	if out, err := run(rulesCommand, "list"); err != nil || !strings.Contains(out, "positive") || !strings.Contains(out, "validation") || !strings.Contains(out, "v2") {
		t.Fatalf("list = %q, %v", out, err)
	}
	if out, err := run(rulesCommand, "versions", "positive"); err != nil || !strings.Contains(out, "v1") || !strings.Contains(out, "v2") {
		t.Fatalf("versions = %q, %v", out, err)
	}
	if out, err := run(rulesCommand, "simulate", "positive", "--file", validation, "--value", "qty=0"); err != nil || !strings.Contains(out, "Outcome: rejected") || !strings.Contains(out, "Quantity must be above zero") {
		t.Fatalf("simulate = %q, %v", out, err)
	}
	derive := write("total.star", "def derive(doc, ctx):\n    set(\"total\", doc.qty * decimal(\"2\"))\n")
	if out, err := run(rulesCommand, "simulate", "double", "--file", derive, "--type", "derivation", "--record-type", "invoice", "--event", "validate", "--value", "qty=4"); err != nil || !strings.Contains(out, "set total: (empty) -> 8") {
		t.Fatalf("simulate draft = %q, %v", out, err)
	}
	if out, err := run(rulesCommand, "runs", "--rule", "double", "--json"); err != nil || !regexp.MustCompile(`"dryRun":\s*true`).MatchString(out) {
		t.Fatalf("runs = %q, %v", out, err)
	}
	if out, err := run(rulesCommand, "show", "positive", "--out", filepath.Join(dir, "out.star")); err != nil || !strings.Contains(out, "Wrote") {
		t.Fatalf("show = %q, %v", out, err)
	}
	if _, err := run(rulesCommand, "rm", "positive"); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("rm without --yes = %v", err)
	}
	if out, err := run(rulesCommand, "rm", "positive", "--yes"); err != nil || !strings.Contains(out, "Deleted positive") {
		t.Fatalf("rm = %q, %v", out, err)
	}
	if _, err := run(rulesCommand, "nope"); err == nil {
		t.Fatal("an unknown subcommand was accepted")
	}
}
