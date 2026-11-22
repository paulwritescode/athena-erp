package server

import (
	"context"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"

	"connectrpc.com/connect/v2"
)

// Root administrator service tests (ADR-0018, task B8): the gate bypass
// over HTTP, the mutation refusals over the API, and audit attribution.
// The auth package covers seeding, sign-in and evaluation; the CLI tests
// cover the operator paths.

// TestRootPassesEveryGateWithoutAssignments signs in as root over HTTP and
// calls one procedure of each gate kind with no assignments at all.
func TestRootPassesEveryGateWithoutAssignments(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	if _, err := f.users.EnsureRoot(ctx, testPassword); err != nil {
		t.Fatal(err)
	}
	f.notes(t)
	b := f.browser(t)
	b.login(t, "administrator")
	// System gates: access management, build, audit.
	if _, err := b.auth.ListUsers(ctx, &athenav1.ListUsersRequest{}); err != nil {
		t.Fatalf("root ListUsers: %v", err)
	}
	if _, err := b.auth.ListAccessLog(ctx, &athenav1.ListAccessLogRequest{PageSize: 10}); err != nil {
		t.Fatalf("root ListAccessLog: %v", err)
	}
	if _, err := b.types.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "memo", Label: "Memo", Fields: noteFields}}); err != nil {
		t.Fatalf("root SaveRecordType: %v", err)
	}
	// Record gates with no Grant: create, list, read.
	created, err := b.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "note", Values: map[string]*athenav1.FieldValue{"title": text("root note"), "salary": text("100")}})
	if err != nil {
		t.Fatalf("root CreateRecord: %v", err)
	}
	listed, err := b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	if err != nil || len(listed.GetRecords()) != 1 {
		t.Fatalf("root ListRecords = %v, %v", listed, err)
	}
	got, err := b.records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "note", Id: created.GetRecord().GetId()})
	if err != nil {
		t.Fatalf("root GetRecord: %v", err)
	}
	// Field level is the maximum: the level-2 salary stays visible.
	if got.GetRecord().GetValues()["salary"].GetStringValue() != "100" {
		t.Fatalf("root values = %v", got.GetRecord().GetValues())
	}
	types, err := b.types.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil || len(types.GetRecordTypes()) != 2 {
		t.Fatalf("root ListRecordTypes = %v, %v", types, err)
	}
	for _, recordType := range types.GetRecordTypes() {
		if len(recordType.GetFields()) != 2 || recordType.GetAccess().GetPermissionLevel() != auth.MaxPermissionLevel {
			t.Fatalf("root sees %d fields at level %d on %s", len(recordType.GetFields()), recordType.GetAccess().GetPermissionLevel(), recordType.GetKey())
		}
	}
	if denials := f.accessEvents(t, auth.EventDenied); len(denials) != 0 {
		t.Fatalf("denials = %v, want none", denials)
	}
}

