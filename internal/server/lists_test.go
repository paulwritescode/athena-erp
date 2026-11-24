package server

import (
	"context"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"connectrpc.com/connect/v2"
)

// NOTE: these tests migrate SystemMigrations(), so they stay red while
// versions 21/22 (C7/C2 lanes) are unmerged — the contiguity gap in
// migration 23's comment, not a failure. They run green after the rebase.

func listTestType(t *testing.T, types recordTypeService) {
	t.Helper()
	ctx := context.Background()
	_, err := types.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{
		Key: "guest", Label: "Guest", Kind: "master",
		Fields: []*athenav1.RecordTypeField{
			{Key: "name", Label: "Name", Type: "Text"},
			{Key: "plus_ones", Label: "Plus-ones", Type: "Number"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func listTestRecord(t *testing.T, records recordService, name string, plusOnes string) {
	t.Helper()
	values := map[string]*athenav1.FieldValue{"name": text(name)}
	if plusOnes != "" {
		values["plus_ones"] = text(plusOnes)
	}
	_, err := records.CreateRecord(context.Background(), &athenav1.CreateRecordRequest{RecordTypeKey: "guest", Values: values})
	if err != nil {
		t.Fatal(err)
	}
}

func TestListSaveGetListDelete(t *testing.T) {
	types, records := newServices(t)
	listTestType(t, types)
	svc := listService{recordService: records}
	ctx := context.Background()
	tree := `{"root":{"all":true,"conditions":[{"field":"doc.name","op":"is_not","values":["x"]}]}}`

	saved, err := svc.SaveList(ctx, &athenav1.SaveListRequest{Name: "Everyone", RecordTypeKey: "guest", ConditionTree: tree})
	if err != nil {
		t.Fatal(err)
	}
	list := saved.GetList()
	if list.GetId() == "" || list.GetRevision() != 1 || list.GetConditionExpr() != `doc.name != 'x'` {
		t.Fatalf("saved: %+v", list)
	}
	if list.GetOwner() == "" {
		t.Fatal("owner not recorded")
	}

	got, err := svc.GetList(ctx, &athenav1.GetListRequest{Id: list.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetList().GetName() != "Everyone" {
		t.Fatalf("get: %+v", got.GetList())
	}

	again, err := svc.SaveList(ctx, &athenav1.SaveListRequest{Id: list.GetId(), Name: "All guests", RecordTypeKey: "guest", ConditionTree: tree, ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if again.GetList().GetRevision() != 2 {
		t.Fatalf("revision: %+v", again.GetList())
	}
	_, err = svc.SaveList(ctx, &athenav1.SaveListRequest{Id: list.GetId(), Name: "Stale", RecordTypeKey: "guest", ConditionTree: tree, ExpectedRevision: 1})
	requireCode(t, err, connect.CodeAborted)

	listed, err := svc.ListLists(ctx, &athenav1.ListListsRequest{RecordTypeKey: "guest"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.GetLists()) != 1 || listed.GetLists()[0].GetName() != "All guests" {
		t.Fatalf("listed: %+v", listed.GetLists())
	}

	_, err = svc.DeleteList(ctx, &athenav1.DeleteListRequest{Id: list.GetId(), ExpectedRevision: 1})
	requireCode(t, err, connect.CodeAborted)
	if _, err := svc.DeleteList(ctx, &athenav1.DeleteListRequest{Id: list.GetId(), ExpectedRevision: 2}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.GetList(ctx, &athenav1.GetListRequest{Id: list.GetId()})
	requireCode(t, err, connect.CodeNotFound)
}

func TestListSaveValidation(t *testing.T) {
	types, records := newServices(t)
	listTestType(t, types)
	svc := listService{recordService: records}
	ctx := context.Background()
	good := `{"root":{"all":true,"conditions":[{"field":"doc.name","op":"is","values":["a"]}]}}`
	cases := []struct {
		name string
		req  *athenav1.SaveListRequest
	}{
		{"empty name", &athenav1.SaveListRequest{Name: "", RecordTypeKey: "guest", ConditionTree: good}},
		{"unknown type", &athenav1.SaveListRequest{Name: "x", RecordTypeKey: "nope", ConditionTree: good}},
		{"bad json", &athenav1.SaveListRequest{Name: "x", RecordTypeKey: "guest", ConditionTree: "{oops"}},
		{"bad tree", &athenav1.SaveListRequest{Name: "x", RecordTypeKey: "guest", ConditionTree: `{"root":{"all":true,"conditions":[{"field":"doc.nope","op":"is","values":["a"]}]}}`}},
		{"bad column", &athenav1.SaveListRequest{Name: "x", RecordTypeKey: "guest", ConditionTree: good, Columns: []string{"nope"}}},
	}
	for _, tc := range cases {
		if _, err := svc.SaveList(ctx, tc.req); err == nil {
			t.Errorf("%s: saved", tc.name)
		}
	}
	if _, err := svc.SaveList(ctx, &athenav1.SaveListRequest{Name: "dup", RecordTypeKey: "guest", ConditionTree: good}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.SaveList(ctx, &athenav1.SaveListRequest{Name: "dup", RecordTypeKey: "guest", ConditionTree: good})
	requireCode(t, err, connect.CodeAlreadyExists)
}

func TestListRunAndExport(t *testing.T) {
	types, records := newServices(t)
	listTestType(t, types)
	listTestRecord(t, records, "Wanjirũ Kamau", "2")
	listTestRecord(t, records, "Jane Doe", "0")
	svc := listService{recordService: records}
	ctx := context.Background()
	tree := `{"root":{"all":true,"conditions":[{"field":"doc.plus_ones","op":"more_than","values":["0"]}]}}`
	saved, err := svc.SaveList(ctx, &athenav1.SaveListRequest{Name: "With plus-ones", RecordTypeKey: "guest", ConditionTree: tree, Columns: []string{"name", "plus_ones"}})
	if err != nil {
		t.Fatal(err)
	}
	id := saved.GetList().GetId()

	run, err := svc.RunList(ctx, &athenav1.RunListRequest{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.GetRecords()) != 1 || run.GetRecords()[0].GetValues()["name"].GetStringValue() != "Wanjirũ Kamau" {
		t.Fatalf("run matched %+v", run.GetRecords())
	}

	out, err := svc.ExportListPdf(ctx, &athenav1.ExportListPdfRequest{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out.GetPdf()), "%PDF") {
		t.Fatal("export is not a PDF")
	}
	if out.GetFilename() != "with-plus-ones.pdf" {
		t.Fatalf("filename = %q", out.GetFilename())
	}
	if !strings.Contains(string(out.GetPdf()), "0169") {
		t.Fatal("export misses U+0169 (ũ)")
	}
}
