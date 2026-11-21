package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/store"
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

func listTypes(t *testing.T, types recordTypeService) map[string]*athenav1.RecordType {
	t.Helper()
	listed, err := types.ListRecordTypes(context.Background(), &athenav1.ListRecordTypesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]*athenav1.RecordType{}
	for _, item := range listed.GetRecordTypes() {
		byKey[item.GetKey()] = item
	}
	return byKey
}

func requireRecent(t *testing.T, at string) {
	t.Helper()
	stamp, err := time.Parse(time.RFC3339, at)
	if err != nil {
		t.Fatalf("updated_at %q is not RFC 3339: %v", at, err)
	}
	if age := time.Since(stamp); age < 0 || age > 5*time.Minute {
		t.Fatalf("updated_at %q is not recent", at)
	}
}

func TestListRecordTypesShowsCountsAndLastSave(t *testing.T) {
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	saveRecordType(t, recordTypes, "order", &athenav1.RecordTypeField{Key: "title", Label: "Title", Type: "Text"})
	newCustomer(t, records, "Acme")
	newCustomer(t, records, "Globex")

	byKey := listTypes(t, recordTypes)
	customer := byKey["customer"]
	if customer.GetRecordCount() != 2 || customer.GetCountHidden() {
		t.Fatalf("customer count = %d, hidden = %v", customer.GetRecordCount(), customer.GetCountHidden())
	}
	if customer.GetUpdatedBy() != "Local administrator" {
		t.Fatalf("customer updated_by = %q", customer.GetUpdatedBy())
	}
	requireRecent(t, customer.GetUpdatedAt())
	order := byKey["order"]
	if order.GetRecordCount() != 0 || order.GetCountHidden() {
		t.Fatalf("order count = %d, hidden = %v", order.GetRecordCount(), order.GetCountHidden())
	}
	requireRecent(t, order.GetUpdatedAt())
	if customer.GetPackKey() != "" || order.GetPackKey() != "" {
		t.Fatal("pack_key must stay empty until C3 installs Packs")
	}
}

func TestListRecordTypesHidesCountsWithoutRead(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	f.notes(t)
	mustSave(t, f.types, &athenav1.RecordType{Key: "order", Label: "Order", Fields: []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text"}}})
	f.operatorNote(t, "hello")
	if _, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "order", Values: map[string]*athenav1.FieldValue{"title": text("first")}}); err != nil {
		t.Fatal(err)
	}

	f.role(t, "Builder", []string{auth.PermissionBuild})
	f.user(t, "bob", "Builder")
	bob := f.browser(t)
	bob.login(t, "bob")
	// A builder without read Grants sees every Record Type but no counts.
	listed, err := bob.types.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]*athenav1.RecordType{}
	for _, item := range listed.GetRecordTypes() {
		seen[item.GetKey()] = item
	}
	for _, key := range []string{"note", "order"} {
		item := seen[key]
		if item == nil || item.GetRecordCount() != 0 || !item.GetCountHidden() {
			t.Fatalf("%s = %+v, want hidden count", key, item)
		}
	}

	// A read Grant on one Record Type reveals only its count.
	f.role(t, "Reader", nil, grantOf("note", 0, "", auth.ActionRead))
	f.assign(t, "bob", "Builder", "Reader")
	bob.login(t, "bob")
	listed, err = bob.types.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	seen = map[string]*athenav1.RecordType{}
	for _, item := range listed.GetRecordTypes() {
		seen[item.GetKey()] = item
	}
	if note := seen["note"]; note.GetRecordCount() != 1 || note.GetCountHidden() {
		t.Fatalf("note count = %d, hidden = %v", note.GetRecordCount(), note.GetCountHidden())
	}
	if order := seen["order"]; order.GetRecordCount() != 0 || !order.GetCountHidden() {
		t.Fatalf("order count = %d, hidden = %v", order.GetRecordCount(), order.GetCountHidden())
	}

	// Without the build permission the unreadable Record Type is not listed
	// at all, and the readable one carries its count.
	f.user(t, "ana", "Reader")
	ana := f.browser(t)
	ana.login(t, "ana")
	listed, err = ana.types.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.GetRecordTypes()) != 1 || listed.GetRecordTypes()[0].GetKey() != "note" {
		t.Fatalf("ana sees %v", listed.GetRecordTypes())
	}
	if note := listed.GetRecordTypes()[0]; note.GetRecordCount() != 1 || note.GetCountHidden() {
		t.Fatalf("ana's note count = %d, hidden = %v", note.GetRecordCount(), note.GetCountHidden())
	}
}

