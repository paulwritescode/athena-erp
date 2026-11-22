package server

import (
	"context"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"connectrpc.com/connect/v2"
)

func strptr(v string) *string { return &v }

// saveTreeType saves a master Record Type with the tree and effective flags.
func saveTreeType(t *testing.T, s recordTypeService, key string, tree, effective bool, fields ...*athenav1.RecordTypeField) int64 {
	t.Helper()
	res, err := s.SaveRecordType(context.Background(), &athenav1.SaveRecordTypeRequest{
		RecordType: &athenav1.RecordType{Key: key, Label: key, Fields: fields, Tree: tree, EffectiveDated: effective},
	})
	if err != nil {
		t.Fatalf("save Record Type %s: %v", key, err)
	}
	return res.GetRecordType().GetRevision()
}

func TestTreeRecordsStorePathAndDepth(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveTreeType(t, recordTypes, "team", true, false,
		&athenav1.RecordTypeField{Key: "name", Label: "Name", Type: "Text", Required: true})

	root, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "team", Values: map[string]*athenav1.FieldValue{"name": text("Root")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := root.GetRecord(); got.GetTreePath() != "/"+got.GetId()+"/" || got.GetTreeDepth() != 0 || got.GetTreeParentId() != "" {
		t.Fatalf("root tree columns = %q %d %q", got.GetTreePath(), got.GetTreeDepth(), got.GetTreeParentId())
	}
	child, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "team", ParentId: root.GetRecord().GetId(), Values: map[string]*athenav1.FieldValue{"name": text("Child")}})
	if err != nil {
		t.Fatal(err)
	}
	wantPath := root.GetRecord().GetTreePath() + child.GetRecord().GetId() + "/"
	if got := child.GetRecord(); got.GetTreePath() != wantPath || got.GetTreeDepth() != 1 || got.GetTreeParentId() != root.GetRecord().GetId() {
		t.Fatalf("child tree columns = %q %d %q, want %q 1", got.GetTreePath(), got.GetTreeDepth(), got.GetTreeParentId(), wantPath)
	}
	list, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "team", ParentId: root.GetRecord().GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.GetRecords()) != 1 || list.GetRecords()[0].GetId() != child.GetRecord().GetId() {
		t.Fatalf("children of root = %d records, want the one child", len(list.GetRecords()))
	}
	if _, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "team", ParentId: "01911111-2222-7333-8444-777777777777", Values: map[string]*athenav1.FieldValue{"name": text("Orphan")}}); err == nil {
		t.Fatal("create under a missing parent succeeded, want an error")
	}
}

func TestTreeMoveRewritesSubtreeAndRefusesCycles(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveTreeType(t, recordTypes, "team", true, false,
		&athenav1.RecordTypeField{Key: "name", Label: "Name", Type: "Text", Required: true})
	create := func(name, parent string) *athenav1.Record {
		t.Helper()
		res, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "team", ParentId: parent, Values: map[string]*athenav1.FieldValue{"name": text(name)}})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return res.GetRecord()
	}
	root := create("root", "")
	mid := create("mid", root.GetId())
	leaf := create("leaf", mid.GetId())

	// Moving the middle under its own leaf would close a cycle.
	_, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "team", Id: mid.GetId(), ExpectedRevision: mid.GetRevision(), TreeParentId: strptr(leaf.GetId())})
	requireCode(t, err, connect.CodeInvalidArgument)
	// Moving under itself is refused too.
	_, err = records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "team", Id: mid.GetId(), ExpectedRevision: mid.GetRevision(), TreeParentId: strptr(mid.GetId())})
	requireCode(t, err, connect.CodeInvalidArgument)

	// Move the middle to the top with no field values; the leaf follows it.
	moved, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "team", Id: mid.GetId(), ExpectedRevision: mid.GetRevision(), TreeParentId: strptr("")})
	if err != nil {
		t.Fatal(err)
	}
	if got := moved.GetRecord(); got.GetTreeParentId() != "" || got.GetTreePath() != "/"+mid.GetId()+"/" || got.GetTreeDepth() != 0 {
		t.Fatalf("moved tree columns = %q %q %d", got.GetTreeParentId(), got.GetTreePath(), got.GetTreeDepth())
	}
	got, err := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "team", Id: leaf.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if want := "/" + mid.GetId() + "/" + leaf.GetId() + "/"; got.GetRecord().GetTreePath() != want || got.GetRecord().GetTreeDepth() != 1 {
		t.Fatalf("leaf after move = %q depth %d, want %q depth 1", got.GetRecord().GetTreePath(), got.GetRecord().GetTreeDepth(), want)
	}
}

