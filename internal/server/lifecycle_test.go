package server

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/lifecycle"

	"connectrpc.com/connect/v2"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// lifecycleFixture defines test-only Record Types: an amendable document with
// lines, and a second document that links to it.
func lifecycleFixture(t *testing.T, recordTypes recordTypeService) {
	t.Helper()
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "order", Label: "Order", Kind: "document", Class: "non_posting", Submittable: true, Amendable: true, Fields: []*athenav1.RecordTypeField{
		{Key: "title", Label: "Title", Type: "Text", Required: true},
		{Key: "reference", Label: "Reference", Type: "Text", Unique: true},
		{Key: "note", Label: "Note", Type: "Text", AllowOnSubmit: true},
		{Key: "secret_code", Label: "Secret code", Type: "Text", NoCopy: true},
	}})
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "order_line", Label: "Order line", Kind: "child", ParentRecordTypeKey: "order", Fields: []*athenav1.RecordTypeField{
		{Key: "item", Label: "Item", Type: "Text"},
		{Key: "remark", Label: "Remark", Type: "Text", AllowOnSubmit: true},
	}})
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "delivery", Label: "Delivery", Kind: "document", Class: "non_posting", Submittable: true, Fields: []*athenav1.RecordTypeField{
		{Key: "order", Label: "Order", Type: "Link", TargetRecordTypeKey: "order"},
	}})
}

func newOrder(t *testing.T, s recordService, title string) *athenav1.Record {
	t.Helper()
	record, err := create(t, s, "order", map[string]*athenav1.FieldValue{"title": text(title), "reference": text("R-" + title), "secret_code": text("shh")})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	return record
}

func submit(t *testing.T, s recordService, key string, record *athenav1.Record) *athenav1.Record {
	t.Helper()
	res, err := s.SubmitRecord(context.Background(), &athenav1.SubmitRecordRequest{RecordTypeKey: key, Id: record.GetId(), ExpectedRevision: record.GetRevision()})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return res.GetRecord()
}

func cancel(s recordService, key string, record *athenav1.Record) (*athenav1.Record, error) {
	res, err := s.CancelRecord(context.Background(), &athenav1.CancelRecordRequest{RecordTypeKey: key, Id: record.GetId(), ExpectedRevision: record.GetRevision()})
	if err != nil {
		return nil, err
	}
	return res.GetRecord(), nil
}

func update(s recordService, key string, record *athenav1.Record, values map[string]*athenav1.FieldValue) error {
	_, err := s.UpdateRecord(context.Background(), &athenav1.UpdateRecordRequest{RecordTypeKey: key, Id: record.GetId(), ExpectedRevision: record.GetRevision(), Values: values})
	return err
}

