package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/auth"

	"connectrpc.com/connect/v2"
)

const (
	sessionCookieName = "athena_session"
	csrfHeader        = "X-CSRF-Token"
)

type policyKind int

const (
	// policyPublic needs no session. The handler may still read one.
	policyPublic policyKind = iota
	// policySession needs a signed-in user and a valid CSRF token.
	policySession
	// policySystem needs a system permission.
	policySystem
	// policyRecord needs a Grant for action on the Record Type the request names.
	policyRecord
)

type policy struct {
	kind       policyKind
	permission string // policySystem
	action     string // policyRecord
}

// procedurePolicies is the permission gate (ADR-0014). A procedure without an
// entry is denied, so a service added later stays closed until someone
// decides who may call it. TestEveryProcedureHasAPolicy checks the table
// against the generated services.
var procedurePolicies = map[string]policy{
	athenav1connect.SystemServiceGetSystemInfoProcedure:  {kind: policyPublic},
	athenav1connect.SystemServiceVerifyAuditLogProcedure: {kind: policySystem, permission: auth.PermissionAudit},

	athenav1connect.AuthServiceLoginProcedure:          {kind: policyPublic},
	athenav1connect.AuthServiceGetSessionProcedure:     {kind: policyPublic},
	athenav1connect.AuthServiceLogoutProcedure:         {kind: policySession},
	athenav1connect.AuthServiceChangePasswordProcedure: {kind: policySession},
	athenav1connect.AuthServiceListUsersProcedure:      {kind: policySystem, permission: auth.PermissionAccess},
	athenav1connect.AuthServiceCreateUserProcedure:     {kind: policySystem, permission: auth.PermissionAccess},
	athenav1connect.AuthServiceUpdateUserProcedure:     {kind: policySystem, permission: auth.PermissionAccess},
	athenav1connect.AuthServiceResetPasswordProcedure:  {kind: policySystem, permission: auth.PermissionAccess},
	athenav1connect.AuthServiceListRolesProcedure:      {kind: policySystem, permission: auth.PermissionAccess},
	athenav1connect.AuthServiceSaveRoleProcedure:       {kind: policySystem, permission: auth.PermissionAccess},
	athenav1connect.AuthServiceDeleteRoleProcedure:     {kind: policySystem, permission: auth.PermissionAccess},
	athenav1connect.AuthServiceListAccessLogProcedure:  {kind: policySystem, permission: auth.PermissionAudit},

	athenav1connect.FileServiceListFilesProcedure:        {kind: policySystem, permission: auth.PermissionFilesRead},
	athenav1connect.FileServiceGetFileProcedure:          {kind: policySystem, permission: auth.PermissionFilesRead},
	athenav1connect.FileServiceListFileVersionsProcedure: {kind: policySystem, permission: auth.PermissionFilesRead},
	athenav1connect.FileServicePutFileProcedure:          {kind: policySystem, permission: auth.PermissionFilesWrite},
	athenav1connect.FileServiceMoveFileProcedure:         {kind: policySystem, permission: auth.PermissionFilesWrite},
	athenav1connect.FileServiceDeleteFileProcedure:       {kind: policySystem, permission: auth.PermissionFilesWrite},
	athenav1connect.RuleServiceListRulesProcedure:        {kind: policySystem, permission: auth.PermissionRulesRead},
	athenav1connect.RuleServiceGetRuleProcedure:          {kind: policySystem, permission: auth.PermissionRulesRead},
	athenav1connect.RuleServiceListRuleVersionsProcedure: {kind: policySystem, permission: auth.PermissionRulesRead},
	athenav1connect.RuleServiceListRuleRunsProcedure:     {kind: policySystem, permission: auth.PermissionRulesRead},
	athenav1connect.RuleServiceSaveRuleProcedure:         {kind: policySystem, permission: auth.PermissionRulesWrite},
	athenav1connect.RuleServiceDeleteRuleProcedure:       {kind: policySystem, permission: auth.PermissionRulesWrite},
	athenav1connect.RuleServiceSimulateRuleProcedure:     {kind: policySystem, permission: auth.PermissionRulesWrite},
	// The handler filters the list to the Record Types the user may read.
	athenav1connect.RecordTypeServiceListRecordTypesProcedure:    {kind: policySession},
	athenav1connect.RecordTypeServicePlanRecordTypeSaveProcedure: {kind: policySystem, permission: auth.PermissionBuild},
	athenav1connect.RecordTypeServiceSaveRecordTypeProcedure:     {kind: policySystem, permission: auth.PermissionBuild},
	athenav1connect.RecordTypeServiceDeleteRecordTypeProcedure:   {kind: policySystem, permission: auth.PermissionBuild},

	athenav1connect.RecordServiceListRecordsProcedure:        {kind: policyRecord, action: auth.ActionRead},
	athenav1connect.RecordServiceGetRecordProcedure:          {kind: policyRecord, action: auth.ActionRead},
	athenav1connect.RecordServiceLookupRecordsProcedure:      {kind: policyRecord, action: auth.ActionRead},
	athenav1connect.RecordServicePreviewRecordProcedure:      {kind: policyRecord, action: auth.ActionRead},
	athenav1connect.RecordServiceEvaluateExpressionProcedure: {kind: policyRecord, action: auth.ActionRead},
	athenav1connect.RecordServiceCreateRecordProcedure:       {kind: policyRecord, action: auth.ActionCreate},
	athenav1connect.RecordServiceUpdateRecordProcedure:       {kind: policyRecord, action: auth.ActionWrite},
	athenav1connect.RecordServiceDeleteRecordProcedure:       {kind: policyRecord, action: auth.ActionDelete},
	athenav1connect.RecordServiceSubmitRecordProcedure:       {kind: policyRecord, action: auth.ActionSubmit},
	athenav1connect.RecordServiceCancelRecordProcedure:       {kind: policyRecord, action: auth.ActionCancel},
	athenav1connect.RecordServiceAmendRecordProcedure:        {kind: policyRecord, action: auth.ActionAmend},
	athenav1connect.RecordServiceListRecordHistoryProcedure:  {kind: policyRecord, action: auth.ActionAudit},
	athenav1connect.RecordServiceListRecordVersionsProcedure: {kind: policyRecord, action: auth.ActionAudit},
	athenav1connect.RecordServiceListRecordActivityProcedure: {kind: policyRecord, action: auth.ActionAudit},
}

