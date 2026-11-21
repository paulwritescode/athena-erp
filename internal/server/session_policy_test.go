package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/auth"
	"connectrpc.com/connect/v2"
)

func TestSessionPolicySnapshots(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	f.role(t, "Builder", []string{auth.PermissionBuild})
	f.user(t, "builder", "Builder")
	f.user(t, "ordinary")
	if _, err := f.users.EnsureRoot(ctx, testPassword); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	svc := &auth.Service{DB: f.db, Now: func() time.Time { return now }}
	for _, tc := range []struct {
		name           string
		remember       bool
		idle, absolute time.Duration
	}{
		{"ordinary", false, 30 * time.Minute, 12 * time.Hour},
		{"ordinary", true, 30 * time.Minute, 168 * time.Hour},
		{"builder", true, 15 * time.Minute, 12 * time.Hour},
		{auth.RootUsername, true, 15 * time.Minute, 12 * time.Hour},
	} {
		res, err := svc.LoginWithOptions(ctx, tc.name, testPassword, newRequestID(), auth.SessionOptions{RememberDevice: tc.remember, Device: "Firefox"})
		if err != nil {
			t.Fatal(err)
		}
		if got := res.Principal.IdleExpiresAt.Sub(now); got != tc.idle || res.ExpiresAt.Sub(now) != tc.absolute || res.Principal.Remembered != (tc.remember && tc.name == "ordinary") {
			t.Fatalf("%s remember %v: idle %v absolute %v", tc.name, tc.remember, got, res.ExpiresAt.Sub(now))
		}
	}
	for key, values := range map[string][]int{auth.IdleMinutes: {4, 481}, auth.BuildIdleMinutes: {4, 481}, auth.AbsoluteHours: {0, 169}, auth.SessionWrites: {0}, auth.SessionReports: {-1}, auth.SessionExports: {10001}, "unknown": {1}} {
		for _, value := range values {
			if err := svc.SetSecurity(ctx, key, value); !errors.Is(err, auth.ErrInvalid) {
				t.Fatalf("accepted %s=%d: %v", key, value, err)
			}
		}
	}
	for key, value := range map[string]int{auth.IdleMinutes: 5, auth.AbsoluteHours: 1} {
		if err := svc.SetSecurity(ctx, key, value); err != nil {
			t.Fatal(err)
		}
	}
	old, err := svc.Login(ctx, "ordinary", testPassword, newRequestID())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetSecurity(ctx, auth.IdleMinutes, 480); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Minute)
	assertEnded := func(token, reason string) {
		t.Helper()
		_, err := svc.Authenticate(ctx, token)
		var ended *auth.SessionEnded
		if !errors.As(err, &ended) || ended.Reason != reason {
			t.Fatalf("session ended: %v, want %s", err, reason)
		}
	}
	assertEnded(old.Token, "idle")
	active, err := svc.ListSessions(ctx, "ordinary")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range active {
		if s.ID == old.Principal.SessionID {
			t.Fatal("expired session listed")
		}
	}
	newLogin, err := svc.Login(ctx, "ordinary", testPassword, newRequestID())
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	assertEnded(newLogin.Token, "absolute")
}

