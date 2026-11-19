package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"athena-erp/internal/testdb"
)

func tableMigration(version int, table string) Migration {
	return Migration{Version: version, Name: table, Up: func(ctx context.Context, x Execer) error {
		_, err := x.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+table+" (id INT PRIMARY KEY)")
		return err
	}}
}

func TestValidateMigrations(t *testing.T) {
	noop := func(context.Context, Execer) error { return nil }
	good := []Migration{{1, "a", noop}, {2, "b", noop}}
	if err := ValidateMigrations(good); err != nil {
		t.Fatal(err)
	}
	for name, ms := range map[string][]Migration{
		"gap":       {{1, "a", noop}, {3, "b", noop}},
		"duplicate": {{1, "a", noop}, {1, "b", noop}},
		"no name":   {{1, "", noop}},
		"no up":     {{1, "a", nil}},
	} {
		if err := ValidateMigrations(ms); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestUpIsIdempotentAndStatusTracksIt(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	d := MariaDB{}
	ms := []Migration{tableMigration(1, "t_one"), tableMigration(2, "t_two")}

	states, err := MigrationStatus(ctx, db, d, ms)
	if err != nil || states[0].Applied || states[1].Applied {
		t.Fatalf("fresh status = %+v, %v", states, err)
	}
	if exists, _ := d.TableExists(ctx, db, MigrationsTable); exists {
		t.Fatal("status created the state table")
	}
	applied, err := Up(ctx, db, d, ms, UpOptions{DryRun: true})
	if err != nil || len(applied) != 2 {
		t.Fatalf("dry run = %v, %v", applied, err)
	}
	if exists, _ := d.TableExists(ctx, db, "t_one"); exists {
		t.Fatal("dry run applied a migration")
	}

	if applied, err = Up(ctx, db, d, ms, UpOptions{}); err != nil || len(applied) != 2 {
		t.Fatalf("first up = %v, %v", applied, err)
	}
	if applied, err = Up(ctx, db, d, ms, UpOptions{}); err != nil || len(applied) != 0 {
		t.Fatalf("second up = %v, %v", applied, err)
	}
	states, _ = MigrationStatus(ctx, db, d, ms)
	if !states[0].Applied || !states[1].Applied || states[1].AppliedAt.IsZero() {
		t.Fatalf("status = %+v", states)
	}
}

func TestUpTarget(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	ms := []Migration{tableMigration(1, "t_one"), tableMigration(2, "t_two")}
	if applied, err := Up(ctx, db, MariaDB{}, ms, UpOptions{Target: 1}); err != nil || len(applied) != 1 {
		t.Fatalf("up --to 1 = %v, %v", applied, err)
	}
	if _, err := Up(ctx, db, MariaDB{}, ms, UpOptions{Target: 3}); err == nil {
		t.Fatal("target beyond the list accepted")
	}
}

// A crash is simulated two ways: a step that fails before it is recorded, and
// a step whose DDL ran but whose record row never did.
func TestUpResumesAfterInterruption(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	d := MariaDB{}
	boom := errors.New("simulated crash")
	failing := Migration{Version: 2, Name: "t_two", Up: func(ctx context.Context, x Execer) error {
		if _, err := x.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS t_two (id INT PRIMARY KEY)"); err != nil {
			return err
		}
		return boom // DDL committed, record row not written
	}}
	ms := []Migration{tableMigration(1, "t_one"), failing, tableMigration(3, "t_three")}

	applied, err := Up(ctx, db, d, ms, UpOptions{})
	if !errors.Is(err, boom) || len(applied) != 1 {
		t.Fatalf("crashing up = %v, %v", applied, err)
	}
	if exists, _ := d.TableExists(ctx, db, "t_three"); exists {
		t.Fatal("migration after the failure ran")
	}
	states, _ := MigrationStatus(ctx, db, d, ms)
	if !states[0].Applied || states[1].Applied {
		t.Fatalf("status after crash = %+v", states)
	}

	ms[1] = tableMigration(2, "t_two") // the step now completes; it re-runs over its own DDL
	applied, err = Up(ctx, db, d, ms, UpOptions{})
	if err != nil || len(applied) != 2 || applied[0].Version != 2 || applied[1].Version != 3 {
		t.Fatalf("resume = %v, %v", applied, err)
	}
}

func TestUpRejectsNewerDatabase(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	d := MariaDB{}
	ms := []Migration{tableMigration(1, "t_one")}
	if _, err := Up(ctx, db, d, ms, UpOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO athena_migrations (version, name) VALUES (7, 'future')"); err != nil {
		t.Fatal(err)
	}
	if _, err := Up(ctx, db, d, ms, UpOptions{}); err == nil || !strings.Contains(err.Error(), "newer athena") {
		t.Fatalf("up on newer database = %v", err)
	}
	if _, err := MigrationStatus(ctx, db, d, ms); err == nil {
		t.Fatal("status accepted a newer database")
	}
}
