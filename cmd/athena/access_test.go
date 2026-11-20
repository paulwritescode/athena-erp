package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"athena-erp/internal/auth"
	"athena-erp/internal/testdb"

	_ "github.com/go-sql-driver/mysql"
)

func TestUserRoleAndAccessLogCommands(t *testing.T) {
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir())
	run := func(command func([]string, *bytes.Buffer) error, args ...string) (string, error) {
		var out bytes.Buffer
		err := command(args, &out)
		return out.String(), err
	}
	user := func(args ...string) (string, error) {
		return run(func(a []string, b *bytes.Buffer) error { return userCommand(a, b) }, args...)
	}
	role := func(args ...string) (string, error) {
		return run(func(a []string, b *bytes.Buffer) error { return roleCommand(a, b) }, args...)
	}
	if out, err := user("help"); err != nil || !strings.Contains(out, "--owner") {
		t.Fatalf("help = %q, %v", out, err)
	}
	if out, err := user("create", "admin", "--owner"); err != nil || !strings.Contains(out, "Created instance owner admin") {
		t.Fatalf("create owner = %q, %v", out, err)
	}
	if _, err := user("create", "admin"); err == nil || !strings.Contains(err.Error(), "taken") {
		t.Fatalf("a duplicate username was accepted: %v", err)
	}
	if out, err := user("create", "weak"); err != nil || !strings.Contains(out, "shown once") {
		t.Fatalf("an invite needs no password: %q, %v", out, err)
	}
	// A Grant for a Record Type that does not exist is refused.
	file := filepath.Join(t.TempDir(), "role.json")
	if err := os.WriteFile(file, []byte(`{"name":"Clerk","systemPermissions":["audit"],"grants":[{"recordTypeKey":"missing","actions":["read"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := role("save", "--file", file); err == nil || !strings.Contains(err.Error(), "no such Record Type") {
		t.Fatalf("role save = %v", err)
	}
	if err := os.WriteFile(file, []byte(`{"name":"Clerk","systemPermissions":["audit"],"grants":[{"recordTypeKey":"*","actions":["read","audit"],"permissionLevel":1}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := role("save", "--file", file); err != nil || !strings.Contains(out, "Saved Role Clerk with 1 Grant") {
		t.Fatalf("role save = %q, %v", out, err)
	}
	if out, err := role("list"); err != nil || !strings.Contains(out, "*:read+audit@1") {
		t.Fatalf("role list = %q, %v", out, err)
	}
	out, err := user("create", "clerk", "--roles", "Clerk", "--display-name", "A Clerk")
	if err != nil || !strings.Contains(out, "Created user clerk") || !strings.Contains(out, "Invite challenge, shown once: ") {
		t.Fatalf("create = %q, %v", out, err)
	}
	// The clerk completes the invite out of band, as the desk first-login
	// screen would, so the account below is active.
	challenge := out[strings.Index(out, "shown once: ")+len("shown once: "):]
	if i := strings.IndexByte(challenge, '\n'); i >= 0 {
		challenge = challenge[:i]
	}
	complete := func(username, token, password string) {
		t.Helper()
		db, err := sql.Open("mysql", cfg.FormatDSN())
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		svc := &auth.Service{DB: db}
		if _, err := svc.CompleteChallenge(context.Background(), username, auth.ChallengePurposeFirstLogin, token, password, "cli-test"); err != nil {
			t.Fatal(err)
		}
	}
	complete("clerk", challenge, "a completed cli passphrase")
	if out, err := user("reset", "clerk"); err != nil || !strings.Contains(out, "ended their sessions") || !strings.Contains(out, "shown once") {
		t.Fatalf("reset = %q, %v", out, err)
	}
	if out, err := user("list"); err != nil || !strings.Contains(out, "admin") || !strings.Contains(out, "owner") || !strings.Contains(out, "Clerk") {
		t.Fatalf("list = %q, %v", out, err)
	}
	if out, err := user("set", "clerk", "--disable", "--json"); err != nil || !strings.Contains(strings.Join(strings.Fields(out), ""), `"disabled":true`) {
		t.Fatalf("set = %q, %v", out, err)
	}
	if _, err := user("set", "admin", "--disable"); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("the last owner was disabled: %v", err)
	}
	if _, err := role("delete", "Clerk"); err == nil {
		t.Fatal("delete without --yes was accepted")
	}
	if out, err := role("delete", "Clerk", "--yes"); err != nil || !strings.Contains(out, "Deleted Role Clerk") {
		t.Fatalf("delete = %q, %v", out, err)
	}
	var log bytes.Buffer
	if err := accessLogCommand(nil, &log); err != nil || !strings.Contains(log.String(), "role_saved") || !strings.Contains(log.String(), "password_reset") {
		t.Fatalf("access-log = %q, %v", log.String(), err)
	}
	if strings.Contains(log.String(), challenge) || strings.Contains(log.String(), "a completed cli passphrase") {
		t.Fatal("the Access Log printed a Challenge value or password")
	}
}

func accessCommands(t *testing.T) (func(...string) (string, error), func(...string) (string, error)) {
	t.Helper()
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir())
	run := func(command func([]string, *bytes.Buffer) error, args ...string) (string, error) {
		var out bytes.Buffer
		err := command(args, &out)
		return out.String(), err
	}
	user := func(args ...string) (string, error) {
		return run(func(a []string, b *bytes.Buffer) error { return userCommand(a, b) }, args...)
	}
	role := func(args ...string) (string, error) {
		return run(func(a []string, b *bytes.Buffer) error { return roleCommand(a, b) }, args...)
	}
	return user, role
}

// completeInvite finishes an invite outside the CLI, as the desk first-login
// screen would, taking the Challenge from the invite output.
func completeInvite(t *testing.T, username, output, password string) {
	t.Helper()
	const marker = "shown once: "
	i := strings.Index(output, marker)
	if i < 0 {
		t.Fatalf("no Challenge in %q", output)
	}
	token := output[i+len(marker):]
	if j := strings.IndexByte(token, '\n'); j >= 0 {
		token = token[:j]
	}
	db, err := sql.Open("mysql", os.Getenv("ATHENA_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &auth.Service{DB: db}
	if _, err := svc.CompleteChallenge(context.Background(), username, auth.ChallengePurposeFirstLogin, token, password, "cli-test"); err != nil {
		t.Fatal(err)
	}
}

func TestInviteLifecycleCommands(t *testing.T) {
	user, _ := accessCommands(t)
	out, err := user("invite", "alice", "--display-name", "A Leece")
	if err != nil || !strings.Contains(out, "Created user alice") || !strings.Contains(out, "shown once") {
		t.Fatalf("invite = %q, %v", out, err)
	}
	resent, err := user("resend", "alice")
	if err != nil || !strings.Contains(resent, "shown once") {
		t.Fatalf("resend = %q, %v", resent, err)
	}
	completeInvite(t, "alice", resent, "an active cli passphrase")
	out, err = user("reset", "alice")
	if err != nil || !strings.Contains(out, "ended their sessions") || !strings.Contains(out, "shown once") {
		t.Fatalf("reset = %q, %v", out, err)
	}
	if out, err := user("sessions", "alice"); err != nil || out != "" {
		t.Fatalf("sessions after a reset = %q, %v", out, err)
	}
	if out, err := user("suspend", "alice"); err != nil || !strings.Contains(out, "Suspended alice") {
		t.Fatalf("suspend = %q, %v", out, err)
	}
	if out, err := user("reactivate", "alice"); err != nil || !strings.Contains(out, "Reactivated alice") {
		t.Fatalf("reactivate = %q, %v", out, err)
	}
	if out, err := user("invite", "boss", "--owner"); err != nil || !strings.Contains(out, "Created instance owner boss") {
		t.Fatalf("invite owner = %q, %v", out, err)
	}
	if _, err := user("invite", "second", "--owner"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("a second bootstrap was accepted: %v", err)
	}
}

func TestRoleAssignRevokeExplain(t *testing.T) {
	user, role := accessCommands(t)
	if _, err := user("invite", "owner", "--owner"); err != nil {
		t.Fatal(err)
	}
	out, err := user("invite", "dave")
	if err != nil {
		t.Fatal(err)
	}
	completeInvite(t, "dave", out, "an active cli passphrase")
	file := filepath.Join(t.TempDir(), "role.json")
	if err := os.WriteFile(file, []byte(`{"name":"Clerk","grants":[{"recordTypeKey":"*","actions":["read"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := role("save", "--file", file); err != nil {
		t.Fatal(err)
	}
	if out, err := role("assign", "dave", "Clerk", "--scope", "company=acme"); err != nil || !strings.Contains(out, "Assigned Clerk to dave (scope company=acme).") {
		t.Fatalf("assign = %q, %v", out, err)
	}
	if out, err := role("assign", "dave", "Clerk", "--from", "2030-01-01"); err != nil || !strings.Contains(out, "active 2030-01-01 to unbounded") {
		t.Fatalf("dated assign = %q, %v", out, err)
	}
	if _, err := role("assign", "dave", "Clerk", "--scope", "Company=acme"); err == nil {
		t.Fatal("an invalid Scope dimension was accepted")
	}
	if _, err := role("assign", "dave", "Clerk", "--from", "yesterday"); err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Fatalf("a bad date was accepted: %v", err)
	}
	if out, err := role("revoke", "dave", "Clerk"); err != nil || !strings.Contains(out, "Revoked 2 assignments of Clerk from dave.") {
		t.Fatalf("revoke = %q, %v", out, err)
	}
	if _, err := role("revoke", "dave", "Clerk"); err == nil || !strings.Contains(err.Error(), "holds no") {
		t.Fatalf("a repeated revoke was accepted: %v", err)
	}
	if _, err := role("explain", "dave", "read", "invoice"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("explain without a type = %v", err)
	}
}
