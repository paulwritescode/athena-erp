package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"athena-erp/internal/auth"
	"athena-erp/internal/testdb"

	_ "github.com/go-sql-driver/mysql"
)

func TestSecurityShowAndSetCommands(t *testing.T) {
	db, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir())
	security := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := securityCommand(args, &out)
		return out.String(), err
	}
	if out, err := security("help"); err != nil || !strings.Contains(out, "athena security show") {
		t.Fatalf("help = %q, %v", out, err)
	}
	out, err := security("show")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"security.session.idle_minutes = 30", "security.session.build_idle_minutes = 15",
		"security.session.absolute_hours = 12", "security.rate.ip_per_minute = 10",
		"security.rate.username_per_15_minutes = 5", "security.rate.writes_per_minute = 60",
		"security.rate.reports_per_minute = 10", "security.rate.exports_per_minute = 10"} {
		if !strings.Contains(out, want) {
			t.Fatalf("show is missing %q:\n%s", want, out)
		}
	}
	if out, err := security("set", "security.session.idle_minutes", "45"); err != nil || !strings.Contains(out, "security.session.idle_minutes = 45") {
		t.Fatalf("set = %q, %v", out, err)
	}
	if out, err := security("show"); err != nil || !strings.Contains(out, "security.session.idle_minutes = 45") {
		t.Fatalf("show after set = %q, %v", out, err)
	}
	for _, args := range [][]string{
		{"set", "security.session.idle_minutes", "4"},
		{"set", "security.session.idle_minutes", "481"},
		{"set", "security.session.absolute_hours", "169"},
		{"set", "security.rate.writes_per_minute", "0"},
		{"set", "security.unknown", "1"},
		{"set", "security.session.idle_minutes", "soon"},
		{"set", "security.session.idle_minutes"},
		{"drop", "security.session.idle_minutes", "45"},
	} {
		if out, err := security(args...); err == nil {
			t.Fatalf("%v was accepted: %q", args, out)
		}
	}
	// A CLI session listing shows the device, last seen and expiry times.
	ctx := context.Background()
	users := &auth.Service{DB: db}
	user, challenge, err := users.CreateUser(ctx, "alice", "", false, "request")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.CompleteChallengeWithOptions(ctx, user.Username, auth.ChallengePurposeFirstLogin, challenge, "a long test passphrase", "request",
		auth.SessionOptions{Device: "Firefox"}); err != nil {
		t.Fatal(err)
	}
	var sessions bytes.Buffer
	if err := userCommand([]string{"sessions", "alice"}, &sessions); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sessions.String(), "Firefox") {
		t.Fatalf("sessions listing has no device:\n%s", sessions.String())
	}
}
