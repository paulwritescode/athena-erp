package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"athena-erp/internal/workspace"
)

// newTestHome stages a home whose server URL points at a closed port, so
// provisioning fails fast after the driver checks run.
func newTestHome(t *testing.T, driver, serverURL string) {
	t.Helper()
	home := t.TempDir()
	if err := workspace.InitHome(home, serverURL); err != nil {
		t.Fatal(err)
	}
	if driver != workspace.DriverMariaDB {
		body := "format_version: 1\ndatabase:\n  driver: " + driver + "\n  credentials_env: ATHENA_SERVER_URL\n"
		if err := os.WriteFile(filepath.Join(home, workspace.HomeFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(home)
}

func TestNewDriverMismatchFailsBeforeTouchingTheDatabase(t *testing.T) {
	newTestHome(t, workspace.DriverMariaDB, "athena:secret@tcp(127.0.0.1:1)/")
	var out bytes.Buffer
	err := newCommand([]string{"--driver", "postgres", "Acme"}, &out)
	if err == nil || !strings.Contains(err.Error(), "does not match the home driver") {
		t.Fatalf("err = %v, want the driver mismatch", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want nothing before the mismatch error", out.String())
	}
}

func TestNewFlagsWorkAfterTheName(t *testing.T) {
	newTestHome(t, workspace.DriverMariaDB, "athena:secret@tcp(127.0.0.1:1)/")
	for _, args := range [][]string{
		{"Acme", "--driver", "postgres"},
		{"Acme", "Trading", "--driver", "postgres"},
	} {
		var out bytes.Buffer
		err := newCommand(args, &out)
		if err == nil || !strings.Contains(err.Error(), "does not match the home driver") {
			t.Fatalf("new %v err = %v, want the driver mismatch", args, err)
		}
	}
}

func TestNewMissingLogoFailsBeforeTheNamePrompt(t *testing.T) {
	newTestHome(t, workspace.DriverMariaDB, "athena:secret@tcp(127.0.0.1:1)/")
	missing := filepath.Join(t.TempDir(), "missing.png")
	for _, args := range [][]string{
		{"Acme", "--logo", missing},
		{"--logo", missing},
		{"--logo", missing, "Acme"},
	} {
		var out bytes.Buffer
		err := newCommand(args, &out)
		if err == nil || !strings.Contains(err.Error(), "read "+missing) {
			t.Fatalf("new %v err = %v, want the logo read error", args, err)
		}
	}
}

func TestNewBadLogoContentFailsBeforeCreating(t *testing.T) {
	newTestHome(t, workspace.DriverMariaDB, "athena:secret@tcp(127.0.0.1:1)/")
	bad := filepath.Join(t.TempDir(), "bad.png")
	if err := os.WriteFile(bad, []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"Acme", "--logo", bad},
		{"--logo", bad},
		{"--logo", bad, "Acme"},
	} {
		var out bytes.Buffer
		err := newCommand(args, &out)
		if err == nil || !strings.Contains(err.Error(), "invalid logo") {
			t.Fatalf("new %v err = %v, want the logo content error", args, err)
		}
	}
}

func TestRunRejectsPositionalArguments(t *testing.T) {
	if err := runCommand([]string{"acme_trading"}); err == nil || !strings.Contains(err.Error(), `unexpected argument "acme_trading"`) {
		t.Fatalf("run err = %v, want the unexpected argument", err)
	}
}

func TestNewPostgresWarns(t *testing.T) {
	newTestHome(t, workspace.DriverPostgres, "postgres://athena:secret@127.0.0.1:1/")
	for _, args := range [][]string{{"--driver", "postgres", "Acme"}, {"Acme"}} {
		var out bytes.Buffer
		err := newCommand(args, &out)
		if err == nil {
			t.Fatalf("new %v unexpectedly provisioned without a database", args)
		}
		if !strings.Contains(out.String(), "implemented but untested") {
			t.Fatalf("new %v output = %q, want the untested warning", args, out.String())
		}
	}
}

func TestNewMariaDBStaysSilent(t *testing.T) {
	newTestHome(t, workspace.DriverMariaDB, "athena:secret@tcp(127.0.0.1:1)/")
	for _, args := range [][]string{{"--driver", "mariadb", "Acme"}, {"Acme"}} {
		var out bytes.Buffer
		err := newCommand(args, &out)
		if err == nil {
			t.Fatalf("new %v unexpectedly provisioned without a database", args)
		}
		if strings.Contains(out.String(), "warning") {
			t.Fatalf("new %v output = %q, want no warning", args, out.String())
		}
	}
}
