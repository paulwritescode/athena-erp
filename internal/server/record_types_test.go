package server

import (
	"context"
	"strings"
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

func pairParent(t *testing.T, recordTypes recordTypeService) *athenav1.RecordType {
	t.Helper()
	parent := &athenav1.RecordType{Key: "order", Label: "Order", Kind: "master", Fields: []*athenav1.RecordTypeField{{Key: "note", Label: "Note", Type: "Text"}}}
	saved, err := recordTypes.SaveRecordType(context.Background(), &athenav1.SaveRecordTypeRequest{RecordType: parent})
	if err != nil {
		t.Fatal(err)
	}
	parent.Revision = saved.GetRecordType().GetRevision()
	return parent
}

func groupStatus(t *testing.T, recordTypes recordTypeService) (string, string) {
	t.Helper()
	var status, lastStep string
	if err := recordTypes.db.QueryRowContext(context.Background(), `SELECT status, last_step FROM athena_record_type_operations WHERE action = 'save_group'`).Scan(&status, &lastStep); err != nil {
		t.Fatalf("group row: %v", err)
	}
	return status, lastStep
}

func TestPairRollsBackNewChildWhenParentFails(t *testing.T) {
	ctx := context.Background()
	recordTypes, _ := newServices(t)
	parent := pairParent(t, recordTypes)
	// An invalid field key passes the expression pre-check and fails the
	// parent prepare after the child has committed.
	parent.Fields = append(parent.Fields, &athenav1.RecordTypeField{Key: "Bad Key!", Label: "Bad", Type: "Text"})
	child := &athenav1.RecordType{Key: "line", Label: "Line", Kind: "child", ParentRecordTypeKey: "order", Fields: []*athenav1.RecordTypeField{{Key: "qty", Label: "Qty", Type: "Number"}}}
	_, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: child, Parent: parent})
	requireCode(t, err, connect.CodeInvalidArgument)
	if !strings.Contains(connectMessage(err), "rolled back") {
		t.Fatalf("error = %q, want the rollback", connectMessage(err))
	}
	if definition, _, _ := loadRecordType(ctx, recordTypes.db, "line"); definition != nil {
		t.Fatal("the child metadata survived the rollback")
	}
	if tableExists(t, recordTypes.db, "p_workspace__line") {
		t.Fatal("the child table survived the rollback")
	}
	if status, step := groupStatus(t, recordTypes); status != operationFailed || step != "compensated" {
		t.Fatalf("group = %s/%s", status, step)
	}
}

func TestPairRestoresUpdatedChildWhenParentFails(t *testing.T) {
	ctx := context.Background()
	recordTypes, _ := newServices(t)
	parent := pairParent(t, recordTypes)
	child := &athenav1.RecordType{Key: "line", Label: "Line", Kind: "child", ParentRecordTypeKey: "order", Fields: []*athenav1.RecordTypeField{{Key: "qty", Label: "Qty", Type: "Number"}, {Key: "oldfield", Label: "Old", Type: "Text"}}}
	if _, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: child}); err != nil {
		t.Fatal(err)
	}
	// The child update drops a field and adds one; the parent then fails.
	child.Fields = []*athenav1.RecordTypeField{{Key: "qty", Label: "Qty", Type: "Number"}, {Key: "newfield", Label: "New", Type: "Text"}}
	child.Revision = 1
	parent.Fields = append(parent.Fields, &athenav1.RecordTypeField{Key: "Bad Key!", Label: "Bad", Type: "Text"})
	_, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: child, ExpectedRevision: 1, ConfirmDataLoss: true, Parent: parent})
	requireCode(t, err, connect.CodeInvalidArgument)
	definition, revision, err := loadRecordType(ctx, recordTypes.db, "line")
	if err != nil || revision != 1 {
		t.Fatalf("child revision = %d, %v", revision, err)
	}
	names := map[string]bool{}
	for _, field := range definition.Fields {
		names[field.Name] = true
	}
	if !names["oldfield"] || names["newfield"] {
		t.Fatalf("child fields = %v, want the prior definition", names)
	}
	var dropped, added bool
	if err := recordTypes.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'p_workspace__line' AND column_name = 'oldfield')`).Scan(&dropped); err != nil {
		t.Fatal(err)
	}
	if err := recordTypes.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'p_workspace__line' AND column_name = 'newfield')`).Scan(&added); err != nil {
		t.Fatal(err)
	}
	if !dropped || added {
		t.Fatal("the child schema was not migrated back")
	}
}

