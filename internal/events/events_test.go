// Package events_test holds the database-backed event bus tests. It is an
// external test package so it can migrate with server.SystemMigrations
// without an import cycle.
package events_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"athena-erp/internal/events"
	"athena-erp/internal/store"

	"pgregory.net/rapid"
)

func setup(t *testing.T) *sql.DB {
	t.Helper()
	return sharedDatabase(t)
}

func emit(t *testing.T, db *sql.DB, envelope events.Envelope) string {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	id, err := events.Emit(context.Background(), tx, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return id
}

func outboxStatus(t *testing.T, db *sql.DB, eventID string) (string, int) {
	t.Helper()
	var status string
	var attempts int
	if err := db.QueryRow(`SELECT status, attempts FROM athena_outbox WHERE event_id = ?`, eventID).Scan(&status, &attempts); err != nil {
		t.Fatalf("outbox row: %v", err)
	}
	return status, attempts
}

func TestEmitWritesEventAndOutboxInOneTransaction(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	id := emit(t, db, events.Envelope{
		EventType: "pack.thing_happened", Workspace: "acme", Producer: "packs",
		SubjectType: "thing", SubjectID: "thing-1", SubjectRevision: 3,
		Actor: "ada", OperationID: "op-1", Payload: map[string]any{"row_count": 2},
	})
	var eventType, producer, subjectType, subjectID, actor, operation, correlation, payload string
	var sequence, revision int64
	var version int
	if err := db.QueryRow(`SELECT event_type, schema_version, sequence, producer, subject_type,
		subject_id, subject_revision, actor, operation_id, correlation_id, CAST(payload AS CHAR)
		FROM athena_events WHERE id = ?`, id).Scan(&eventType, &version, &sequence,
		&producer, &subjectType, &subjectID, &revision, &actor, &operation, &correlation, &payload); err != nil {
		t.Fatal(err)
	}
	if eventType != "pack.thing_happened" || version != 1 || sequence == 0 || producer != "packs" ||
		subjectType != "thing" || subjectID != "thing-1" ||
		revision != 3 || actor != "ada" || operation != "op-1" || correlation == "" || payload != `{"row_count":2}` {
		t.Fatalf("envelope mismatch: %+v", map[string]any{"type": eventType, "version": version,
			"sequence": sequence, "producer": producer, "subject": subjectType + "/" + subjectID,
			"revision": revision, "actor": actor, "operation": operation, "correlation": correlation, "payload": payload})
	}
	var outboxType, outboxPayload, status string
	var outboxEvent sql.NullString
	if err := db.QueryRow(`SELECT event_id, event_type, CAST(payload AS CHAR), status FROM athena_outbox WHERE event_id = ?`, id).
		Scan(&outboxEvent, &outboxType, &outboxPayload, &status); err != nil {
		t.Fatal(err)
	}
	if !outboxEvent.Valid || outboxType != "pack.thing_happened" || outboxPayload != `{"row_count":2}` || status != "pending" {
		t.Fatalf("outbox mismatch: %v %q %q %q", outboxEvent, outboxType, outboxPayload, status)
	}

	// A rolled-back emit writes neither row.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Emit(ctx, tx, events.Envelope{EventType: "pack.aborted", Producer: "packs", SubjectType: "thing", SubjectID: "x"}); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_events WHERE event_type = 'pack.aborted'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("aborted events = %d, %v, want 0", count, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_outbox WHERE event_type = 'pack.aborted'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("aborted outbox = %d, %v, want 0", count, err)
	}
}

func TestEmitRefusesAnIncompleteEnvelope(t *testing.T) {
	db := setup(t)
	for _, envelope := range []events.Envelope{
		{Producer: "p", SubjectType: "s", SubjectID: "i"},
		{EventType: "t", SubjectType: "s", SubjectID: "i"},
		{EventType: "t", Producer: "p", SubjectID: "i"},
		{EventType: "t", Producer: "p", SubjectType: "s"},
	} {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := events.Emit(context.Background(), tx, envelope); err == nil {
			t.Fatalf("emit of %+v succeeded, want an error", envelope)
		}
		_ = tx.Rollback()
	}
}