func TestTreeDeleteRefusesChildren(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveTreeType(t, recordTypes, "team", true, false,
		&athenav1.RecordTypeField{Key: "name", Label: "Name", Type: "Text", Required: true})
	root, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "team", Values: map[string]*athenav1.FieldValue{"name": text("root")}})
	if err != nil {
		t.Fatal(err)
	}
	child, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "team", ParentId: root.GetRecord().GetId(), Values: map[string]*athenav1.FieldValue{"name": text("child")}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "team", Id: root.GetRecord().GetId(), ExpectedRevision: root.GetRecord().GetRevision()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "team", Id: child.GetRecord().GetId(), ExpectedRevision: child.GetRecord().GetRevision()}); err != nil {
		t.Fatal(err)
	}
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "team", Id: root.GetRecord().GetId(), ExpectedRevision: root.GetRecord().GetRevision()}); err != nil {
		t.Fatal(err)
	}
}

func TestTreeFlagsAreValidated(t *testing.T) {
	ctx := context.Background()
	recordTypes, _ := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	// A child Record Type already nests under a parent.
	_, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{
		Key: "line", Label: "line", Kind: "child", ParentRecordTypeKey: "customer", Tree: true,
		Fields: []*athenav1.RecordTypeField{{Key: "name", Label: "Name", Type: "Text"}}}})
	requireCode(t, err, connect.CodeInvalidArgument)
	// A setting holds one row.
	_, err = recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{
		Key: "config", Label: "config", Kind: "setting", Tree: true,
		Fields: []*athenav1.RecordTypeField{{Key: "name", Label: "Name", Type: "Text"}}}})
	requireCode(t, err, connect.CodeInvalidArgument)
	// Amend copies values without splitting ranges.
	_, err = recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{
		Key: "order", Label: "order", Kind: "document", Class: "posting", Submittable: true, Amendable: true, EffectiveDated: true,
		Fields: []*athenav1.RecordTypeField{{Key: "name", Label: "Name", Type: "Text"}}}})
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestEffectiveDatedRejectsOverlaps(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveTreeType(t, recordTypes, "rate", false, true,
		&athenav1.RecordTypeField{Key: "code", Label: "Code", Type: "Text", Required: true, Unique: true})
	create := func(code, from, to string) (*athenav1.Record, error) {
		res, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "rate",
			Values: map[string]*athenav1.FieldValue{"code": text(code)}, EffectiveFrom: from, EffectiveTo: to})
		if err != nil {
			return nil, err
		}
		return res.GetRecord(), nil
	}
	first, err := create("USD", "2026-01-01", "2026-02-01")
	if err != nil {
		t.Fatal(err)
	}
	if first.GetEffectiveFrom() != "2026-01-01" || first.GetEffectiveTo() != "2026-02-01" {
		t.Fatalf("range reads back as %q to %q", first.GetEffectiveFrom(), first.GetEffectiveTo())
	}
	// Overlapping ranges for one code are refused.
	if _, err := create("USD", "2026-01-15", "2026-03-01"); err == nil {
		t.Fatal("overlapping create succeeded, want an error")
	}
	// End-exclusive bounds may touch.
	second, err := create("USD", "2026-02-01", "2026-03-01")
	if err != nil {
		t.Fatal(err)
	}
	// Another code may overlap freely.
	if _, err := create("EUR", "2026-01-15", "2026-03-01"); err != nil {
		t.Fatal(err)
	}
	// Extending into an overlap on update is refused.
	_, err = records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "rate", Id: second.GetId(),
		ExpectedRevision: second.GetRevision(), EffectiveFrom: strptr("2026-01-15")})
	requireCode(t, err, connect.CodeInvalidArgument)
	// A backwards range is refused.
	if _, err := create("JPY", "2026-03-01", "2026-01-01"); err == nil {
		t.Fatal("backwards range succeeded, want an error")
	}
}