func TestListRecordTypesShowsDisplayNameOfSaver(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	user, challenge, err := f.users.CreateUser(ctx, "ada", "Ada Lovelace", false, newRequestID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.users.CompleteChallenge(ctx, user.Username, auth.ChallengePurposeFirstLogin, challenge, testPassword, newRequestID()); err != nil {
		t.Fatal(err)
	}
	f.role(t, "Builder", []string{auth.PermissionBuild})
	f.assign(t, "ada", "Builder")
	ada := f.browser(t)
	ada.login(t, "ada")
	if _, err := ada.types.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "note", Label: "Note", Fields: noteFields}}); err != nil {
		t.Fatal(err)
	}
	listed, err := ada.types.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.GetRecordTypes()) != 1 {
		t.Fatalf("ada sees %v", listed.GetRecordTypes())
	}
	note := listed.GetRecordTypes()[0]
	if note.GetUpdatedBy() != "Ada Lovelace" {
		t.Fatalf("updated_by = %q", note.GetUpdatedBy())
	}
	requireRecent(t, note.GetUpdatedAt())
}

func listAs(t *testing.T, f *accessFixture, username string) map[string]*athenav1.RecordType {
	t.Helper()
	b := f.browser(t)
	b.login(t, username)
	listed, err := b.types.ListRecordTypes(context.Background(), &athenav1.ListRecordTypesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]*athenav1.RecordType{}
	for _, item := range listed.GetRecordTypes() {
		seen[item.GetKey()] = item
	}
	return seen
}

