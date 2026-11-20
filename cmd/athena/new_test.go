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
