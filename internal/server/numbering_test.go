package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/lifecycle"

	"connectrpc.com/connect/v2"
)

// seriesFixture saves a submittable invoice with a gap-free series.
func seriesFixture(t *testing.T, recordTypes recordTypeService, mutate func(*athenav1.RecordType)) {
	t.Helper()
	recordType := &athenav1.RecordType{Key: "invoice", Label: "Invoice", Kind: "document", Class: "posting", Submittable: true, Amendable: true,
		NamingSeries: "INV-{yyyy}-{####}", GapFree: true, AllowManualName: true,
		Fields: []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text", Required: true}}}
	if mutate != nil {
		mutate(recordType)
	}
	mustSave(t, recordTypes, recordType)
}

func newInvoice(t *testing.T, s recordService, title string) *athenav1.Record {
	t.Helper()
	record, err := create(t, s, "invoice", map[string]*athenav1.FieldValue{"title": text(title)})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	return record
}

func seriesName(n int) string {
	return fmt.Sprintf("INV-%04d-%04d", time.Now().UTC().Year(), n)
}

func TestSeriesNamesAreAssignedAtSubmitAndStayPermanent(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	seriesFixture(t, recordTypes, nil)
	draft := newInvoice(t, records, "first")
	if draft.GetName() != draft.GetId() {
		t.Fatalf("a draft keeps its placeholder name, got %q", draft.GetName())
	}
	submitted := submit(t, records, "invoice", draft)
	if submitted.GetName() != seriesName(1) {
		t.Fatalf("name = %q, want %q", submitted.GetName(), seriesName(1))
	}
	// A rejected edit cannot rename it, and neither can a field named name.
	_, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "invoice", Id: submitted.GetId(), ExpectedRevision: submitted.GetRevision(), Values: map[string]*athenav1.FieldValue{"name": text("HACK")}})
	if err == nil {
		t.Fatal("a name cannot be changed through an update")
	}
	cancelled, err := cancel(records, "invoice", submitted)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.GetName() != seriesName(1) {
		t.Fatalf("cancel renamed the document to %q", cancelled.GetName())
	}
	amended, err := records.AmendRecord(ctx, &athenav1.AmendRecordRequest{RecordTypeKey: "invoice", Id: cancelled.GetId(), ExpectedRevision: cancelled.GetRevision()})
	if err != nil {
		t.Fatal(err)
	}
	if amended.GetRecord().GetName() != seriesName(1)+"-1" {
		t.Fatalf("amended name = %q", amended.GetRecord().GetName())
	}
	again := submit(t, records, "invoice", amended.GetRecord())
	if again.GetName() != seriesName(1)+"-1" {
		t.Fatalf("an amendment keeps its suffixed name on submit, got %q", again.GetName())
	}
	// The next new document continues the series, and the audit trail has the name change.
	next := submit(t, records, "invoice", newInvoice(t, records, "second"))
	if next.GetName() != seriesName(2) {
		t.Fatalf("next name = %q", next.GetName())
	}
	history, _ := records.ListRecordHistory(ctx, &athenav1.ListRecordHistoryRequest{RecordTypeKey: "invoice", Id: submitted.GetId()})
	var named bool
	for _, change := range history.GetEvents()[len(history.GetEvents())-2].GetChanges() {
		named = named || change.GetField() == "name" && change.GetNewValue() == seriesName(1)
	}
	if !named {
		t.Fatalf("the submit audit row must record the new name: %v", history.GetEvents())
	}
}

