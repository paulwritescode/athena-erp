package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/auth"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const testPassword = "a long test passphrase"

// accessFixture serves the real ConnectRPC stack, with the permission gate,
// over HTTP against a fresh database.
type accessFixture struct {
	db    *sql.DB
	url   string
	users *auth.Service
	// Operator services: they run as the CLI does, with no principal.
	types   recordTypeService
	records recordService
	rules   ruleService
}

func newAccessFixture(t *testing.T) *accessFixture {
	t.Helper()
	return newAccessFixtureAt(t, "")
}

// newAccessFixtureAt is newAccessFixture with a Workspace directory, which
// FileService needs.
func newAccessFixtureAt(t *testing.T, root string) *accessFixture {
	t.Helper()
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	mux := http.NewServeMux()
	mountRPC(mux, Config{Workspace: &Workspace{Root: root}}, db, false, []connect.ServerInterceptor{requestInterceptor()})
	srv := httptest.NewServer(securityHeaders(mux))
	t.Cleanup(srv.Close)
	hooks := lifecycle.NewRegistry()
	registerRules(hooks, db)
	return &accessFixture{db: db, url: srv.URL, users: &auth.Service{DB: db}, types: recordTypeService{db: db}, records: recordService{db: db, hooks: hooks}, rules: newRuleService(db)}
}

// csrfTransport sends the CSRF token the last sign-in returned.
type csrfTransport struct {
	token *string
}

