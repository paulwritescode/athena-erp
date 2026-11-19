package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"athena-erp/internal/testdb"
)

func TestUserRoleAndAccessLogCommands(t *testing.T) {
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Setenv("ATHENA_USER_PASSWORD", "a long cli passphrase")
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
	t.Setenv("ATHENA_USER_PASSWORD", "short")
	if _, err := user("create", "weak"); err == nil {
		t.Fatal("a short password was accepted")
	}
	t.Setenv("ATHENA_USER_PASSWORD", "a long cli passphrase")
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
	if out, err := user("create", "clerk", "--roles", "Clerk", "--display-name", "A Clerk"); err != nil || !strings.Contains(out, "Created user clerk") {
		t.Fatalf("create = %q, %v", out, err)
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
	if out, err := user("reset-password", "clerk"); err != nil || !strings.Contains(out, "ended their sessions") {
		t.Fatalf("reset = %q, %v", out, err)
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
	if strings.Contains(log.String(), "a long cli passphrase") {
		t.Fatal("the Access Log printed a password")
	}
}