type sessionTokenKey struct{}

// sessionTokenFrom returns the session token the request carried, or "".
func sessionTokenFrom(ctx context.Context) string {
	token, _ := ctx.Value(sessionTokenKey{}).(string)
	return token
}

// recordTypeKeyed is implemented by every request that names a Record Type.
type recordTypeKeyed interface{ GetRecordTypeKey() string }

// gateInterceptor authenticates the session cookie, checks the CSRF token and
// applies procedurePolicies before any handler runs. It also writes denials
// to the Access Log. It must come after requestInterceptor.
func gateInterceptor(svc *auth.Service) connect.ServerInterceptor {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			if svc == nil {
				return connect.NewError(connect.CodeUnavailable, "the database is not configured")
			}
			requestID, _ := requestFrom(ctx)
			deny := func(p *auth.Principal, recordType, action, detail string, code connect.Code, message string) error {
				event := auth.AccessEvent{Event: auth.EventDenied, Outcome: auth.OutcomeDenied, Procedure: spec.Procedure, RecordType: recordType, Action: action, RequestID: requestID, Detail: detail}
				if p != nil {
					event.UserID, event.Username = p.UserID, p.Username
				}
				svc.LogAccess(ctx, event)
				return connect.NewError(code, message)
			}
			pol, known := procedurePolicies[spec.Procedure]
			if !known {
				return deny(nil, "", "", "no policy", connect.CodePermissionDenied, "this operation is not allowed")
			}
			token, sent := "", ""
			if info, ok := connect.CallInfoForServerContext(ctx); ok {
				token = cookieValue(info.RequestHeader(), sessionCookieName)
				sent = info.RequestHeader().Get(csrfHeader)
			}
			var principal *auth.Principal
			if token != "" {
				p, err := svc.Authenticate(ctx, token)
				switch {
				case err == nil:
					principal = p
				case !errors.Is(err, auth.ErrUnauthenticated):
					log.Printf("authenticate: session lookup failed")
					if isConnectionError(err) {
						return connect.NewError(connect.CodeUnavailable, "the database is not reachable")
					}
					return connect.NewError(connect.CodeInternal, "could not check the session")
				}
			}
			if pol.kind != policyPublic {
				if principal == nil {
					return connect.NewError(connect.CodeUnauthenticated, "sign in to continue")
				}
				if !auth.CheckCSRF(token, sent) {
					_ = deny(principal, "", "", "csrf", connect.CodePermissionDenied, "")
					return failureError(connect.CodePermissionDenied, "the request did not carry a valid CSRF token; reload the page", &athenav1.AuthFailure{Reason: "csrf"})
				}
			}
			if pol.kind == policySystem && !principal.HasSystem(pol.permission) {
				return deny(principal, "", pol.permission, "system permission", connect.CodePermissionDenied, "you do not have permission to do this")
			}
			if principal != nil {
				ctx = auth.WithPrincipal(ctx, principal)
			}
			ctx = context.WithValue(ctx, sessionTokenKey{}, token)
			if pol.kind == policyRecord {
				stream = gatedStream{ServerStream: stream, check: func(msg any) error {
					keyed, ok := msg.(recordTypeKeyed)
					if !ok {
						return deny(principal, "", pol.action, "no Record Type in request", connect.CodePermissionDenied, "this operation is not allowed")
					}
					key := keyed.GetRecordTypeKey()
					if !principal.Can(key, pol.action) {
						return deny(principal, key, pol.action, "no grant", connect.CodePermissionDenied, fmt.Sprintf("you do not have permission to %s these records", pol.action))
					}
					return nil
				}}
			}
			return next(ctx, spec, stream)
		}
	}
}

