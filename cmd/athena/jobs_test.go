package main

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"strings"
	"testing"

	"athena-erp/internal/jobs"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
)

func TestJobsCommands(t *testing.T) {
	db, cfg := testdb.NewWithConfig(t)
	t.Setenv("ATHENA_DATABASE_URL", cfg.FormatDSN())
	t.Chdir(t.TempDir())
	run := func(command func([]string, io.Writer) error, args ...string) (string, error) {
		var out bytes.Buffer
		err := command(args, &out)
		return out.String(), err
	}
	if _, err := run(migrateCommand, "up"); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	ctx := context.Background()
	first, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "probe-a",
		Payload: map[string]any{"marker": "job-one"}, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	second, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "probe-b",
		Payload: map[string]any{"marker": "job-two"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE athena_jobs SET status = 'dead', attempts = 3,
		last_error = 'boom' WHERE id = ?`, first); err != nil {
		t.Fatal(err)
	}

	out, err := run(jobsCommand, "list")
	if err != nil || !strings.Contains(out, "probe-a") || !strings.Contains(out, "probe-b") ||
		!strings.Contains(out, "dead") || !strings.Contains(out, "queued") {
		t.Fatalf("list = %q, %v", out, err)
	}
	out, err = run(jobsCommand, "list", "--status", "dead")
	if err != nil || !strings.Contains(out, first) || strings.Contains(out, second) {
		t.Fatalf("list --status dead = %q, %v", out, err)
	}
	out, err = run(jobsCommand, "list", "--kind", "probe-b")
	if err != nil || !strings.Contains(out, second) || strings.Contains(out, first) {
		t.Fatalf("list --kind probe-b = %q, %v", out, err)
	}
	out, err = run(jobsCommand, "list", "--limit", "1")
	if err != nil || !strings.Contains(out, "next page token: ") {
		t.Fatalf("list --limit 1 = %q, %v", out, err)
	}
	token := strings.TrimSpace(strings.SplitN(out[strings.Index(out, "next page token: ")+len("next page token: "):], "\n", 2)[0])
	out, err = run(jobsCommand, "list", "--limit", "1", "--page", token)
	if err != nil || strings.Contains(out, "next page token: ") {
		t.Fatalf("second page = %q, %v", out, err)
	}
	if out, err := run(jobsCommand, "list", "--json"); err != nil || !strings.Contains(out, `"jobs"`) {
		t.Fatalf("list --json = %q, %v", out, err)
	}

	out, err = run(jobsCommand, "show", first)
	if err != nil || !strings.Contains(out, "kind: probe-a") || !strings.Contains(out, "status: dead") ||
		!strings.Contains(out, "job-one") || !strings.Contains(out, "boom") {
		t.Fatalf("show = %q, %v", out, err)
	}
	if _, err := run(jobsCommand, "show", "00000000-0000-0000-0000-000000000000"); err == nil ||
		!strings.Contains(err.Error(), "no job with that id") {
		t.Fatalf("show missing = %v", err)
	}

	out, err = run(jobsCommand, "retry", first)
	if err != nil || !strings.Contains(out, "retried "+first+" as ") {
		t.Fatalf("retry = %q, %v", out, err)
	}
	fresh := regexp.MustCompile(`[0-9a-f-]{36}`).FindAllString(out, -1)[1]
	out, err = run(jobsCommand, "show", fresh)
	if err != nil || !strings.Contains(out, "status: queued") || !strings.Contains(out, "job-one") {
		t.Fatalf("show retried = %q, %v", out, err)
	}
	if _, err := run(jobsCommand, "retry", fresh); err == nil || !strings.Contains(err.Error(), "only a dead job") {
		t.Fatalf("retry queued = %v", err)
	}

	if _, err := jobs.EnsureOccurrence(ctx, db, store.MariaDB{}, jobs.EnsureOptions{
		Workspace: "acme", Schedule: "nightly", Period: "2026-11-21"}); err != nil {
		t.Fatal(err)
	}
	out, err = run(jobsCommand, "occurrences", "--schedule", "nightly")
	if err != nil || !strings.Contains(out, "2026-11-21") || !strings.Contains(out, "pending") {
		t.Fatalf("occurrences = %q, %v", out, err)
	}

	if _, err := run(jobsCommand, "frobnicate"); err == nil {
		t.Fatal("unknown subcommand succeeded")
	}
	if _, err := run(jobsCommand, "list", "--limit", "501"); err == nil {
		t.Fatal("limit 501 succeeded")
	}
	if _, err := run(jobsCommand, "list", "--status", "bogus"); err == nil {
		t.Fatal("bogus status succeeded")
	}
}
