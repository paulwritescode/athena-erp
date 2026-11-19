package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// MigrationsTable records which system migrations have been applied.
const MigrationsTable = "athena_migrations"

// Execer is the part of *sql.Conn that a migration step needs.
type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Migration is one numbered system migration. Up must be idempotent: engines
// such as MariaDB commit DDL implicitly, so a crash between Up and the record
// row means Up runs again on the next start.
type Migration struct {
	Version int
	Name    string
	Up      func(ctx context.Context, x Execer) error
}

// MigrationState is one migration and whether the database has applied it.
type MigrationState struct {
	Version   int
	Name      string
	Applied   bool
	AppliedAt time.Time
}

// UpOptions limits what Up does. Target 0 means every pending migration.
type UpOptions struct {
	Target int
	DryRun bool
}

// ValidateMigrations checks that versions run 1, 2, 3, ... without gaps.
func ValidateMigrations(ms []Migration) error {
	for i, m := range ms {
		if m.Version != i+1 {
			return fmt.Errorf("migration %q has version %d; expected %d (versions must be contiguous from 1)", m.Name, m.Version, i+1)
		}
		if m.Name == "" || m.Up == nil {
			return fmt.Errorf("migration %d needs a name and an Up function", m.Version)
		}
	}
	return nil
}

// MigrationStatus lists every known migration with its applied state. It
// writes nothing. A database that records a version this build does not know
// is an error.
func MigrationStatus(ctx context.Context, db *sql.DB, d Dialect, ms []Migration) ([]MigrationState, error) {
	if err := ValidateMigrations(ms); err != nil {
		return nil, err
	}
	applied, err := readApplied(ctx, db, d, len(ms))
	if err != nil {
		return nil, err
	}
	states := make([]MigrationState, len(ms))
	for i, m := range ms {
		at, ok := applied[m.Version]
		states[i] = MigrationState{Version: m.Version, Name: m.Name, Applied: ok, AppliedAt: at}
	}
	return states, nil
}

func readApplied(ctx context.Context, db *sql.DB, d Dialect, known int) (map[int]time.Time, error) {
	applied := map[int]time.Time{}
	exists, err := d.TableExists(ctx, db, MigrationsTable)
	if err != nil {
		return nil, err
	}
	if !exists {
		return applied, nil
	}
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT version, applied_at FROM %s ORDER BY version", d.QuoteIdent(MigrationsTable)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var version int
		var at time.Time
		if err := rows.Scan(&version, &at); err != nil {
			return nil, err
		}
		if version > known {
			return nil, fmt.Errorf("database has migration %d but this build knows only %d; run a newer athena", version, known)
		}
		applied[version] = at
	}
	return applied, rows.Err()
}

// Up applies pending migrations in order and returns the ones it applied (or,
// with DryRun, would apply). It holds the schema lock on one connection for
// the whole run and records each migration after its Up succeeds. Running it
// again after a crash re-runs only the step that was not recorded.
func Up(ctx context.Context, db *sql.DB, d Dialect, ms []Migration, opts UpOptions) (applied []Migration, err error) {
	ctx, span := otel.Tracer("athena/store").Start(ctx, "athena.migrate.up")
	defer func() {
		span.SetAttributes(attribute.Int("athena.migrations.applied", len(applied)), attribute.Bool("athena.migrations.dry_run", opts.DryRun))
		if err != nil {
			span.SetStatus(codes.Error, "migration failed")
		}
		span.End()
	}()
	if err := ValidateMigrations(ms); err != nil {
		return nil, err
	}
	if opts.Target < 0 || opts.Target > len(ms) {
		return nil, fmt.Errorf("target migration %d is outside 1 to %d", opts.Target, len(ms))
	}
	limit := len(ms)
	if opts.Target > 0 {
		limit = opts.Target
	}
	if opts.DryRun {
		have, err := readApplied(ctx, db, d, len(ms))
		if err != nil {
			return nil, err
		}
		for _, m := range ms[:limit] {
			if _, done := have[m.Version]; !done {
				applied = append(applied, m)
			}
		}
		return applied, nil
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	acquired, release, err := d.LockSchema(ctx, conn)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, errors.New("another process is changing the schema; try again shortly")
	}
	defer release()
	if _, err := conn.ExecContext(ctx, d.CreateMigrationsTable()); err != nil {
		return nil, fmt.Errorf("create %s: %w", MigrationsTable, err)
	}
	// Read under the lock so a concurrent runner's work is seen.
	have, err := readApplied(ctx, db, d, len(ms))
	if err != nil {
		return nil, err
	}
	record := fmt.Sprintf("INSERT INTO %s (version, name) VALUES (?, ?)", d.QuoteIdent(MigrationsTable))
	for _, m := range ms[:limit] {
		if _, done := have[m.Version]; done {
			continue
		}
		if err := m.Up(ctx, conn); err != nil {
			return applied, fmt.Errorf("migration %d %s: %w", m.Version, m.Name, err)
		}
		if _, err := conn.ExecContext(ctx, record, m.Version, m.Name); err != nil {
			return applied, fmt.Errorf("record migration %d %s: %w", m.Version, m.Name, err)
		}
		applied = append(applied, m)
	}
	return applied, nil
}