func (c csrfTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if *c.token != "" {
		r = r.Clone(r.Context())
		r.Header.Set(csrfHeader, *c.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// session is one browser: a cookie jar and the CSRF token it learned.
type session struct {
	inbox     athenav1connect.NotificationServiceClient
	workspace athenav1connect.WorkspaceServiceClient
	auth      athenav1connect.AuthServiceClient
	records   athenav1connect.RecordServiceClient
	types     athenav1connect.RecordTypeServiceClient
	system    athenav1connect.SystemServiceClient
	files     athenav1connect.FileServiceClient
	rules     athenav1connect.RuleServiceClient
	jar       *cookiejar.Jar
	csrf      *string
	base      string
}

func (f *accessFixture) browser(t *testing.T) *session {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	token := ""
	client := connect.NewClient(connecthttp.NewTransport(&http.Client{Jar: jar, Transport: csrfTransport{token: &token}}, f.url))
	return &session{inbox: athenav1connect.NewNotificationServiceClient(client), workspace: athenav1connect.NewWorkspaceServiceClient(client), auth: athenav1connect.NewAuthServiceClient(client), records: athenav1connect.NewRecordServiceClient(client), types: athenav1connect.NewRecordTypeServiceClient(client),
		system: athenav1connect.NewSystemServiceClient(client), files: athenav1connect.NewFileServiceClient(client), rules: athenav1connect.NewRuleServiceClient(client), jar: jar, csrf: &token, base: f.url}
}

func (s *session) login(t *testing.T, username string) {
	t.Helper()
	res, err := s.auth.Login(context.Background(), &athenav1.LoginRequest{Username: username, Password: testPassword})
	if err != nil {
		t.Fatalf("login %s: %v", username, err)
	}
	*s.csrf = res.GetSession().GetCsrfToken()
}

func (s *session) cookie() string {
	u, _ := url.Parse(s.base)
	for _, c := range s.jar.Cookies(u) {
		if c.Name == sessionCookieName {
			return c.Value
		}
	}
	return ""
}

func (f *accessFixture) user(t *testing.T, username string, roles ...string) {
	t.Helper()
	ctx := context.Background()
	user, challenge, err := f.users.CreateUser(ctx, username, "", false, newRequestID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.users.CompleteChallenge(ctx, user.Username, auth.ChallengePurposeFirstLogin, challenge, testPassword, newRequestID()); err != nil {
		t.Fatal(err)
	}
	f.assign(t, username, roles...)
}

func (f *accessFixture) assign(t *testing.T, username string, roles ...string) {
	t.Helper()
	if _, err := f.users.UpdateUser(context.Background(), username, auth.UserUpdate{Roles: &roles}); err != nil {
		t.Fatal(err)
	}
}

func (f *accessFixture) role(t *testing.T, name string, system []string, grants ...auth.Grant) {
	t.Helper()
	if err := f.users.SaveRole(context.Background(), auth.Role{Name: name, SystemPermissions: system, Grants: grants}); err != nil {
		t.Fatal(err)
	}
}

func grantOf(recordType string, level int, condition string, actions ...string) auth.Grant {
	g := auth.Grant{RecordType: recordType, Level: level, Condition: condition, Actions: map[string]bool{}}
	for _, a := range actions {
		g.Actions[a] = true
	}
	return g
}

func (f *accessFixture) accessEvents(t *testing.T, event string) []auth.AccessRow {
	t.Helper()
	rows, err := f.users.ListAccessLog(context.Background(), 200, 0, event)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

var noteFields = []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text", Required: true}, {Key: "salary", Label: "Salary", Type: "Number", PermissionLevel: 2}}

func (f *accessFixture) notes(t *testing.T) {
	t.Helper()
	mustSave(t, f.types, &athenav1.RecordType{Key: "note", Label: "Note", Fields: noteFields})
}

func (f *accessFixture) operatorNote(t *testing.T, title string) *athenav1.Record {
	t.Helper()
	res, err := f.records.CreateRecord(context.Background(), &athenav1.CreateRecordRequest{RecordTypeKey: "note", Values: map[string]*athenav1.FieldValue{"title": text(title), "salary": text("100")}})
	if err != nil {
		t.Fatal(err)
	}
	return res.GetRecord()
}

func TestEveryProcedureHasAPolicy(t *testing.T) {
	seen := map[string]bool{}
	protoregistry.GlobalFiles.RangeFilesByPackage("athena.v1", func(file protoreflect.FileDescriptor) bool {
		for i := 0; i < file.Services().Len(); i++ {
			service := file.Services().Get(i)
			for j := 0; j < service.Methods().Len(); j++ {
				procedure := "/" + string(service.FullName()) + "/" + string(service.Methods().Get(j).Name())
				seen[procedure] = true
				if _, ok := procedurePolicies[procedure]; !ok {
					t.Errorf("%s has no policy, so the gate denies it; add one deliberately", procedure)
				}
			}
		}
		return true
	})
	for procedure := range procedurePolicies {
		if !seen[procedure] {
			t.Errorf("policy for %s, which does not exist", procedure)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no athena.v1 services are registered")
	}
}

func TestSignedOutCallsAreRefused(t *testing.T) {
	f := newAccessFixture(t)
	f.notes(t)
	b := f.browser(t)
	ctx := context.Background()
	if _, err := b.system.GetSystemInfo(ctx, &athenav1.GetSystemInfoRequest{}); err != nil {
		t.Fatalf("system info must stay open: %v", err)
	}
	session, err := b.auth.GetSession(ctx, &athenav1.GetSessionRequest{})
	if err != nil || session.GetSession().GetAuthenticated() || session.GetHasUsers() {
		t.Fatalf("session = %v, %v", session, err)
	}
	_, err = b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	requireCode(t, err, connect.CodeUnauthenticated)
	_, err = b.types.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	requireCode(t, err, connect.CodeUnauthenticated)
	_, err = b.auth.ListUsers(ctx, &athenav1.ListUsersRequest{})
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestNoGrantMeansDenyAndTheDenialIsLogged(t *testing.T) {
	f := newAccessFixture(t)
	f.notes(t)
	f.operatorNote(t, "hidden")
	f.user(t, "nobody")
	b := f.browser(t)
	b.login(t, "nobody")
	ctx := context.Background()
	_, err := b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	requireCode(t, err, connect.CodePermissionDenied)
	_, err = b.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "note", Values: map[string]*athenav1.FieldValue{"title": text("x")}})
	requireCode(t, err, connect.CodePermissionDenied)
	_, err = b.types.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "other", Label: "Other", Fields: noteFields}})
	requireCode(t, err, connect.CodePermissionDenied)
	listed, err := b.types.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil || len(listed.GetRecordTypes()) != 0 {
		t.Fatalf("a user with no Grant sees Record Types: %v, %v", listed, err)
	}
	denials := f.accessEvents(t, auth.EventDenied)
	if len(denials) != 3 {
		t.Fatalf("denials = %d, want 3", len(denials))
	}
	if denials[2].Username != "nobody" || denials[2].RecordType != "note" || denials[2].Action != auth.ActionRead || denials[2].RequestID == "" {
		t.Fatalf("denial row = %+v", denials[2])
	}
}

func TestCSRFTokenIsRequired(t *testing.T) {
	f := newAccessFixture(t)
	f.notes(t)
	f.role(t, "reader", nil, grantOf("note", 0, "", auth.ActionRead))
	f.user(t, "reader", "reader")
	b := f.browser(t)
	b.login(t, "reader")
	ctx := context.Background()
	if _, err := b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"}); err != nil {
		t.Fatal(err)
	}
	*b.csrf = "forged"
	_, err := b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	requireCode(t, err, connect.CodePermissionDenied)
	// A request from another site is refused before ConnectRPC sees it.
	req, _ := http.NewRequest(http.MethodPost, f.url+athenav1connect.RecordServiceListRecordsProcedure, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || res.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("cross-origin status = %d, CSP = %q", res.StatusCode, res.Header.Get("Content-Security-Policy"))
	}
}

func TestListsNeverReturnRowsTheUserCannotOpen(t *testing.T) {
	f := newAccessFixture(t)
	f.notes(t)
	other := f.operatorNote(t, "operator's")
	f.role(t, "own", nil, grantOf("note", 0, "doc.owner == user.name", auth.ActionRead, auth.ActionCreate, auth.ActionWrite))
	f.user(t, "alice", "own")
	b := f.browser(t)
	b.login(t, "alice")
	ctx := context.Background()
	mine, err := b.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "note", Values: map[string]*athenav1.FieldValue{"title": text("alice's")}})
	if err != nil {
		t.Fatal(err)
	}
	if mine.GetRecord().GetOwner() != "alice" || mine.GetRecord().GetCreatedBy() != "alice" {
		t.Fatalf("actor = %q", mine.GetRecord().GetOwner())
	}
	listed, err := b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	if err != nil || len(listed.GetRecords()) != 1 || listed.GetRecords()[0].GetId() != mine.GetRecord().GetId() {
		t.Fatalf("list = %v, %v", listed, err)
	}
	// A filter cannot widen the scope.
	listed, err = b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note", Filter: `doc.title == "operator's" || true`})
	if err != nil || len(listed.GetRecords()) != 1 {
		t.Fatalf("filtered list = %v, %v", listed, err)
	}
	_, err = b.records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "note", Id: other.GetId()})
	requireCode(t, err, connect.CodeNotFound)
	_, err = b.records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "note", Id: other.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"title": text("taken")}})
	requireCode(t, err, connect.CodeNotFound)
	lookup, err := b.records.LookupRecords(ctx, &athenav1.LookupRecordsRequest{RecordTypeKey: "note"})
	if err != nil || len(lookup.GetItems()) != 1 {
		t.Fatalf("lookup = %v, %v", lookup, err)
	}
	lookup, err = b.records.LookupRecords(ctx, &athenav1.LookupRecordsRequest{RecordTypeKey: "note", Ids: []string{other.GetId()}})
	if err != nil || len(lookup.GetItems()) != 0 {
		t.Fatalf("lookup by id = %v, %v", lookup, err)
	}
}

