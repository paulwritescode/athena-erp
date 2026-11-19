package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"athena-erp/internal/server"

	"github.com/go-sql-driver/mysql"
)

// migrationLine is one migration in --json output.
type migrationLine struct {
	Version   int     `json:"version"`
	Name      string  `json:"name"`
	Applied   bool    `json:"applied"`
	AppliedAt *string `json:"applied_at,omitempty"`
}

func migrateCommand(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "usage: athena migrate status|up [--project slug] [--json]; up also takes [--to N] [--dry-run]")
		return nil
	}
	sub := args[0]
	if sub != "status" && sub != "up" {
		return fmt.Errorf("unknown migrate command %q; use status or up", sub)
	}
	flags := flag.NewFlagSet("migrate "+sub, flag.ContinueOnError)
	slug := flags.String("project", "", "project slug, when running from the workspace directory")
	asJSON := flags.Bool("json", false, "print JSON")
	var target *int
	var dryRun *bool
	if sub == "up" {
		target = flags.Int("to", 0, "stop after this migration number (default: all)")
		dryRun = flags.Bool("dry-run", false, "list pending migrations without applying them")
	}
	if ok, err := parse(flags, args[1:]); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	cfg, project, err := migrationTarget(*slug)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if sub == "status" {
		states, err := server.MigrationStatus(ctx, cfg, project)
		if err != nil {
			return err
		}
		lines := make([]migrationLine, len(states))
		for i, st := range states {
			lines[i] = migrationLine{Version: st.Version, Name: st.Name, Applied: st.Applied}
			if st.Applied {
				at := st.AppliedAt.UTC().Format(time.RFC3339)
				lines[i].AppliedAt = &at
			}
		}
		if *asJSON {
			return writeJSON(out, lines)
		}
		for _, l := range lines {
			state := "pending"
			if l.Applied {
				state = "applied " + *l.AppliedAt
			}
			fmt.Fprintf(out, "%d\t%s\t%s\n", l.Version, l.Name, state)
		}
		return nil
	}
	applied, err := server.Migrate(ctx, cfg, project, server.MigrationOptions{Target: *target, DryRun: *dryRun})
	if err != nil {
		return err
	}
	lines := make([]migrationLine, len(applied))
	for i, m := range applied {
		lines[i] = migrationLine{Version: m.Version, Name: m.Name, Applied: !*dryRun}
	}
	if *asJSON {
		return writeJSON(out, lines)
	}
	if len(lines) == 0 {
		fmt.Fprintln(out, "No pending migrations.")
		return nil
	}
	verb := "Applied"
	if *dryRun {
		verb = "Would apply"
	}
	for _, l := range lines {
		fmt.Fprintf(out, "%s %s %s\n", verb, strconv.Itoa(l.Version), l.Name)
	}
	return nil
}

// migrationTarget resolves the project like `run`, or falls back to
// ATHENA_DATABASE_URL like `serve`. It never prints the DSN.
func migrationTarget(slug string) (*mysql.Config, *server.Project, error) {
	p, cfg, err := resolveProject(slug)
	if err == nil {
		return cfg, &server.Project{ID: p.Manifest.ID, Name: p.Manifest.Name, Slug: p.Manifest.Slug}, nil
	}
	dsn := os.Getenv("ATHENA_DATABASE_URL")
	if slug != "" || dsn == "" {
		return nil, nil, err
	}
	cfg, perr := mysql.ParseDSN(dsn)
	if perr != nil {
		return nil, nil, errors.New("parse ATHENA_DATABASE_URL: invalid DSN")
	}
	return cfg, nil, nil
}

func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