func TestGapFreeSeriesHasNoGapsUnderConcurrentAndFailedSubmits(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	seriesFixture(t, recordTypes, nil)
	var fail sync.Map
	records.hooks = lifecycle.NewRegistry()
	// Every third document is rejected after it has taken its number, so
	// the rollback has to give the number back.
	records.hooks.On(lifecycle.OnSubmit, "invoice", func(_ context.Context, e *lifecycle.Event) error {
		if _, ok := fail.Load(e.RecordID); ok {
			return errors.New("rejected")
		}
		return nil
	})
	const total = 18
	invoices := make([]*athenav1.Record, total)
	for i := range invoices {
		invoices[i] = newInvoice(t, records, fmt.Sprint("d", i))
		if i%3 == 2 {
			fail.Store(invoices[i].GetId(), true)
		}
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var names []string
	failed := 0
	for _, invoice := range invoices {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: "invoice", Id: invoice.GetId(), ExpectedRevision: invoice.GetRevision()})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				return
			}
			names = append(names, res.GetRecord().GetName())
		}()
	}
	wg.Wait()
	if failed != total/3 {
		t.Fatalf("%d submits failed, want %d", failed, total/3)
	}
	sort.Strings(names)
	for i, name := range names {
		if name != seriesName(i+1) {
			t.Fatalf("names = %v: number %d is missing or repeated", names, i+1)
		}
	}
	var last int64
	if err := records.db.QueryRowContext(ctx, "SELECT last_number FROM athena_number_counters").Scan(&last); err != nil || int(last) != len(names) {
		t.Fatalf("counter = %d, want %d (%v)", last, len(names), err)
	}
}

func TestSeriesWithoutGapFreeNeverBlocksButMaySkip(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	seriesFixture(t, recordTypes, func(r *athenav1.RecordType) { r.GapFree = false })
	first := submit(t, records, "invoice", newInvoice(t, records, "a"))
	if first.GetName() != seriesName(1) {
		t.Fatalf("first = %q", first.GetName())
	}
	records.hooks = lifecycle.NewRegistry()
	records.hooks.On(lifecycle.OnSubmit, "invoice", func(context.Context, *lifecycle.Event) error { return errors.New("rejected") })
	rejected := newInvoice(t, records, "b")
	if _, err := records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: "invoice", Id: rejected.GetId(), ExpectedRevision: 1}); err == nil {
		t.Fatal("the hook must reject")
	}
	records.hooks = lifecycle.NewRegistry()
	third := submit(t, records, "invoice", newInvoice(t, records, "c"))
	if third.GetName() != seriesName(3) {
		t.Fatalf("a non-gap-free series keeps the number a failed submit took; got %q", third.GetName())
	}
}

func TestManualNamesAreCheckedForUniquenessAndLogged(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	seriesFixture(t, recordTypes, nil)
	manual := func(name string) (*athenav1.CreateRecordResponse, error) {
		return records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "invoice", Name: name, Values: map[string]*athenav1.FieldValue{"title": text("old")}})
	}
	first, err := manual("LEGACY-0001")
	if err != nil || first.GetRecord().GetName() != "LEGACY-0001" {
		t.Fatalf("manual name = %v, %v", first, err)
	}
	_, err = manual("LEGACY-0001")
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = manual(first.GetRecord().GetId())
	requireCode(t, err, connect.CodeInvalidArgument)
	submitted := submit(t, records, "invoice", first.GetRecord())
	if submitted.GetName() != "LEGACY-0001" {
		t.Fatalf("a manual name must survive submit, got %q", submitted.GetName())
	}
	var overrides int
	if err := records.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM athena_audit_log WHERE action = 'name_override' AND record_id = ?", first.GetRecord().GetId()).Scan(&overrides); err != nil || overrides != 1 {
		t.Fatalf("name_override rows = %d (%v)", overrides, err)
	}
	// A type that does not allow overrides refuses them.
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "memo", Label: "Memo", Kind: "document", Class: "non_posting", Submittable: true, Fields: []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text"}}})
	_, err = records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "memo", Name: "X", Values: map[string]*athenav1.FieldValue{"title": text("t")}})
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestNamingSettingsAreValidated(t *testing.T) {
	recordTypes, _ := newServices(t)
	field := []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text"}}
	for name, recordType := range map[string]*athenav1.RecordType{
		"series on a master":       {Key: "a", Label: "A", Kind: "master", NamingSeries: "A-{###}", Fields: field},
		"series without a counter": {Key: "b", Label: "B", Kind: "document", Submittable: true, NamingSeries: "B-{yyyy}", Fields: field},
		"gap-free without series":  {Key: "c", Label: "C", Kind: "document", Submittable: true, GapFree: true, Fields: field},
		"manual on a master":       {Key: "d", Label: "D", Kind: "master", AllowManualName: true, Fields: field},
		"unknown label field":      {Key: "e", Label: "E", Kind: "master", LabelField: "nope", Fields: field},
	} {
		if err := saveType(t, recordTypes, recordType); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: err = %v, want InvalidArgument", name, err)
		}
	}
}