func TestSubmittedDocumentIsImmutableExceptAllowOnSubmitFields(t *testing.T) {
	recordTypes, records := newServices(t)
	lifecycleFixture(t, recordTypes)
	order := newOrder(t, records, "A")
	if order.GetDocstate() != int32(lifecycle.Draft) || order.GetName() != order.GetId() {
		t.Fatalf("new document = state %d name %q", order.GetDocstate(), order.GetName())
	}
	if err := update(records, "order", order, map[string]*athenav1.FieldValue{"title": text("A2")}); err != nil {
		t.Fatalf("a draft is editable: %v", err)
	}
	order, _ = records.load(context.Background(), mustType(t, records, "order"), order.GetId())
	order = submit(t, records, "order", order)
	if order.GetDocstate() != int32(lifecycle.Submitted) {
		t.Fatalf("state = %d", order.GetDocstate())
	}
	// Every field that is not allow_on_submit is rejected, one by one.
	for _, key := range []string{"title", "reference", "secret_code"} {
		requireCode(t, update(records, "order", order, map[string]*athenav1.FieldValue{key: text("changed")}), connect.CodeFailedPrecondition)
	}
	if err := update(records, "order", order, map[string]*athenav1.FieldValue{"note": text("late note")}); err != nil {
		t.Fatalf("an allow_on_submit field may change: %v", err)
	}
	activity, err := records.ListRecordActivity(context.Background(), &athenav1.ListRecordActivityRequest{RecordTypeKey: "order", Id: order.GetId()})
	if err != nil || activity.GetEvents()[0].GetEvent() != "edited_after_submit" || activity.GetEvents()[0].GetChangedFields()[0] != "note" {
		t.Fatalf("the edit after submit is not audited: %v %v", activity, err)
	}
	// A submitted document cannot be deleted.
	order, _ = records.load(context.Background(), mustType(t, records, "order"), order.GetId())
	_, err = records.DeleteRecord(context.Background(), &athenav1.DeleteRecordRequest{RecordTypeKey: "order", Id: order.GetId(), ExpectedRevision: order.GetRevision()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	// A cancelled document cannot change at all.
	cancelled, err := cancel(records, "order", order)
	if err != nil {
		t.Fatal(err)
	}
	requireCode(t, update(records, "order", cancelled, map[string]*athenav1.FieldValue{"note": text("x")}), connect.CodeFailedPrecondition)
}

func mustType(t *testing.T, s recordService, key string) recordTypeDefinition {
	t.Helper()
	recordType, err := s.recordType(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	return recordType
}

func TestLinesFollowTheDocumentState(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	lifecycleFixture(t, recordTypes)
	order := newOrder(t, records, "L")
	line, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "order_line", ParentId: order.GetId(), Values: map[string]*athenav1.FieldValue{"item": text("bolt")}})
	if err != nil {
		t.Fatal(err)
	}
	submit(t, records, "order", order)
	_, err = records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "order_line", ParentId: order.GetId(), Values: map[string]*athenav1.FieldValue{"item": text("nut")}})
	requireCode(t, err, connect.CodeFailedPrecondition)
	requireCode(t, update(records, "order_line", line.GetRecord(), map[string]*athenav1.FieldValue{"item": text("screw")}), connect.CodeFailedPrecondition)
	if err := update(records, "order_line", line.GetRecord(), map[string]*athenav1.FieldValue{"remark": text("ok")}); err != nil {
		t.Fatalf("allow_on_submit on a line: %v", err)
	}
	_, err = records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "order_line", Id: line.GetRecord().GetId(), ExpectedRevision: 2})
	requireCode(t, err, connect.CodeFailedPrecondition)
}

func TestCancelIsBlockedByALiveDownstreamDocument(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	lifecycleFixture(t, recordTypes)
	order := submit(t, records, "order", newOrder(t, records, "D"))
	delivery, err := create(t, records, "delivery", map[string]*athenav1.FieldValue{"order": text(order.GetId())})
	if err != nil {
		t.Fatal(err)
	}
	// A draft downstream document does not block the cancel; a submitted one does.
	delivery = submit(t, records, "delivery", delivery)
	_, err = cancel(records, "order", order)
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(err.Error(), "Delivery") {
		t.Fatalf("the message should name the blocking document type: %v", err)
	}
	if _, err := cancel(records, "delivery", delivery); err != nil {
		t.Fatal(err)
	}
	if _, err := cancel(records, "order", order); err != nil {
		t.Fatalf("cancel after the downstream document is cancelled: %v", err)
	}
	current, _ := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "order", Id: order.GetId()})
	if current.GetRecord().GetDocstate() != int32(lifecycle.Cancelled) {
		t.Fatalf("state = %d", current.GetRecord().GetDocstate())
	}
}