func crashGroup(t *testing.T, recordTypes recordTypeService, child recordTypeDefinition, parentName string, parentExpected int64) saveGroup {
	t.Helper()
	ctx := context.Background()
	conn, err := recordTypes.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	prepared, err := prepareOne(ctx, conn, child, 0, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	group := saveGroup{Child: child, ChildExpected: 0, ChildPrevious: prepared.previous, ChildOp: "test-child-op", ParentName: parentName, ParentExpected: parentExpected, ParentOp: "test-parent-op"}
	if _, err := startGroup(ctx, conn, group); err != nil {
		t.Fatal(err)
	}
	if _, _, err := commitOne(ctx, conn, prepared, group.ChildOp); err != nil {
		t.Fatal(err)
	}
	return group
}

func TestGroupResumeRollsBackBetweenMembers(t *testing.T) {
	ctx := context.Background()
	recordTypes, _ := newServices(t)
	pairParent(t, recordTypes)
	child := recordTypeFromProto(&athenav1.RecordType{Key: "line", Label: "Line", Kind: "child", ParentRecordTypeKey: "order", Fields: []*athenav1.RecordTypeField{{Key: "qty", Label: "Qty", Type: "Number"}}})
	crashGroup(t, recordTypes, child, "order", 1)
	// The crash lands here: the child committed, the parent never started.
	if err := resumeOperations(ctx, recordTypes.db); err != nil {
		t.Fatal(err)
	}
	if definition, _, _ := loadRecordType(ctx, recordTypes.db, "line"); definition != nil {
		t.Fatal("the child survived the resume rollback")
	}
	if tableExists(t, recordTypes.db, "p_workspace__line") {
		t.Fatal("the child table survived the resume rollback")
	}
	if status, step := groupStatus(t, recordTypes); status != operationFailed || step != "compensated" {
		t.Fatalf("group = %s/%s", status, step)
	}
	var members int
	if err := recordTypes.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_record_type_operations WHERE id IN ('test-child-op', 'test-parent-op') AND status = 'failed'`).Scan(&members); err != nil || members != 1 {
		t.Fatalf("failed members = %d, %v (only the journaled child op exists)", members, err)
	}
}

func TestGroupResumeCompletesWhenBothLanded(t *testing.T) {
	ctx := context.Background()
	recordTypes, _ := newServices(t)
	parent := pairParent(t, recordTypes)
	child := recordTypeFromProto(&athenav1.RecordType{Key: "line", Label: "Line", Kind: "child", ParentRecordTypeKey: "order", Fields: []*athenav1.RecordTypeField{{Key: "qty", Label: "Qty", Type: "Number"}}})
	group := crashGroup(t, recordTypes, child, "order", 1)
	conn, err := recordTypes.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	prepared, err := prepareOne(ctx, conn, recordTypeFromProto(parent), 1, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := commitOne(ctx, conn, prepared, group.ParentOp); err != nil {
		t.Fatal(err)
	}
	// Both members landed; only the group mark is missing.
	if err := resumeOperations(ctx, recordTypes.db); err != nil {
		t.Fatal(err)
	}
	if status, step := groupStatus(t, recordTypes); status != operationCompleted || step != "completed" {
		t.Fatalf("group = %s/%s", status, step)
	}
	if _, revision, _ := loadRecordType(ctx, recordTypes.db, "line"); revision != 1 {
		t.Fatalf("child revision = %d", revision)
	}
	if _, revision, _ := loadRecordType(ctx, recordTypes.db, "order"); revision != 2 {
		t.Fatalf("parent revision = %d", revision)
	}
}

func TestGroupResumeRefusesWhenRecordsExist(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	pairParent(t, recordTypes)
	child := recordTypeFromProto(&athenav1.RecordType{Key: "line", Label: "Line", Kind: "child", ParentRecordTypeKey: "order", Fields: []*athenav1.RecordTypeField{{Key: "qty", Label: "Qty", Type: "Number"}}})
	crashGroup(t, recordTypes, child, "order", 1)
	created, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "order", Values: map[string]*athenav1.FieldValue{"note": text("n")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "line", ParentId: created.GetRecord().GetId(), Values: map[string]*athenav1.FieldValue{"qty": text("2")}}); err != nil {
		t.Fatal(err)
	}
	if err := resumeOperations(ctx, recordTypes.db); err != nil {
		t.Fatal(err)
	}
	if status, step := groupStatus(t, recordTypes); status != operationFailed || step != "compensation_failed" {
		t.Fatalf("group = %s/%s", status, step)
	}
	if _, revision, _ := loadRecordType(ctx, recordTypes.db, "line"); revision != 1 {
		t.Fatal("the child with Records was rolled back")
	}
}
