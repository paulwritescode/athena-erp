package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"athena-erp/internal/testdb"
	"athena-erp/internal/workspace"
)

// testImageFile encodes a w-by-h PNG for upload tests.
func testImageFile(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 0, 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// newTestWorkspace creates a home and a Workspace with its own database, and
// drops the database when the test ends.
func newTestWorkspace(t *testing.T) (slug string) {
	t.Helper()
	db, cfg := testdb.NewWithConfig(t)
	server := cfg.Clone()
	server.DBName = ""
	home := t.TempDir()
	if err := workspace.InitHome(home, server.FormatDSN()); err != nil {
		t.Fatal(err)
	}
	result, err := workspace.Create(context.Background(), home, "test athena files cli "+strconv.FormatInt(time.Now().UnixNano(), 36), server.FormatDSN(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP DATABASE IF EXISTS `" + result.Manifest.Database.Name + "`") })
	t.Chdir(home)
	return result.Manifest.Slug
}

func TestFilesPutImageCommands(t *testing.T) {
	slug := newTestWorkspace(t)
	run := func(stdin string, args ...string) (string, error) {
		var out bytes.Buffer
		err := filesCommand(append(args, "--workspace", slug), strings.NewReader(stdin), &out)
		return out.String(), err
	}
	content := testImageFile(t, 16, 12)
	local := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(local, content, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := run("", "put-image", "images/logo.png", "--file", local, "--new")
	if err != nil || !strings.Contains(out, "Saved images/logo.png (16x12, revision 1") {
		t.Fatalf("put-image = %q, %v", out, err)
	}
	if out, err := run("", "get", "images/logo.png", "--out", filepath.Join(t.TempDir(), "back.png")); err != nil {
		t.Fatalf("get = %q, %v", out, err)
	}
	if _, err := run("not an image", "put-image", "images/evil.png", "--new"); err == nil {
		t.Fatal("text uploaded as an image was accepted")
	}
	if _, err := run("", "put-image", "rules/logo.png", "--file", local, "--new"); err == nil {
		t.Fatal("an image outside images/ was accepted")
	}
}

func TestFilesCommands(t *testing.T) {
	slug := newTestWorkspace(t)
	run := func(stdin string, args ...string) (string, error) {
		var out bytes.Buffer
		err := filesCommand(append(args, "--workspace", slug), strings.NewReader(stdin), &out)
		return out.String(), err
	}
	const recordType = "api: athena/v1\nrecord_type: customer\nlabel: Customer\nkind: master\nfields:\n  - name: name\n    type: Text\n"

	var help bytes.Buffer
	if err := filesCommand([]string{"help"}, nil, &help); err != nil || !strings.Contains(help.String(), "--workspace") || !strings.Contains(help.String(), "--json") {
		t.Fatalf("help = %q, %v", help.String(), err)
	}
	if out, err := run(recordType, "put", "record_types/customer.yaml", "--new"); err != nil || !strings.Contains(out, "Saved record_types/customer.yaml (revision 1") {
		t.Fatalf("put = %q, %v", out, err)
	}
	if _, err := run(recordType, "put", "record_types/customer.yaml", "--new"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("put --new over an existing file = %v", err)
	}
	if _, err := run(recordType, "put", "record_types/customer.yaml", "--expected-hash", "stale"); err == nil || !strings.Contains(err.Error(), "no longer matches") {
		t.Fatalf("put with a stale hash = %v", err)
	}
	// The same checks as the API: an escaping path and a bad Record Type file are refused.
	if _, err := run("x: 1\n", "put", "../x.yaml"); err == nil {
		t.Fatal("an escaping path was accepted")
	}
	if _, err := run("record_type: customer\n", "put", "record_types/customer.yaml"); err == nil || !strings.Contains(err.Error(), "missing required key") {
		t.Fatalf("an invalid Record Type file = %v", err)
	}
	if out, err := run(recordType+"# edited\n", "put", "record_types/customer.yaml"); err != nil || !strings.Contains(out, "revision 2") {
		t.Fatalf("put replace = %q, %v", out, err)
	}
	if out, err := run("", "list"); err != nil || !strings.Contains(out, "record_types/customer.yaml") || !strings.Contains(out, "record_type customer") {
		t.Fatalf("list = %q, %v", out, err)
	}
	if out, err := run("", "list", "--json"); err != nil || !strings.Contains(out, `"maxFileBytes"`) {
		t.Fatalf("list --json = %q, %v", out, err)
	}
	if out, err := run("", "get", "record_types/customer.yaml"); err != nil || out != recordType+"# edited\n" {
		t.Fatalf("get = %q, %v", out, err)
	}
	if out, err := run("", "get", "record_types/customer.yaml", "--revision", "1"); err != nil || out != recordType {
		t.Fatalf("get --revision 1 = %q, %v", out, err)
	}
	target := filepath.Join(t.TempDir(), "copy.yaml")
	if out, err := run("", "get", "record_types/customer.yaml", "--out", target); err != nil || !strings.Contains(out, "Wrote") {
		t.Fatalf("get --out = %q, %v", out, err)
	}
	if data, _ := os.ReadFile(target); string(data) != recordType+"# edited\n" {
		t.Fatal("get --out wrote other content")
	}
	if out, err := run(recordType, "put", "rules/note.star", "--new"); err != nil {
		t.Fatalf("put rule = %q, %v", out, err)
	}
	if out, err := run("", "mv", "rules/note.star", "rules/memo.star"); err != nil || !strings.Contains(out, "Moved to rules/memo.star") {
		t.Fatalf("mv = %q, %v", out, err)
	}
	if _, err := run("", "rm", "rules/memo.star"); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("rm without --yes = %v", err)
	}
	if out, err := run("", "rm", "rules/memo.star", "--yes"); err != nil || !strings.Contains(out, "Deleted") {
		t.Fatalf("rm = %q, %v", out, err)
	}
	if out, err := run("", "history", "rules/memo.star"); err != nil || !strings.Contains(out, "deleted") || !strings.Contains(out, "moved") || !strings.Contains(out, "created") {
		t.Fatalf("history = %q, %v", out, err)
	}
	if _, err := run("", "get", "rules/memo.star"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("get after rm = %v", err)
	}
	if _, err := run("", "frobnicate"); err == nil {
		t.Fatal("an unknown subcommand was accepted")
	}
}