func TestListRecordTypesHidesCountsForScopedRead(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	f.notes(t)
	f.operatorNote(t, "hello")

	// A row condition restricts the read: the count stays hidden.
	f.role(t, "Owned", nil, grantOf("note", 0, "doc.owner == user.name", auth.ActionRead))
	f.user(t, "clara", "Owned")
	if note := listAs(t, f, "clara")["note"]; note == nil || note.GetRecordCount() != 0 || !note.GetCountHidden() {
		t.Fatalf("conditioned read: %+v, want hidden count", note)
	}

	// A scoped Assignment restricts the read the same way.
	f.role(t, "Reader", nil, grantOf("note", 0, "", auth.ActionRead))
	f.user(t, "dave")
	if _, err := f.users.SaveAssignment(ctx, "dave", auth.AssignmentInput{Role: "Reader", Scope: auth.Scope{"company": {"acme"}}}); err != nil {
		t.Fatal(err)
	}
	if note := listAs(t, f, "dave")["note"]; note == nil || note.GetRecordCount() != 0 || !note.GetCountHidden() {
		t.Fatalf("scoped read: %+v, want hidden count", note)
	}

	// A separate unrestricted Grant makes the count visible again.
	f.user(t, "erin")
	if _, err := f.users.SaveAssignment(ctx, "erin", auth.AssignmentInput{Role: "Reader", Scope: auth.Scope{"company": {"acme"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.users.SaveAssignment(ctx, "erin", auth.AssignmentInput{Role: "Reader"}); err != nil {
		t.Fatal(err)
	}
	if note := listAs(t, f, "erin")["note"]; note.GetRecordCount() != 1 || note.GetCountHidden() {
		t.Fatalf("unrestricted read: count = %d, hidden = %v", note.GetRecordCount(), note.GetCountHidden())
	}
}

func TestListShowsSurvivingActorAfterCompensatedPairSave(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	f.role(t, "Builder", []string{auth.PermissionBuild})
	for _, user := range [][2]string{{"ada", "Ada Lovelace"}, {"bob", "Bob Jones"}} {
		created, challenge, err := f.users.CreateUser(ctx, user[0], user[1], false, newRequestID())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.users.CompleteChallenge(ctx, created.Username, auth.ChallengePurposeFirstLogin, challenge, testPassword, newRequestID()); err != nil {
			t.Fatal(err)
		}
		f.assign(t, user[0], "Builder")
	}
	ada := f.browser(t)
	ada.login(t, "ada")
	if _, err := ada.types.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "order", Label: "Order", Kind: "master", Fields: []*athenav1.RecordTypeField{{Key: "note", Label: "Note", Type: "Text"}}}}); err != nil {
		t.Fatal(err)
	}
	child := &athenav1.RecordType{Key: "line", Label: "Line", Kind: "child", ParentRecordTypeKey: "order", Fields: []*athenav1.RecordTypeField{{Key: "qty", Label: "Qty", Type: "Number"}}}
	if _, err := ada.types.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: child}); err != nil {
		t.Fatal(err)
	}

	// Bob's pair save commits the child, then the parent fails and the child
	// is compensated.
	bob := f.browser(t)
	bob.login(t, "bob")
	bobChild := &athenav1.RecordType{Key: "line", Label: "Line", Kind: "child", ParentRecordTypeKey: "order", Fields: []*athenav1.RecordTypeField{{Key: "qty", Label: "Qty", Type: "Number"}, {Key: "note", Label: "Note", Type: "Text"}}}
	badParent := &athenav1.RecordType{Key: "order", Label: "Order", Kind: "master", Revision: 1, Fields: []*athenav1.RecordTypeField{{Key: "note", Label: "Note", Type: "Text"}, {Key: "Bad Key!", Label: "Bad", Type: "Text"}}}
	_, err := bob.types.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: bobChild, ExpectedRevision: 1, Parent: badParent})
	requireCode(t, err, connect.CodeInvalidArgument)
	if !strings.Contains(connectMessage(err), "rolled back") {
		t.Fatalf("error = %q, want the rollback", connectMessage(err))
	}

	listed, err := ada.types.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var line *athenav1.RecordType
	for _, item := range listed.GetRecordTypes() {
		if item.GetKey() == "line" {
			line = item
		}
	}
	if line.GetUpdatedBy() != "Ada Lovelace" || line.GetRevision() != 1 {
		t.Fatalf("line = %+v, want Ada's surviving revision 1", line)
	}
	var compensated int
	if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_record_type_operations WHERE record_type_name = 'line' AND status = 'compensated'`).Scan(&compensated); err != nil || compensated != 1 {
		t.Fatalf("compensated member saves = %d, %v", compensated, err)
	}
}

func TestListRecordTypesSkipsSoftDeletedRecords(t *testing.T) {
	recordTypes, records := newServices(t)
	ctx := context.Background()
	saveRecordType(t, recordTypes, "customer", customerFields...)
	record := newCustomer(t, records, "Acme")
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	customer := listTypes(t, recordTypes)["customer"]
	if customer.GetRecordCount() != 0 || customer.GetCountHidden() {
		t.Fatalf("count = %d, hidden = %v, want a visible 0", customer.GetRecordCount(), customer.GetCountHidden())
	}
}

func TestListRecordTypesCountsMissingTableAsZero(t *testing.T) {
	recordTypes, _ := newServices(t)
	ctx := context.Background()
	saveRecordType(t, recordTypes, "customer", customerFields...)
	if _, err := recordTypes.db.ExecContext(ctx, "DROP TABLE p_workspace__customer"); err != nil {
		t.Fatal(err)
	}
	customer := listTypes(t, recordTypes)["customer"]
	if customer.GetRecordCount() != 0 || customer.GetCountHidden() {
		t.Fatalf("count = %d, hidden = %v, want a visible 0", customer.GetRecordCount(), customer.GetCountHidden())
	}
}

func TestCountMemberSQLQuotesPerDialect(t *testing.T) {
	cases := []struct {
		dialect store.Dialect
		key     string
		kind    string
		want    string
	}{
		{store.MariaDB{}, "customer", "master", "SELECT ? AS name, COUNT(*) AS n FROM `p_workspace__customer` WHERE `deleted_at` IS NULL"},
		{store.MariaDB{}, "legacy", "", "SELECT ? AS name, COUNT(*) AS n FROM `p_workspace__legacy` WHERE `deleted_at` IS NULL"},
		{store.MariaDB{}, "invoice", "document", "SELECT ? AS name, COUNT(*) AS n FROM `p_workspace__invoice`"},
		{store.PostgreSQL{}, "customer", "master", `SELECT ? AS name, COUNT(*) AS n FROM "p_workspace__customer" WHERE "deleted_at" IS NULL`},
		{store.PostgreSQL{}, "invoice", "document", `SELECT ? AS name, COUNT(*) AS n FROM "p_workspace__invoice"`},
	}
	for _, tc := range cases {
		if got := countMemberSQL(tc.dialect, tc.key, tc.kind); got != tc.want {
			t.Errorf("countMemberSQL(%s, %q, %q) = %q, want %q", tc.dialect.Name(), tc.key, tc.kind, got, tc.want)
		}
	}
	pg := store.PostgreSQL{}
	union := countMemberSQL(pg, "customer", "master") + " UNION ALL " + countMemberSQL(pg, "invoice", "document")
	if got := pg.Rebind(union); !strings.Contains(got, "$1") || !strings.Contains(got, "$2") || strings.Contains(got, "?") {
		t.Errorf("rebound UNION = %q, want $1 and $2", got)
	}
}

func TestListIgnoresRolledBackChildSaveFromOldGroups(t *testing.T) {
	recordTypes, _ := newServices(t)
	ctx := context.Background()
	saveRecordType(t, recordTypes, "order", &athenav1.RecordTypeField{Key: "note", Label: "Note", Type: "Text"})
	if _, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "line", Label: "Line", Kind: "child", ParentRecordTypeKey: "order", Fields: []*athenav1.RecordTypeField{{Key: "qty", Label: "Qty", Type: "Number"}}}}); err != nil {
		t.Fatal(err)
	}
	// The pre-fix shape: a completed member save whose group never completed.
	// Its later timestamp and sorting-last ID would win the ranking without
	// the group-membership exclusion.
	ghostOp := "ffffffff-ffff-7fff-8000-ffffffffffff"
	group, err := json.Marshal(saveGroup{ChildOp: ghostOp, ParentOp: "ffffffff-ffff-7fff-8000-000000000000"})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id, name, action string
		definition       []byte
		options          any
		status, step     string
	}{
		{"ffffffff-0000-7000-8000-000000000000", "line", "save_group", group, nil, "failed", "compensated"},
		{ghostOp, "line", "save", []byte("{}"), `{"actor":"Ghost Editor"}`, "completed", "activated"},
	} {
		if _, err := recordTypes.db.ExecContext(ctx, `INSERT INTO athena_record_type_operations (id, record_type_name, action, expected_revision, definition, options, status, last_step) VALUES (?,?,?,1,?,?,?,?)`,
			row.id, row.name, row.action, row.definition, row.options, row.status, row.step); err != nil {
			t.Fatal(err)
		}
	}
	line := listTypes(t, recordTypes)["line"]
	if line.GetUpdatedBy() != "Local administrator" {
		t.Fatalf("updated_by = %q, want the surviving save's actor", line.GetUpdatedBy())
	}
}

func TestRetryOnMissingTableRetriesOnce(t *testing.T) {
	gone := errors.New("table is gone")
	calls := 0
	out, err := retryOnMissingTable(func(err error) bool { return errors.Is(err, gone) }, func() (string, error) {
		calls++
		if calls == 1 {
			return "", gone
		}
		return "counted", nil
	})
	if err != nil || out != "counted" || calls != 2 {
		t.Fatalf("retry = %q, %v after %d calls", out, err, calls)
	}
	calls = 0
	if _, err := retryOnMissingTable(func(err error) bool { return errors.Is(err, gone) }, func() (string, error) {
		calls++
		return "", gone
	}); err != gone || calls != 2 {
		t.Fatalf("a second miss must surface after 2 calls, got %v after %d", err, calls)
	}
	other := errors.New("locked")
	calls = 0
	if _, err := retryOnMissingTable(func(err error) bool { return errors.Is(err, gone) }, func() (string, error) {
		calls++
		return "", other
	}); err != other || calls != 1 {
		t.Fatalf("another error must not retry, got %v after %d calls", err, calls)
	}
}
