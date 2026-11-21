package server

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/expr"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"
	"github.com/go-sql-driver/mysql"
)

// authService implements athena.v1.AuthService over internal/auth. The gate
// has already checked the caller's permission for every method but Login and
// GetSession.
type authService struct {
	auth *auth.Service
}

// authError maps an auth package error to a ConnectRPC error whose message is
// safe to show.
func authError(op string, err error) error {
	var ended *auth.SessionEnded
	var locked *auth.LockedError
	switch {
	case errors.As(err, &ended):
		return failureError(connect.CodeUnauthenticated, ended.Error(), &athenav1.AuthFailure{Reason: ended.Reason})
	case errors.As(err, &locked):
		return failureError(connect.CodeFailedPrecondition, err.Error(), &athenav1.AuthFailure{Reason: "locked", UnlocksAt: locked.Until.UTC().Format(time.RFC3339)})
	case errors.Is(err, auth.ErrDisabled):
		return failureError(connect.CodeFailedPrecondition, err.Error(), &athenav1.AuthFailure{Reason: "disabled"})
	case isConnectionError(err):
		return connect.NewError(connect.CodeUnavailable, "the database is not reachable")
	case errors.Is(err, auth.ErrWrongPassword):
		return connect.NewError(connect.CodeUnauthenticated, err.Error())
	case errors.Is(err, auth.ErrInvalidCredentials):
		return connect.NewError(connect.CodeUnauthenticated, err.Error())
	case errors.Is(err, auth.ErrChallengeRefused):
		return connect.NewError(connect.CodeUnauthenticated, err.Error())
	case errors.Is(err, auth.ErrUnauthenticated):
		return connect.NewError(connect.CodeUnauthenticated, err.Error())
	case errors.Is(err, auth.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err.Error())
	case errors.Is(err, auth.ErrExists):
		return connect.NewError(connect.CodeAlreadyExists, err.Error())
	case errors.Is(err, auth.ErrConflict):
		return connect.NewError(connect.CodeAborted, err.Error())
	case errors.Is(err, auth.ErrLastOwner):
		return connect.NewError(connect.CodeFailedPrecondition, err.Error())
	case errors.Is(err, auth.ErrForbidden):
		return connect.NewError(connect.CodePermissionDenied, err.Error())
	case errors.Is(err, auth.ErrInvalid):
		return connect.NewError(connect.CodeInvalidArgument, err.Error())
	}
	return internalError(op, err)
}

func sessionInfo(p *auth.Principal, token string) *athenav1.SessionInfo {
	if p == nil {
		return &athenav1.SessionInfo{}
	}
	info := &athenav1.SessionInfo{Authenticated: true, Username: p.Username, DisplayName: p.DisplayName, Owner: p.Owner, CsrfToken: auth.CSRFToken(token), IdleExpiresAt: p.IdleExpiresAt.UTC().Format(time.RFC3339Nano), ExpiresAt: p.ExpiresAt.UTC().Format(time.RFC3339Nano)}
	for _, permission := range auth.SystemPermissions {
		if p.HasSystem(permission) {
			info.SystemPermissions = append(info.SystemPermissions, permission)
		}
	}
	return info
}

func (s authService) Login(ctx context.Context, req *athenav1.LoginRequest) (res *athenav1.LoginResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "login", "")
	defer func() { end(err) }()
	result, err := s.auth.LoginWithOptions(ctx, req.GetUsername(), req.GetPassword(), requestID, sessionOptions(ctx, req.GetRememberDevice()))
	if err != nil {
		return nil, authError("sign in", err)
	}
	setSessionCookie(ctx, result.Token, result.ExpiresAt, result.Principal.Remembered)
	return &athenav1.LoginResponse{Session: sessionInfo(result.Principal, result.Token)}, nil
}