func TestClaimSkipsARecordedPair(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	claim := func(consumer, event, effect string) bool {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		claimed, err := events.Claim(ctx, tx, store.MariaDB{}, consumer, event, effect, "")
		if err != nil {
			t.Fatal(err)
		}
		if claimed {
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
		}
		return claimed
	}
	if !claim("ledger", "event-1", "") {
		t.Fatal("first claim was skipped, want claimed")
	}
	if claim("ledger", "event-1", "") {
		t.Fatal("second claim was claimed, want skipped")
	}
	if !claim("ledger", "event-1", "post-lines") {
		t.Fatal("a new effect key was skipped, want claimed")
	}
	if !claim("notify", "event-1", "") {
		t.Fatal("a new consumer was skipped, want claimed")
	}
	claimed, err := events.Claimed(ctx, db, "ledger", "event-1", "")
	if err != nil || !claimed {
		t.Fatalf("claimed = %v, %v, want true", claimed, err)
	}
	claimed, err = events.Claimed(ctx, db, "ledger", "event-9", "")
	if err != nil || claimed {
		t.Fatalf("claimed = %v, %v, want false", claimed, err)
	}
}

func TestSubscriptionsDeclareAndMatch(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	declare := func(sub events.Subscription) {
		t.Helper()
		if err := events.Declare(ctx, db, store.MariaDB{}, sub); err != nil {
			t.Fatal(err)
		}
	}
	declare(events.Subscription{Consumer: "ledger", EventType: "posting.posted", Action: "post", EffectKey: "lines", Enabled: true})
	declare(events.Subscription{Consumer: "notify", EventType: "posting.posted", Action: "tell", Pack: "packs/notify", Enabled: true})
	declare(events.Subscription{Consumer: "audit", EventType: "posting.posted", Action: "watch", Enabled: false})
	declare(events.Subscription{Consumer: "ledger", EventType: "document.submitted", Action: "post", Enabled: true})
	matched, err := events.Match(ctx, db, "posting.posted")
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 2 || matched[0].Consumer != "ledger" || matched[1].Consumer != "notify" {
		t.Fatalf("matched = %+v, want ledger and notify only", matched)
	}
	if matched[0].EffectKey != "lines" || matched[1].Pack != "packs/notify" {
		t.Fatalf("matched = %+v, want effect key and pack carried", matched)
	}
	// Redeclaring replaces the row.
	declare(events.Subscription{Consumer: "ledger", EventType: "posting.posted", Action: "post-v2", EffectKey: "lines", Enabled: false})
	matched, err = events.Match(ctx, db, "posting.posted")
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 || matched[0].Consumer != "notify" {
		t.Fatalf("matched after replace = %+v, want notify only", matched)
	}
	if err := events.Declare(ctx, db, store.MariaDB{}, events.Subscription{Consumer: "x"}); err == nil {
		t.Fatal("incomplete declare succeeded, want an error")
	}
}

