package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"

	"connectrpc.com/connect/v2"
)

// scopedNotes saves a Record Type with a company field for Scope tests.
func (f *accessFixture) scopedNotes(t *testing.T) {
	t.Helper()
	mustSave(t, f.types, &athenav1.RecordType{Key: "snote", Label: "Scoped note", Fields: []*athenav1.RecordTypeField{
		{Key: "title", Label: "Title", Type: "Text", Required: true},
		{Key: "company", Label: "Company", Type: "Text"},
	}})
}

func (f *accessFixture) operatorScopedNote(t *testing.T, title, company string) *athenav1.Record {
	t.Helper()
	res, err := f.records.CreateRecord(context.Background(), &athenav1.CreateRecordRequest{RecordTypeKey: "snote",
		Values: map[string]*athenav1.FieldValue{"title": text(title), "company": text(company)}})
	if err != nil {
		t.Fatal(err)
	}
	return res.GetRecord()
}

func requireScope(t *testing.T, f *accessFixture, role string, dimensions ...string) {
	t.Helper()
	if dimensions == nil {
		dimensions = []string{}
	}
	payload, err := json.Marshal(dimensions)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE athena_roles SET requires_scope = ? WHERE name = ?`, payload, role); err != nil {
		t.Fatal(err)
	}
}

func listTitles(t *testing.T, b *session, recordType string) []string {
	t.Helper()
	res, err := b.records.ListRecords(context.Background(), &athenav1.ListRecordsRequest{RecordTypeKey: recordType})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range res.GetRecords() {
		out = append(out, r.GetValues()["title"].GetStringValue())
	}
	return out
}

func TestScopedAssignmentsFilterLists(t *testing.T) {
	f := newAccessFixture(t)
	f.scopedNotes(t)
	f.operatorScopedNote(t, "A1", "acme")
	f.operatorScopedNote(t, "A2", "acme")
	f.operatorScopedNote(t, "G1", "globex")
	f.role(t, "reader", nil, grantOf("snote", 0, "", auth.ActionRead))
	f.user(t, "clerk")
	ctx := context.Background()
	if _, err := f.users.SaveAssignment(ctx, "clerk", auth.AssignmentInput{Role: "reader", Scope: auth.Scope{"company": {"acme"}}}); err != nil {
		t.Fatal(err)
	}
	b := f.browser(t)
	b.login(t, "clerk")
	if got := listTitles(t, b, "snote"); len(got) != 2 {
		t.Fatalf("scoped list = %v, want the two acme notes", got)
	}
	g1, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "snote",
		Values: map[string]*athenav1.FieldValue{"title": text("G2"), "company": text("globex")}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "snote", Id: g1.GetRecord().GetId()})
	requireCode(t, err, connect.CodeNotFound)
}

func TestRequiredScopeDeniesAnUnscopedAssignment(t *testing.T) {
	f := newAccessFixture(t)
	f.scopedNotes(t)
	f.operatorScopedNote(t, "A1", "acme")
	f.role(t, "clerk", nil, grantOf("snote", 0, "", auth.ActionRead))
	requireScope(t, f, "clerk", "company")
	f.user(t, "clerk", "clerk")
	b := f.browser(t)
	b.login(t, "clerk")
	_, err := b.records.ListRecords(context.Background(), &athenav1.ListRecordsRequest{RecordTypeKey: "snote"})
	requireCode(t, err, connect.CodePermissionDenied)
	ctx := context.Background()
	if _, err := f.users.SaveAssignment(ctx, "clerk", auth.AssignmentInput{Role: "clerk", Scope: auth.Scope{"company": {"acme"}}}); err != nil {
		t.Fatal(err)
	}
	if got := listTitles(t, b, "snote"); len(got) != 1 || got[0] != "A1" {
		t.Fatalf("scoped list = %v, want [A1]", got)
	}
}

func TestDatedAssignmentsGateAccess(t *testing.T) {
	f := newAccessFixture(t)
	f.scopedNotes(t)
	f.operatorScopedNote(t, "A1", "acme")
	f.role(t, "reader", nil, grantOf("snote", 0, "", auth.ActionRead))
	f.user(t, "clerk")
	ctx := context.Background()
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	tomorrow := time.Now().UTC().AddDate(0, 0, 1)
	expired, err := f.users.SaveAssignment(ctx, "clerk", auth.AssignmentInput{Role: "reader", ExpiresAt: &yesterday})
	if err != nil {
		t.Fatal(err)
	}
	if expired.Active {
		t.Fatal("an expired Assignment is active")
	}
	b := f.browser(t)
	b.login(t, "clerk")
	_, err = b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "snote"})
	requireCode(t, err, connect.CodePermissionDenied)
	future, err := f.users.SaveAssignment(ctx, "clerk", auth.AssignmentInput{Role: "reader", ActiveFrom: &tomorrow})
	if err != nil {
		t.Fatal(err)
	}
	if future.Active {
		t.Fatal("an Assignment starting tomorrow is active")
	}
	_, err = b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "snote"})
	requireCode(t, err, connect.CodePermissionDenied)
	// A stale revision loses; the current one extends the Assignment to today.
	if _, err := f.users.SaveAssignment(ctx, "clerk", auth.AssignmentInput{ID: expired.ID, Role: "reader", ExpectedRevision: expired.Revision + 1}); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("a stale revision saved: %v", err)
	}
	revived, err := f.users.SaveAssignment(ctx, "clerk", auth.AssignmentInput{ID: expired.ID, Role: "reader", ExpectedRevision: expired.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if !revived.Active || revived.Revision != expired.Revision+1 {
		t.Fatalf("revived Assignment = %+v", revived)
	}
	if got := listTitles(t, b, "snote"); len(got) != 1 {
		t.Fatalf("list after reviving = %v", got)
	}
	if err := f.users.RevokeAssignment(ctx, expired.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.users.RevokeAssignment(ctx, future.ID); err != nil {
		t.Fatal(err)
	}
	_, err = b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "snote"})
	requireCode(t, err, connect.CodePermissionDenied)
}

func TestUpdateRechecksBothRows(t *testing.T) {
	f := newAccessFixture(t)
	f.scopedNotes(t)
	note := f.operatorScopedNote(t, "A1", "acme")
	f.role(t, "writer", nil, grantOf("snote", 0, "", auth.ActionRead, auth.ActionWrite))
	f.user(t, "clerk")
	ctx := context.Background()
	if _, err := f.users.SaveAssignment(ctx, "clerk", auth.AssignmentInput{Role: "writer", Scope: auth.Scope{"company": {"acme"}}}); err != nil {
		t.Fatal(err)
	}
	b := f.browser(t)
	b.login(t, "clerk")
	if _, err := b.records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "snote", Id: note.GetId(), ExpectedRevision: 1,
		Values: map[string]*athenav1.FieldValue{"title": text("A1 renamed")}}); err != nil {
		t.Fatalf("an update inside the Scope was refused: %v", err)
	}
	_, err := b.records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "snote", Id: note.GetId(), ExpectedRevision: 2,
		Values: map[string]*athenav1.FieldValue{"company": text("globex")}})
	requireCode(t, err, connect.CodePermissionDenied)
	current, err := f.records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "snote", Id: note.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if got := current.GetRecord().GetValues()["company"].GetStringValue(); got != "acme" {
		t.Fatalf("the refused move landed: company = %q", got)
	}
	// The other direction: a row moved out of Scope by the operator can no
	// longer be touched.
	if _, err := f.records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "snote", Id: note.GetId(), ExpectedRevision: 2,
		Values: map[string]*athenav1.FieldValue{"company": text("globex")}}); err != nil {
		t.Fatal(err)
	}
	_, err = b.records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "snote", Id: note.GetId(), ExpectedRevision: 3,
		Values: map[string]*athenav1.FieldValue{"title": text("takeover")}})
	requireCode(t, err, connect.CodeNotFound)
}

func TestWholeAssignmentsDoNotCombineAcrossScopes(t *testing.T) {
	f := newAccessFixture(t)
	f.scopedNotes(t)
	north := f.operatorScopedNote(t, "N1", "north")
	south := f.operatorScopedNote(t, "S1", "south")
	f.role(t, "ro", nil, grantOf("snote", 0, "", auth.ActionRead))
	f.role(t, "rw", nil, grantOf("snote", 0, "", auth.ActionRead, auth.ActionWrite))
	requireScope(t, f, "ro", "company")
	requireScope(t, f, "rw", "company")
	f.user(t, "clerk")
	ctx := context.Background()
	// Read on north plus read and write on south: the clerk reads both but
	// writes only in the south.
	if _, err := f.users.SaveAssignment(ctx, "clerk", auth.AssignmentInput{Role: "ro", Scope: auth.Scope{"company": {"north"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.users.SaveAssignment(ctx, "clerk", auth.AssignmentInput{Role: "rw", Scope: auth.Scope{"company": {"south"}}}); err != nil {
		t.Fatal(err)
	}
	b := f.browser(t)
	b.login(t, "clerk")
	if got := listTitles(t, b, "snote"); len(got) != 2 {
		t.Fatalf("read union = %v, want both notes", got)
	}
	if _, err := b.records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "snote", Id: south.GetId(), ExpectedRevision: 1,
		Values: map[string]*athenav1.FieldValue{"title": text("S1 south")}}); err != nil {
		t.Fatalf("a write inside its Scope was refused: %v", err)
	}
	_, err := b.records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "snote", Id: north.GetId(), ExpectedRevision: 1,
		Values: map[string]*athenav1.FieldValue{"title": text("N1 north")}})
	requireCode(t, err, connect.CodePermissionDenied)
}