// Store a coarse device label. Raw user agents can contain arbitrary values.
func sessionOptions(ctx context.Context, remember bool) auth.SessionOptions {
	opt := auth.SessionOptions{RememberDevice: remember, PreviousToken: sessionTokenFrom(ctx), Device: "Other client"}
	if info, ok := connect.CallInfoForServerContext(ctx); ok {
		ua := info.RequestHeader().Get("User-Agent")
		for _, browser := range []struct{ match, label string }{{"Edg/", "Edge"}, {"Firefox/", "Firefox"}, {"Chrome/", "Chrome"}, {"Safari/", "Safari"}, {"Go-http-client/", "Go HTTP client"}} {
			if strings.Contains(ua, browser.match) {
				opt.Device = browser.label
				break
			}
		}
	}
	return opt
}

func (s authService) StepUp(ctx context.Context, req *athenav1.StepUpRequest) (res *athenav1.StepUpResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "step_up", "")
	defer func() { end(err) }()
	result, err := s.auth.StepUp(ctx, auth.PrincipalFrom(ctx), req.GetPassword(), requestID)
	if err != nil {
		return nil, authError("step up", err)
	}
	setSessionCookie(ctx, result.Token, result.ExpiresAt, result.Principal.Remembered)
	return &athenav1.StepUpResponse{Session: sessionInfo(result.Principal, result.Token)}, nil
}

func clearSiteData(ctx context.Context) {
	if info, ok := connect.CallInfoForServerContext(ctx); ok {
		info.ResponseHeader().Set("Clear-Site-Data", `"cache", "cookies", "storage"`)
	}
}

func (s authService) Logout(ctx context.Context, _ *athenav1.LogoutRequest) (res *athenav1.LogoutResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "logout", "")
	defer func() { end(err) }()
	p := auth.PrincipalFrom(ctx)
	if p == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, "sign in to continue")
	}
	if err := s.auth.Logout(ctx, p.SessionID); err != nil {
		return nil, internalError("sign out", err)
	}
	clearSiteData(ctx)
	setSessionCookie(ctx, "", time.Time{}, false)
	s.auth.LogAccess(ctx, auth.AccessEvent{Event: auth.EventLogout, Outcome: auth.OutcomeOK, UserID: p.UserID, Username: p.Username, RequestID: requestID})
	return &athenav1.LogoutResponse{}, nil
}

func (s authService) GetSession(ctx context.Context, _ *athenav1.GetSessionRequest) (*athenav1.GetSessionResponse, error) {
	has, err := s.auth.HasUsers(ctx)
	if err != nil {
		return nil, internalError("read users", err)
	}
	return &athenav1.GetSessionResponse{Session: sessionInfo(auth.PrincipalFrom(ctx), sessionTokenFrom(ctx)), HasUsers: has}, nil
}

func (s authService) ChangePassword(ctx context.Context, req *athenav1.ChangePasswordRequest) (res *athenav1.ChangePasswordResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "change_password", "")
	defer func() { end(err) }()
	p := auth.PrincipalFrom(ctx)
	if p == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, "sign in to continue")
	}
	if err := s.auth.ChangePassword(ctx, p.UserID, req.GetCurrentPassword(), req.GetNewPassword(), ""); err != nil {
		return nil, authError("change password", err)
	}
	clearSiteData(ctx)
	setSessionCookie(ctx, "", time.Time{}, false)
	s.auth.LogAccess(ctx, auth.AccessEvent{Event: auth.EventPasswordChanged, Outcome: auth.OutcomeOK, UserID: p.UserID, Username: p.Username, RequestID: requestID})
	return &athenav1.ChangePasswordResponse{}, nil
}

// AccessUserMessage converts a user for the API and the CLI.
func AccessUserMessage(u auth.User, now time.Time) *athenav1.AccessUser {
	out := &athenav1.AccessUser{Username: u.Username, DisplayName: u.DisplayName, Owner: u.Owner, Disabled: u.Disabled, Locked: u.Locked(now), Roles: u.Roles, State: u.State}
	if u.LastLoginAt != nil {
		out.LastLoginAt = u.LastLoginAt.UTC().Format(time.RFC3339)
	}
	return out
}

