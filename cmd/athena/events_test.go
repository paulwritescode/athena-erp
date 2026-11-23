package main

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"strings"
	"testing"

	"athena-erp/internal/events"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
)

func TestEventsCommands(t *testing.T) {
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
	emit := func(envelope events.Envelope) string {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		id, err := events.Emit(ctx, tx, envelope)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := emit(events.Envelope{EventType: "posting.posted", Producer: "posting",
		SubjectType: "bill", SubjectID: "b1", SubjectRevision: 2, Actor: "bob",
		Payload: map[string]any{"revision": 2}})
	second := emit(events.Envelope{EventType: "document.submitted", Producer: "lifecycle",
		SubjectType: "order", SubjectID: "o9", Payload: map[string]any{"step": "submit"}})

	out, err := run(eventsCommand, "list")
	if err != nil || !strings.Contains(out, "posting.posted") || !strings.Contains(out, "document.submitted") {
		t.Fatalf("list = %q, %v", out, err)

	}
	out, err = run(eventsCommand, "list", "--type", "posting.posted")
	if err != nil || !strings.Contains(out, first) || strings.Contains(out, second) {
		t.Fatalf("list --type = %q, %v", out, err)
	}
	out, err = run(eventsCommand, "list", "--subject-type", "order", "--subject", "o9")
	if err != nil || !strings.Contains(out, second) || strings.Contains(out, first) {
		t.Fatalf("list --subject = %q, %v", out, err)
	}
	out, err = run(eventsCommand, "list", "--limit", "1")
	if err != nil || !strings.Contains(out, "next page token: ") {
		t.Fatalf("list --limit 1 = %q, %v", out, err)
	}
	token := strings.TrimSpace(strings.SplitN(out[strings.Index(out, "next page token: ")+len("next page token: "):], "\n", 2)[0])
	out, err = run(eventsCommand, "list", "--limit", "1", "--page", token)
	if err != nil || strings.Contains(out, "next page token: ") {
		t.Fatalf("second page = %q, %v", out, err)
	}

	out, err = run(eventsCommand, "show", first)
	if err != nil || !strings.Contains(out, "type: posting.posted") ||
		!strings.Contains(out, "subject: bill b1 revision 2") || !strings.Contains(out, `"revision":2`) {
		t.Fatalf("show = %q, %v", out, err)
	}
	if _, err := run(eventsCommand, "show", "00000000-0000-0000-0000-000000000000"); err == nil ||
		!strings.Contains(err.Error(), "no event with that id") {
		t.Fatalf("show missing = %v", err)
	}

	out, err = run(eventsCommand, "outbox")
	if err != nil || !strings.Contains(out, "pending") || !strings.Contains(out, first) {
		t.Fatalf("outbox = %q, %v", out, err)
	}
	rowID := strings.SplitN(out, "\t", 2)[0]
	if _, err := db.ExecContext(ctx, `UPDATE athena_outbox SET status = 'dead', attempts = 5,
		last_error = 'boom' WHERE id = ?`, rowID); err != nil {
		t.Fatal(err)
	}
	out, err = run(eventsCommand, "outbox", "--status", "dead")
	if err != nil || !strings.Contains(out, rowID) || !strings.Contains(out, "boom") {
		t.Fatalf("outbox --status dead = %q, %v", out, err)
	}
	out, err = run(eventsCommand, "requeue", rowID)
	if err != nil || !strings.Contains(out, "requeued "+rowID+" as event ") {
		t.Fatalf("requeue = %q, %v", out, err)
	}
	fresh := regexp.MustCompile(`[0-9a-f-]{36}`).FindAllString(out, -1)[1]
	out, err = run(eventsCommand, "show", fresh)
	if err != nil || !strings.Contains(out, "type: ") || !strings.Contains(out, "caused by: ") {
		t.Fatalf("show requeued = %q, %v", out, err)
	}
	if _, err := run(eventsCommand, "requeue", "00000000-0000-0000-0000-000000000000"); err == nil ||
		!strings.Contains(err.Error(), "no outbox row") {
		t.Fatalf("requeue missing = %v", err)
	}

	for _, sub := range []struct{ consumer, typ string }{{"probe", "posting.posted"}, {"archive", "document.submitted"}} {
		if err := events.Declare(ctx, db, store.MariaDB{}, events.Subscription{Consumer: sub.consumer,
			EventType: sub.typ, Action: "record", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	out, err = run(eventsCommand, "subscriptions")
	if err != nil || !strings.Contains(out, "probe") || !strings.Contains(out, "archive") {
		t.Fatalf("subscriptions = %q, %v", out, err)
	}
	out, err = run(eventsCommand, "subscriptions", "--type", "posting.posted")
	if err != nil || !strings.Contains(out, "probe") || strings.Contains(out, "archive") {
		t.Fatalf("subscriptions --type = %q, %v", out, err)
	}

	if _, err := run(eventsCommand, "frobnicate"); err == nil {
		t.Fatal("unknown subcommand succeeded")
	}
	if _, err := run(eventsCommand, "outbox", "--status", "bogus"); err == nil {
		t.Fatal("bogus status succeeded")
	}
}