func TestAmendKeepsTheOriginalCancelledAndCreatesADraft(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	lifecycleFixture(t, recordTypes)
	order := newOrder(t, records, "M")
	if _, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "order_line", ParentId: order.GetId(), Values: map[string]*athenav1.FieldValue{"item": text("bolt")}}); err != nil {
		t.Fatal(err)
	}
	order, _ = records.load(ctx, mustType(t, records, "order"), order.GetId())
	// Amend needs a cancelled document.
	_, err := records.AmendRecord(ctx, &athenav1.AmendRecordRequest{RecordTypeKey: "order", Id: order.GetId(), ExpectedRevision: order.GetRevision()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	order = submit(t, records, "order", order)
	cancelled, err := cancel(records, "order", order)
	if err != nil {
		t.Fatal(err)
	}
	res, err := records.AmendRecord(ctx, &athenav1.AmendRecordRequest{RecordTypeKey: "order", Id: cancelled.GetId(), ExpectedRevision: cancelled.GetRevision()})
	if err != nil {
		t.Fatalf("amend: %v", err)
	}
	draft := res.GetRecord()
	if draft.GetDocstate() != int32(lifecycle.Draft) || draft.GetAmendedFrom() != order.GetId() || draft.GetName() != order.GetName()+"-1" {
		t.Fatalf("successor = state %d from %q name %q", draft.GetDocstate(), draft.GetAmendedFrom(), draft.GetName())
	}
	if draft.GetValues()["title"].GetStringValue() != "M" || !draft.GetValues()["secret_code"].GetNullValue() || !draft.GetValues()["reference"].GetNullValue() {
		t.Fatalf("copied values = %v", draft.GetValues())
	}
	lines, _ := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "order_line", ParentId: draft.GetId()})
	if len(lines.GetRecords()) != 1 || lines.GetRecords()[0].GetValues()["item"].GetStringValue() != "bolt" {
		t.Fatalf("lines were not copied: %v", lines)
	}
	original, _ := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "order", Id: order.GetId()})
	if original.GetRecord().GetDocstate() != int32(lifecycle.Cancelled) {
		t.Fatal("the original must stay cancelled")
	}
	// One successor at most; a second amend of the same original is refused.
	_, err = records.AmendRecord(ctx, &athenav1.AmendRecordRequest{RecordTypeKey: "order", Id: cancelled.GetId(), ExpectedRevision: cancelled.GetRevision()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	// Amending the amendment names it X-2.
	second := submit(t, records, "order", draft)
	secondCancelled, _ := cancel(records, "order", second)
	again, err := records.AmendRecord(ctx, &athenav1.AmendRecordRequest{RecordTypeKey: "order", Id: secondCancelled.GetId(), ExpectedRevision: secondCancelled.GetRevision()})
	if err != nil || again.GetRecord().GetName() != order.GetName()+"-2" {
		t.Fatalf("second amend = %v, %v", again.GetRecord().GetName(), err)
	}
	versions, _ := records.ListRecordVersions(ctx, &athenav1.ListRecordVersionsRequest{RecordTypeKey: "order", Id: order.GetId()})
	if len(versions.GetVersions()) != 2 || versions.GetVersions()[0].GetEvent() != "cancel" || versions.GetVersions()[1].GetEvent() != "submit" {
		t.Fatalf("versions of the original = %v", versions)
	}
	if !strings.Contains(versions.GetVersions()[0].GetDiff(), "docstate") || !strings.Contains(versions.GetVersions()[0].GetSnapshot(), "bolt") {
		t.Fatalf("version diff or snapshot is missing data: %v", versions.GetVersions()[0])
	}
	first, _ := records.ListRecordVersions(ctx, &athenav1.ListRecordVersionsRequest{RecordTypeKey: "order", Id: draft.GetId()})
	if first.GetVersions()[len(first.GetVersions())-1].GetEvent() != "amend" {
		t.Fatalf("the successor's first version should be the amend: %v", first)
	}
}

func TestOnlyDraftsCanBeTrashedAndFireOnTrash(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	lifecycleFixture(t, recordTypes)
	var trashed []string
	records.hooks = lifecycle.NewRegistry()
	records.hooks.On(lifecycle.OnTrash, "order", func(_ context.Context, e *lifecycle.Event) error {
		trashed = append(trashed, e.RecordID)
		return nil
	})
	order := newOrder(t, records, "T")
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "order", Id: order.GetId(), ExpectedRevision: order.GetRevision()}); err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 1 || trashed[0] != order.GetId() {
		t.Fatalf("on_trash calls = %v", trashed)
	}
	submitted := submit(t, records, "order", newOrder(t, records, "T2"))
	_, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "order", Id: submitted.GetId(), ExpectedRevision: submitted.GetRevision()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if len(trashed) != 1 {
		t.Fatal("on_trash fired for a submitted document")
	}
}