// AssignmentMessage converts an Assignment for the API and the CLI.
func AssignmentMessage(a auth.Assignment) *athenav1.Assignment {
	out := &athenav1.Assignment{Id: a.ID, Role: a.RoleName, Revision: a.Revision, Scope: scopeMessage(a.Scope)}
	if a.ActiveFrom != nil {
		out.ActiveFrom = a.ActiveFrom.Format("2006-01-02")
	}
	if a.ExpiresAt != nil {
		out.ExpiresAt = a.ExpiresAt.Format("2006-01-02")
	}
	return out
}

func scopeMessage(scope auth.Scope) map[string]*athenav1.ScopeValues {
	if len(scope) == 0 {
		return nil
	}
	out := map[string]*athenav1.ScopeValues{}
	for dimension, values := range scope {
		out[dimension] = &athenav1.ScopeValues{Values: append([]string{}, values...)}
	}
	return out
}

func formatDay(at *time.Time) string {
	if at == nil {
		return ""
	}
	return at.Format("2006-01-02")
}

// AssignmentFromMessage converts an API Assignment. Dates must be whole
// days in YYYY-MM-DD form; empty means unbounded.
func AssignmentFromMessage(m *athenav1.Assignment) (auth.AssignmentInput, error) {
	in := auth.AssignmentInput{ID: m.GetId(), Role: m.GetRole(), ExpectedRevision: m.GetRevision()}
	if len(m.GetScope()) > 0 {
		in.Scope = auth.Scope{}
		for dimension, values := range m.GetScope() {
			in.Scope[dimension] = append([]string{}, values.GetValues()...)
		}
	}
	for _, dated := range []struct {
		text string
		name string
		set  func(time.Time)
	}{
		{m.GetActiveFrom(), "active_from", func(at time.Time) { in.ActiveFrom = &at }},
		{m.GetExpiresAt(), "expires_at", func(at time.Time) { in.ExpiresAt = &at }},
	} {
		if dated.text == "" {
			continue
		}
		at, err := time.Parse("2006-01-02", dated.text)
		if err != nil {
			return auth.AssignmentInput{}, fmt.Errorf("%s: use YYYY-MM-DD", dated.name)
		}
		dated.set(at)
	}
	return in, nil
}

func (s authService) ListUsers(ctx context.Context, _ *athenav1.ListUsersRequest) (*athenav1.ListUsersResponse, error) {
	users, err := s.auth.ListUsers(ctx)
	if err != nil {
		return nil, authError("list users", err)
	}
	res := &athenav1.ListUsersResponse{}
	now := time.Now()
	for _, u := range users {
		res.Users = append(res.Users, AccessUserMessage(u, now))
	}
	return res, nil
}

// logAdmin records an access-management change made by the caller.
func (s authService) logAdmin(ctx context.Context, event, requestID, subject string) {
	e := auth.AccessEvent{Event: event, Outcome: auth.OutcomeOK, RequestID: requestID, Detail: subject}
	if p := auth.PrincipalFrom(ctx); p != nil {
		e.UserID, e.Username = p.UserID, p.Username
	}
	s.auth.LogAccess(ctx, e)
}

func (s authService) CreateUser(ctx context.Context, _ *athenav1.CreateUserRequest) (res *athenav1.CreateUserResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "create_user", "")
	defer func() { end(err) }()
	// Administrator-set passwords are removed (ADR-0017): creating a user
	// now returns a one-time Challenge the current response cannot carry.
	// Invite-only creation arrives with InviteUser in task 1.9; until then
	// the athena command line invites users through the auth store directly.
	err = connect.NewError(connect.CodeUnimplemented, "user creation is invite-only; invite users with the athena command line")
	return nil, err
}

