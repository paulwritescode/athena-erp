package server

import (
	"context"
	"errors"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"

	"connectrpc.com/connect/v2"
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
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		return connect.NewError(connect.CodeUnauthenticated, err.Error())
	case errors.Is(err, auth.ErrUnauthenticated):
		return connect.NewError(connect.CodeUnauthenticated, err.Error())
	case errors.Is(err, auth.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err.Error())
	case errors.Is(err, auth.ErrExists):
		return connect.NewError(connect.CodeAlreadyExists, err.Error())
	case errors.Is(err, auth.ErrLastOwner):
		return connect.NewError(connect.CodeFailedPrecondition, err.Error())
	case errors.Is(err, auth.ErrInvalid):
		return connect.NewError(connect.CodeInvalidArgument, err.Error())
	}
	return internalError(op, err)
}

func sessionInfo(p *auth.Principal, token string) *athenav1.SessionInfo {
	if p == nil {
		return &athenav1.SessionInfo{}
	}
	info := &athenav1.SessionInfo{Authenticated: true, Username: p.Username, DisplayName: p.DisplayName, Owner: p.Owner, CsrfToken: auth.CSRFToken(token)}
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
	result, err := s.auth.Login(ctx, req.GetUsername(), req.GetPassword(), requestID)
	if err != nil {
		return nil, authError("sign in", err)
	}
	setSessionCookie(ctx, result.Token, result.ExpiresAt)
	return &athenav1.LoginResponse{Session: sessionInfo(result.Principal, result.Token)}, nil
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
	setSessionCookie(ctx, "", time.Time{})
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
	if err := s.auth.ChangePassword(ctx, p.UserID, req.GetCurrentPassword(), req.GetNewPassword(), p.SessionID); err != nil {
		return nil, authError("change password", err)
	}
	s.auth.LogAccess(ctx, auth.AccessEvent{Event: auth.EventPasswordChanged, Outcome: auth.OutcomeOK, UserID: p.UserID, Username: p.Username, RequestID: requestID})
	return &athenav1.ChangePasswordResponse{}, nil
}

// AccessUserMessage converts a user for the API and the CLI.
func AccessUserMessage(u auth.User, now time.Time) *athenav1.AccessUser {
	out := &athenav1.AccessUser{Username: u.Username, DisplayName: u.DisplayName, Owner: u.Owner, Disabled: u.Disabled, Locked: u.Locked(now), Roles: u.Roles}
	if u.LastLoginAt != nil {
		out.LastLoginAt = u.LastLoginAt.UTC().Format(time.RFC3339)
	}
	return out
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

func (s authService) CreateUser(ctx context.Context, req *athenav1.CreateUserRequest) (res *athenav1.CreateUserResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "create_user", "")
	defer func() { end(err) }()
	user, err := s.auth.CreateUser(ctx, req.GetUsername(), req.GetDisplayName(), req.GetPassword(), false)
	if err != nil {
		return nil, authError("create user", err)
	}
	if len(req.GetRoles()) > 0 {
		roles := req.GetRoles()
		if user, err = s.auth.UpdateUser(ctx, user.Username, auth.UserUpdate{Roles: &roles}); err != nil {
			return nil, authError("create user", err)
		}
	}
	s.logAdmin(ctx, auth.EventUserCreated, requestID, "user "+user.Username)
	return &athenav1.CreateUserResponse{User: AccessUserMessage(user, time.Now())}, nil
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

func (s authService) ResetPassword(ctx context.Context, req *athenav1.ResetPasswordRequest) (res *athenav1.ResetPasswordResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "reset_password", "")
	defer func() { end(err) }()
	user, err := s.auth.ResetPassword(ctx, req.GetUsername(), req.GetNewPassword())
	if err != nil {
		return nil, authError("reset password", err)
	}
	s.logAdmin(ctx, auth.EventPasswordReset, requestID, "user "+user.Username)
	return &athenav1.ResetPasswordResponse{}, nil
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