// gatedStream runs check on every request message before the handler sees it.
type gatedStream struct {
	connect.ServerStream
	check func(msg any) error
}

func (s gatedStream) Receive(msg any) error {
	if err := s.ServerStream.Receive(msg); err != nil {
		return err
	}
	return s.check(msg)
}

func cookieValue(header *connect.Header, name string) string {
	cookie, err := (&http.Request{Header: http.Header{"Cookie": header.Values("Cookie")}}).Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// setSessionCookie sends the session token in an HttpOnly, SameSite=Strict
// cookie. Secure is set when the browser reached the server over HTTPS, which
// its Origin header shows; on plain loopback HTTP it cannot be.
func setSessionCookie(ctx context.Context, token string, expires time.Time) {
	info, ok := connect.CallInfoForServerContext(ctx)
	if !ok {
		return
	}
	maxAge := int(time.Until(expires).Seconds())
	if token == "" {
		maxAge = -1
	}
	cookie := &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: strings.HasPrefix(info.RequestHeader().Get("Origin"), "https://"), MaxAge: maxAge,
	}
	info.ResponseHeader().Add("Set-Cookie", cookie.String())
}

// securityHeaders wraps the whole HTTP handler. It sets content security
// headers and refuses a state-changing request whose Origin is another site,
// which is a second guard behind SameSite and the CSRF token.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if origin := r.Header.Get("Origin"); origin != "" && !sameOrigin(origin, r) {
				http.Error(w, "cross-origin request refused", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(origin string, r *http.Request) bool {
	for _, scheme := range []string{"http://", "https://"} {
		if host, ok := strings.CutPrefix(origin, scheme); ok {
			return host == r.Host
		}
	}
	return false
}