func (s authService) UpdateUser(ctx context.Context, req *athenav1.UpdateUserRequest) (res *athenav1.UpdateUserResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "update_user", "")
	defer func() { end(err) }()
	update := auth.UserUpdate{DisplayName: req.DisplayName, Disabled: req.Disabled, Unlock: req.GetUnlock()}
	if req.Roles != nil {
		names := req.GetRoles().GetNames()
		update.Roles = &names
	}
	user, err := s.auth.UpdateUser(ctx, req.GetUsername(), update)
	if err != nil {
		return nil, authError("update user", err)
	}
	s.logAdmin(ctx, auth.EventUserUpdated, requestID, "user "+user.Username)
	return &athenav1.UpdateUserResponse{User: AccessUserMessage(user, time.Now())}, nil
}

func (s authService) ResetPassword(ctx context.Context, _ *athenav1.ResetPasswordRequest) (res *athenav1.ResetPasswordResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "reset_password", "")
	defer func() { end(err) }()
	// Administrator-set passwords are removed (ADR-0017): the request
	// carries no password anymore, so the method always fails.
	err = connect.NewError(connect.CodeUnimplemented, "administrator-set passwords are removed; start a reset instead")
	return nil, err
}

func (s authService) InviteUser(ctx context.Context, req *athenav1.InviteUserRequest) (res *athenav1.InviteUserResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "invite_user", "")
	defer func() { end(err) }()
	user, challenge, err := s.auth.CreateUser(ctx, req.GetUsername(), req.GetDisplayName(), false, requestID)
	if err != nil {
		return nil, authError("invite user", err)
	}
	if len(req.GetRoles()) > 0 {
		roles := req.GetRoles()
		if user, err = s.auth.UpdateUser(ctx, user.Username, auth.UserUpdate{Roles: &roles}); err != nil {
			return nil, authError("invite user", err)
		}
	}
	s.logAdmin(ctx, auth.EventUserCreated, requestID, "user "+user.Username)
	return &athenav1.InviteUserResponse{User: AccessUserMessage(user, time.Now()), Challenge: challenge}, nil
}

func (s authService) CompleteChallenge(ctx context.Context, req *athenav1.CompleteChallengeRequest) (res *athenav1.CompleteChallengeResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "complete_challenge", "")
	defer func() { end(err) }()
	result, err := s.auth.CompleteChallengeWithOptions(ctx, req.GetUsername(), req.GetPurpose(), req.GetChallenge(), req.GetNewPassword(), requestID, sessionOptions(ctx, false))
	if err != nil {
		return nil, authError("complete sign-in", err)
	}
	setSessionCookie(ctx, result.Token, result.ExpiresAt, result.Principal.Remembered)
	return &athenav1.CompleteChallengeResponse{Session: sessionInfo(result.Principal, result.Token)}, nil
}

func (s authService) ResendInvite(ctx context.Context, req *athenav1.ResendInviteRequest) (res *athenav1.ResendInviteResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "resend_invite", "")
	defer func() { end(err) }()
	user, challenge, err := s.auth.ResendInvite(ctx, req.GetUsername(), requestID)
	if err != nil {
		return nil, authError("resend invite", err)
	}
	return &athenav1.ResendInviteResponse{User: AccessUserMessage(user, time.Now()), Challenge: challenge}, nil
}

func (s authService) StartReset(ctx context.Context, req *athenav1.StartResetRequest) (res *athenav1.StartResetResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "start_reset", "")
	defer func() { end(err) }()
	user, challenge, err := s.auth.StartReset(ctx, req.GetUsername(), req.GetActorPassword(), requestID)
	if err != nil {
		return nil, authError("start reset", err)
	}
	s.logAdmin(ctx, auth.EventPasswordReset, requestID, "user "+user.Username)
	return &athenav1.StartResetResponse{User: AccessUserMessage(user, time.Now()), Challenge: challenge}, nil
}

