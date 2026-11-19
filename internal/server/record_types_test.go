package server

import (
	"context"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"connectrpc.com/connect/v2"
)

func TestSaveRecordTypeCreatesTableAndChecksRevision(t *testing.T) {
	ctx := context.Background()
	recordTypes, _ := newServices(t)
	revision := saveRecordType(t, recordTypes, "customer", customerFields...)
	if revision != 1 {
		t.Fatalf("first revision = %d", revision)
	}
	var exists bool
	if err := recordTypes.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'p_workspace__customer')`).Scan(&exists); err != nil || !exists {
		t.Fatalf("table was not provisioned: %v", err)
	}

	// A new Record Type whose key is taken is refused rather than overwriting it.
	_, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "customer", Label: "Again", Fields: customerFields}})
	requireCode(t, err, connect.CodeAlreadyExists)

	// An additive save at the current revision succeeds and bumps it.
	withEmail := append([]*athenav1.RecordTypeField{}, customerFields...)
	withEmail = append(withEmail, &athenav1.RecordTypeField{Key: "email", Label: "Email", Type: "Email"})
	res, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "customer", Label: "customer", Fields: withEmail}, ExpectedRevision: 1})
	if err != nil || res.GetRecordType().GetRevision() != 2 {
		t.Fatalf("additive save: %v %v", res, err)
	}

	// A save from a stale tab still holding revision 1 is refused.
	_, err = recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "customer", Label: "customer", Fields: withEmail}, ExpectedRevision: 1})
	requireCode(t, err, connect.CodeAborted)
}

func TestSaveRecordTypeRejectsInvalidDefinitions(t *testing.T) {
	ctx := context.Background()
	recordTypes, _ := newServices(t)
	cases := map[string]*athenav1.RecordType{
		"bad key":         {Key: "Bad Key", Label: "x"},
		"reserved field":  {Key: "thing", Label: "x", Fields: []*athenav1.RecordTypeField{{Key: "id", Label: "Id", Type: "Text"}}},
		"unknown type":    {Key: "thing", Label: "x", Fields: []*athenav1.RecordTypeField{{Key: "a", Label: "A", Type: "Blob"}}},
		"link to nothing": {Key: "thing", Label: "x", Fields: []*athenav1.RecordTypeField{{Key: "a", Label: "A", Type: "Link", TargetRecordTypeKey: "ghost"}}},
	}
	for name, recordType := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: recordType})
			requireCode(t, err, connect.CodeInvalidArgument)
		})
	}
}

func TestPlanAndSaveRequireConfirmationForFieldRemoval(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	// Removing a field only needs confirmation when it holds values.
	if _, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "customer", Values: map[string]*athenav1.FieldValue{"name": text("Acme"), "balance": text("5")}}); err != nil {
		t.Fatal(err)
	}
	smaller := &athenav1.RecordType{Key: "customer", Label: "customer", Fields: customerFields[:1]}

	plan, err := recordTypes.PlanRecordTypeSave(ctx, &athenav1.PlanRecordTypeSaveRequest{RecordType: smaller, ExpectedRevision: 1})
	if err != nil || !plan.GetRequiresConfirmation() || len(plan.GetSteps()) != 1 || !plan.GetSteps()[0].GetDestructive() {
		t.Fatalf("plan = %v, %v", plan, err)
	}
	_, err = recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: smaller, ExpectedRevision: 1})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if _, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: smaller, ExpectedRevision: 1, ConfirmDataLoss: true}); err != nil {
		t.Fatalf("confirmed removal: %v", err)
	}
}

func TestDeleteRecordTypeGuards(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	linked := append([]*athenav1.RecordTypeField{}, &athenav1.RecordTypeField{Key: "customer", Label: "Customer", Type: "Link", TargetRecordTypeKey: "customer"})
	saveRecordType(t, recordTypes, "invoice", linked...)
	newCustomer(t, records, "Acme")

	// Another Record Type links to customer.
	_, err := recordTypes.DeleteRecordType(ctx, &athenav1.DeleteRecordTypeRequest{Key: "customer", ExpectedRevision: 1, DeleteRecords: true})
	requireCode(t, err, connect.CodeFailedPrecondition)

	// Stale revision.
	_, err = recordTypes.DeleteRecordType(ctx, &athenav1.DeleteRecordTypeRequest{Key: "invoice", ExpectedRevision: 9})
	requireCode(t, err, connect.CodeAborted)
	_, err = recordTypes.DeleteRecordType(ctx, &athenav1.DeleteRecordTypeRequest{Key: "ghost", ExpectedRevision: 1})
	requireCode(t, err, connect.CodeNotFound)

	// Records need explicit confirmation.
	if _, err := recordTypes.DeleteRecordType(ctx, &athenav1.DeleteRecordTypeRequest{Key: "invoice", ExpectedRevision: 1}); err != nil {
		t.Fatalf("delete an empty Record Type: %v", err)
	}
	_, err = recordTypes.DeleteRecordType(ctx, &athenav1.DeleteRecordTypeRequest{Key: "customer", ExpectedRevision: 1})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if _, err := recordTypes.DeleteRecordType(ctx, &athenav1.DeleteRecordTypeRequest{Key: "customer", ExpectedRevision: 1, DeleteRecords: true}); err != nil {
		t.Fatalf("delete with records: %v", err)
	}
	list, err := recordTypes.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil || len(list.GetRecordTypes()) != 0 {
		t.Fatalf("record types after delete: %v %v", list, err)
	}
}