func TestFieldAboveTheUserLevelIsHiddenAndMaskedInHistory(t *testing.T) {
	f := newAccessFixture(t)
	f.notes(t)
	note := f.operatorNote(t, "pay")
	f.role(t, "clerk", nil, grantOf("note", 1, "", auth.ActionRead, auth.ActionCreate, auth.ActionWrite, auth.ActionAudit))
	f.user(t, "clerk", "clerk")
	b := f.browser(t)
	b.login(t, "clerk")
	ctx := context.Background()
	got, err := b.records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "note", Id: note.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if _, leaked := got.GetRecord().GetValues()["salary"]; leaked || got.GetRecord().GetValues()["title"].GetStringValue() != "pay" {
		t.Fatalf("values = %v", got.GetRecord().GetValues())
	}
	listed, err := b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	if err != nil || len(listed.GetRecords()) != 1 {
		t.Fatal(err)
	}
	if _, leaked := listed.GetRecords()[0].GetValues()["salary"]; leaked {
		t.Fatal("list returns a hidden field")
	}
	types, err := b.types.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil || len(types.GetRecordTypes()) != 1 || len(types.GetRecordTypes()[0].GetFields()) != 1 || types.GetRecordTypes()[0].GetAccess().GetPermissionLevel() != 1 {
		t.Fatalf("Record Types = %v, %v", types, err)
	}
	_, err = b.records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "note", Id: note.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"salary": text("1")}})
	requireCode(t, err, connect.CodePermissionDenied)
	_, err = b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note", Filter: `doc.salary > dec("1")`})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = b.records.EvaluateExpression(ctx, &athenav1.EvaluateExpressionRequest{RecordTypeKey: "note", Expression: `doc.salary`, Id: note.GetId()})
	requireCode(t, err, connect.CodeInvalidArgument)
	history, err := b.records.ListRecordHistory(ctx, &athenav1.ListRecordHistoryRequest{RecordTypeKey: "note", Id: note.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	maskedSeen := false
	for _, event := range history.GetEvents() {
		for _, change := range event.GetChanges() {
			if change.GetField() == "salary" {
				if !change.GetMasked() || change.OldValue != nil || change.NewValue != nil {
					t.Fatalf("salary change is not masked: %v", change)
				}
				maskedSeen = true
			}
		}
	}
	if !maskedSeen {
		t.Fatal("the history has no salary change to mask")
	}
	// The stored chain still holds the real value and verifies.
	verified, err := systemService{db: f.db}.VerifyAuditLog(ctx, &athenav1.VerifyAuditLogRequest{})
	if err != nil || !verified.GetOk() {
		t.Fatalf("verify = %v, %v", verified, err)
	}
	// The operator still reads the value.
	full, err := f.records.ListRecordHistory(ctx, &athenav1.ListRecordHistoryRequest{RecordTypeKey: "note", Id: note.GetId()})
	if err != nil || !strings.Contains(full.String(), "100") {
		t.Fatalf("operator history = %v, %v", full, err)
	}
}

func TestReadingTheAuditLogNeedsItsOwnPermission(t *testing.T) {
	f := newAccessFixture(t)
	f.notes(t)
	note := f.operatorNote(t, "x")
	f.role(t, "reader", nil, grantOf("note", 9, "", auth.ActionRead))
	f.user(t, "reader", "reader")
	b := f.browser(t)
	b.login(t, "reader")
	_, err := b.records.ListRecordHistory(context.Background(), &athenav1.ListRecordHistoryRequest{RecordTypeKey: "note", Id: note.GetId()})
	requireCode(t, err, connect.CodePermissionDenied)
	_, err = b.system.VerifyAuditLog(context.Background(), &athenav1.VerifyAuditLogRequest{})
	requireCode(t, err, connect.CodePermissionDenied)
}

func TestTheCreatorCannotSubmitUnderSeparateDuties(t *testing.T) {
	f := newAccessFixture(t)
	mustSave(t, f.types, &athenav1.RecordType{Key: "claim", Label: "Claim", Kind: "document", Class: "non_posting", Submittable: true, Fields: []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text", Required: true}}})
	approve := grantOf("claim", 0, "", auth.ActionRead, auth.ActionCreate, auth.ActionSubmit)
	approve.SeparateDuties = true
	f.role(t, "staff", nil, approve)
	f.user(t, "maker", "staff")
	f.user(t, "checker", "staff")
	maker, checker := f.browser(t), f.browser(t)
	maker.login(t, "maker")
	checker.login(t, "checker")
	ctx := context.Background()
	created, err := maker.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "claim", Values: map[string]*athenav1.FieldValue{"title": text("travel")}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = maker.records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: "claim", Id: created.GetRecord().GetId(), ExpectedRevision: created.GetRecord().GetRevision()})
	requireCode(t, err, connect.CodePermissionDenied)
	submitted, err := checker.records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: "claim", Id: created.GetRecord().GetId(), ExpectedRevision: created.GetRecord().GetRevision()})
	if err != nil || submitted.GetRecord().GetModifiedBy() != "checker" {
		t.Fatalf("checker submit = %v, %v", submitted, err)
	}
}