func (s authService) ListAssignments(ctx context.Context, req *athenav1.ListAssignmentsRequest) (*athenav1.ListAssignmentsResponse, error) {
	assignments, err := s.auth.ListAssignments(ctx, req.GetUsername())
	if err != nil {
		return nil, authError("list assignments", err)
	}
	res := &athenav1.ListAssignmentsResponse{}
	for _, a := range assignments {
		res.Assignments = append(res.Assignments, AssignmentMessage(a))
	}
	return res, nil
}

func (s authService) SaveAssignment(ctx context.Context, req *athenav1.SaveAssignmentRequest) (res *athenav1.SaveAssignmentResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "save_assignment", "")
	defer func() { end(err) }()
	if req.GetAssignment() == nil {
		return nil, invalid("assignment is required")
	}
	in, err := AssignmentFromMessage(req.GetAssignment())
	if err != nil {
		return nil, invalid(err.Error())
	}
	out, err := s.auth.SaveAssignment(ctx, req.GetUsername(), in)
	if err != nil {
		return nil, authError("save assignment", err)
	}
	s.logAdmin(ctx, auth.EventUserUpdated, requestID, "assignment saved for "+req.GetUsername())
	return &athenav1.SaveAssignmentResponse{Assignment: AssignmentMessage(out)}, nil
}

func (s authService) RevokeAssignment(ctx context.Context, req *athenav1.RevokeAssignmentRequest) (res *athenav1.RevokeAssignmentResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "revoke_assignment", "")
	defer func() { end(err) }()
	if err := s.auth.RevokeAssignment(ctx, req.GetId()); err != nil {
		return nil, authError("revoke assignment", err)
	}
	s.logAdmin(ctx, auth.EventUserUpdated, requestID, "assignment revoked "+req.GetId())
	return &athenav1.RevokeAssignmentResponse{}, nil
}

func (s authService) ExplainAccess(ctx context.Context, req *athenav1.ExplainAccessRequest) (res *athenav1.ExplainAccessResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "explain_access", "")
	defer func() { end(err) }()
	if !auth.ValidAction(req.GetAction()) {
		return nil, invalid(fmt.Sprintf("unknown action %q", req.GetAction()))
	}
	recordType, err := loadRecordTypeFrom(ctx, s.auth.DB, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	var row map[string]string
	truth := map[string]bool{}
	if req.GetRecordId() != "" {
		record, err := loadRecord(ctx, s.auth.DB, recordType, req.GetRecordId(), "")
		if err != nil {
			return nil, err
		}
		row = scopeRow(recordType, record)
		conditions, err := grantConditions(ctx, s.auth, req.GetUsername(), req.GetRecordTypeKey(), req.GetAction())
		if err != nil {
			return nil, authError("explain access", err)
		}
		if truth, err = evalGrantConditions(ctx, s.auth.DB, recordType, record, req.GetRecordId(), req.GetUsername(), conditions); err != nil {
			return nil, err
		}
	}
	out, err := s.auth.ExplainAccess(ctx, req.GetUsername(), req.GetAction(), req.GetRecordTypeKey(), row, truth, time.Now())
	if err != nil {
		return nil, authError("explain access", err)
	}
	res = &athenav1.ExplainAccessResponse{Allowed: out.Allowed}
	for _, v := range out.Assignments {
		res.Assignments = append(res.Assignments, &athenav1.AssignmentExplanation{AssignmentId: v.AssignmentID, Role: v.Role, Matched: v.Matched, Reasons: v.Reasons, SeparateDutiesConflict: v.SeparateDutiesConflict})
	}
	return res, nil
}

