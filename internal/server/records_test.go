package server

import (
	"context"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"connectrpc.com/connect/v2"
)

func TestSaveRecordCreatesTableAndChecksRevision(t *testing.T) {
	ctx := context.Background()
	records, _ := newServices(t)
	revision := saveRecord(t, records, "customer", customerFields...)
	if revision != 1 {
		t.Fatalf("first revision = %d", revision)
	}
	var exists bool
	if err := records.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'athena_record_customer')`).Scan(&exists); err != nil || !exists {
		t.Fatalf("table was not provisioned: %v", err)
	}

	// A new Record whose key is taken is refused rather than overwriting it.
	_, err := records.SaveRecord(ctx, &athenav1.SaveRecordRequest{Record: &athenav1.Record{Key: "customer", Label: "Again", Fields: customerFields}})
	requireCode(t, err, connect.CodeAlreadyExists)

	// An additive save at the current revision succeeds and bumps it.
	withEmail := append([]*athenav1.RecordField{}, customerFields...)
	withEmail = append(withEmail, &athenav1.RecordField{Key: "email", Label: "Email", Type: "Email"})
	res, err := records.SaveRecord(ctx, &athenav1.SaveRecordRequest{Record: &athenav1.Record{Key: "customer", Label: "customer", Fields: withEmail}, ExpectedRevision: 1})
	if err != nil || res.GetRecord().GetRevision() != 2 {
		t.Fatalf("additive save: %v %v", res, err)
	}

	// A save from a stale tab still holding revision 1 is refused.
	_, err = records.SaveRecord(ctx, &athenav1.SaveRecordRequest{Record: &athenav1.Record{Key: "customer", Label: "customer", Fields: withEmail}, ExpectedRevision: 1})
	requireCode(t, err, connect.CodeAborted)
}

func TestSaveRecordRejectsInvalidDefinitions(t *testing.T) {
	ctx := context.Background()
	records, _ := newServices(t)
	cases := map[string]*athenav1.Record{
		"bad key":         {Key: "Bad Key", Label: "x"},
		"reserved field":  {Key: "thing", Label: "x", Fields: []*athenav1.RecordField{{Key: "id", Label: "Id", Type: "Text"}}},
		"unknown type":    {Key: "thing", Label: "x", Fields: []*athenav1.RecordField{{Key: "a", Label: "A", Type: "Blob"}}},
		"link to nothing": {Key: "thing", Label: "x", Fields: []*athenav1.RecordField{{Key: "a", Label: "A", Type: "Link", TargetRecordKey: "ghost"}}},
	}
	for name, record := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := records.SaveRecord(ctx, &athenav1.SaveRecordRequest{Record: record})
			requireCode(t, err, connect.CodeInvalidArgument)
		})
	}
}

func TestPlanAndSaveRequireConfirmationForFieldRemoval(t *testing.T) {
	ctx := context.Background()
	records, entries := newServices(t)
	saveRecord(t, records, "customer", customerFields...)
	// Removing a field only needs confirmation when it holds values.
	if _, err := entries.CreateEntry(ctx, &athenav1.CreateEntryRequest{RecordKey: "customer", Values: map[string]*athenav1.FieldValue{"name": text("Acme"), "balance": text("5")}}); err != nil {
		t.Fatal(err)
	}
	smaller := &athenav1.Record{Key: "customer", Label: "customer", Fields: customerFields[:1]}

	plan, err := records.PlanRecordSave(ctx, &athenav1.PlanRecordSaveRequest{Record: smaller, ExpectedRevision: 1})
	if err != nil || !plan.GetRequiresConfirmation() || len(plan.GetSteps()) != 1 || !plan.GetSteps()[0].GetDestructive() {
		t.Fatalf("plan = %v, %v", plan, err)
	}
	_, err = records.SaveRecord(ctx, &athenav1.SaveRecordRequest{Record: smaller, ExpectedRevision: 1})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if _, err := records.SaveRecord(ctx, &athenav1.SaveRecordRequest{Record: smaller, ExpectedRevision: 1, ConfirmDataLoss: true}); err != nil {
		t.Fatalf("confirmed removal: %v", err)
	}
}

func TestDeleteRecordGuards(t *testing.T) {
	ctx := context.Background()
	records, entries := newServices(t)
	saveRecord(t, records, "customer", customerFields...)
	linked := append([]*athenav1.RecordField{}, &athenav1.RecordField{Key: "customer", Label: "Customer", Type: "Link", TargetRecordKey: "customer"})
	saveRecord(t, records, "invoice", linked...)
	newCustomer(t, entries, "Acme")

	// Another Record links to customer.
	_, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{Key: "customer", ExpectedRevision: 1, DeleteEntries: true})
	requireCode(t, err, connect.CodeFailedPrecondition)

	// Stale revision.
	_, err = records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{Key: "invoice", ExpectedRevision: 9})
	requireCode(t, err, connect.CodeAborted)
	_, err = records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{Key: "ghost", ExpectedRevision: 1})
	requireCode(t, err, connect.CodeNotFound)

	// Entries need explicit confirmation.
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{Key: "invoice", ExpectedRevision: 1}); err != nil {
		t.Fatalf("delete an empty Record: %v", err)
	}
	_, err = records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{Key: "customer", ExpectedRevision: 1})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{Key: "customer", ExpectedRevision: 1, DeleteEntries: true}); err != nil {
		t.Fatalf("delete with entries: %v", err)
	}
	list, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{})
	if err != nil || len(list.GetRecords()) != 0 {
		t.Fatalf("records after delete: %v %v", list, err)
	}
}