func TestRoleChangeTakesEffectOnTheNextCall(t *testing.T) {
	f := newAccessFixture(t)
	f.notes(t)
	f.role(t, "reader", nil, grantOf("note", 0, "", auth.ActionRead))
	f.user(t, "bob")
	b := f.browser(t)
	b.login(t, "bob")
	ctx := context.Background()
	_, err := b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	requireCode(t, err, connect.CodePermissionDenied)
	f.assign(t, "bob", "reader")
	if _, err := b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"}); err != nil {
		t.Fatalf("after the Role was granted: %v", err)
	}
	f.assign(t, "bob")
	_, err = b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	requireCode(t, err, connect.CodePermissionDenied)
}

func TestLogoutEndsTheSessionAndDisablingEndsAllSessions(t *testing.T) {
	f := newAccessFixture(t)
	f.role(t, "admin", []string{auth.PermissionAccessUsers})
	f.user(t, "carol", "admin")
	b, other := f.browser(t), f.browser(t)
	b.login(t, "carol")
	other.login(t, "carol")
	ctx := context.Background()
	if _, err := b.auth.ListUsers(ctx, &athenav1.ListUsersRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.auth.Logout(ctx, &athenav1.LogoutRequest{}); err != nil {
		t.Fatal(err)
	}
	_, err := b.auth.ListUsers(ctx, &athenav1.ListUsersRequest{})
	requireCode(t, err, connect.CodeUnauthenticated)
	disabled := true
	if _, err := f.users.UpdateUser(ctx, "carol", auth.UserUpdate{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	_, err = other.auth.ListUsers(ctx, &athenav1.ListUsersRequest{})
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestSessionsEndAfterIdleAndAbsoluteTimeouts(t *testing.T) {
	f := newAccessFixture(t)
	f.user(t, "frank")
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	svc := &auth.Service{DB: f.db, Now: func() time.Time { return now }}
	ctx := context.Background()
	login := func() string {
		t.Helper()
		res, err := svc.Login(ctx, "frank", testPassword, newRequestID())
		if err != nil {
			t.Fatal(err)
		}
		return res.Token
	}

	idle := login()
	now = now.Add(auth.IdleTimeout - time.Minute)
	if _, err := svc.Authenticate(ctx, idle); err != nil {
		t.Fatalf("a session inside the idle timeout was refused: %v", err)
	}
	now = now.Add(auth.IdleTimeout + time.Minute)
	if _, err := svc.Authenticate(ctx, idle); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("a session past the idle timeout = %v, want ErrUnauthenticated", err)
	}

	// Regular use keeps the session idle-fresh, yet it still ends at the
	// absolute lifetime.
	busy := login()
	start := now
	for now.Sub(start) < auth.AbsoluteLifetime-auth.IdleTimeout {
		now = now.Add(auth.IdleTimeout / 2)
		if _, err := svc.Authenticate(ctx, busy); err != nil {
			t.Fatalf("an active session was refused at %v: %v", now.Sub(start), err)
		}
	}
	now = start.Add(auth.AbsoluteLifetime + time.Second)
	if _, err := svc.Authenticate(ctx, busy); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("a session past the absolute lifetime = %v, want ErrUnauthenticated", err)
	}
}

func TestPasswordChangeAndResetEndSessions(t *testing.T) {
	f := newAccessFixture(t)
	f.user(t, "gina")
	svc := f.users
	ctx := context.Background()
	login := func() *auth.LoginResult {
		t.Helper()
		res, err := svc.Login(ctx, "gina", testPassword, newRequestID())
		if err != nil {
			t.Fatal(err)
		}
		return &res
	}
	alive := func(token string) bool {
		t.Helper()
		_, err := svc.Authenticate(ctx, token)
		if err != nil && !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatal(err)
		}
		return err == nil
	}

	current, other := login(), login()
	const changed = "another long passphrase"
	if err := svc.ChangePassword(ctx, current.Principal.UserID, testPassword, changed, current.Principal.SessionID); err != nil {
		t.Fatal(err)
	}
	if !alive(current.Token) || alive(other.Token) {
		t.Fatalf("after ChangePassword: current alive = %v (want true), other alive = %v (want false)", alive(current.Token), alive(other.Token))
	}

	_, resetToken, err := svc.StartReset(ctx, "gina", "", newRequestID())
	if err != nil {
		t.Fatal(err)
	}
	if alive(current.Token) {
		t.Fatal("StartReset left a session alive")
	}
	const reset = "a reset long passphrase"
	if _, err := svc.CompleteChallenge(ctx, "gina", auth.ChallengePurposeReset, resetToken, reset, newRequestID()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, "gina", reset, newRequestID()); err != nil {
		t.Fatalf("sign-in after a reset: %v", err)
	}
}

func TestFailedLoginsLockTheUser(t *testing.T) {
	f := newAccessFixture(t)
	f.user(t, "dave")
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	svc := &auth.Service{DB: f.db, Now: func() time.Time { return now }}
	ctx := context.Background()
	for i := 0; i < auth.MaxFailedAttempts; i++ {
		if _, err := svc.Login(ctx, "dave", "wrong password!", newRequestID()); err == nil {
			t.Fatal("a wrong password signed in")
		}
	}
	if _, err := svc.Login(ctx, "dave", testPassword, newRequestID()); err == nil {
		t.Fatal("a locked user signed in with the right password")
	}
	if len(f.accessEvents(t, auth.EventLockout)) != 1 || len(f.accessEvents(t, auth.EventLoginFailed)) != auth.MaxFailedAttempts {
		t.Fatalf("lockout rows = %d, failures = %d", len(f.accessEvents(t, auth.EventLockout)), len(f.accessEvents(t, auth.EventLoginFailed)))
	}
	now = now.Add(auth.LockDuration + time.Second)
	if _, err := svc.Login(ctx, "dave", testPassword, newRequestID()); err != nil {
		t.Fatalf("the lock did not expire: %v", err)
	}
	if _, err := svc.Login(ctx, "nobody-here", testPassword, newRequestID()); err == nil {
		t.Fatal("an unknown user signed in")
	}
}

func TestPasswordsAndTokensNeverReachLogsSpansOrTables(t *testing.T) {
	f := newAccessFixture(t)
	f.notes(t)
	f.role(t, "reader", nil, grantOf("note", 0, "", auth.ActionRead))
	f.user(t, "erin", "reader")
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	var logged bytes.Buffer
	original := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(original) })

	b := f.browser(t)
	ctx := context.Background()
	const wrong = "wrong-secret-passphrase"
	_, _ = b.auth.Login(ctx, &athenav1.LoginRequest{Username: "erin", Password: wrong})
	b.login(t, "erin")
	_, _ = b.records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "note"})
	_, _ = b.auth.ChangePassword(ctx, &athenav1.ChangePasswordRequest{CurrentPassword: testPassword, NewPassword: "another long passphrase"})
	token := b.cookie()
	if token == "" {
		t.Fatal("no session cookie")
	}
	secrets := []string{testPassword, wrong, "another long passphrase", token, *b.csrf}
	check := func(where, text string) {
		for _, secret := range secrets {
			if strings.Contains(text, secret) {
				t.Fatalf("%s carries a secret", where)
			}
		}
	}
	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("no spans were recorded")
	}
	for _, span := range spans {
		for _, attribute := range span.Attributes {
			check("span "+span.Name+" attribute "+string(attribute.Key), attribute.Value.Emit())
		}
		check("span "+span.Name+" status", span.Status.Description)
	}
	check("the process log", logged.String())
	for _, query := range []string{"SELECT CONCAT_WS('|', event, outcome, COALESCE(username,''), COALESCE(procedure_name,''), COALESCE(detail,'')) FROM athena_access_log", "SELECT CONCAT_WS('|', token_hash, id) FROM athena_sessions", "SELECT password_hash FROM athena_users"} {
		rows, err := f.db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var text string
			if err := rows.Scan(&text); err != nil {
				t.Fatal(err)
			}
			check("table row", text)
		}
		rows.Close()
	}
}