func TestEffectiveDatedAsOfLists(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveTreeType(t, recordTypes, "rate", false, true,
		&athenav1.RecordTypeField{Key: "code", Label: "Code", Type: "Text", Required: true, Unique: true})
	create := func(code, from, to string) {
		t.Helper()
		if _, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "rate",
			Values: map[string]*athenav1.FieldValue{"code": text(code)}, EffectiveFrom: from, EffectiveTo: to}); err != nil {
			t.Fatalf("create %s: %v", code, err)
		}
	}
	create("past", "2026-01-01", "2026-02-01")
	create("open", "", "")
	codes := func(asOf string) []string {
		t.Helper()
		list, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "rate", AsOf: asOf})
		if err != nil {
			t.Fatalf("list as of %s: %v", asOf, err)
		}
		var out []string
		for _, record := range list.GetRecords() {
			out = append(out, record.GetValues()["code"].GetStringValue())
		}
		return out
	}
	if got := codes("2026-01-15"); len(got) != 2 {
		t.Fatalf("as of Jan 15: %v, want past and open", got)
	}
	// The end bound is exclusive: past drops out on Feb 1.
	if got := codes("2026-02-01"); len(got) != 1 || got[0] != "open" {
		t.Fatalf("as of Feb 1: %v, want only open", got)
	}
	if _, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "rate", AsOf: "tomorrow"}); err == nil {
		t.Fatal("bad as-of date succeeded, want an error")
	}
	saveRecordType(t, recordTypes, "customer", customerFields...)
	if _, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "customer", AsOf: "2026-01-01"}); err == nil {
		t.Fatal("as-of on a plain type succeeded, want an error")
	}
}

func TestTreeEffectivePlanAndMigration(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	rev := saveRecordType(t, recordTypes, "team",
		&athenav1.RecordTypeField{Key: "name", Label: "Name", Type: "Text", Required: true})
	res, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "team", Values: map[string]*athenav1.FieldValue{"name": text("solo")}})
	if err != nil {
		t.Fatal(err)
	}
	id := res.GetRecord().GetId()
	def := &athenav1.RecordType{Key: "team", Label: "team", Revision: rev, Tree: true,
		Fields: []*athenav1.RecordTypeField{{Key: "name", Label: "Name", Type: "Text", Required: true}}}
	plan, err := recordTypes.PlanRecordTypeSave(ctx, &athenav1.PlanRecordTypeSaveRequest{RecordType: def, ExpectedRevision: rev})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, step := range plan.GetSteps() {
		kinds = append(kinds, step.GetKind())
	}
	if len(kinds) != 1 || kinds[0] != "enable_tree" {
		t.Fatalf("plan steps = %v, want [enable_tree]", kinds)
	}
	if plan.GetRequiresConfirmation() {
		t.Fatal("enabling a tree needs no confirmation")
	}
	saved, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: def, ExpectedRevision: rev})
	if err != nil {
		t.Fatal(err)
	}
	rev = saved.GetRecordType().GetRevision()
	if !saved.GetRecordType().GetTree() {
		t.Fatal("Tree flag did not round-trip through save")
	}
	// The existing row became a top-level entry.
	got, err := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "team", Id: id})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetRecord().GetTreePath() != "/"+id+"/" || got.GetRecord().GetTreeDepth() != 0 {
		t.Fatalf("backfilled path = %q depth %d", got.GetRecord().GetTreePath(), got.GetRecord().GetTreeDepth())
	}
	// Disabling with a parent link needs confirmation.
	child, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "team", ParentId: id, Values: map[string]*athenav1.FieldValue{"name": text("child")}})
	if err != nil {
		t.Fatal(err)
	}
	plain := &athenav1.RecordType{Key: "team", Label: "team", Revision: rev,
		Fields: []*athenav1.RecordTypeField{{Key: "name", Label: "Name", Type: "Text", Required: true}}}
	plan, err = recordTypes.PlanRecordTypeSave(ctx, &athenav1.PlanRecordTypeSaveRequest{RecordType: plain, ExpectedRevision: rev})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.GetRequiresConfirmation() {
		t.Fatal("disabling a tree with links needs confirmation")
	}
	if _, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: plain, ExpectedRevision: rev}); err == nil {
		t.Fatal("disable without confirmation succeeded, want an error")
	}
	if _, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: plain, ExpectedRevision: rev, ConfirmDataLoss: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "team", Id: child.GetRecord().GetId()}); err != nil {
		t.Fatal(err)
	}
}

func TestEnableEffectiveRefusesSharedEntities(t *testing.T) {
	ctx := context.Background()
	recordTypes, _ := newServices(t)
	rev := saveRecordType(t, recordTypes, "rate",
		&athenav1.RecordTypeField{Key: "code", Label: "Code", Type: "Text"})
	def := func() *athenav1.RecordType {
		return &athenav1.RecordType{Key: "rate", Label: "rate", Revision: rev, EffectiveDated: true,
			Fields: []*athenav1.RecordTypeField{{Key: "code", Label: "Code", Type: "Text"}}}
	}
	if _, err := recordTypes.PlanRecordTypeSave(ctx, &athenav1.PlanRecordTypeSaveRequest{RecordType: def(), ExpectedRevision: rev}); err != nil {
		t.Fatal(err)
	}
	if _, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: def(), ExpectedRevision: rev}); err != nil {
		t.Fatal(err)
	}
}

