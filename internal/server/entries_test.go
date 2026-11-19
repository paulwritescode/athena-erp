package server

import (
	"context"
	"fmt"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"connectrpc.com/connect/v2"
)

var customerFields = []*athenav1.RecordField{
	{Key: "name", Label: "Name", Type: "Text", Required: true},
	{Key: "balance", Label: "Balance", Type: "Currency"},
}

func newCustomer(t *testing.T, s entryService, name string) *athenav1.Entry {
	t.Helper()
	res, err := s.CreateEntry(context.Background(), &athenav1.CreateEntryRequest{RecordKey: "customer", Values: map[string]*athenav1.FieldValue{"name": text(name)}})
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	return res.GetEntry()
}

func TestEntryCreateGetAndValidation(t *testing.T) {
	ctx := context.Background()
	records, entries := newServices(t)
	saveRecord(t, records, "customer", customerFields...)

	entry := newCustomer(t, entries, "Acme")
	if entry.GetRevision() != 1 || entry.GetValues()["name"].GetStringValue() != "Acme" {
		t.Fatalf("unexpected entry: %v", entry)
	}
	got, err := entries.GetEntry(ctx, &athenav1.GetEntryRequest{RecordKey: "customer", Id: entry.GetId()})
	if err != nil || got.GetEntry().GetId() != entry.GetId() {
		t.Fatalf("get entry: %v", err)
	}
	if !got.GetEntry().GetValues()["balance"].GetNullValue() {
		t.Fatal("an unset field reads back as null")
	}

	_, err = entries.CreateEntry(ctx, &athenav1.CreateEntryRequest{RecordKey: "customer", Values: map[string]*athenav1.FieldValue{}})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = entries.CreateEntry(ctx, &athenav1.CreateEntryRequest{RecordKey: "customer", Values: map[string]*athenav1.FieldValue{"name": text("x"), "nope": text("y")}})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = entries.CreateEntry(ctx, &athenav1.CreateEntryRequest{RecordKey: "missing", Values: map[string]*athenav1.FieldValue{}})
	requireCode(t, err, connect.CodeNotFound)
}