// grantConditions lists the distinct row conditions of the user's Grants
// covering the action, for evaluation against one Record.
func grantConditions(ctx context.Context, svc *auth.Service, username, recordType, action string) ([]string, error) {
	assignments, err := svc.ListAssignments(ctx, username)
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	for _, a := range assignments {
		for _, g := range a.Grants {
			if g.RecordType != recordType && g.RecordType != auth.AllRecordTypes {
				continue
			}
			if !g.Actions[action] || g.Condition == "" || seen[g.Condition] {
				continue
			}
			seen[g.Condition] = true
			out = append(out, g.Condition)
		}
	}
	return out, nil
}

// evalGrantConditions evaluates row conditions against one Record, binding
// user.name to the explained user. A condition that does not compile or is
// not true-or-false counts as false, matching the list path, which drops
// disjuncts it cannot compile.
func evalGrantConditions(ctx context.Context, q querier, d recordTypeDefinition, record *athenav1.Record, recordID, username string, conditions []string) (map[string]bool, error) {
	truth := map[string]bool{}
	if len(conditions) == 0 {
		return truth, nil
	}
	environment, err := newExprEnvironment(ctx, q, d, nil)
	if err != nil {
		return nil, err
	}
	plan := &exprPlan{exprEnvironment: environment, def: d, rollupOf: map[string]bool{}}
	compiled := map[string]*expr.Program{}
	for _, condition := range conditions {
		program, err := environment.env.Compile(bindUser(condition, username))
		if err != nil || program.Result != expr.Bool {
			continue
		}
		compiled[condition] = program
		for _, dep := range program.Deps {
			for _, child := range environment.children {
				if len(dep) > len(child.Name)+5 && dep[:len(child.Name)+5] == "doc."+child.Name+"." {
					plan.usesChildren, plan.rollupOf[child.Name] = true, true
				}
			}
		}
	}
	values, err := recordArgs(d, record)
	if err != nil {
		return nil, err
	}
	children, err := loadRollupChildren(ctx, q, plan, recordID)
	if err != nil {
		return nil, err
	}
	e, err := newEvaluation(plan, values, systemValues(record, d), children)
	if err != nil {
		return nil, err
	}
	for condition, program := range compiled {
		value, err := program.Eval(e.scope)
		ok, _ := value.(bool)
		truth[condition] = err == nil && ok
	}
	return truth, nil
}

// scopeRow renders a Record's text fields for Scope matching. Only text
// fields participate: Scope dimensions compare as strings, and any other
// field behaves as absent, matching nothing.
func scopeRow(d recordTypeDefinition, record *athenav1.Record) map[string]string {
	row := map[string]string{}
	for _, field := range d.Fields {
		if value, ok := record.GetValues()[field.Name].GetKind().(*athenav1.FieldValue_StringValue); ok {
			row[field.Name] = value.StringValue
		}
	}
	return row
}

func (s authService) ListSessions(ctx context.Context, req *athenav1.ListSessionsRequest) (res *athenav1.ListSessionsResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "list_sessions", "")
	defer func() { end(err) }()
	p := auth.PrincipalFrom(ctx)
	if p == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, "sign in to continue")
	}
	target := req.GetUsername()
	if target == "" {
		target = p.Username
	}
	if !equalUsername(target, p.Username) && !p.HasSystem(auth.PermissionAccessSessions) {
		return nil, connect.NewError(connect.CodePermissionDenied, "you do not have permission to do this")
	}
	sessions, err := s.auth.ListSessions(ctx, target)
	if err != nil {
		return nil, authError("list sessions", err)
	}
	res = &athenav1.ListSessionsResponse{}
	for _, session := range sessions {
		res.Sessions = append(res.Sessions, SessionMessage(session, session.ID == p.SessionID))
	}
	return res, nil
}

func (s authService) RevokeSession(ctx context.Context, req *athenav1.RevokeSessionRequest) (res *athenav1.RevokeSessionResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "revoke_session", "")
	defer func() { end(err) }()
	p := auth.PrincipalFrom(ctx)
	if p == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, "sign in to continue")
	}
	if err := s.auth.RevokeSession(ctx, p.UserID, req.GetId(), p.HasSystem(auth.PermissionAccessSessions)); err != nil {
		return nil, authError("revoke session", err)
	}
	s.logAdmin(ctx, auth.EventLogout, requestID, "session revoked "+req.GetId())
	return &athenav1.RevokeSessionResponse{}, nil
}