func TestDispatchDeliversOncePerConsumer(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	if err := events.Declare(ctx, db, store.MariaDB{}, events.Subscription{Consumer: "ledger", EventType: "posting.posted", Action: "post", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := events.Declare(ctx, db, store.MariaDB{}, events.Subscription{Consumer: "notify", EventType: "posting.posted", Action: "tell", EffectKey: "tell", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	var ledgerCalls, notifyCalls atomic.Int32
	dispatcher := &events.Dispatcher{DB: db, Errors: store.MariaDB{}, Handlers: map[string]events.Handler{
		"ledger": func(_ context.Context, _ *sql.Tx, event events.Envelope) error {
			ledgerCalls.Add(1)
			if event.SubjectID != "voucher-1" {
				return fmt.Errorf("subject = %q", event.SubjectID)
			}
			return nil
		},
		"notify": func(_ context.Context, _ *sql.Tx, _ events.Envelope) error { notifyCalls.Add(1); return nil },
	}}
	id := emit(t, db, events.Envelope{EventType: "posting.posted", Producer: "posting",
		SubjectType: "journal", SubjectID: "voucher-1", SubjectRevision: 2,
		Payload: map[string]any{"row_count": 4}})
	delivered, err := dispatcher.DispatchNext(ctx)
	if err != nil || !delivered {
		t.Fatalf("dispatch = %v, %v, want true", delivered, err)
	}
	if ledgerCalls.Load() != 1 || notifyCalls.Load() != 1 {
		t.Fatalf("calls = %d/%d, want 1/1", ledgerCalls.Load(), notifyCalls.Load())
	}
	if status, _ := outboxStatus(t, db, id); status != "dispatched" {
		t.Fatalf("outbox status = %q, want dispatched", status)
	}
	delivered, err = dispatcher.DispatchNext(ctx)
	if err != nil || delivered {
		t.Fatalf("second dispatch = %v, %v, want nothing due", delivered, err)
	}
	if ledgerCalls.Load() != 1 || notifyCalls.Load() != 1 {
		t.Fatalf("calls after idle = %d/%d, want still 1/1", ledgerCalls.Load(), notifyCalls.Load())
	}
}

func TestDispatchRetriesThenParksDead(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	if err := events.Declare(ctx, db, store.MariaDB{}, events.Subscription{Consumer: "ledger", EventType: "posting.posted", Action: "post", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	dispatcher := &events.Dispatcher{DB: db, Errors: store.MariaDB{},
		MaxAttempts: 3, Backoff: func(int) time.Duration { return 0 },
		Handlers: map[string]events.Handler{
			"ledger": func(_ context.Context, _ *sql.Tx, _ events.Envelope) error {
				calls.Add(1)
				return fmt.Errorf("downstream is down")
			},
		}}
	id := emit(t, db, events.Envelope{EventType: "posting.posted", Producer: "posting",
		SubjectType: "journal", SubjectID: "voucher-9"})
	for i := 0; i < 2; i++ {
		if _, err := dispatcher.DispatchNext(ctx); err == nil {
			t.Fatalf("dispatch %d succeeded, want the handler error", i+1)
		}
		if status, attempts := outboxStatus(t, db, id); status != "pending" || attempts != i+1 {
			t.Fatalf("after failure %d: status %q attempts %d, want pending %d", i+1, status, attempts, i+1)
		}
	}
	if _, err := dispatcher.DispatchNext(ctx); err == nil {
		t.Fatal("third dispatch succeeded, want the handler error")
	}
	if status, attempts := outboxStatus(t, db, id); status != "dead" || attempts != 3 {
		t.Fatalf("status %q attempts %d, want dead 3", status, attempts)
	}
	if calls.Load() != 3 {
		t.Fatalf("handler calls = %d, want 3", calls.Load())
	}
	delivered, err := dispatcher.DispatchNext(ctx)
	if err != nil || delivered {
		t.Fatalf("dispatch after dead = %v, %v, want nothing due", delivered, err)
	}
	// The rolled-back attempts left no inbox row, so a fix replays cleanly.
	claimed, err := events.Claimed(ctx, db, "ledger", id, "")
	if err != nil || claimed {
		t.Fatalf("claimed = %v, %v, want false", claimed, err)
	}
}

func TestDispatchRecoversALegacyOutboxRow(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	if err := events.Declare(ctx, db, store.MariaDB{}, events.Subscription{Consumer: "ledger", EventType: "posting.posted", Action: "post", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	var got events.Envelope
	dispatcher := &events.Dispatcher{DB: db, Errors: store.MariaDB{}, Handlers: map[string]events.Handler{
		"ledger": func(_ context.Context, _ *sql.Tx, event events.Envelope) error { got = event; return nil },
	}}
	if _, err := db.Exec(`INSERT INTO athena_outbox (id, event_type, payload) VALUES ('legacy-row-1', 'posting.posted', '{"row_count":2}')`); err != nil {
		t.Fatal(err)
	}
	delivered, err := dispatcher.DispatchNext(ctx)
	if err != nil || !delivered {
		t.Fatalf("dispatch = %v, %v, want true", delivered, err)
	}
	if got.ID != "legacy-row-1" || got.EventType != "posting.posted" {
		t.Fatalf("envelope = %+v, want the legacy row", got)
	}
	if got.Payload["row_count"] != float64(2) {
		t.Fatalf("payload = %v, want row_count 2", got.Payload)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM athena_outbox WHERE id = 'legacy-row-1'`).Scan(&status); err != nil || status != "dispatched" {
		t.Fatalf("legacy status = %q, %v, want dispatched", status, err)
	}
}

// TestInboxDeduplicationProperty checks Property 13 over drawn consumers,
// events and effect keys: concurrent claims of one pair admit exactly one,
// and redelivering one event through the dispatcher applies each
// subscribed effect exactly once.
//
// Feature: athena-batch-2, Property 13: Inbox deduplication.
func TestInboxDeduplicationProperty(t *testing.T) {
	db := setup(t)
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		// Each iteration starts with an empty bus: leftovers from an
		// earlier iteration would otherwise deliver under this one's
		// consumer and effect key.
		for _, table := range []string{"athena_outbox", "athena_inbox", "athena_event_subscriptions"} {
			if _, err := db.Exec(`DELETE FROM ` + table); err != nil {
				t.Fatal(err)
			}
		}
		consumers := []string{"ledger", "notify", "audit"}
		consumer := consumers[rapid.IntRange(0, len(consumers)-1).Draw(t, "consumer")]
		effect := rapid.StringMatching(`[a-z]{0,8}`).Draw(t, "effect")
		rounds := rapid.IntRange(1, 4).Draw(t, "rounds")
		racers := rapid.IntRange(2, 6).Draw(t, "racers")

		// Concurrent claims of one pair admit exactly one winner.
		eventID := fmt.Sprintf("prop-%d-%s", time.Now().UnixNano(), rapid.StringMatching(`[a-z0-9]{4}`).Draw(t, "salt"))
		var winners atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					return
				}
				defer tx.Rollback()
				claimed, err := events.Claim(ctx, tx, store.MariaDB{}, consumer, eventID, effect, "")
				if err != nil || !claimed {
					return
				}
				if err := tx.Commit(); err == nil {
					winners.Add(1)
				}
			}()
		}
		wg.Wait()
		if winners.Load() != 1 {
			t.Fatalf("concurrent claims admitted %d, want 1", winners.Load())
		}

		// Redelivering through the dispatcher applies the effect once.
		fresh := fmt.Sprintf("prop-d-%s", eventID)
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		produced, err := events.Emit(ctx, tx, events.Envelope{ID: fresh, EventType: "prop.dedup",
			Producer: "prop", SubjectType: "thing", SubjectID: "thing-1"})
		if err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		_ = produced
		if err := events.Declare(ctx, db, store.MariaDB{}, events.Subscription{Consumer: consumer,
			EventType: "prop.dedup", Action: "apply", EffectKey: effect, Enabled: true}); err != nil {
			t.Fatal(err)
		}
		var applied atomic.Int32
		dispatcher := &events.Dispatcher{DB: db, Errors: store.MariaDB{},
			Handlers: map[string]events.Handler{
				consumer: func(_ context.Context, _ *sql.Tx, _ events.Envelope) error {
					applied.Add(1)
					return nil
				},
			}}
		for i := 0; i < rounds; i++ {
			if _, err := dispatcher.DispatchNext(ctx); err != nil {
				t.Fatal(err)
			}
			// Force a redelivery: the inbox must skip it.
			if _, err := db.Exec(`UPDATE athena_outbox SET status = 'pending' WHERE event_id = ?`, fresh); err != nil {
				t.Fatal(err)
			}
		}
		if applied.Load() != 1 {
			t.Fatalf("redelivered %d times, effect applied %d, want 1", rounds, applied.Load())
		}
	})
}