func TestEntryDecimalKeepsExactValue(t *testing.T) {
	ctx := context.Background()
	records, entries := newServices(t)
	saveRecord(t, records, "customer", customerFields...)
	res, err := entries.CreateEntry(ctx, &athenav1.CreateEntryRequest{RecordKey: "customer", Values: map[string]*athenav1.FieldValue{"name": text("Acme"), "balance": text("0.100000")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.GetEntry().GetValues()["balance"].GetStringValue(); got != "0.1" {
		t.Fatalf("balance = %q, want 0.1", got)
	}
}

func TestEntryUpdateRejectsStaleRevision(t *testing.T) {
	ctx := context.Background()
	records, entries := newServices(t)
	saveRecord(t, records, "customer", customerFields...)
	entry := newCustomer(t, entries, "Acme")

	update := func(revision int64, name string) (*athenav1.UpdateEntryResponse, error) {
		return entries.UpdateEntry(ctx, &athenav1.UpdateEntryRequest{RecordKey: "customer", Id: entry.GetId(), ExpectedRevision: revision, Values: map[string]*athenav1.FieldValue{"name": text(name)}})
	}
	first, err := update(1, "Acme Ltd")
	if err != nil || first.GetEntry().GetRevision() != 2 {
		t.Fatalf("first update: %v %v", first, err)
	}
	// A second writer still holding revision 1 must be refused and change nothing.
	_, err = update(1, "Other")
	requireCode(t, err, connect.CodeAborted)
	got, _ := entries.GetEntry(ctx, &athenav1.GetEntryRequest{RecordKey: "customer", Id: entry.GetId()})
	if got.GetEntry().GetValues()["name"].GetStringValue() != "Acme Ltd" || got.GetEntry().GetRevision() != 2 {
		t.Fatalf("a refused update changed the entry: %v", got.GetEntry())
	}
}

func TestEntryUnchangedUpdateKeepsRevisionAndWritesNoHistory(t *testing.T) {
	ctx := context.Background()
	records, entries := newServices(t)
	saveRecord(t, records, "customer", customerFields...)
	entry := newCustomer(t, entries, "Acme")
	res, err := entries.UpdateEntry(ctx, &athenav1.UpdateEntryRequest{RecordKey: "customer", Id: entry.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"name": text("Acme")}})
	if err != nil || res.GetEntry().GetRevision() != 1 {
		t.Fatalf("unchanged update: %v %v", res, err)
	}
	history, err := entries.ListEntryHistory(ctx, &athenav1.ListEntryHistoryRequest{RecordKey: "customer", Id: entry.GetId()})
	if err != nil || len(history.GetEvents()) != 1 || history.GetEvents()[0].GetAction() != "created" {
		t.Fatalf("history = %v, %v", history, err)
	}
}

func TestEntryDeleteRejectsStaleRevision(t *testing.T) {
	ctx := context.Background()
	records, entries := newServices(t)
	saveRecord(t, records, "customer", customerFields...)
	entry := newCustomer(t, entries, "Acme")

	_, err := entries.DeleteEntry(ctx, &athenav1.DeleteEntryRequest{RecordKey: "customer", Id: entry.GetId(), ExpectedRevision: 7})
	requireCode(t, err, connect.CodeAborted)
	if _, err := entries.DeleteEntry(ctx, &athenav1.DeleteEntryRequest{RecordKey: "customer", Id: entry.GetId(), ExpectedRevision: 1}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = entries.GetEntry(ctx, &athenav1.GetEntryRequest{RecordKey: "customer", Id: entry.GetId()})
	requireCode(t, err, connect.CodeNotFound)
	_, err = entries.DeleteEntry(ctx, &athenav1.DeleteEntryRequest{RecordKey: "customer", Id: entry.GetId(), ExpectedRevision: 1})
	requireCode(t, err, connect.CodeNotFound)
}

func TestEntryListPagesInIDOrderWithoutGaps(t *testing.T) {
	ctx := context.Background()
	records, entries := newServices(t)
	saveRecord(t, records, "customer", customerFields...)
	var created []string
	for i := range 5 {
		created = append(created, newCustomer(t, entries, fmt.Sprintf("Customer %d", i)).GetId())
	}

	var listed []string
	token := ""
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("paging did not finish")
		}
		res, err := entries.ListEntries(ctx, &athenav1.ListEntriesRequest{RecordKey: "customer", PageSize: 2, PageToken: token})
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range res.GetEntries() {
			listed = append(listed, e.GetId())
		}
		if token = res.GetNextPageToken(); token == "" {
			break
		}
	}
	if fmt.Sprint(listed) != fmt.Sprint(created) {
		t.Fatalf("listed %v, want %v", listed, created)
	}

	// A token issued for one Record is refused for another.
	saveRecord(t, records, "supplier", customerFields...)
	first, _ := entries.ListEntries(ctx, &athenav1.ListEntriesRequest{RecordKey: "customer", PageSize: 2})
	_, err := entries.ListEntries(ctx, &athenav1.ListEntriesRequest{RecordKey: "supplier", PageToken: first.GetNextPageToken()})
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestEntryHistoryRecordsEachChange(t *testing.T) {
	ctx := context.Background()
	records, entries := newServices(t)
	saveRecord(t, records, "customer", customerFields...)
	entry := newCustomer(t, entries, "Acme")
	if _, err := entries.UpdateEntry(ctx, &athenav1.UpdateEntryRequest{RecordKey: "customer", Id: entry.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"name": text("Acme Ltd")}}); err != nil {
		t.Fatal(err)
	}
	history, err := entries.ListEntryHistory(ctx, &athenav1.ListEntryHistoryRequest{RecordKey: "customer", Id: entry.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	events := history.GetEvents()
	if len(events) != 2 || events[0].GetAction() != "updated" || events[0].GetRevision() != 2 || fmt.Sprint(events[0].GetChangedFields()) != "[name]" || events[1].GetAction() != "created" {
		t.Fatalf("events = %v", events)
	}
	if events[0].GetActor() != localActor {
		t.Fatalf("actor = %q", events[0].GetActor())
	}
}

// The change and its history row share one transaction: when the history write
// fails, the change must not survive.
func TestEntryChangeAndHistoryShareATransaction(t *testing.T) {
	ctx := context.Background()
	records, entries := newServices(t)
	saveRecord(t, records, "customer", customerFields...)
	entry := newCustomer(t, entries, "Acme")
	if _, err := entries.db.ExecContext(ctx, "DROP TABLE athena_entry_history"); err != nil {
		t.Fatal(err)
	}

	_, err := entries.UpdateEntry(ctx, &athenav1.UpdateEntryRequest{RecordKey: "customer", Id: entry.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"name": text("Changed")}})
	requireCode(t, err, connect.CodeInternal)
	_, err = entries.DeleteEntry(ctx, &athenav1.DeleteEntryRequest{RecordKey: "customer", Id: entry.GetId(), ExpectedRevision: 1})
	requireCode(t, err, connect.CodeInternal)
	_, err = entries.CreateEntry(ctx, &athenav1.CreateEntryRequest{RecordKey: "customer", Values: map[string]*athenav1.FieldValue{"name": text("Second")}})
	requireCode(t, err, connect.CodeInternal)

	got, err := entries.GetEntry(ctx, &athenav1.GetEntryRequest{RecordKey: "customer", Id: entry.GetId()})
	if err != nil {
		t.Fatalf("entry should survive a failed delete: %v", err)
	}
	if got.GetEntry().GetRevision() != 1 || got.GetEntry().GetValues()["name"].GetStringValue() != "Acme" {
		t.Fatalf("a failed history write left a change behind: %v", got.GetEntry())
	}
	list, _ := entries.ListEntries(ctx, &athenav1.ListEntriesRequest{RecordKey: "customer"})
	if len(list.GetEntries()) != 1 {
		t.Fatalf("a failed create left %d entries", len(list.GetEntries()))
	}
}
