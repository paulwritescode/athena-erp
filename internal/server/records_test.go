package server

import (
	"context"
	"fmt"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"connectrpc.com/connect/v2"
)

var customerFields = []*athenav1.RecordTypeField{
	{Key: "name", Label: "Name", Type: "Text", Required: true},
	{Key: "balance", Label: "Balance", Type: "Currency"},
}

func newCustomer(t *testing.T, s recordService, name string) *athenav1.Record {
	t.Helper()
	res, err := s.CreateRecord(context.Background(), &athenav1.CreateRecordRequest{RecordTypeKey: "customer", Values: map[string]*athenav1.FieldValue{"name": text(name)}})
	if err != nil {
		t.Fatalf("create record: %v", err)
	}
	return res.GetRecord()
}

func TestRecordCreateGetAndValidation(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)

	record := newCustomer(t, records, "Acme")
	if record.GetRevision() != 1 || record.GetValues()["name"].GetStringValue() != "Acme" {
		t.Fatalf("unexpected record: %v", record)
	}
	got, err := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "customer", Id: record.GetId()})
	if err != nil || got.GetRecord().GetId() != record.GetId() {
		t.Fatalf("get record: %v", err)
	}
	if !got.GetRecord().GetValues()["balance"].GetNullValue() {
		t.Fatal("an unset field reads back as null")
	}

	_, err = records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "customer", Values: map[string]*athenav1.FieldValue{}})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "customer", Values: map[string]*athenav1.FieldValue{"name": text("x"), "nope": text("y")}})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "missing", Values: map[string]*athenav1.FieldValue{}})
	requireCode(t, err, connect.CodeNotFound)
}

func TestRecordDecimalKeepsExactValue(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	res, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "customer", Values: map[string]*athenav1.FieldValue{"name": text("Acme"), "balance": text("0.100000")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.GetRecord().GetValues()["balance"].GetStringValue(); got != "0.1" {
		t.Fatalf("balance = %q, want 0.1", got)
	}
}

func TestRecordUpdateRejectsStaleRevision(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	record := newCustomer(t, records, "Acme")

	update := func(revision int64, name string) (*athenav1.UpdateRecordResponse, error) {
		return records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: revision, Values: map[string]*athenav1.FieldValue{"name": text(name)}})
	}
	first, err := update(1, "Acme Ltd")
	if err != nil || first.GetRecord().GetRevision() != 2 {
		t.Fatalf("first update: %v %v", first, err)
	}
	// A second writer still holding revision 1 must be refused and change nothing.
	_, err = update(1, "Other")
	requireCode(t, err, connect.CodeAborted)
	got, _ := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "customer", Id: record.GetId()})
	if got.GetRecord().GetValues()["name"].GetStringValue() != "Acme Ltd" || got.GetRecord().GetRevision() != 2 {
		t.Fatalf("a refused update changed the record: %v", got.GetRecord())
	}
}

func TestRecordUnchangedUpdateKeepsRevisionAndWritesNoHistory(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	record := newCustomer(t, records, "Acme")
	res, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"name": text("Acme")}})
	if err != nil || res.GetRecord().GetRevision() != 1 {
		t.Fatalf("unchanged update: %v %v", res, err)
	}
	history, err := records.ListRecordHistory(ctx, &athenav1.ListRecordHistoryRequest{RecordTypeKey: "customer", Id: record.GetId()})
	if err != nil || len(history.GetEvents()) != 1 || history.GetEvents()[0].GetAction() != "created" {
		t.Fatalf("history = %v, %v", history, err)
	}
}

func TestRecordDeleteRejectsStaleRevision(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	record := newCustomer(t, records, "Acme")

	_, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 7})
	requireCode(t, err, connect.CodeAborted)
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 1}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "customer", Id: record.GetId()})
	requireCode(t, err, connect.CodeNotFound)
	_, err = records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 1})
	requireCode(t, err, connect.CodeNotFound)
}

func TestRecordListPagesInIDOrderWithoutGaps(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	var created []string
	for i := range 5 {
		created = append(created, newCustomer(t, records, fmt.Sprintf("Customer %d", i)).GetId())
	}

	var listed []string
	token := ""
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("paging did not finish")
		}
		res, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "customer", PageSize: 2, PageToken: token})
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range res.GetRecords() {
			listed = append(listed, e.GetId())
		}
		if token = res.GetNextPageToken(); token == "" {
			break
		}
	}
	if fmt.Sprint(listed) != fmt.Sprint(created) {
		t.Fatalf("listed %v, want %v", listed, created)
	}

	// A token issued for one Record Type is refused for another.
	saveRecordType(t, recordTypes, "supplier", customerFields...)
	first, _ := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "customer", PageSize: 2})
	_, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "supplier", PageToken: first.GetNextPageToken()})
	requireCode(t, err, connect.CodeInvalidArgument)
}

func recordNames(records []*athenav1.Record) []string {
	out := make([]string, len(records))
	for i, record := range records {
		out[i] = record.GetValues()["name"].GetStringValue()
	}
	return out
}