func TestStepUpAndCookieRotation(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	if _, err := f.users.EnsureRoot(ctx, testPassword); err != nil {
		t.Fatal(err)
	}
	f.role(t, "temporary", nil)
	b := f.browser(t)
	b.login(t, auth.RootUsername)
	first := b.cookie()
	b.login(t, auth.RootUsername)
	second := b.cookie()
	if first == second {
		t.Fatal("login reused the cookie")
	}
	if _, err := f.users.Authenticate(ctx, first); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("previous cookie survived login")
	}
	p, err := f.users.Authenticate(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("UPDATE athena_sessions SET step_up_at=? WHERE id=?", time.Now().UTC().Add(-16*time.Minute), p.SessionID); err != nil {
		t.Fatal(err)
	}
	tryDelete := func() error { _, err := b.auth.DeleteRole(ctx, &v1.DeleteRoleRequest{Name: "temporary"}); return err }
	err = tryDelete()
	requireCode(t, err, connect.CodeFailedPrecondition)
	if failureOf(t, err).GetReason() != "step_up_required" {
		t.Fatal("missing step-up reason")
	}
	_, err = b.auth.StepUp(ctx, &v1.StepUpRequest{Password: "wrong"})
	requireCode(t, err, connect.CodeUnauthenticated)
	requireCode(t, tryDelete(), connect.CodeFailedPrecondition)
	up, err := b.auth.StepUp(ctx, &v1.StepUpRequest{Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	if second == b.cookie() || *b.csrf == up.Session.CsrfToken {
		t.Fatal("step-up did not rotate credentials")
	}
	*b.csrf = up.Session.CsrfToken
	fresh, err := f.users.Authenticate(ctx, b.cookie())
	if err != nil {
		t.Fatal(err)
	}
	if fresh.SessionID == p.SessionID || fresh.RateID != p.RateID || !fresh.ExpiresAt.Equal(p.ExpiresAt) {
		t.Fatal("rotation changed the lifetime or rate budget, or reused the session ID")
	}
	if _, err := f.users.Authenticate(ctx, second); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("previous cookie survived step-up")
	}
	if err := tryDelete(); err != nil {
		t.Fatal(err)
	}
	rows, err := f.users.ListSessions(ctx, auth.RootUsername)
	if err != nil || len(rows) != 1 || rows[0].Device == "" {
		t.Fatalf("missing device or unexpected session count: %v", err)
	}
	for _, op := range []string{"logout", "change password"} {
		call, info := connect.NewClientContext(ctx)
		if op == "logout" {
			_, err = b.auth.Logout(call, &v1.LogoutRequest{})
		} else {
			b.login(t, auth.RootUsername)
			_, err = b.auth.ChangePassword(call, &v1.ChangePasswordRequest{CurrentPassword: testPassword, NewPassword: "a new local passphrase"})
		}
		if err != nil || info.ResponseHeader().Get("Clear-Site-Data") != `"cache", "cookies", "storage"` {
			t.Fatalf("%s clear-site-data: %v", op, err)
		}
	}
}

func TestSessionFailureDetails(t *testing.T) {
	f := newAccessFixture(t)
	f.user(t, "ordinary")
	b := f.browser(t)
	for _, tc := range []struct{ reason, assignment string }{
		{"idle", "last_seen_at=UTC_TIMESTAMP(3)-INTERVAL 31 MINUTE"},
		{"absolute", "expires_at=UTC_TIMESTAMP(3)"},
		{"revoked", "revoked_at=UTC_TIMESTAMP(3)"},
	} {
		b.login(t, "ordinary")
		if _, err := f.db.Exec("UPDATE athena_sessions SET " + tc.assignment); err != nil {
			t.Fatal(err)
		}
		_, err := b.auth.GetSession(context.Background(), &v1.GetSessionRequest{})
		requireCode(t, err, connect.CodeUnauthenticated)
		if failureOf(t, err).GetReason() != tc.reason {
			t.Fatalf("missing %s detail", tc.reason)
		}
	}
}

func TestInviteUserStepUpWhenRolesGranted(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	if _, err := f.users.EnsureRoot(ctx, testPassword); err != nil {
		t.Fatal(err)
	}
	f.role(t, "usher", nil)
	b := f.browser(t)
	b.login(t, auth.RootUsername)
	p, err := f.users.Authenticate(ctx, b.cookie())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("UPDATE athena_sessions SET step_up_at=? WHERE id=?", time.Now().UTC().Add(-16*time.Minute), p.SessionID); err != nil {
		t.Fatal(err)
	}
	// A bare invite carries no access, so it needs no fresh password proof.
	if _, err := b.auth.InviteUser(ctx, &v1.InviteUserRequest{Username: "bare"}); err != nil {
		t.Fatalf("bare invite: %v", err)
	}
	_, err = b.auth.InviteUser(ctx, &v1.InviteUserRequest{Username: "privileged", Roles: []string{"usher"}})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if failureOf(t, err).GetReason() != "step_up_required" {
		t.Fatal("missing step-up reason")
	}
	up, err := b.auth.StepUp(ctx, &v1.StepUpRequest{Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	*b.csrf = up.Session.CsrfToken
	if _, err := b.auth.InviteUser(ctx, &v1.InviteUserRequest{Username: "privileged", Roles: []string{"usher"}}); err != nil {
		t.Fatalf("invite after step-up: %v", err)
	}
}

func TestStepUpFailuresLockTheAccount(t *testing.T) {
	f := newAccessFixture(t)
	f.user(t, "bob")
	b := f.browser(t)
	b.login(t, "bob")
	ctx := context.Background()
	for i := 0; i < auth.MaxFailedAttempts; i++ {
		_, err := b.auth.StepUp(ctx, &v1.StepUpRequest{Password: "wrong password here"})
		requireCode(t, err, connect.CodeUnauthenticated)
		if msg := connectMessage(err); !strings.Contains(msg, "password") || strings.Contains(msg, "username") {
			t.Fatalf("step-up failure names more than the password: %q", msg)
		}
	}
	// The fifth failure locked the account: even the right password is
	// refused now, at sign-in and at step-up alike.
	_, err := f.browser(t).auth.Login(ctx, &v1.LoginRequest{Username: "bob", Password: testPassword})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if failureOf(t, err).GetReason() != "locked" {
		t.Fatal("missing locked reason")
	}
	_, err = b.auth.StepUp(ctx, &v1.StepUpRequest{Password: testPassword})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if failureOf(t, err).GetReason() != "locked" {
		t.Fatal("step-up missed the lockout")
	}
	if n := len(f.accessEvents(t, auth.EventStepUpFailed)); n != auth.MaxFailedAttempts+1 {
		t.Fatalf("step_up_failed rows = %d", n)
	}
	if n := len(f.accessEvents(t, auth.EventLockout)); n != 1 {
		t.Fatalf("lockout rows = %d", n)
	}
	if _, err := f.users.UpdateUser(ctx, "bob", auth.UserUpdate{Unlock: true}); err != nil {
		t.Fatal(err)
	}
	b.login(t, "bob")
	up, err := b.auth.StepUp(ctx, &v1.StepUpRequest{Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	*b.csrf = up.Session.CsrfToken
	if n := len(f.accessEvents(t, auth.EventStepUp)); n != 1 {
		t.Fatalf("step_up rows = %d", n)
	}
}

func TestPrivilegedActorPasswordFailuresCountTowardLockout(t *testing.T) {
	f := newAccessFixture(t)
	f.owner(t, "ada")
	f.user(t, "clerk")
	owner := f.browser(t)
	owner.login(t, "ada")
	ctx := context.Background()
	// Three wrong actor passwords on StartReset plus two on
	// TransferOwnership share one counter with sign-ins: the fifth locks.
	for i := 0; i < 3; i++ {
		_, err := owner.auth.StartReset(ctx, &v1.StartResetRequest{Username: "clerk", ActorPassword: "wrong password here"})
		requireCode(t, err, connect.CodeUnauthenticated)
	}
	for i := 0; i < 2; i++ {
		_, err := owner.auth.TransferOwnership(ctx, &v1.TransferOwnershipRequest{Username: "clerk", ActorPassword: "wrong password here"})
		requireCode(t, err, connect.CodeUnauthenticated)
		if msg := connectMessage(err); !strings.Contains(msg, "password") || strings.Contains(msg, "username") {
			t.Fatalf("transfer failure names more than the password: %q", msg)
		}
	}
	_, err := f.browser(t).auth.Login(ctx, &v1.LoginRequest{Username: "ada", Password: testPassword})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if failureOf(t, err).GetReason() != "locked" {
		t.Fatal("missing locked reason")
	}
	if n := len(f.accessEvents(t, auth.EventStepUpFailed)); n != auth.MaxFailedAttempts {
		t.Fatalf("step_up_failed rows = %d", n)
	}
}

func TestDestructiveProcedureCoverage(t *testing.T) {
	for proc := range procedurePolicies {
		_, method, _ := strings.Cut(strings.TrimPrefix(proc, "/"), "/")
		for _, verb := range []string{"Delete", "Cancel", "Revoke", "Discard", "Transfer", "StartReset", "Resend", "Invite", "Publish", "SetPeriodStatus", "SaveRecordType", "SaveRole", "SaveAssignment", "UpdateUser", "SaveRule", "MoveFile", "PutFile"} {
			if strings.HasPrefix(method, verb) && !destructiveProcedures[proc] {
				t.Errorf("%s has no step-up classification", proc)
			}
		}
	}
	for proc := range destructiveProcedures {
		if _, ok := procedurePolicies[proc]; !ok {
			t.Errorf("unknown destructive procedure %s", proc)
		}
	}
}

func TestRateClassesAndBoundedBuckets(t *testing.T) {
	b := newRateBuckets(4)
	now := time.Now()
	classes := map[string]string{
		athenav1connect.RecordServiceCreateRecordProcedure:   auth.SessionWrites,
		athenav1connect.SystemServiceVerifyAuditLogProcedure: auth.SessionReports,
		"/athena.v1.ReportService/GetReport":                 auth.SessionReports,
		"/athena.v1.ReportService/ExportReport":              auth.SessionExports,
	}
	for proc, want := range classes {
		if got := sessionRateClass(proc); got != want {
			t.Errorf("%s class %s", proc, got)
		}
	}
	for _, class := range []string{auth.SignInUsername, auth.SessionWrites, auth.SessionReports, auth.SessionExports} {
		if delay := b.retry(class, "same", 1, time.Minute, now); delay != 0 {
			t.Fatalf("%s first: %v", class, delay)
		}
		if delay := b.retry(class, "same", 1, time.Minute, now); delay != time.Minute {
			t.Fatalf("%s retry: %v", class, delay)
		}
	}
	if b.retry(auth.SignInIP, "another", 1, time.Minute, now) <= 0 || len(b.byKey) != 4 {
		t.Fatal("capacity bypassed")
	}
	if b.retry(auth.SessionExports, "same", 1, time.Minute, now.Add(time.Minute)) != 0 {
		t.Fatal("bucket failed to refill")
	}
	if b.retry(auth.SignInIP, "another", 1, time.Minute, now.Add(bucketIdle)) != 0 || len(b.byKey) != 2 {
		t.Fatal("idle keys were not collected")
	}
}

func TestRPCRateLimitsAndAudit(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	if _, err := f.users.EnsureRoot(ctx, testPassword); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]int{auth.SignInIP: 3, auth.SignInUsername: 1, auth.SessionWrites: 1, auth.SessionReports: 1} {
		if err := f.users.SetSecurity(ctx, key, value); err != nil {
			t.Fatal(err)
		}
	}
	check := func(err error, info *connect.CallInfo, category string) {
		t.Helper()
		requireCode(t, err, connect.CodeResourceExhausted)
		detail := failureOf(t, err)
		if detail.GetReason() != "rate_limited" || detail.GetRetryAfterSeconds() == 0 || info.ResponseHeader().Get("Retry-After") == "" {
			t.Fatal("missing retry information")
		}
		var count int
		if err := f.db.QueryRow(`SELECT COUNT(*) FROM athena_access_log WHERE event='rate_limited' AND detail=? AND username IS NULL AND user_id IS NULL`, category).Scan(&count); err != nil || count == 0 {
			t.Fatalf("missing redacted %s audit: %v", category, err)
		}
	}
	b := f.browser(t)
	b.login(t, auth.RootUsername)
	call, info := connect.NewClientContext(ctx)
	_, err := b.auth.CompleteChallenge(call, &v1.CompleteChallengeRequest{Username: " Administrator "})
	check(err, info, auth.SignInUsername)
	call, info = connect.NewClientContext(ctx)
	_, err = b.auth.StartReset(call, &v1.StartResetRequest{Username: auth.RootUsername})
	check(err, info, auth.SignInUsername)
	call, info = connect.NewClientContext(ctx)
	_, err = b.auth.Login(call, &v1.LoginRequest{Username: auth.RootUsername, Password: testPassword})
	check(err, info, auth.SignInIP)
	if _, err = b.system.VerifyAuditLog(ctx, &v1.VerifyAuditLogRequest{}); err != nil {
		t.Fatal(err)
	}
	call, info = connect.NewClientContext(ctx)
	_, err = b.system.VerifyAuditLog(call, &v1.VerifyAuditLogRequest{})
	check(err, info, auth.SessionReports)
	_, err = b.auth.StepUp(ctx, &v1.StepUpRequest{Password: "wrong"})
	requireCode(t, err, connect.CodeUnauthenticated)
	call, info = connect.NewClientContext(ctx)
	_, err = b.auth.StepUp(call, &v1.StepUpRequest{Password: testPassword})
	check(err, info, auth.SessionWrites)
}
