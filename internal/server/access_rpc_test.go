package server

import (
	"context"
	"testing"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"

	"connectrpc.com/connect/v2"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ownerFixtureUser bootstraps an Instance Owner through the service layer,
// which fixtures must seed explicitly because they bypass startup.
func (f *accessFixture) owner(t *testing.T, username string) {
	t.Helper()
	ctx := context.Background()
	if err := f.users.EnsureSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	_, challenge, err := f.users.BootstrapOwner(ctx, username, "", newRequestID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.users.CompleteChallenge(ctx, username, auth.ChallengePurposeFirstLogin, challenge, testPassword, newRequestID()); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordFieldsAreReserved(t *testing.T) {
	reserved := func(message protoreflect.Message, field protoreflect.FieldNumber, name protoreflect.Name) {
		t.Helper()
		descriptor := message.Descriptor()
		if !descriptor.ReservedNames().Has(name) || !descriptor.ReservedRanges().Has(field) {
			t.Fatalf("%s does not reserve field %d %q", descriptor.FullName(), field, name)
		}
	}
	reserved((&athenav1.CreateUserRequest{}).ProtoReflect(), 3, "password")
	reserved((&athenav1.ResetPasswordRequest{}).ProtoReflect(), 2, "new_password")
}

func TestInviteCompleteLoginFlow(t *testing.T) {
	f := newAccessFixture(t)
	f.owner(t, "ada")
	owner := f.browser(t)
	owner.login(t, "ada")
	ctx := context.Background()
	invited, err := owner.auth.InviteUser(ctx, &athenav1.InviteUserRequest{Username: "clerk", DisplayName: "A Clerk"})
	if err != nil {
		t.Fatal(err)
	}
	if invited.GetChallenge() == "" || invited.GetUser().GetState() != auth.UserStateInvited {
		t.Fatalf("invite = %+v", invited)
	}
	if _, err := owner.auth.InviteUser(ctx, &athenav1.InviteUserRequest{Username: "clerk"}); err == nil {
		t.Fatal("a duplicate invite was accepted")
	}
	clerk := f.browser(t)
	completed, err := clerk.auth.CompleteChallenge(ctx, &athenav1.CompleteChallengeRequest{Username: "clerk", Purpose: auth.ChallengePurposeFirstLogin, Challenge: invited.GetChallenge(), NewPassword: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	if !completed.GetSession().GetAuthenticated() || clerk.cookie() == "" {
		t.Fatal("completing the invite did not sign in")
	}
	// No Grants: authenticated but denied, not signed out.
	_, err = clerk.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	requireCode(t, err, connect.CodePermissionDenied)
	_, err = clerk.auth.CompleteChallenge(ctx, &athenav1.CompleteChallengeRequest{Username: "clerk", Purpose: auth.ChallengePurposeFirstLogin, Challenge: "wrong", NewPassword: testPassword})
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestResendInviteSupersedes(t *testing.T) {
	f := newAccessFixture(t)
	f.owner(t, "ada")
	owner := f.browser(t)
	owner.login(t, "ada")
	ctx := context.Background()
	first, err := owner.auth.InviteUser(ctx, &athenav1.InviteUserRequest{Username: "clerk"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := owner.auth.ResendInvite(ctx, &athenav1.ResendInviteRequest{Username: "clerk"})
	if err != nil {
		t.Fatal(err)
	}
	clerk := f.browser(t)
	_, err = clerk.auth.CompleteChallenge(ctx, &athenav1.CompleteChallengeRequest{Username: "clerk", Purpose: auth.ChallengePurposeFirstLogin, Challenge: first.GetChallenge(), NewPassword: testPassword})
	requireCode(t, err, connect.CodeUnauthenticated)
	if _, err := clerk.auth.CompleteChallenge(ctx, &athenav1.CompleteChallengeRequest{Username: "clerk", Purpose: auth.ChallengePurposeFirstLogin, Challenge: second.GetChallenge(), NewPassword: testPassword}); err != nil {
		t.Fatalf("the re-sent Challenge did not complete: %v", err)
	}
}

func TestStartResetFlow(t *testing.T) {
	f := newAccessFixture(t)
	// The flow signs clerk in six times; the default sign-in bucket would
	// cut the last one off. Rate limits have their own tests.
	if err := f.users.SetSecurity(context.Background(), auth.SignInUsername, 100); err != nil {
		t.Fatal(err)
	}
	f.owner(t, "ada")
	f.user(t, "clerk")
	owner := f.browser(t)
	owner.login(t, "ada")
	clerk, other := f.browser(t), f.browser(t)
	clerk.login(t, "clerk")
	other.login(t, "clerk")
	ctx := context.Background()
	_, err := owner.auth.StartReset(ctx, &athenav1.StartResetRequest{Username: "clerk", ActorPassword: "wrong password here"})
	requireCode(t, err, connect.CodeUnauthenticated)
	started, err := owner.auth.StartReset(ctx, &athenav1.StartResetRequest{Username: "clerk", ActorPassword: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	if started.GetChallenge() == "" {
		t.Fatal("no reset Challenge was returned")
	}
	_, err = other.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	requireCode(t, err, connect.CodeUnauthenticated)
	const replacement = "a replacement passphrase"
	if _, err := clerk.auth.CompleteChallenge(ctx, &athenav1.CompleteChallengeRequest{Username: "clerk", Purpose: auth.ChallengePurposeReset, Challenge: started.GetChallenge(), NewPassword: replacement}); err != nil {
		t.Fatal(err)
	}
	fresh := f.browser(t)
	if _, err := fresh.auth.Login(ctx, &athenav1.LoginRequest{Username: "clerk", Password: replacement}); err != nil {
		t.Fatalf("sign-in after the reset: %v", err)
	}
}

func TestAssignmentRPCRoundTrip(t *testing.T) {
	f := newAccessFixture(t)
	f.owner(t, "ada")
	f.user(t, "clerk")
	owner := f.browser(t)
	owner.login(t, "ada")
	ctx := context.Background()
	// The Instance Owner holds assign but not roles, so the role is saved
	// directly; curator below covers SaveRole over RPC.
	f.role(t, "Reader", nil, grantOf("note", 0, "", auth.ActionRead))
	saved, err := owner.auth.SaveAssignment(ctx, &athenav1.SaveAssignmentRequest{Username: "clerk", Assignment: &athenav1.Assignment{
		Role: "Reader", Scope: map[string]*athenav1.ScopeValues{"company": {Values: []string{"acme"}}}, ActiveFrom: "2026-01-01",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if saved.GetAssignment().GetId() == "" || saved.GetAssignment().GetRevision() != 1 {
		t.Fatalf("saved Assignment = %+v", saved.GetAssignment())
	}
	listed, err := owner.auth.ListAssignments(ctx, &athenav1.ListAssignmentsRequest{Username: "clerk"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.GetAssignments()) != 1 || listed.GetAssignments()[0].GetScope()["company"].GetValues()[0] != "acme" {
		t.Fatalf("listed Assignments = %+v", listed.GetAssignments())
	}
	_, err = owner.auth.SaveAssignment(ctx, &athenav1.SaveAssignmentRequest{Username: "clerk", Assignment: &athenav1.Assignment{
		Role: "Reader", ActiveFrom: "January",
	}})
	requireCode(t, err, connect.CodeInvalidArgument)
	stale := listed.GetAssignments()[0]
	stale.Revision = 99
	_, err = owner.auth.SaveAssignment(ctx, &athenav1.SaveAssignmentRequest{Username: "clerk", Assignment: stale})
	requireCode(t, err, connect.CodeAborted)
	if _, err := owner.auth.RevokeAssignment(ctx, &athenav1.RevokeAssignmentRequest{Id: saved.GetAssignment().GetId()}); err != nil {
		t.Fatal(err)
	}
	listed, err = owner.auth.ListAssignments(ctx, &athenav1.ListAssignmentsRequest{Username: "clerk"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.GetAssignments()) != 0 {
		t.Fatalf("Assignments after revoke = %+v", listed.GetAssignments())
	}
}

func TestTransferOwnershipFlow(t *testing.T) {
	f := newAccessFixture(t)
	f.owner(t, "ada")
	f.user(t, "grace")
	owner := f.browser(t)
	owner.login(t, "ada")
	ctx := context.Background()
	if _, err := owner.auth.TransferOwnership(ctx, &athenav1.TransferOwnershipRequest{Username: "grace", ActorPassword: testPassword}); err != nil {
		t.Fatal(err)
	}
	successor := f.browser(t)
	successor.login(t, "grace")
	if _, err := successor.auth.InviteUser(ctx, &athenav1.InviteUserRequest{Username: "newcomer"}); err != nil {
		t.Fatalf("the appointed owner cannot invite: %v", err)
	}
	if _, err := owner.auth.TransferOwnership(ctx, &athenav1.TransferOwnershipRequest{Username: "grace", ActorPassword: testPassword, Relinquish: true}); err != nil {
		t.Fatal(err)
	}
	// Relinquishing ends the actor's own session; after signing in again
	// the former owner is refused for lack of permission.
	_, err := owner.auth.InviteUser(ctx, &athenav1.InviteUserRequest{Username: "another"})
	requireCode(t, err, connect.CodeUnauthenticated)
	owner.login(t, "ada")
	_, err = owner.auth.InviteUser(ctx, &athenav1.InviteUserRequest{Username: "another"})
	requireCode(t, err, connect.CodePermissionDenied)
	if _, err := successor.auth.InviteUser(ctx, &athenav1.InviteUserRequest{Username: "second"}); err != nil {
		t.Fatalf("the remaining owner lost access: %v", err)
	}
}

func TestDelegationDenialOverRPC(t *testing.T) {
	f := newAccessFixture(t)
	f.owner(t, "ada")
	f.user(t, "manager", auth.RoleSystemManager)
	manager := f.browser(t)
	manager.login(t, "manager")
	ctx := context.Background()
	_, err := manager.auth.SaveAssignment(ctx, &athenav1.SaveAssignmentRequest{Username: "manager", Assignment: &athenav1.Assignment{Role: auth.RoleInstanceOwner}})
	requireCode(t, err, connect.CodePermissionDenied)
	_, err = manager.auth.TransferOwnership(ctx, &athenav1.TransferOwnershipRequest{Username: "manager", ActorPassword: testPassword})
	requireCode(t, err, connect.CodePermissionDenied)
}

func TestRetiredRPCsFail(t *testing.T) {
	f := newAccessFixture(t)
	f.owner(t, "ada")
	owner := f.browser(t)
	owner.login(t, "ada")
	ctx := context.Background()
	_, err := owner.auth.CreateUser(ctx, &athenav1.CreateUserRequest{Username: "x"})
	requireCode(t, err, connect.CodeUnimplemented)
	_, err = owner.auth.ResetPassword(ctx, &athenav1.ResetPasswordRequest{Username: "ada"})
	requireCode(t, err, connect.CodeUnimplemented)
}

func TestSessionManagement(t *testing.T) {
	f := newAccessFixture(t)
	f.owner(t, "ada")
	f.user(t, "clerk")
	owner := f.browser(t)
	owner.login(t, "ada")
	clerk, other := f.browser(t), f.browser(t)
	clerk.login(t, "clerk")
	other.login(t, "clerk")
	ctx := context.Background()

	// Users list their own sessions, with the current one marked: the two
	// browser logins plus the session the fixture completion started.
	listed, err := clerk.auth.ListSessions(ctx, &athenav1.ListSessionsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.GetSessions()) != 3 {
		t.Fatalf("own sessions = %+v", listed.GetSessions())
	}
	current := 0
	ids := map[string]bool{}
	for _, session := range listed.GetSessions() {
		ids[session.GetId()] = true
		if session.GetCurrent() {
			current++
		}
		if session.GetCreatedAt() == "" || session.GetExpiresAt() == "" {
			t.Fatalf("session timestamps = %+v", session)
		}
	}
	if current != 1 {
		t.Fatalf("current sessions = %d, want 1", current)
	}

	// Another user's sessions need the sessions permission.
	_, err = clerk.auth.ListSessions(ctx, &athenav1.ListSessionsRequest{Username: "ada"})
	requireCode(t, err, connect.CodePermissionDenied)
	adaSessions, err := f.users.ListSessions(ctx, "ada")
	if err != nil || len(adaSessions) != 2 {
		t.Fatalf("ada sessions = %+v, %v", adaSessions, err)
	}
	_, err = clerk.auth.RevokeSession(ctx, &athenav1.RevokeSessionRequest{Id: adaSessions[0].ID})
	requireCode(t, err, connect.CodePermissionDenied)

	// Revoking an own session signs that browser out.
	otherListed, err := other.auth.ListSessions(ctx, &athenav1.ListSessionsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var victim string
	for _, session := range otherListed.GetSessions() {
		if session.GetCurrent() {
			victim = session.GetId()
		}
	}
	if victim == "" || !ids[victim] {
		t.Fatal("no current session found")
	}
	if _, err := clerk.auth.RevokeSession(ctx, &athenav1.RevokeSessionRequest{Id: victim}); err != nil {
		t.Fatal(err)
	}
	_, err = other.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	requireCode(t, err, connect.CodeUnauthenticated)
	_, err = clerk.auth.RevokeSession(ctx, &athenav1.RevokeSessionRequest{Id: victim})
	requireCode(t, err, connect.CodeNotFound)

	// With the sessions permission, another user's sessions open up.
	f.role(t, "observer", []string{auth.PermissionAccessSessions})
	f.assign(t, "ada", auth.RoleInstanceOwner, "observer")
	owner.login(t, "ada")
	listed, err = owner.auth.ListSessions(ctx, &athenav1.ListSessionsRequest{Username: "clerk"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.GetSessions()) != 2 {
		t.Fatalf("clerk sessions = %+v", listed.GetSessions())
	}
	mine, err := clerk.auth.ListSessions(ctx, &athenav1.ListSessionsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var currentID string
	for _, session := range mine.GetSessions() {
		if session.GetCurrent() {
			currentID = session.GetId()
		}
	}
	if currentID == "" {
		t.Fatal("no current session found")
	}
	if _, err := owner.auth.RevokeSession(ctx, &athenav1.RevokeSessionRequest{Id: currentID}); err != nil {
		t.Fatal(err)
	}
	_, err = clerk.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestExplainAccess(t *testing.T) {
	f := newAccessFixture(t)
	f.scopedNotes(t)
	north := f.operatorScopedNote(t, "N1", "north")
	south := f.operatorScopedNote(t, "S1", "south")
	f.role(t, "ro", nil, grantOf("snote", 0, "", auth.ActionRead))
	f.role(t, "rw", nil, grantOf("snote", 0, "", auth.ActionRead, auth.ActionWrite))
	f.role(t, "titled", nil, grantOf("snote", 0, `doc.title == "N1"`, auth.ActionRead))
	f.role(t, "owned", nil, grantOf("snote", 0, "doc.owner == user.name", auth.ActionRead))
	f.role(t, "creator", nil, grantOf("snote", 0, "", auth.ActionCreate, auth.ActionRead))
	submit := grantOf("snote", 0, "", auth.ActionSubmit)
	submit.SeparateDuties = true
	f.role(t, "approver", nil, submit)
	f.owner(t, "ada")
	f.user(t, "clerk")
	ctx := context.Background()
	assign := func(role string, scope auth.Scope) {
		t.Helper()
		if _, err := f.users.SaveAssignment(ctx, "clerk", auth.AssignmentInput{Role: role, Scope: scope}); err != nil {
			t.Fatal(err)
		}
	}
	assign("ro", auth.Scope{"company": {"north"}})
	assign("rw", auth.Scope{"company": {"south"}})
	assign("titled", nil)
	assign("owned", nil)
	assign("creator", nil)
	assign("approver", nil)
	f.role(t, "observer", []string{auth.PermissionAccessExplore})
	f.assign(t, "ada", auth.RoleInstanceOwner, "observer")
	owner := f.browser(t)
	owner.login(t, "ada")
	clerk := f.browser(t)
	clerk.login(t, "clerk")
	created, err := clerk.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "snote",
		Values: map[string]*athenav1.FieldValue{"title": text("C1"), "company": text("north")}})
	if err != nil {
		t.Fatal(err)
	}
	mine := created.GetRecord()

	explain := func(action, recordID string) *athenav1.ExplainAccessResponse {
		t.Helper()
		res, err := owner.auth.ExplainAccess(ctx, &athenav1.ExplainAccessRequest{Username: "clerk", Action: action, RecordTypeKey: "snote", RecordId: recordID})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	byRole := func(res *athenav1.ExplainAccessResponse) map[string]*athenav1.AssignmentExplanation {
		out := map[string]*athenav1.AssignmentExplanation{}
		for _, v := range res.GetAssignments() {
			out[v.GetRole()] = v
		}
		return out
	}

	// Read on the north record: the north, title and creator assignments
	// match; the south one misses its Scope.
	res := explain(auth.ActionRead, north.GetId())
	if !res.GetAllowed() {
		t.Fatal("read on the north record was not allowed")
	}
	verdicts := byRole(res)
	if !verdicts["ro"].GetMatched() || !verdicts["titled"].GetMatched() || !verdicts["creator"].GetMatched() {
		t.Fatalf("read verdicts = %+v", res.GetAssignments())
	}
	if verdicts["rw"].GetMatched() {
		t.Fatalf("the south assignment matched a north record: %+v", verdicts["rw"])
	}
	// The owned assignment binds user.name to the clerk, not to ada, the
	// caller: the operator-owned record does not match.
	found := false
	for _, reason := range verdicts["owned"].GetReasons() {
		found = found || reason == "row condition false"
	}
	if verdicts["owned"].GetMatched() || !found {
		t.Fatalf("owned verdict on an operator record = %+v", verdicts["owned"])
	}
	res = explain(auth.ActionRead, mine.GetId())
	if !byRole(res)["owned"].GetMatched() {
		t.Fatalf("owned verdict on the clerk's own record = %+v", res.GetAssignments())
	}

	// Write on the north record: nothing matches, and each verdict says why.
	res = explain(auth.ActionWrite, north.GetId())
	if res.GetAllowed() {
		t.Fatal("write on the north record was allowed")
	}
	verdicts = byRole(res)
	if verdicts["rw"].GetMatched() || verdicts["rw"].GetReasons()[0] != "scope does not match this record" {
		t.Fatalf("south write verdict = %+v", verdicts["rw"])
	}
	res = explain(auth.ActionWrite, south.GetId())
	if !res.GetAllowed() || !byRole(res)["rw"].GetMatched() {
		t.Fatalf("write on the south record = %+v", res.GetAssignments())
	}

	// Without a record the row checks stay untested.
	res = explain(auth.ActionWrite, "")
	if !res.GetAllowed() || !byRole(res)["rw"].GetMatched() {
		t.Fatalf("write without a record = %+v", res.GetAssignments())
	}

	// Submit carries the separation conflict: the clerk creates and approves.
	res = explain(auth.ActionSubmit, "")
	if !res.GetAllowed() {
		t.Fatal("submit was not allowed")
	}
	if !byRole(res)["approver"].GetSeparateDutiesConflict() {
		t.Fatalf("approver verdict = %+v", byRole(res)["approver"])
	}

	// Errors name their cause.
	_, err = owner.auth.ExplainAccess(ctx, &athenav1.ExplainAccessRequest{Username: "nobody", Action: auth.ActionRead, RecordTypeKey: "snote"})
	requireCode(t, err, connect.CodeNotFound)
	_, err = owner.auth.ExplainAccess(ctx, &athenav1.ExplainAccessRequest{Username: "clerk", Action: auth.ActionRead, RecordTypeKey: "missing"})
	requireCode(t, err, connect.CodeNotFound)
	_, err = owner.auth.ExplainAccess(ctx, &athenav1.ExplainAccessRequest{Username: "clerk", Action: "fly", RecordTypeKey: "snote"})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = owner.auth.ExplainAccess(ctx, &athenav1.ExplainAccessRequest{Username: "clerk", Action: auth.ActionRead, RecordTypeKey: "snote", RecordId: "00000000-0000-0000-0000-000000000000"})
	requireCode(t, err, connect.CodeNotFound)
	_, err = clerk.auth.ExplainAccess(ctx, &athenav1.ExplainAccessRequest{Username: "clerk", Action: auth.ActionRead, RecordTypeKey: "snote"})
	requireCode(t, err, connect.CodePermissionDenied)
}

func TestListRoleHolders(t *testing.T) {
	f := newAccessFixture(t)
	f.role(t, "Reader", nil, grantOf("invoice", 0, "", auth.ActionRead))
	f.user(t, "clerk")
	f.user(t, "temp")
	f.user(t, "other")
	f.role(t, "roles-only", []string{auth.PermissionAccessRoles})
	f.user(t, "curator", "roles-only")
	ctx := context.Background()
	if _, err := f.users.SaveAssignment(ctx, "clerk", auth.AssignmentInput{Role: "Reader", Scope: auth.Scope{"company": {"acme"}}}); err != nil {
		t.Fatal(err)
	}
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	if _, err := f.users.SaveAssignment(ctx, "temp", auth.AssignmentInput{Role: "Reader", ExpiresAt: &yesterday}); err != nil {
		t.Fatal(err)
	}
	curator := f.browser(t)
	curator.login(t, "curator")
	res, err := curator.auth.ListRoleHolders(ctx, &athenav1.ListRoleHoldersRequest{Role: "Reader", RecordTypeKey: "invoice", Action: auth.ActionRead})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetHolders()) != 2 || res.GetHolders()[0].GetUsername() != "clerk" || !res.GetHolders()[0].GetUsable() {
		t.Fatalf("holders = %+v", res.GetHolders())
	}
	if res.GetHolders()[1].GetUsername() != "temp" || res.GetHolders()[1].GetUsable() {
		t.Fatalf("holders = %+v", res.GetHolders())
	}
	if got := res.GetHolders()[0].GetScope()["company"].GetValues(); len(got) != 1 || got[0] != "acme" {
		t.Fatalf("clerk scope = %+v", res.GetHolders()[0].GetScope())
	}
	res, err = curator.auth.ListRoleHolders(ctx, &athenav1.ListRoleHoldersRequest{Role: "Reader", RecordTypeKey: "invoice", Action: auth.ActionWrite})
	if err != nil || len(res.GetHolders()) != 0 {
		t.Fatalf("write holders = %+v, %v", res.GetHolders(), err)
	}
	_, err = curator.auth.ListRoleHolders(ctx, &athenav1.ListRoleHoldersRequest{Role: "Missing", RecordTypeKey: "invoice", Action: auth.ActionRead})
	requireCode(t, err, connect.CodeNotFound)
	_, err = curator.auth.ListRoleHolders(ctx, &athenav1.ListRoleHoldersRequest{Role: "Reader", RecordTypeKey: "invoice", Action: "fly"})
	requireCode(t, err, connect.CodeInvalidArgument)
	clerk := f.browser(t)
	clerk.login(t, "clerk")
	_, err = clerk.auth.ListRoleHolders(ctx, &athenav1.ListRoleHoldersRequest{Role: "Reader", RecordTypeKey: "invoice", Action: auth.ActionRead})
	requireCode(t, err, connect.CodePermissionDenied)
}

func TestGranularGateSeparatesDuties(t *testing.T) {
	f := newAccessFixture(t)
	f.notes(t)
	f.role(t, "roles-only", []string{auth.PermissionAccessRoles})
	f.user(t, "curator", "roles-only")
	curator := f.browser(t)
	curator.login(t, "curator")
	ctx := context.Background()
	if _, err := curator.auth.ListRoles(ctx, &athenav1.ListRolesRequest{}); err != nil {
		t.Fatalf("roles reader listing roles: %v", err)
	}
	_, err := curator.auth.ListUsers(ctx, &athenav1.ListUsersRequest{})
	requireCode(t, err, connect.CodePermissionDenied)
	if _, err := curator.auth.SaveRole(ctx, &athenav1.SaveRoleRequest{Role: &athenav1.Role{Name: "Empty"}}); err != nil {
		t.Fatalf("roles holder saving an empty role: %v", err)
	}
	_, err = curator.auth.SaveRole(ctx, &athenav1.SaveRoleRequest{Role: &athenav1.Role{Name: "Greedy", Grants: []*athenav1.Grant{{RecordTypeKey: "note", Actions: []string{auth.ActionRead}}}}})
	requireCode(t, err, connect.CodePermissionDenied)
}