func TestRecordListQueryMatchesLabelAndTextFields(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer",
		&athenav1.RecordTypeField{Key: "name", Label: "Name", Type: "Text", Required: true},
		&athenav1.RecordTypeField{Key: "notes", Label: "Notes", Type: "Long text"},
		&athenav1.RecordTypeField{Key: "email", Label: "Email", Type: "Email"},
		&athenav1.RecordTypeField{Key: "balance", Label: "Balance", Type: "Currency"},
	)
	for _, values := range []map[string]*athenav1.FieldValue{
		{"name": text("Acme"), "notes": text("likes apples"), "balance": text("100")},
		{"name": text("Bolt"), "email": text("bolt@example.com")},
		{"name": text("Acorn"), "notes": text("oak fan")},
		{"name": text("Zed 100%")},
		{"name": text("Zed 1_0")},
	} {
		if _, err := create(t, records, "customer", values); err != nil {
			t.Fatal(err)
		}
	}
	list := func(query string) []string {
		t.Helper()
		res, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "customer", Query: query})
		if err != nil {
			t.Fatalf("list query %q: %v", query, err)
		}
		return recordNames(res.GetRecords())
	}
	all := "[Acme Bolt Acorn Zed 100% Zed 1_0]"
	for query, want := range map[string]string{
		"ac":            "[Acme Acorn]", // the label, whatever the case
		"ACME":          "[Acme]",
		"apples":        "[Acme]", // a text field that is not the label
		"oak":           "[Acorn]",
		"example.com":   "[Bolt]",     // Email counts as a text field
		"100":           "[Zed 100%]", // the label matches, but the 100 balance must not
		"%":             "[Zed 100%]", // a typed % is literal
		"1_0":           "[Zed 1_0]",  // a typed _ is literal
		"":              all,          // empty means no filter
		"   ":           all,          // surrounding blanks are ignored
		"no-such-thing": "[]",
	} {
		if got := fmt.Sprint(list(query)); got != want {
			t.Fatalf("query %q = %v, want %v", query, got, want)
		}
	}

	// Paging still walks the whole filtered set without gaps.
	var paged []string
	token := ""
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("paging did not finish")
		}
		res, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "customer", Query: "zed", PageSize: 1, PageToken: token})
		if err != nil {
			t.Fatal(err)
		}
		paged = append(paged, recordNames(res.GetRecords())...)
		if token = res.GetNextPageToken(); token == "" {
			break
		}
	}
	if fmt.Sprint(paged) != fmt.Sprint(list("zed")) || fmt.Sprint(paged) != "[Zed 100% Zed 1_0]" {
		t.Fatalf("paged = %v", paged)
	}
}

func TestRecordListQuerySearchesTheLabelField(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	// The label field is a choice, not a text field: the label still matches.
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "ticket", Label: "Ticket", Kind: "master", LabelField: "state", Fields: []*athenav1.RecordTypeField{
		{Key: "name", Label: "Name", Type: "Text", Required: true},
		{Key: "state", Label: "State", Type: "Enum", EnumValues: []*athenav1.EnumValue{{Value: "open", Label: "Open"}, {Value: "closed", Label: "Closed"}}},
	}})
	if _, err := create(t, records, "ticket", map[string]*athenav1.FieldValue{"name": text("Crash on save"), "state": text("open")}); err != nil {
		t.Fatal(err)
	}
	if _, err := create(t, records, "ticket", map[string]*athenav1.FieldValue{"name": text("Typo in help"), "state": text("closed")}); err != nil {
		t.Fatal(err)
	}
	list := func(query string) []string {
		t.Helper()
		res, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "ticket", Query: query})
		if err != nil {
			t.Fatalf("list query %q: %v", query, err)
		}
		return recordNames(res.GetRecords())
	}
	if got := fmt.Sprint(list("OPEN")); got != "[Crash on save]" {
		t.Fatalf("label query = %v", got)
	}
	if got := fmt.Sprint(list("typo")); got != "[Typo in help]" {
		t.Fatalf("text query = %v", got)
	}
}

func TestRecordHistoryRecordsEachChange(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	record := newCustomer(t, records, "Acme")
	if _, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"name": text("Acme Ltd")}}); err != nil {
		t.Fatal(err)
	}
	history, err := records.ListRecordHistory(ctx, &athenav1.ListRecordHistoryRequest{RecordTypeKey: "customer", Id: record.GetId()})
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

// The change and its Audit Log row share one transaction: when the audit write
// fails, the change must not survive.
func TestRecordChangeAndHistoryShareATransaction(t *testing.T) {
	ctx := context.Background()
	// Isolated: this test drops the audit table to simulate a failing write.
	recordTypes, records := newIsolatedServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	record := newCustomer(t, records, "Acme")
	if _, err := records.db.ExecContext(ctx, "DROP TABLE athena_audit_log"); err != nil {
		t.Fatal(err)
	}

	_, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"name": text("Changed")}})
	requireCode(t, err, connect.CodeInternal)
	_, err = records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 1})
	requireCode(t, err, connect.CodeInternal)
	_, err = records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "customer", Values: map[string]*athenav1.FieldValue{"name": text("Second")}})
	requireCode(t, err, connect.CodeInternal)

	got, err := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "customer", Id: record.GetId()})
	if err != nil {
		t.Fatalf("record should survive a failed delete: %v", err)
	}
	if got.GetRecord().GetRevision() != 1 || got.GetRecord().GetValues()["name"].GetStringValue() != "Acme" {
		t.Fatalf("a failed audit write left a change behind: %v", got.GetRecord())
	}
	list, _ := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "customer"})
	if len(list.GetRecords()) != 1 {
		t.Fatalf("a failed create left %d records", len(list.GetRecords()))
	}
}
