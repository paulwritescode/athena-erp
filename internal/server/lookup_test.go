package server

import (
	"context"
	"fmt"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"

	"connectrpc.com/connect/v2"
)

func lookup(t *testing.T, s recordService, req *athenav1.LookupRecordsRequest) *athenav1.LookupRecordsResponse {
	t.Helper()
	res, err := s.LookupRecords(context.Background(), req)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	return res
}

func labels(items []*athenav1.LookupItem) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.GetLabel()
	}
	return out
}

func TestLookupListsLabelsFiltersAndPages(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	var acme *athenav1.Record
	for _, name := range []string{"Zed 100%", "Acme", "Bolt", "Acorn", "Zed 1_0"} {
		record := newCustomer(t, records, name)
		if name == "Acme" {
			acme = record
		}
	}
	all := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "customer"})
	if fmt.Sprint(labels(all.GetItems())) != "[Acme Acorn Bolt Zed 1_0 Zed 100%]" {
		t.Fatalf("labels = %v", labels(all.GetItems()))
	}
	if all.GetItems()[0].GetId() != acme.GetId() {
		t.Fatal("the item carries the Record ID as its value")
	}
	if got := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "customer", Query: "ac"}); fmt.Sprint(labels(got.GetItems())) != "[Acme Acorn]" {
		t.Fatalf("filtered = %v", labels(got.GetItems()))
	}
	if got := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "customer", Query: "%"}); fmt.Sprint(labels(got.GetItems())) != "[Zed 100%]" {
		t.Fatalf("a typed %% is literal, got %v", labels(got.GetItems()))
	}
	if got := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "customer", Query: "1_0"}); fmt.Sprint(labels(got.GetItems())) != "[Zed 1_0]" {
		t.Fatalf("a typed _ is literal, got %v", labels(got.GetItems()))
	}
	var paged []string
	token := ""
	for pages := 0; pages < 5; pages++ {
		page := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "customer", PageSize: 2, PageToken: token})
		paged = append(paged, labels(page.GetItems())...)
		if token = page.GetNextPageToken(); token == "" {
			break
		}
	}
	if fmt.Sprint(paged) != fmt.Sprint(labels(all.GetItems())) {
		t.Fatalf("paged = %v", paged)
	}
	_, err := records.LookupRecords(ctx, &athenav1.LookupRecordsRequest{RecordTypeKey: "customer", PageToken: "garbage"})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = records.LookupRecords(ctx, &athenav1.LookupRecordsRequest{RecordTypeKey: "customer", Ids: []string{"not-an-id"}})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = records.LookupRecords(ctx, &athenav1.LookupRecordsRequest{RecordTypeKey: "missing"})
	requireCode(t, err, connect.CodeNotFound)
}

func TestLookupHidesDeletedAndCancelledButStillLabelsThemByID(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	gone, kept := newCustomer(t, records, "Gone"), newCustomer(t, records, "Kept")
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "customer", Id: gone.GetId(), ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if got := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "customer"}); fmt.Sprint(labels(got.GetItems())) != "[Kept]" {
		t.Fatalf("a soft-deleted Record must not be offered: %v", labels(got.GetItems()))
	}
	got := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "customer", Ids: []string{gone.GetId(), kept.GetId()}})
	if len(got.GetItems()) != 2 || got.GetItems()[0].GetLabel() != "Gone" || got.GetItems()[0].GetState() != "deleted" || got.GetItems()[1].GetState() != "" {
		t.Fatalf("resolve = %v", got.GetItems())
	}

	lifecycleFixture(t, recordTypes)
	open, done := newOrder(t, records, "Open"), newOrder(t, records, "Done")
	done = submit(t, records, "order", done)
	if _, err := cancel(records, "order", done); err != nil {
		t.Fatal(err)
	}
	if got := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "order"}); fmt.Sprint(labels(got.GetItems())) != "[Open]" {
		t.Fatalf("a cancelled document must not be offered: %v", labels(got.GetItems()))
	}
	if got := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "order", Ids: []string{done.GetId(), open.GetId()}}); got.GetItems()[0].GetState() != "cancelled" || got.GetItems()[0].GetLabel() != "Done" {
		t.Fatalf("resolve = %v", got.GetItems())
	}
}