func TestEventsFireInTheFixedOrder(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	lifecycleFixture(t, recordTypes)
	var mu sync.Mutex
	var fired []lifecycle.Name
	records.hooks = lifecycle.NewRegistry()
	for _, name := range lifecycle.Order {
		records.hooks.On(name, "order", func(_ context.Context, e *lifecycle.Event) error {
			mu.Lock()
			defer mu.Unlock()
			fired = append(fired, e.Name)
			return nil
		})
	}
	order := newOrder(t, records, "E")
	order, _ = records.load(ctx, mustType(t, records, "order"), order.GetId())
	if err := update(records, "order", order, map[string]*athenav1.FieldValue{"title": text("E2")}); err != nil {
		t.Fatal(err)
	}
	order, _ = records.load(ctx, mustType(t, records, "order"), order.GetId())
	order = submit(t, records, "order", order)
	cancelled, _ := cancel(records, "order", order)
	if _, err := records.AmendRecord(ctx, &athenav1.AmendRecordRequest{RecordTypeKey: "order", Id: cancelled.GetId(), ExpectedRevision: cancelled.GetRevision()}); err != nil {
		t.Fatal(err)
	}
	var want []lifecycle.Name
	for _, action := range []lifecycle.Action{lifecycle.SaveNew, lifecycle.SaveEdit, lifecycle.Submit, lifecycle.Cancel, lifecycle.Amend} {
		for _, step := range lifecycle.Sequence(action) {
			want = append(want, step.Name)
		}
	}
	if strings.Join(names(fired), ",") != strings.Join(names(want), ",") {
		t.Fatalf("events fired\n got %v\nwant %v", fired, want)
	}
}

func names(list []lifecycle.Name) []string {
	out := make([]string, len(list))
	for i, name := range list {
		out[i] = string(name)
	}
	return out
}

func TestFailureInOnSubmitRollsBackStateAndVersions(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	lifecycleFixture(t, recordTypes)
	records.hooks = lifecycle.NewRegistry()
	records.hooks.On(lifecycle.OnSubmit, "order", func(context.Context, *lifecycle.Event) error { return errors.New("posting failed") })
	order := newOrder(t, records, "F")
	_, err := records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: "order", Id: order.GetId(), ExpectedRevision: order.GetRevision()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(err.Error(), "posting failed") {
		t.Fatalf("the hook's reason should reach the caller: %v", err)
	}
	after, _ := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "order", Id: order.GetId()})
	if after.GetRecord().GetDocstate() != int32(lifecycle.Draft) || after.GetRecord().GetRevision() != order.GetRevision() {
		t.Fatalf("state or revision changed despite the failure: %v", after.GetRecord())
	}
	for _, table := range []string{"athena_document_versions", "athena_activity"} {
		var rows int
		if err := records.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE record_id = ?", order.GetId()).Scan(&rows); err != nil || rows != 0 {
			t.Fatalf("%s kept %d rows after the rollback (%v)", table, rows, err)
		}
	}
	history, _ := records.ListRecordHistory(ctx, &athenav1.ListRecordHistoryRequest{RecordTypeKey: "order", Id: order.GetId()})
	if len(history.GetEvents()) != 1 {
		t.Fatalf("history = %v", history.GetEvents())
	}
}

func TestLifecycleRejectsStaleRevisionAndWrongState(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	lifecycleFixture(t, recordTypes)
	order := newOrder(t, records, "S")
	_, err := records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: "order", Id: order.GetId(), ExpectedRevision: 99})
	requireCode(t, err, connect.CodeAborted)
	_, err = cancel(records, "order", order)
	requireCode(t, err, connect.CodeFailedPrecondition)
	submitted := submit(t, records, "order", order)
	_, err = records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: "order", Id: submitted.GetId(), ExpectedRevision: order.GetRevision()})
	requireCode(t, err, connect.CodeAborted)
	_, err = records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: "order", Id: submitted.GetId(), ExpectedRevision: submitted.GetRevision()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "plain", Label: "Plain", Fields: []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text"}}})
	plain, _ := create(t, records, "plain", map[string]*athenav1.FieldValue{"title": text("p")})
	_, err = records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: "plain", Id: plain.GetId(), ExpectedRevision: 1})
	requireCode(t, err, connect.CodeFailedPrecondition)
}

