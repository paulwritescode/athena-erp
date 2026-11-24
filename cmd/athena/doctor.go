package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"syscall"
	"time"

	"athena-erp/internal/server"
	"athena-erp/internal/webui"
	"athena-erp/internal/workspace"
)

const doctorUsage = `usage: athena doctor [--workspace slug] [--addr 127.0.0.1:3000] [--json]

Check that this machine can run Athena: MariaDB is reachable, all system
migrations are applied, local disk has room, the HTTP address is free and
the UI is built into this binary (node is needed only to build it).

Exit status is 0 when no check fails and 1 when any check fails. The last
line is always machine-readable: doctor status=<ok|warn|fail> followed by
checks=, ok=, warn= and fail= counts. --json prints the report as JSON.`

type doctorStatus string

const (
	doctorOK   doctorStatus = "ok"
	doctorWarn doctorStatus = "warn"
	doctorFail doctorStatus = "fail"
)

type doctorCheck struct {
	Name   string       `json:"name"`
	Status doctorStatus `json:"status"`
	Detail string       `json:"detail"`
}

type doctorReport struct {
	Checks []doctorCheck `json:"checks"`
	Status doctorStatus  `json:"status"`
	Total  int           `json:"total"`
	OK     int           `json:"ok"`
	Warn   int           `json:"warn"`
	Fail   int           `json:"fail"`
}

// doctorWarnDiskFree warns when less than 1 GiB is free where Athena writes.
const doctorWarnDiskFree = 1 << 30

func doctorCommand(args []string, out io.Writer) error {
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(out, doctorUsage)
		return nil
	}
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	addr := flags.String("addr", defaultAddr, "HTTP listen address to probe")
	asJSON := flags.Bool("json", false, "print JSON")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	report := runDoctorChecks(*slug, *addr)
	if *asJSON {
		if err := writeJSON(out, report); err != nil {
			return err
		}
	} else {
		for _, c := range report.Checks {
			fmt.Fprintf(out, "%-4s %-10s %s\n", c.Status, c.Name, c.Detail)
		}
		fmt.Fprintf(out, "doctor status=%s checks=%d ok=%d warn=%d fail=%d\n",
			report.Status, report.Total, report.OK, report.Warn, report.Fail)
	}
	if report.Fail > 0 {
		return fmt.Errorf("doctor: %d of %d checks failed", report.Fail, report.Total)
	}
	return nil
}

func runDoctorChecks(slug, addr string) doctorReport {
	endpoint, svc, targetErr := migrationTarget(slug)
	diskPath := "."
	if svc != nil {
		diskPath = svc.Root
	}
	checks := []doctorCheck{
		checkDoctorMariaDB(endpoint, targetErr),
		checkDoctorMigrations(endpoint, svc, targetErr),
		checkDoctorDisk(diskPath),
		checkDoctorPort(addr),
		checkDoctorFrontend(),
	}
	report := doctorReport{Checks: checks, Status: doctorOK, Total: len(checks)}
	for _, c := range checks {
		switch c.Status {
		case doctorOK:
			report.OK++
		case doctorWarn:
			report.Warn++
		case doctorFail:
			report.Fail++
		}
	}
	if report.Fail > 0 {
		report.Status = doctorFail
	} else if report.Warn > 0 {
		report.Status = doctorWarn
	}
	return report
}

// doctorTargetLabel names the database without its secrets.
func doctorTargetLabel(endpoint *workspace.DBEndpoint) string {
	if endpoint == nil {
		return "unknown database"
	}
	return endpoint.Driver + "/" + endpoint.DatabaseName()
}

func checkDoctorMariaDB(endpoint *workspace.DBEndpoint, targetErr error) doctorCheck {
	const name = "mariadb"
	if targetErr != nil {
		return doctorCheck{Name: name, Status: doctorFail, Detail: fmt.Sprintf("cannot resolve the database: %v", targetErr)}
	}
	connector, err := endpoint.Connector()
	if err != nil {
		return doctorCheck{Name: name, Status: doctorFail, Detail: fmt.Sprintf("cannot use %s: %v", doctorTargetLabel(endpoint), err)}
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return doctorCheck{Name: name, Status: doctorFail, Detail: fmt.Sprintf("cannot reach %s: %v", doctorTargetLabel(endpoint), err)}
	}
	return doctorCheck{Name: name, Status: doctorOK, Detail: fmt.Sprintf("%s reachable", doctorTargetLabel(endpoint))}
}

func checkDoctorMigrations(endpoint *workspace.DBEndpoint, svc *server.Workspace, targetErr error) doctorCheck {
	const name = "migrations"
	if targetErr != nil {
		return doctorCheck{Name: name, Status: doctorFail, Detail: "unknown: the database is unreachable"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	states, err := server.MigrationStatus(ctx, endpoint, svc)
	if err != nil {
		return doctorCheck{Name: name, Status: doctorFail, Detail: fmt.Sprintf("cannot read migration state: %v", err)}
	}
	pending := 0
	for _, st := range states {
		if !st.Applied {
			pending++
		}
	}
	if pending > 0 {
		return doctorCheck{Name: name, Status: doctorWarn, Detail: fmt.Sprintf("%d of %d applied, %d pending (run athena migrate up)", len(states)-pending, len(states), pending)}
	}
	return doctorCheck{Name: name, Status: doctorOK, Detail: fmt.Sprintf("%d of %d applied", len(states), len(states))}
}

func checkDoctorDisk(dir string) doctorCheck {
	const name = "disk"
	abs, err := filepath.Abs(dir)
	if err != nil {
		return doctorCheck{Name: name, Status: doctorFail, Detail: fmt.Sprintf("cannot resolve %s: %v", dir, err)}
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(abs, &stat); err != nil {
		return doctorCheck{Name: name, Status: doctorFail, Detail: fmt.Sprintf("cannot stat %s: %v", abs, err)}
	}
	free := uint64(stat.Bavail) * uint64(stat.Bsize)
	if free < doctorWarnDiskFree {
		return doctorCheck{Name: name, Status: doctorWarn, Detail: fmt.Sprintf("%s has only %s free", abs, doctorBytes(free))}
	}
	return doctorCheck{Name: name, Status: doctorOK, Detail: fmt.Sprintf("%s has %s free", abs, doctorBytes(free))}
}

func doctorBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit || suffix == "TiB" {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%d B", n)
}

func checkDoctorPort(addr string) doctorCheck {
	const name = "port"
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return doctorCheck{Name: name, Status: doctorWarn, Detail: fmt.Sprintf("%s is busy (%v); another server may be running", addr, err)}
	}
	listener.Close()
	return doctorCheck{Name: name, Status: doctorOK, Detail: fmt.Sprintf("%s is free", addr)}
}

// checkDoctorFrontend proves node is not needed at runtime: the UI must
// come from this binary. It asks the real handler for the entry page.
func checkDoctorFrontend() doctorCheck {
	const name = "frontend"
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	webui.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		return doctorCheck{Name: name, Status: doctorWarn, Detail: fmt.Sprintf("no built UI in this binary (entry page status %d); build the web UI", recorder.Code)}
	}
	return doctorCheck{Name: name, Status: doctorOK, Detail: "UI is served from this binary; node is not needed"}
}