// SessionMessage converts a session for the API and the CLI. current marks
// the session making the call.
func SessionMessage(s auth.Session, current bool) *athenav1.Session {
	return &athenav1.Session{Id: s.ID, CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339), LastSeenAt: s.LastSeenAt.UTC().Format(time.RFC3339),
		ExpiresAt: s.ExpiresAt.UTC().Format(time.RFC3339), Device: s.Device, Current: current}
}

func equalUsername(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func (s authService) ListRoleHolders(ctx context.Context, req *athenav1.ListRoleHoldersRequest) (*athenav1.ListRoleHoldersResponse, error) {
	holders, err := s.auth.ListRoleHolders(ctx, req.GetRole(), req.GetRecordTypeKey(), req.GetAction())
	if err != nil {
		return nil, authError("list role holders", err)
	}
	res := &athenav1.ListRoleHoldersResponse{}
	for _, h := range holders {
		res.Holders = append(res.Holders, &athenav1.RoleHolder{Username: h.Username, DisplayName: h.DisplayName, State: h.State,
			Usable: h.Usable, Scope: scopeMessage(h.Scope), ActiveFrom: formatDay(h.ActiveFrom), ExpiresAt: formatDay(h.ExpiresAt)})
	}
	return res, nil
}

func (s authService) TransferOwnership(ctx context.Context, req *athenav1.TransferOwnershipRequest) (res *athenav1.TransferOwnershipResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "transfer_ownership", "")
	defer func() { end(err) }()
	user, err := s.auth.TransferOwnership(ctx, req.GetUsername(), req.GetActorPassword(), req.GetRelinquish(), requestID)
	if err != nil {
		return nil, authError("transfer ownership", err)
	}
	s.logAdmin(ctx, auth.EventUserUpdated, requestID, "ownership transferred to "+user.Username)
	return &athenav1.TransferOwnershipResponse{User: AccessUserMessage(user, time.Now())}, nil
}

// RoleMessage converts a Role for the API and the CLI.
func RoleMessage(r auth.Role) *athenav1.Role {
	out := &athenav1.Role{Name: r.Name, Description: r.Description, SystemPermissions: r.SystemPermissions}
	for _, g := range r.Grants {
		grant := &athenav1.Grant{RecordTypeKey: g.RecordType, PermissionLevel: int32(g.Level), Condition: g.Condition, SeparateDuties: g.SeparateDuties}
		for _, action := range auth.Actions {
			if g.Actions[action] {
				grant.Actions = append(grant.Actions, action)
			}
		}
		out.Grants = append(out.Grants, grant)
	}
	return out
}

// RoleFromMessage converts an API Role.
func RoleFromMessage(m *athenav1.Role) auth.Role {
	r := auth.Role{Name: m.GetName(), Description: m.GetDescription(), SystemPermissions: m.GetSystemPermissions()}
	for _, g := range m.GetGrants() {
		grant := auth.Grant{RecordType: g.GetRecordTypeKey(), Level: int(g.GetPermissionLevel()), Condition: g.GetCondition(), SeparateDuties: g.GetSeparateDuties(), Actions: map[string]bool{}}
		for _, action := range g.GetActions() {
			grant.Actions[action] = true
		}
		r.Grants = append(r.Grants, grant)
	}
	return r
}

func (s authService) ListRoles(ctx context.Context, _ *athenav1.ListRolesRequest) (*athenav1.ListRolesResponse, error) {
	roles, err := s.auth.ListRoles(ctx)
	if err != nil {
		return nil, authError("list roles", err)
	}
	res := &athenav1.ListRolesResponse{}
	for _, r := range roles {
		res.Roles = append(res.Roles, RoleMessage(r))
	}
	return res, nil
}