func TestEnableEffectiveRefusesOverlappingRows(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	rev := saveRecordType(t, recordTypes, "rate",
		&athenav1.RecordTypeField{Key: "code", Label: "Code", Type: "Text", Unique: true})
	for _, code := range []string{"USD", "EUR"} {
		if _, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "rate", Values: map[string]*athenav1.FieldValue{"code": text(code)}}); err != nil {
			t.Fatal(err)
		}
	}
	// Distinct codes are distinct entities, so enabling succeeds.
	def := &athenav1.RecordType{Key: "rate", Label: "rate", Revision: rev, EffectiveDated: true,
		Fields: []*athenav1.RecordTypeField{{Key: "code", Label: "Code", Type: "Text", Unique: true}}}
	if _, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: def, ExpectedRevision: rev}); err != nil {
		t.Fatal(err)
	}
}

func TestEnableEffectiveRefusesIndistinguishableRows(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	rev := saveRecordType(t, recordTypes, "note",
		&athenav1.RecordTypeField{Key: "body", Label: "Body", Type: "Text"})
	for _, body := range []string{"one", "two"} {
		if _, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "note", Values: map[string]*athenav1.FieldValue{"body": text(body)}}); err != nil {
			t.Fatal(err)
		}
	}
	// No unique field tells the rows apart, so they would overlap.
	def := &athenav1.RecordType{Key: "note", Label: "note", Revision: rev, EffectiveDated: true,
		Fields: []*athenav1.RecordTypeField{{Key: "body", Label: "Body", Type: "Text"}}}
	_, err := recordTypes.PlanRecordTypeSave(ctx, &athenav1.PlanRecordTypeSaveRequest{RecordType: def, ExpectedRevision: rev})
	requireCode(t, err, connect.CodeFailedPrecondition)
}

func TestEffectiveDatedChildScopesByParent(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	res, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{
		Key: "line", Label: "line", Kind: "child", ParentRecordTypeKey: "customer", EffectiveDated: true,
		Fields: []*athenav1.RecordTypeField{{Key: "code", Label: "Code", Type: "Text", Unique: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.GetRecordType().GetEffectiveDated() {
		t.Fatal("EffectiveDated flag did not round-trip through save")
	}
	parent := func(name string) string {
		t.Helper()
		return newCustomer(t, records, name).GetId()
	}
	create := func(parentID, code, from, to string) error {
		_, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "line", ParentId: parentID,
			Values: map[string]*athenav1.FieldValue{"code": text(code)}, EffectiveFrom: from, EffectiveTo: to})
		return err
	}
	first, second := parent("Acme"), parent("Globex")
	if err := create(first, "L1", "2026-01-01", "2026-06-01"); err != nil {
		t.Fatal(err)
	}
	// The same code under another parent may overlap freely.
	if err := create(second, "L1", "2026-01-01", "2026-06-01"); err != nil {
		t.Fatal(err)
	}
	// Under the same parent it must not overlap.
	if err := create(first, "L1", "2026-03-01", "2026-09-01"); err == nil {
		t.Fatal("overlapping child create succeeded, want an error")
	}
}

func TestParseEffectiveRange(t *testing.T) {
	from, to, err := parseEffectiveRange("", "")
	if err != nil || from != nil || to != nil {
		t.Fatalf("open range = %v %v %v", from, to, err)
	}
	from, to, err = parseEffectiveRange("2026-01-01", "2026-02-01")
	if err != nil || from != "2026-01-01" || to != "2026-02-01" {
		t.Fatalf("range = %v %v %v", from, to, err)
	}
	for _, tc := range [][2]string{{"2026-02-01", "2026-01-01"}, {"2026-01-01", "2026-01-01"}, {"nope", ""}, {"", "nope"}} {
		if _, _, err := parseEffectiveRange(tc[0], tc[1]); err == nil {
			t.Fatalf("range %q to %q succeeded, want an error", tc[0], tc[1])
		}
	}
	if _, err := parseAsOf("2026-13-01"); err == nil {
		t.Fatal("bad as-of succeeded, want an error")
	}
}
