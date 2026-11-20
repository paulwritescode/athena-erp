package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEndpointMariaDB(t *testing.T) {
	endpoint, err := ParseEndpoint(DriverMariaDB, "athena:secret@tcp(127.0.0.1:3306)/", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Driver != DriverMariaDB || endpoint.MySQL == nil || endpoint.Postgres != nil {
		t.Fatalf("driver routing wrong: %+v", endpoint)
	}
	if endpoint.DatabaseName() != "acme" {
		t.Fatalf("database = %q, want acme", endpoint.DatabaseName())
	}
	if endpoint.Password() != "secret" {
		t.Fatal("password accessor did not return the DSN password")
	}
	connector, err := endpoint.Connector()
	if err != nil || connector == nil {
		t.Fatalf("connector = %v, %v", connector, err)
	}
}

func TestParseEndpointPostgres(t *testing.T) {
	endpoint, err := ParseEndpoint(DriverPostgres, "postgres://athena:secret@127.0.0.1:5432/", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Driver != DriverPostgres || endpoint.Postgres == nil || endpoint.MySQL != nil {
		t.Fatalf("driver routing wrong: %+v", endpoint)
	}
	pg := endpoint.Postgres
	if pg.Database != "acme" || pg.User != "athena" || pg.Password != "secret" || pg.Host != "127.0.0.1" {
		t.Fatalf("parsed config wrong: database=%q user=%q host=%q", pg.Database, pg.User, pg.Host)
	}
	if pg.RuntimeParams["timezone"] != "UTC" {
		t.Fatalf("timezone = %q, want UTC", pg.RuntimeParams["timezone"])
	}
	if pg.RuntimeParams["datestyle"] == "" {
		t.Fatal("datestyle was not pinned")
	}
	if endpoint.DatabaseName() != "acme" || endpoint.Password() != "secret" {
		t.Fatal("accessors did not return the postgres database and password")
	}
	connector, err := endpoint.Connector()
	if err != nil || connector == nil {
		t.Fatalf("connector = %v, %v", connector, err)
	}
}

func TestParseEndpointPostgresForcesUTC(t *testing.T) {
	endpoint, err := ParseEndpoint(DriverPostgres, "postgres://athena:secret@127.0.0.1:5432/?timezone=America%2FNew_York", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if got := endpoint.Postgres.RuntimeParams["timezone"]; got != "UTC" {
		t.Fatalf("timezone = %q, want UTC", got)
	}
}

func TestParseEndpointRejectsADatabaseInTheServerURL(t *testing.T) {
	if _, err := ParseEndpoint(DriverMariaDB, "athena:secret@tcp(127.0.0.1:3306)/acme", "acme"); err == nil {
		t.Fatal("mariadb server URL with a database was accepted")
	}
	if _, err := ParseEndpoint(DriverPostgres, "postgres://athena:secret@127.0.0.1:5432/acme", "acme"); err == nil {
		t.Fatal("postgres server URL with a database was accepted")
	}
}

func TestParseEndpointEmptyURLKeepsTheInitHint(t *testing.T) {
	_, err := ParseEndpoint(DriverMariaDB, "", "acme")
	if err == nil || !strings.Contains(err.Error(), "run athena init") {
		t.Fatalf("error = %v, want the athena init hint", err)
	}
}

func TestParseEndpointUnknownDriver(t *testing.T) {
	if _, err := ParseEndpoint("sqlite", "x", "acme"); err == nil {
		t.Fatal("unknown driver was accepted")
	}
	if _, err := (&DBEndpoint{Driver: "sqlite"}).Connector(); err == nil {
		t.Fatal("connector for an unknown driver was accepted")
	}
	if got := (&DBEndpoint{Driver: DriverPostgres}).Password(); got != "" {
		t.Fatalf("nil postgres password = %q, want empty", got)
	}
}

func TestParseDatabaseURL(t *testing.T) {
	maria, err := ParseDatabaseURL("athena:secret@tcp(127.0.0.1:3306)/acme")
	if err != nil {
		t.Fatal(err)
	}
	if maria.Driver != DriverMariaDB || maria.DatabaseName() != "acme" {
		t.Fatalf("mariadb URL parsed as %+v", maria)
	}
	pg, err := ParseDatabaseURL("postgres://athena:secret@127.0.0.1:5432/acme")
	if err != nil {
		t.Fatal(err)
	}
	if pg.Driver != DriverPostgres || pg.DatabaseName() != "acme" || pg.Postgres.RuntimeParams["timezone"] != "UTC" {
		t.Fatalf("postgres URL parsed as %+v", pg)
	}
	if _, err := ParseDatabaseURL("postgres://athena:secret@127.0.0.1:5432/"); err == nil {
		t.Fatal("postgres URL without a database was accepted")
	}
	if _, err := ParseDatabaseURL("not a dsn at all"); err == nil {
		t.Fatal("garbage URL was accepted")
	}
}

func TestLoadHomeAcceptsPostgres(t *testing.T) {
	dir := t.TempDir()
	body := "format_version: 1\ndatabase:\n  driver: postgres\n  credentials_env: ATHENA_SERVER_URL\n"
	if err := os.WriteFile(filepath.Join(dir, HomeFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	home, err := LoadHome(dir)
	if err != nil {
		t.Fatal(err)
	}
	if home.Database.Driver != DriverPostgres {
		t.Fatalf("driver = %q", home.Database.Driver)
	}
	if err := os.WriteFile(filepath.Join(dir, HomeFile), []byte("format_version: 1\ndatabase:\n  driver: sqlite\n  credentials_env: ATHENA_SERVER_URL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadHome(dir); err == nil || !strings.Contains(err.Error(), "use mariadb or postgres") {
		t.Fatalf("error = %v, want the driver hint", err)
	}
}