func TestLifecycleDefinitionRules(t *testing.T) {
	recordTypes, records := newServices(t)
	field := []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text"}}
	for name, rt := range map[string]*athenav1.RecordType{
		"master":            {Key: "m", Label: "M", Kind: "master", Submittable: true, Fields: field},
		"work":              {Key: "w", Label: "W", Kind: "document", Class: "work", Submittable: true, Fields: field},
		"amend only":        {Key: "a", Label: "A", Kind: "document", Class: "posting", Amendable: true, Fields: field},
		"reserved name":     {Key: "n", Label: "N", Kind: "document", Class: "posting", Submittable: true, Fields: []*athenav1.RecordTypeField{{Key: "name", Label: "Name", Type: "Text"}}},
		"reserved docstate": {Key: "d", Label: "D", Kind: "master", Fields: []*athenav1.RecordTypeField{{Key: "docstate", Label: "S", Type: "Text"}}},
		"allow on master":   {Key: "o", Label: "O", Kind: "master", Fields: []*athenav1.RecordTypeField{{Key: "x", Label: "X", Type: "Text", AllowOnSubmit: true}}},
	} {
		requireCode(t, saveType(t, recordTypes, rt), connect.CodeInvalidArgument)
		_ = name
	}
	// A document cannot stop being submittable once a document is submitted.
	lifecycleFixture(t, recordTypes)
	submit(t, records, "delivery", mustDelivery(t, records))
	current := mustType(t, records, "delivery")
	update := recordTypeToProto(current, current.Revision)
	update.Submittable = false
	_, err := recordTypes.SaveRecordType(context.Background(), &athenav1.SaveRecordTypeRequest{RecordType: update, ExpectedRevision: current.Revision})
	requireCode(t, err, connect.CodeInvalidArgument)
}

func mustDelivery(t *testing.T, s recordService) *athenav1.Record {
	t.Helper()
	record, err := create(t, s, "delivery", map[string]*athenav1.FieldValue{})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// A Record Type that becomes submittable later keeps its Records as drafts.
func TestExistingRecordsBecomeDraftsWhenTheTypeBecomesSubmittable(t *testing.T) {
	recordTypes, records := newServices(t)
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "memo", Label: "Memo", Kind: "document", Class: "non_posting", Fields: []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text"}}})
	memo, err := create(t, records, "memo", map[string]*athenav1.FieldValue{"title": text("old")})
	if err != nil {
		t.Fatal(err)
	}
	current := mustType(t, records, "memo")
	next := recordTypeToProto(current, current.Revision)
	next.Submittable = true
	if _, err := recordTypes.SaveRecordType(context.Background(), &athenav1.SaveRecordTypeRequest{RecordType: next, ExpectedRevision: current.Revision}); err != nil {
		t.Fatal(err)
	}
	got, err := records.GetRecord(context.Background(), &athenav1.GetRecordRequest{RecordTypeKey: "memo", Id: memo.GetId()})
	if err != nil || got.GetRecord().GetDocstate() != 0 || got.GetRecord().GetName() != memo.GetId() {
		t.Fatalf("legacy record = %v, %v", got.GetRecord(), err)
	}
	submit(t, records, "memo", got.GetRecord())
}

func TestTransitionSpansCarryStatesAndNoValues(t *testing.T) {
	recordTypes, records := newServices(t)
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	lifecycleFixture(t, recordTypes)
	const secret = "s3cret-order-title"
	order := newOrder(t, records, secret)
	order = submit(t, records, "order", order)
	if _, err := cancel(records, "order", order); err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, span := range exporter.GetSpans() {
		for _, attribute := range span.Attributes {
			if strings.Contains(attribute.Value.Emit(), secret) {
				t.Fatalf("span %s attribute %s carries a value", span.Name, attribute.Key)
			}
			if attribute.Key == "athena.state_to" {
				found[span.Name] = attribute.Value.Emit()
			}
		}
	}
	if found["athena.submit_record"] != "submitted" || found["athena.cancel_record"] != "cancelled" {
		t.Fatalf("transition spans = %v", found)
	}
}
