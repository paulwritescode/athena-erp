package main

import (
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"athena-erp/internal/testdb"
)

func doctorTestEnv(t *testing.T) {
	t.Helper()
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir()) // no home here, so the DSN fallback applies
}

func runDoctor(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := doctorCommand(args, &out)
	return out.String(), err
}

// Against a live database the command runs all five checks, ends with the
// machine-readable line and exits successfully: a fresh test database has
// pending migrations and this tree has no built UI, both warnings.
func TestDoctorRunsFiveChecks(t *testing.T) {
	doctorTestEnv(t)
	out, err := runDoctor(t, "--addr", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("doctor = %v\n%s", err, out)
	}
	for _, name := range []string{"mariadb", "migrations", "disk", "port", "frontend"} {
		if !strings.Contains(out, name) {
			t.Fatalf("doctor output %q misses the %s check", out, name)
		}
	}
	if !strings.Contains(out, "ok   mariadb") {
		t.Fatalf("doctor output %q misses the ok mariadb line", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	machine := lines[len(lines)-1]
	if !strings.HasPrefix(machine, "doctor status=") {
		t.Fatalf("last line %q is not the machine line", machine)
	}
	for _, want := range []string{"checks=5", "ok=", "warn=", "fail=0"} {
		if !strings.Contains(machine, want) {
			t.Fatalf("machine line %q misses %q", machine, want)
		}
	}
}

// --json prints the same report as parseable JSON with consistent counts.
func TestDoctorJSONParses(t *testing.T) {
	doctorTestEnv(t)
	out, err := runDoctor(t, "--json", "--addr", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("doctor --json = %v\n%s", err, out)
	}
	var report doctorReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("doctor --json = %q: %v", out, err)
	}
	if len(report.Checks) != 5 || report.Total != 5 {
		t.Fatalf("report has %d checks: %+v", len(report.Checks), report)
	}
	wantNames := []string{"mariadb", "migrations", "disk", "port", "frontend"}
	ok, warn, fail := 0, 0, 0
	for i, c := range report.Checks {
		if c.Name != wantNames[i] {
			t.Fatalf("check %d = %q, want %q", i, c.Name, wantNames[i])
		}
		switch c.Status {
		case doctorOK:
			ok++
		case doctorWarn:
			warn++
		case doctorFail:
			fail++
		default:
			t.Fatalf("check %q has status %q", c.Name, c.Status)
		}
	}
	if ok != report.OK || warn != report.Warn || fail != report.Fail || ok+warn+fail != report.Total {
		t.Fatalf("counts %+v do not match the checks", report)
	}
}

// An unreachable database fails the command, and neither the output nor
// the error ever carries the password from the DSN.
func TestDoctorUnreachableDatabaseFailsWithoutSecrets(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATHENA_DATABASE_URL", "athena:doctor-s3cret@tcp(127.0.0.1:1)/nodb")
	out, err := runDoctor(t, "--addr", "127.0.0.1:0")
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("doctor = %q, %v; want a failure", out, err)
	}
	if !strings.Contains(out, "fail mariadb") || !strings.Contains(out, "status=fail") {
		t.Fatalf("doctor output %q misses the failed check and machine line", out)
	}
	if strings.Contains(out, "doctor-s3cret") || strings.Contains(err.Error(), "doctor-s3cret") {
		t.Fatalf("doctor leaked the password: %q / %v", out, err)
	}
}

// The live-database output carries no password either (migrate_test.go
// asserts the same for its command).
func TestDoctorLiveOutputHasNoPassword(t *testing.T) {
	_, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir())
	out, _ := runDoctor(t, "--addr", "127.0.0.1:0")
	if cfg.Passwd != "" && strings.Contains(out, cfg.Passwd) {
		t.Fatal("doctor output contains the password")
	}
}

func TestDoctorPortBusyWarns(t *testing.T) {
	if got := checkDoctorPort("127.0.0.1:0"); got.Status != doctorOK {
		t.Fatalf("free port = %+v", got)
	}
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if got := checkDoctorPort(held.Addr().String()); got.Status != doctorWarn {
		t.Fatalf("busy port = %+v, want a warning", got)
	}
}

func TestDoctorDiskReportsFree(t *testing.T) {
	if got := checkDoctorDisk(t.TempDir()); got.Status != doctorOK || !strings.Contains(got.Detail, "free") {
		t.Fatalf("temp dir = %+v", got)
	}
	if got := checkDoctorDisk("/nonexistent-athena-dir"); got.Status != doctorFail {
		t.Fatalf("missing dir = %+v, want a failure", got)
	}
}

func TestDoctorHelp(t *testing.T) {
	out, err := runDoctor(t, "help")
	if err != nil || !strings.Contains(out, "usage: athena doctor") {
		t.Fatalf("doctor help = %q, %v", out, err)
	}
}

// The new command appears in the menu with no menu change (dynamic by design).
func TestMenuListsDoctor(t *testing.T) {
	if out := runMenuWith(t, "q\n"); !strings.Contains(out, "doctor") {
		t.Fatalf("menu %q lists no doctor entry", out)
	}
}