// TestRootMutationsAreRefusedOverTheAPI calls every user-mutating procedure
// against the root account: suspending, editing, role changes and inviting
// the reserved name all fail with a plain invalid-argument error.
func TestRootMutationsAreRefusedOverTheAPI(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	if _, err := f.users.EnsureRoot(ctx, testPassword); err != nil {
		t.Fatal(err)
	}
	f.role(t, "Clerk", nil, grantOf("note", 0, "", auth.ActionRead))
	b := f.browser(t)
	b.login(t, "administrator")
	name := "Renamed Root"
	disabled := true
	calls := []struct {
		action string
		phrase string
		call   func() error
	}{
		{"edit", "cannot be edited", func() error {
			_, err := b.auth.UpdateUser(ctx, &athenav1.UpdateUserRequest{Username: "administrator", DisplayName: &name})
			return err
		}},
		{"suspend", "cannot be suspended", func() error {
			_, err := b.auth.UpdateUser(ctx, &athenav1.UpdateUserRequest{Username: "administrator", Disabled: &disabled})
			return err
		}},
		{"roles", "cannot be given Roles", func() error {
			_, err := b.auth.UpdateUser(ctx, &athenav1.UpdateUserRequest{Username: "administrator", Roles: &athenav1.RoleNames{Names: []string{"Clerk"}}})
			return err
		}},
		{"assign", "cannot be given Roles", func() error {
			_, err := b.auth.SaveAssignment(ctx, &athenav1.SaveAssignmentRequest{Username: "administrator", Assignment: &athenav1.Assignment{Role: "Clerk"}})
			return err
		}},
		{"invite", "reserved for the workspace root", func() error {
			_, err := b.auth.InviteUser(ctx, &athenav1.InviteUserRequest{Username: "administrator"})
			return err
		}},
	}
	for _, c := range calls {
		err := c.call()
		requireCode(t, err, connect.CodeInvalidArgument)
		if !strings.Contains(connectMessage(err), c.phrase) {
			t.Fatalf("%s root: message %q misses %q", c.action, connectMessage(err), c.phrase)
		}
	}
	// The refusals changed nothing: the name is fixed, no assignment
	// landed, and the account still signs in.
	users, err := b.auth.ListUsers(ctx, &athenav1.ListUsersRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(users.GetUsers()) != 1 || users.GetUsers()[0].GetDisplayName() != "Administrator" {
		t.Fatalf("users = %v", users.GetUsers())
	}
	assignments, err := b.auth.ListAssignments(ctx, &athenav1.ListAssignmentsRequest{Username: "administrator"})
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments.GetAssignments()) != 0 {
		t.Fatalf("root assignments = %v", assignments.GetAssignments())
	}
	fresh := f.browser(t)
	fresh.login(t, "administrator")
}

// TestRootActionsAreLoggedAsRoot performs a sign-in, an invite and a Record
// write as root and checks each audit surface names the root account.
func TestRootActionsAreLoggedAsRoot(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	if _, err := f.users.EnsureRoot(ctx, testPassword); err != nil {
		t.Fatal(err)
	}
	b := f.browser(t)
	b.login(t, "administrator")
	if _, err := b.auth.InviteUser(ctx, &athenav1.InviteUserRequest{Username: "amy", DisplayName: "Amy"}); err != nil {
		t.Fatal(err)
	}
	logins := f.accessEvents(t, auth.EventLogin)
	if len(logins) != 1 || logins[0].Username != "administrator" {
		t.Fatalf("login rows = %+v", logins)
	}
	// Inviting logs the new account with the acting root as the user; the
	// seeding row from EnsureRoot is the other user_created row.
	var invite *auth.AccessRow
	for _, row := range f.accessEvents(t, auth.EventUserCreated) {
		if row.Detail == "user amy" {
			invite = &row
		}
	}
	if invite == nil || invite.Username != "administrator" {
		t.Fatalf("invite row = %+v", invite)
	}
	f.notes(t)
	created, err := b.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "note", Values: map[string]*athenav1.FieldValue{"title": text("root note"), "salary": text("100")}})
	if err != nil {
		t.Fatal(err)
	}
	if created.GetRecord().GetCreatedBy() != "administrator" || created.GetRecord().GetModifiedBy() != "administrator" {
		t.Fatalf("record actors = %q, %q", created.GetRecord().GetCreatedBy(), created.GetRecord().GetModifiedBy())
	}
	updated, err := b.records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "note", Id: created.GetRecord().GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"title": text("root note edited")}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetRecord().GetModifiedBy() != "administrator" {
		t.Fatalf("modified by = %q", updated.GetRecord().GetModifiedBy())
	}
	var actor string
	if err := f.db.QueryRow("SELECT actor FROM athena_audit_log ORDER BY seq DESC LIMIT 1").Scan(&actor); err != nil || actor != "administrator" {
		t.Fatalf("audit actor = %q, %v", actor, err)
	}
}
