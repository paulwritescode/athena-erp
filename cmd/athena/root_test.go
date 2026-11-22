package main

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"athena-erp/internal/auth"
)

// Root administrator command tests (ADR-0018, task B8): the first kernel
// open seeds the account, and every user- or role-changing command refuses
// it with a plain one-line error.

// rootRow reads the seeded root account back from the test database.
func rootRow(t *testing.T) (displayName, delivery, state string, disabled bool, assignments int) {
	t.Helper()
	db, err := sql.Open("mysql", os.Getenv("ATHENA_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var username string
	if err := db.QueryRow("SELECT username, display_name, invite_delivery, state, disabled FROM athena_users WHERE username = 'administrator'").
		Scan(&username, &displayName, &delivery, &state, &disabled); err != nil {
		t.Fatalf("root account: %v", err)
	}
	list, err := (&auth.Service{DB: db}).ListAssignments(context.Background(), "administrator")
	if err != nil {
		t.Fatal(err)
	}
	return displayName, delivery, state, disabled, len(list)
}

func TestRootIsSeededOnFirstCommandAndRefusesEdits(t *testing.T) {
	user, role := accessCommands(t)
	// The first command opens the kernel, which seeds the root account
	// (ADR-0018: first boot after migrations is the setup seam).
	out, err := user("list")
	if err != nil || !strings.Contains(out, "administrator") {
		t.Fatalf("list = %q, %v", out, err)
	}
	if displayName, delivery, state, disabled, assignments := rootRow(t); displayName != "Administrator" || delivery != "setup" || state != "active" || disabled || assignments != 0 {
		t.Fatalf("root = %q, %q, %q, disabled=%v, assignments=%d", displayName, delivery, state, disabled, assignments)
	}
	calls := []struct {
		action []string
		phrase string
	}{
		{[]string{"suspend", "administrator"}, "cannot be suspended"},
		{[]string{"set", "administrator", "--disable"}, "cannot be suspended"},
		{[]string{"set", "administrator", "--display-name", "Renamed Root"}, "cannot be edited"},
		{[]string{"set", "administrator", "--roles", "Clerk"}, "cannot be given Roles"},
		{[]string{"invite", "administrator"}, "reserved for the workspace root"},
		{[]string{"invite", "administrator", "--owner"}, "reserved for the workspace root"},
	}
	for _, c := range calls {
		_, err := user(c.action...)
		if err == nil || !strings.Contains(err.Error(), c.phrase) {
			t.Fatalf("user %s = %v, want %q", strings.Join(c.action, " "), err, c.phrase)
		}
		if strings.Contains(err.Error(), "\n") {
			t.Fatalf("user %s error is not one plain line: %q", strings.Join(c.action, " "), err.Error())
		}
	}
	if _, err := role("assign", "administrator", "Clerk"); err == nil || !strings.Contains(err.Error(), "cannot be given Roles") {
		t.Fatalf("role assign = %v", err)
	} else if strings.Contains(err.Error(), "\n") {
		t.Fatalf("role assign error is not one plain line: %q", err.Error())
	}
	// The refusals changed nothing.
	if displayName, delivery, state, disabled, assignments := rootRow(t); displayName != "Administrator" || delivery != "setup" || state != "active" || disabled || assignments != 0 {
		t.Fatalf("root after the refusals = %q, %q, %q, disabled=%v, assignments=%d", displayName, delivery, state, disabled, assignments)
	}
}
