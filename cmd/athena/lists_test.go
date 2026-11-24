package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
)

func TestListsExportArgs(t *testing.T) {
	var out bytes.Buffer
	if err := listsCommand([]string{"--help"}, &out); err != nil || !strings.Contains(out.String(), "athena lists export") {
		t.Fatalf("help err = %v, out = %q", err, out.String())
	}
	for _, args := range [][]string{
		{"frobnicate"},
		{"export"},
		{"export", "some-id"},
	} {
		var ignored bytes.Buffer
		if err := listsCommand(args, &ignored); err == nil {
			t.Fatalf("%v: expected an error", args)
		}
	}
}

// TestListsExportFlow runs the full export path. Like every DB-backed test
// on this branch, it stays red until migrations 21/22 land (see the
// migration 23 contiguity note); the service paths it covers were verified
// on a contiguous tree.
func TestListsExportFlow(t *testing.T) {
	slug := newTestWorkspace(t)
	kernel, ctx, closeKernel, err := openKernel(slug)
	if err != nil {
		t.Fatal(err)
	}
	defer closeKernel()
	if _, err := kernel.RecordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{
		Key: "guest", Label: "Guest", Kind: "master",
		Fields: []*athenav1.RecordTypeField{{Key: "name", Label: "Name", Type: "Text"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := kernel.Records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "guest", Values: map[string]*athenav1.FieldValue{
		"name": {Kind: &athenav1.FieldValue_StringValue{StringValue: "Wanjirũ Kamau"}},
	}}); err != nil {
		t.Fatal(err)
	}
	saved, err := kernel.Lists.SaveList(ctx, &athenav1.SaveListRequest{
		Name: "Everyone", RecordTypeKey: "guest",
		ConditionTree: `{"root":{"all":true,"conditions":[{"field":"doc.name","op":"is_not_empty"}]}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "out.pdf")
	var out bytes.Buffer
	if err := listsCommand([]string{"export", saved.GetList().GetId(), "--pdf", "--out", target, "--workspace", slug}, &out); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), "%PDF") {
		t.Fatal("export is not a PDF")
	}
	if !strings.Contains(out.String(), "Wrote ") {
		t.Fatalf("output = %q", out.String())
	}
}