func TestChildAndParentRollupSaveInOneRequest(t *testing.T) {
	recordTypes, records := newServices(t)
	ctx := context.Background()
	order := &athenav1.RecordType{Key: "order", Label: "Order", Kind: "master", Fields: []*athenav1.RecordTypeField{{Key: "note", Label: "Note", Type: "Text"}, numField("grand", "Grand")}}
	saved, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: order})
	if err != nil {
		t.Fatal(err)
	}
	order.Fields[1].Formula = "sum(doc.line.qty)"
	order.Revision = saved.GetRecordType().GetRevision()
	line := &athenav1.RecordType{Key: "line", Label: "Line", Kind: "child", ParentRecordTypeKey: "order", Fields: []*athenav1.RecordTypeField{numField("qty", "Qty")}}
	// Without the parent in the request the old flow still needs two saves;
	// with it, one request saves both.
	res, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: line, Parent: order})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetParent().GetRevision() != order.GetRevision()+1 || res.GetParent().GetFields()[1].GetFormula() == "" {
		t.Fatalf("parent = %v", res.GetParent())
	}
	created, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "order", Values: map[string]*athenav1.FieldValue{"note": text("n")}})
	if err != nil {
		t.Fatal(err)
	}
	for _, qty := range []string{"2", "3"} {
		if _, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "line", ParentId: created.GetRecord().GetId(), Values: map[string]*athenav1.FieldValue{"qty": text(qty)}}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "order", Id: created.GetRecord().GetId()})
	if err != nil || numeric(valueOf(got.GetRecord(), "grand")) != "5" {
		t.Fatalf("grand = %q, %v", valueOf(got.GetRecord(), "grand"), err)
	}
	// A parent formula that does not compile against the child saves nothing.
	bad := &athenav1.RecordType{Key: "order2", Label: "Order 2", Kind: "master", Fields: []*athenav1.RecordTypeField{numField("grand", "Grand")}}
	saved, err = recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: bad})
	if err != nil {
		t.Fatal(err)
	}
	bad.Fields[0].Formula = "sum(doc.line2.missing)"
	bad.Revision = saved.GetRecordType().GetRevision()
	_, err = recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "line2", Label: "Line 2", Kind: "child", ParentRecordTypeKey: "order2", Fields: []*athenav1.RecordTypeField{numField("qty", "Qty")}}, Parent: bad})
	requireCode(t, err, connect.CodeInvalidArgument)
	if definition, _, _ := loadRecordType(ctx, recordTypes.db, "line2"); definition != nil {
		t.Fatal("the child was saved although its parent was refused")
	}
}