func TestLookupLabelPrefersLabelFieldThenNameThenFirstRequiredText(t *testing.T) {
	recordTypes, records := newServices(t)
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "part", Label: "Part", Kind: "master", LabelField: "code", Fields: []*athenav1.RecordTypeField{
		{Key: "title", Label: "Title", Type: "Text", Required: true}, {Key: "code", Label: "Code", Type: "Text"}}})
	if _, err := create(t, records, "part", map[string]*athenav1.FieldValue{"title": text("Widget"), "code": text("P-1")}); err != nil {
		t.Fatal(err)
	}
	if got := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "part"}); fmt.Sprint(labels(got.GetItems())) != "[P-1]" {
		t.Fatalf("label field = %v", labels(got.GetItems()))
	}
	// Without a label field a required text field wins over an earlier optional one.
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "tool", Label: "Tool", Kind: "master", Fields: []*athenav1.RecordTypeField{
		{Key: "note", Label: "Note", Type: "Text"}, {Key: "title", Label: "Title", Type: "Text", Required: true}}})
	if _, err := create(t, records, "tool", map[string]*athenav1.FieldValue{"note": text("n"), "title": text("Hammer")}); err != nil {
		t.Fatal(err)
	}
	if got := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "tool"}); fmt.Sprint(labels(got.GetItems())) != "[Hammer]" {
		t.Fatalf("required text = %v", labels(got.GetItems()))
	}
	// A numbered document shows its name; a draft shows its title.
	seriesFixture(t, recordTypes, nil)
	draft, numbered := newInvoice(t, records, "Draft one"), newInvoice(t, records, "Numbered")
	numbered = submit(t, records, "invoice", numbered)
	got := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "invoice", Ids: []string{draft.GetId(), numbered.GetId()}})
	if fmt.Sprint(labels(got.GetItems())) != "["+"Draft one "+numbered.GetName()+"]" && fmt.Sprint(labels(got.GetItems())) != "["+numbered.GetName()+" Draft one]" {
		t.Fatalf("document labels = %v", labels(got.GetItems()))
	}
	// A record with nothing to show falls back to its ID.
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "flag", Label: "Flag", Kind: "master", Fields: []*athenav1.RecordTypeField{{Key: "on", Label: "On", Type: "Checkbox"}}})
	record, err := create(t, records, "flag", map[string]*athenav1.FieldValue{})
	if err != nil {
		t.Fatal(err)
	}
	if got := lookup(t, records, &athenav1.LookupRecordsRequest{RecordTypeKey: "flag"}); got.GetItems()[0].GetLabel() != record.GetId() {
		t.Fatalf("fallback label = %v", labels(got.GetItems()))
	}
}

func TestLinkMustPointToAnExistingRecordOfTheTargetType(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	saveRecordType(t, recordTypes, "invoice_note", &athenav1.RecordTypeField{Key: "customer", Label: "Customer", Type: "Link", TargetRecordTypeKey: "customer"})
	customer := newCustomer(t, records, "Acme")
	if _, err := create(t, records, "invoice_note", map[string]*athenav1.FieldValue{"customer": text(customer.GetId())}); err != nil {
		t.Fatalf("a real Record ID must be accepted: %v", err)
	}
	_, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "invoice_note", Values: map[string]*athenav1.FieldValue{"customer": text("0190a1b2-ffff-7000-8000-00000000ffff")}})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "invoice_note", Values: map[string]*athenav1.FieldValue{"customer": text("Acme")}})
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestNewLinksRefuseCancelledAndDeletedTargetsButOldOnesStay(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	lifecycleFixture(t, recordTypes)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	saveRecordType(t, recordTypes, "note", &athenav1.RecordTypeField{Key: "customer", Label: "Customer", Type: "Link", TargetRecordTypeKey: "customer"}, &athenav1.RecordTypeField{Key: "text", Label: "Text", Type: "Text"})
	gone, live := newCustomer(t, records, "Gone"), newCustomer(t, records, "Live")
	note, err := create(t, records, "note", map[string]*athenav1.FieldValue{"customer": text(gone.GetId())})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "customer", Id: gone.GetId(), ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	// The old link survives an unrelated edit, but cannot be chosen again.
	if _, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "note", Id: note.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"text": text("edited")}}); err != nil {
		t.Fatalf("an unchanged link must not be rechecked: %v", err)
	}
	_, err = create(t, records, "note", map[string]*athenav1.FieldValue{"customer": text(gone.GetId())})
	requireCode(t, err, connect.CodeInvalidArgument)
	if _, err := create(t, records, "note", map[string]*athenav1.FieldValue{"customer": text(live.GetId())}); err != nil {
		t.Fatal(err)
	}
}