func (s authService) SaveRole(ctx context.Context, req *athenav1.SaveRoleRequest) (res *athenav1.SaveRoleResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "save_role", "")
	defer func() { end(err) }()
	if req.GetRole() == nil {
		return nil, invalid("role is required")
	}
	role := RoleFromMessage(req.GetRole())
	if err := checkGrants(ctx, s.auth, role); err != nil {
		return nil, err
	}
	if err := s.auth.SaveRole(ctx, role); err != nil {
		return nil, authError("save role", err)
	}
	s.logAdmin(ctx, auth.EventRoleSaved, requestID, "role "+role.Name)
	return &athenav1.SaveRoleResponse{Role: RoleMessage(role)}, nil
}

// checkGrants refuses a Grant for a Record Type that does not exist or whose
// row condition does not compile against it, so a mistake shows when the Role
// is saved rather than as an empty list later.
func checkGrants(ctx context.Context, svc *auth.Service, role auth.Role) error {
	for _, g := range role.Grants {
		if g.RecordType == auth.AllRecordTypes {
			continue
		}
		recordType, _, err := loadRecordType(ctx, svc.DB, g.RecordType)
		if err != nil {
			return internalError("load Record Type", err)
		}
		if recordType == nil {
			return invalid("Grant for " + g.RecordType + ": no such Record Type")
		}
		if g.Condition != "" {
			if _, err := filterPredicate(ctx, svc.DB, *recordType, bindUser(g.Condition, "user")); err != nil {
				return invalid("Grant for " + g.RecordType + ": " + connectMessage(err))
			}
		}
	}
	return nil
}

func (s authService) DeleteRole(ctx context.Context, req *athenav1.DeleteRoleRequest) (res *athenav1.DeleteRoleResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "delete_role", "")
	defer func() { end(err) }()
	if err := s.auth.DeleteRole(ctx, req.GetName()); err != nil {
		return nil, authError("delete role", err)
	}
	s.logAdmin(ctx, auth.EventRoleDeleted, requestID, "role "+req.GetName())
	return &athenav1.DeleteRoleResponse{}, nil
}

// AccessEventMessage converts an Access Log row for the API and the CLI.
func AccessEventMessage(r auth.AccessRow) *athenav1.AccessEvent {
	return &athenav1.AccessEvent{Id: r.ID, At: r.At.UTC().Format(time.RFC3339), Event: r.Event, Outcome: r.Outcome, Username: r.Username,
		Procedure: r.Procedure, RecordTypeKey: r.RecordType, Action: r.Action, RequestId: r.RequestID, Detail: r.Detail}
}

func (s authService) ListAccessLog(ctx context.Context, req *athenav1.ListAccessLogRequest) (*athenav1.ListAccessLogResponse, error) {
	size := int(req.GetPageSize())
	if size <= 0 {
		size = defaultPageSize
	}
	rows, err := s.auth.ListAccessLog(ctx, min(size, maxPageSize), req.GetBeforeId(), req.GetEvent())
	if err != nil {
		return nil, authError("read access log", err)
	}
	res := &athenav1.ListAccessLogResponse{}
	for _, r := range rows {
		res.Events = append(res.Events, AccessEventMessage(r))
	}
	return res, nil
}

// failureError attaches an AuthFailure detail to an error.
func failureError(code connect.Code, message string, failure *athenav1.AuthFailure) error {
	err := connect.NewError(code, message)
	if detail, derr := connectproto.NewErrorDetail(failure); derr == nil {
		err = err.WithDetail(detail)
	}
	return err
}

// isConnectionError reports a failure to reach the database, as opposed to a
// failure of a statement.
func isConnectionError(err error) bool {
	var netErr net.Error
	return errors.Is(err, driver.ErrBadConn) || errors.Is(err, mysql.ErrInvalidConn) || errors.As(err, &netErr)
}
