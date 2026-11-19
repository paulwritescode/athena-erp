package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"
	"github.com/go-sql-driver/mysql"
)

// failureOf returns the AuthFailure detail of a Connect error, or nil.
func failureOf(t *testing.T, err error) *athenav1.AuthFailure {
	t.Helper()
	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("not a Connect error: %v", err)
	}
	for _, d := range ce.Details() {
		if msg, err := connectproto.UnmarshalErrorDetail(d); err == nil {
			if f, ok := msg.(*athenav1.AuthFailure); ok {
				return f
			}
		}
	}
	return nil
}

func TestSignInFailuresHaveDistinctCodesWithoutRevealingUsers(t *testing.T) {
	f := newAccessFixture(t)
	f.user(t, "dave")
	f.user(t, "dora")
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	svc := authService{auth: &auth.Service{DB: f.db, Now: func() time.Time { return now }}}
	ctx := context.Background()
	login := func(user, password string) error {
		_, err := svc.Login(ctx, &athenav1.LoginRequest{Username: user, Password: password})
		return err
	}

	// Wrong password and unknown user give the same code, message and no detail.
	wrong, unknown := login("dave", "wrong password!"), login("nobody-here", "wrong password!")
	for _, err := range []error{wrong, unknown} {
		requireCode(t, err, connect.CodeUnauthenticated)
		if failureOf(t, err) != nil {
			t.Fatal("a plain failure carries a detail")
		}
	}
	if connectMessage(wrong) != connectMessage(unknown) {
		t.Fatalf("messages differ: %q vs %q", connectMessage(wrong), connectMessage(unknown))
	}

	// Lock dave. Only the right password learns the state.
	for i := 1; i < auth.MaxFailedAttempts; i++ {
		_ = login("dave", "wrong password!")
	}
	locked := login("dave", testPassword)
	requireCode(t, locked, connect.CodeFailedPrecondition)
	detail := failureOf(t, locked)
	if detail == nil || detail.GetReason() != "locked" || detail.GetUnlocksAt() != now.Add(auth.LockDuration).Format(time.RFC3339) {
		t.Fatalf("locked detail = %v", detail)
	}
	guess := login("dave", "another guess!!")
	requireCode(t, guess, connect.CodeUnauthenticated)
	if failureOf(t, guess) != nil {
		t.Fatal("a wrong password learned that the account is locked")
	}

	// A disabled account says so only to the right password.
	disabled := true
	if _, err := f.users.UpdateUser(ctx, "dora", auth.UserUpdate{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	err := login("dora", testPassword)
	requireCode(t, err, connect.CodeFailedPrecondition)
	if d := failureOf(t, err); d == nil || d.GetReason() != "disabled" {
		t.Fatalf("disabled detail = %v", d)
	}
	requireCode(t, login("dora", "wrong password!"), connect.CodeUnauthenticated)
}

func TestSignInReportsAnUnreachableDatabaseAsUnavailable(t *testing.T) {
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr, cfg.User, cfg.DBName, cfg.Timeout = "tcp", "127.0.0.1:1", "x", "x", time.Second
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	svc := authService{auth: &auth.Service{DB: db}}
	_, err = svc.Login(context.Background(), &athenav1.LoginRequest{Username: "dave", Password: testPassword})
	requireCode(t, err, connect.CodeUnavailable)
	if strings.Contains(connectMessage(err), "127.0.0.1") {
		t.Fatalf("the message names the database: %q", connectMessage(err))
	}
}

func TestSignedInCallWithoutAValidCSRFTokenIsForbiddenWithADetail(t *testing.T) {
	f := newAccessFixture(t)
	f.user(t, "dave")
	b := f.browser(t)
	b.login(t, "dave")
	*b.csrf = "not-the-token"
	_, err := b.auth.ChangePassword(context.Background(), &athenav1.ChangePasswordRequest{})
	requireCode(t, err, connect.CodePermissionDenied)
	if d := failureOf(t, err); d == nil || d.GetReason() != "csrf" {
		t.Fatalf("csrf detail = %v", d)
	}
	// A request with no session at all is Unauthenticated, which the desk
	// shows as an expired session.
	_, err = f.browser(t).auth.ChangePassword(context.Background(), &athenav1.ChangePasswordRequest{})
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestEveryFailureCarriesARequestIDHeader(t *testing.T) {
	f := newAccessFixture(t)
	resp, err := http.Post(f.url+"/athena.v1.AuthService/Login", "application/json", strings.NewReader(`{"username":"x","password":"y"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get(requestIDHeader) == "" {
		t.Fatal("a failed sign-in has no X-Request-Id, so the desk cannot show a reference")
	}
}
