package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/auth"
	"athena-erp/internal/events"
	"athena-erp/internal/jobs"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
)

// queueBrowser is one signed-in caller of the Events and Jobs services over
// the real HTTP stack with the permission gate. access_test.go owns the
// shared session shape, which has no queue clients, so this file keeps its
// own small browser on the shared CSRF transport.
type queueBrowser struct {
	events athenav1connect.EventServiceClient
	jobs   athenav1connect.JobServiceClient
	auth   athenav1connect.AuthServiceClient
	csrf   *string
}

func newQueueBrowser(t *testing.T, f *accessFixture) *queueBrowser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	token := ""
	client := connect.NewClient(connecthttp.NewTransport(&http.Client{Jar: jar, Transport: csrfTransport{token: &token}}, f.url))
	return &queueBrowser{
		events: athenav1connect.NewEventServiceClient(client),
		jobs:   athenav1connect.NewJobServiceClient(client),
		auth:   athenav1connect.NewAuthServiceClient(client),
		csrf:   &token,
	}
}

func (b *queueBrowser) login(t *testing.T, username string) {
	t.Helper()
	res, err := b.auth.Login(context.Background(), &athenav1.LoginRequest{Username: username, Password: testPassword})
	if err != nil {
		t.Fatalf("login %s: %v", username, err)
	}
	*b.csrf = res.GetSession().GetCsrfToken()
}

// emitQueueEvent commits one envelope through the bus and returns its id.
func emitQueueEvent(t *testing.T, db *sql.DB, envelope events.Envelope) string {
	t.Helper()
	ctx := context.Background()
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

func queueBuilder(t *testing.T, f *accessFixture) *queueBrowser {
	t.Helper()
	f.role(t, "Builder", []string{auth.PermissionBuild})
	f.user(t, "bob", "Builder")
	b := newQueueBrowser(t, f)
	b.login(t, "bob")
	return b
}

func TestQueueProceduresNeedBuild(t *testing.T) {
	f := newAccessFixture(t)
	f.role(t, "Builder", []string{auth.PermissionBuild})
	f.user(t, "bob", "Builder")
	f.user(t, "ann")
	ctx := context.Background()
	anon := newQueueBrowser(t, f)
	if _, err := anon.events.ListEvents(ctx, &athenav1.ListEventsRequest{}); err == nil {
		t.Fatal("signed-out ListEvents succeeded")
	} else {
		requireCode(t, err, connect.CodeUnauthenticated)
	}
	clerk := newQueueBrowser(t, f)
	clerk.login(t, "ann")
	denied := []func() error{
		func() error { _, err := clerk.events.ListEvents(ctx, &athenav1.ListEventsRequest{}); return err },
		func() error { _, err := clerk.events.GetEvent(ctx, &athenav1.GetEventRequest{Id: "nope"}); return err },
		func() error { _, err := clerk.events.ListOutbox(ctx, &athenav1.ListOutboxRequest{}); return err },
		func() error {
			_, err := clerk.events.RequeueOutbox(ctx, &athenav1.RequeueOutboxRequest{Id: "nope"})
			return err
		},
		func() error {
			_, err := clerk.events.ListSubscriptions(ctx, &athenav1.ListSubscriptionsRequest{})
			return err
		},
		func() error { _, err := clerk.jobs.ListJobs(ctx, &athenav1.ListJobsRequest{}); return err },
		func() error { _, err := clerk.jobs.GetJob(ctx, &athenav1.GetJobRequest{Id: "nope"}); return err },
		func() error { _, err := clerk.jobs.RetryJob(ctx, &athenav1.RetryJobRequest{Id: "nope"}); return err },
		func() error {
			_, err := clerk.jobs.ListOccurrences(ctx, &athenav1.ListOccurrencesRequest{})
			return err
		},
	}
	for _, call := range denied {
		requireCode(t, call(), connect.CodePermissionDenied)
	}
	builder := newQueueBrowser(t, f)
	builder.login(t, "bob")
	if _, err := builder.events.ListEvents(ctx, &athenav1.ListEventsRequest{}); err != nil {
		t.Fatalf("builder ListEvents: %v", err)
	}
	if _, err := builder.jobs.ListJobs(ctx, &athenav1.ListJobsRequest{}); err != nil {
		t.Fatalf("builder ListJobs: %v", err)
	}
}

func TestEventServiceListsPagesAndGets(t *testing.T) {
	f := newAccessFixture(t)
	b := queueBuilder(t, f)
	ctx := context.Background()
	first := emitQueueEvent(t, f.db, events.Envelope{EventType: "posting.posted", Producer: "posting",
		SubjectType: "bill", SubjectID: "b1", SubjectRevision: 2, Actor: "bob",
		Payload: map[string]any{"revision": 2}})
	second := emitQueueEvent(t, f.db, events.Envelope{EventType: "posting.posted", Producer: "posting",
		SubjectType: "bill", SubjectID: "b2", Actor: "bob", Payload: map[string]any{"revision": 1}})
	third := emitQueueEvent(t, f.db, events.Envelope{EventType: "document.submitted", Producer: "lifecycle",
		SubjectType: "order", SubjectID: "o9", Actor: "ann", Payload: map[string]any{"step": "submit"}})
	p1, err := b.events.ListEvents(ctx, &athenav1.ListEventsRequest{PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(p1.GetEvents()) != 2 || p1.GetNextPageToken() == "" {
		t.Fatalf("first page: %v", p1)
	}
	p2, err := b.events.ListEvents(ctx, &athenav1.ListEventsRequest{PageSize: 2, PageToken: p1.GetNextPageToken()})
	if err != nil {
		t.Fatal(err)
	}
	if len(p2.GetEvents()) != 1 || p2.GetNextPageToken() != "" {
		t.Fatalf("second page: %v", p2)
	}
	seen := map[string]bool{}
	for _, event := range append(p1.GetEvents(), p2.GetEvents()...) {
		seen[event.GetId()] = true
		if event.GetSequence() <= 0 || event.GetPayload() == "" {
			t.Fatalf("thin envelope: %v", event)
		}
		if _, err := time.Parse(time.RFC3339, event.GetCreatedAt()); err != nil {
			t.Fatalf("created_at %q: %v", event.GetCreatedAt(), err)
		}
	}
	for _, id := range []string{first, second, third} {
		if !seen[id] {
			t.Fatalf("page union misses %s: %v", id, seen)
		}
	}
	byType, err := b.events.ListEvents(ctx, &athenav1.ListEventsRequest{EventType: "posting.posted"})
	if err != nil || len(byType.GetEvents()) != 2 {
		t.Fatalf("type filter: %v %v", byType, err)
	}
	bySubject, err := b.events.ListEvents(ctx, &athenav1.ListEventsRequest{SubjectType: "order", SubjectId: "o9"})
	if err != nil || len(bySubject.GetEvents()) != 1 || bySubject.GetEvents()[0].GetId() != third {
		t.Fatalf("subject filter: %v %v", bySubject, err)
	}
	if _, err := b.events.ListEvents(ctx, &athenav1.ListEventsRequest{PageToken: "bogus"}); err == nil {
		t.Fatal("bogus page token succeeded")
	} else {
		requireCode(t, err, connect.CodeInvalidArgument)
	}
	got, err := b.events.GetEvent(ctx, &athenav1.GetEventRequest{Id: first})
	if err != nil {
		t.Fatal(err)
	}
	event := got.GetEvent()
	if event.GetEventType() != "posting.posted" || event.GetSubjectId() != "b1" ||
		event.GetSubjectRevision() != 2 || !strings.Contains(event.GetPayload(), `"revision":2`) {
		t.Fatalf("get: %v", event)
	}
	_, err = b.events.GetEvent(ctx, &athenav1.GetEventRequest{Id: "00000000-0000-0000-0000-000000000000"})
	requireCode(t, err, connect.CodeNotFound)
}

func TestOutboxListAndRequeue(t *testing.T) {
	f := newAccessFixture(t)
	b := queueBuilder(t, f)
	ctx := context.Background()
	old := emitQueueEvent(t, f.db, events.Envelope{EventType: "posting.posted", Producer: "posting",
		SubjectType: "bill", SubjectID: "b1", SubjectRevision: 2, Actor: "bob",
		OperationID: "op-1", Payload: map[string]any{"revision": 2}})
	pending, err := b.events.ListOutbox(ctx, &athenav1.ListOutboxRequest{})
	if err != nil || len(pending.GetRows()) != 1 || pending.GetRows()[0].GetStatus() != events.StatusPending {
		t.Fatalf("outbox: %v %v", pending, err)
	}
	rowID := pending.GetRows()[0].GetId()
	if _, err := f.db.ExecContext(ctx, `UPDATE athena_outbox SET status = 'dead', attempts = 5,
		last_error = 'boom' WHERE id = ?`, rowID); err != nil {
		t.Fatal(err)
	}
	dead, err := b.events.ListOutbox(ctx, &athenav1.ListOutboxRequest{Status: events.StatusDead})
	if err != nil || len(dead.GetRows()) != 1 || dead.GetRows()[0].GetLastError() != "boom" {
		t.Fatalf("dead filter: %v %v", dead, err)
	}
	none, err := b.events.ListOutbox(ctx, &athenav1.ListOutboxRequest{Status: events.StatusPending})
	if err != nil || len(none.GetRows()) != 0 {
		t.Fatalf("pending filter: %v %v", none, err)
	}
	if _, err := b.events.ListOutbox(ctx, &athenav1.ListOutboxRequest{Status: "bogus"}); err == nil {
		t.Fatal("bogus status succeeded")
	} else {
		requireCode(t, err, connect.CodeInvalidArgument)
	}
	requeued, err := b.events.RequeueOutbox(ctx, &athenav1.RequeueOutboxRequest{Id: rowID})
	if err != nil {
		t.Fatal(err)
	}
	if requeued.GetEventId() == "" || requeued.GetEventId() == old {
		t.Fatalf("requeue returned %q for %s", requeued.GetEventId(), old)
	}
	original, err := b.events.GetEvent(ctx, &athenav1.GetEventRequest{Id: old})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := b.events.GetEvent(ctx, &athenav1.GetEventRequest{Id: requeued.GetEventId()})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.GetEvent().GetEventType() != original.GetEvent().GetEventType() ||
		fresh.GetEvent().GetSubjectType() != original.GetEvent().GetSubjectType() ||
		fresh.GetEvent().GetSubjectId() != original.GetEvent().GetSubjectId() ||
		fresh.GetEvent().GetPayload() != original.GetEvent().GetPayload() ||
		fresh.GetEvent().GetCorrelationId() != original.GetEvent().GetCorrelationId() ||
		fresh.GetEvent().GetCausationId() != old {
		t.Fatalf("requeued envelope:\n%v\noriginal:\n%v", fresh.GetEvent(), original.GetEvent())
	}
	still, err := b.events.ListOutbox(ctx, &athenav1.ListOutboxRequest{Status: events.StatusDead})
	if err != nil || len(still.GetRows()) != 1 || still.GetRows()[0].GetId() != rowID {
		t.Fatalf("dead row moved: %v %v", still, err)
	}
	// A pending delivery cannot be requeued, and a missing row is NotFound.
	freshRows, err := b.events.ListOutbox(ctx, &athenav1.ListOutboxRequest{Status: events.StatusPending})
	if err != nil || len(freshRows.GetRows()) != 1 {
		t.Fatalf("fresh pending: %v %v", freshRows, err)
	}
	_, err = b.events.RequeueOutbox(ctx, &athenav1.RequeueOutboxRequest{Id: freshRows.GetRows()[0].GetId()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	_, err = b.events.RequeueOutbox(ctx, &athenav1.RequeueOutboxRequest{Id: "00000000-0000-0000-0000-000000000000"})
	requireCode(t, err, connect.CodeNotFound)
	// A legacy row without an envelope is refused with a plain reason.
	if _, err := f.db.ExecContext(ctx, `INSERT INTO athena_outbox (id, event_type, payload, status)
		VALUES ('00000000-0000-0000-0000-000000000001', 'legacy.type', '{}', 'dead')`); err != nil {
		t.Fatal(err)
	}
	_, err = b.events.RequeueOutbox(ctx, &athenav1.RequeueOutboxRequest{Id: "00000000-0000-0000-0000-000000000001"})
	requireCode(t, err, connect.CodeFailedPrecondition)
}

func TestSubscriptionsList(t *testing.T) {
	f := newAccessFixture(t)
	b := queueBuilder(t, f)
	ctx := context.Background()
	for _, sub := range []events.Subscription{
		{Consumer: "probe", EventType: "posting.posted", Mode: events.ModeAsync, Action: "record", Enabled: true},
		{Consumer: "archive", EventType: "document.submitted", Mode: events.ModeAsync, Action: "copy", Pack: "archive", Enabled: false},
	} {
		if err := events.Declare(ctx, f.db, dialect, sub); err != nil {
			t.Fatal(err)
		}
	}
	all, err := b.events.ListSubscriptions(ctx, &athenav1.ListSubscriptionsRequest{})
	if err != nil || len(all.GetSubscriptions()) != 2 {
		t.Fatalf("subscriptions: %v %v", all, err)
	}
	if all.GetSubscriptions()[0].GetConsumer() != "archive" || all.GetSubscriptions()[1].GetConsumer() != "probe" {
		t.Fatalf("consumer order: %v", all.GetSubscriptions())
	}
	filtered, err := b.events.ListSubscriptions(ctx, &athenav1.ListSubscriptionsRequest{EventType: "posting.posted"})
	if err != nil || len(filtered.GetSubscriptions()) != 1 || !filtered.GetSubscriptions()[0].GetEnabled() {
		t.Fatalf("filtered: %v %v", filtered, err)
	}
	empty, err := b.events.ListSubscriptions(ctx, &athenav1.ListSubscriptionsRequest{EventType: "nothing.here"})
	if err != nil || len(empty.GetSubscriptions()) != 0 {
		t.Fatalf("unknown type: %v %v", empty, err)
	}
}

func TestJobsListGetAndRetry(t *testing.T) {
	f := newAccessFixture(t)
	b := queueBuilder(t, f)
	ctx := context.Background()
	enqueued := map[string]string{}
	for _, kind := range []string{"probe-a", "probe-a", "probe-b"} {
		id, err := jobs.Enqueue(ctx, f.db, jobs.EnqueueOptions{Kind: kind,
			Payload: map[string]any{"marker": kind}, MaxAttempts: 7,
			OwnerUserID: "bob", OwnerRole: "Builder"})
		if err != nil {
			t.Fatal(err)
		}
		enqueued[kind+id] = id
	}
	var doomed string
	for key, id := range enqueued {
		if strings.HasPrefix(key, "probe-a") {
			doomed = id
			break
		}
	}
	if _, err := f.db.ExecContext(ctx, `UPDATE athena_jobs SET status = 'dead', attempts = 7,
		last_error = 'boom' WHERE id = ?`, doomed); err != nil {
		t.Fatal(err)
	}
	p1, err := b.jobs.ListJobs(ctx, &athenav1.ListJobsRequest{PageSize: 2})
	if err != nil || len(p1.GetJobs()) != 2 || p1.GetNextPageToken() == "" {
		t.Fatalf("first page: %v %v", p1, err)
	}
	p2, err := b.jobs.ListJobs(ctx, &athenav1.ListJobsRequest{PageSize: 2, PageToken: p1.GetNextPageToken()})
	if err != nil || len(p2.GetJobs()) != 1 || p2.GetNextPageToken() != "" {
		t.Fatalf("second page: %v %v", p2, err)
	}
	seen := map[string]bool{}
	for _, job := range append(p1.GetJobs(), p2.GetJobs()...) {
		seen[job.GetId()] = true
		if job.GetPayload() != "" || job.GetResult() != "" {
			t.Fatalf("list carries payload or result: %v", job)
		}
	}
	for _, id := range enqueued {
		if !seen[id] {
			t.Fatalf("page union misses %s", id)
		}
	}
	byStatus, err := b.jobs.ListJobs(ctx, &athenav1.ListJobsRequest{Status: jobs.StatusDead})
	if err != nil || len(byStatus.GetJobs()) != 1 || byStatus.GetJobs()[0].GetId() != doomed {
		t.Fatalf("status filter: %v %v", byStatus, err)
	}
	byKind, err := b.jobs.ListJobs(ctx, &athenav1.ListJobsRequest{Kind: "probe-b"})
	if err != nil || len(byKind.GetJobs()) != 1 {
		t.Fatalf("kind filter: %v %v", byKind, err)
	}
	if _, err := b.jobs.ListJobs(ctx, &athenav1.ListJobsRequest{Status: "bogus"}); err == nil {
		t.Fatal("bogus status succeeded")
	} else {
		requireCode(t, err, connect.CodeInvalidArgument)
	}
	got, err := b.jobs.GetJob(ctx, &athenav1.GetJobRequest{Id: doomed})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetJob().GetStatus() != jobs.StatusDead || got.GetJob().GetAttempts() != 7 ||
		got.GetJob().GetLastError() != "boom" || !strings.Contains(got.GetJob().GetPayload(), "probe-a") {
		t.Fatalf("get: %v", got.GetJob())
	}
	_, err = b.jobs.GetJob(ctx, &athenav1.GetJobRequest{Id: "00000000-0000-0000-0000-000000000000"})
	requireCode(t, err, connect.CodeNotFound)
	retried, err := b.jobs.RetryJob(ctx, &athenav1.RetryJobRequest{Id: doomed})
	if err != nil {
		t.Fatal(err)
	}
	if retried.GetJobId() == "" || retried.GetJobId() == doomed {
		t.Fatalf("retry returned %q for %s", retried.GetJobId(), doomed)
	}
	fresh, err := b.jobs.GetJob(ctx, &athenav1.GetJobRequest{Id: retried.GetJobId()})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.GetJob().GetStatus() != jobs.StatusQueued || fresh.GetJob().GetAttempts() != 0 ||
		fresh.GetJob().GetKind() != "probe-a" || fresh.GetJob().GetMaxAttempts() != 7 ||
		!strings.Contains(fresh.GetJob().GetPayload(), "probe-a") ||
		fresh.GetJob().GetOwnerRole() != "Builder" || fresh.GetJob().GetLastError() != "" {
		t.Fatalf("retried job: %v", fresh.GetJob())
	}
	still, err := b.jobs.GetJob(ctx, &athenav1.GetJobRequest{Id: doomed})
	if err != nil || still.GetJob().GetStatus() != jobs.StatusDead {
		t.Fatalf("dead row moved: %v %v", still, err)
	}
	_, err = b.jobs.RetryJob(ctx, &athenav1.RetryJobRequest{Id: retried.GetJobId()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	_, err = b.jobs.RetryJob(ctx, &athenav1.RetryJobRequest{Id: "00000000-0000-0000-0000-000000000000"})
	requireCode(t, err, connect.CodeNotFound)
}

func TestOccurrencesList(t *testing.T) {
	f := newAccessFixture(t)
	b := queueBuilder(t, f)
	ctx := context.Background()
	for _, period := range []string{"2026-11-20", "2026-11-21", "2026-11-22"} {
		if _, err := jobs.EnsureOccurrence(ctx, f.db, dialect, jobs.EnsureOptions{
			Workspace: "acme", Schedule: "nightly", Period: period, Timezone: "UTC"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := jobs.EnsureOccurrence(ctx, f.db, dialect, jobs.EnsureOptions{
		Workspace: "acme", Schedule: "hourly", Period: "2026-11-22T10", Timezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	all, err := b.jobs.ListOccurrences(ctx, &athenav1.ListOccurrencesRequest{Schedule: "nightly"})
	if err != nil || len(all.GetOccurrences()) != 3 {
		t.Fatalf("occurrences: %v %v", all, err)
	}
	for i, want := range []string{"2026-11-20", "2026-11-21", "2026-11-22"} {
		if all.GetOccurrences()[i].GetPeriod() != want {
			t.Fatalf("period order: %v", all.GetOccurrences())
		}
	}
	capped, err := b.jobs.ListOccurrences(ctx, &athenav1.ListOccurrencesRequest{Schedule: "nightly", Limit: 2})
	if err != nil || len(capped.GetOccurrences()) != 2 {
		t.Fatalf("limit: %v %v", capped, err)
	}
	byStatus, err := b.jobs.ListOccurrences(ctx, &athenav1.ListOccurrencesRequest{Status: jobs.OccurrencePending})
	if err != nil || len(byStatus.GetOccurrences()) != 4 {
		t.Fatalf("status filter: %v %v", byStatus, err)
	}
	empty, err := b.jobs.ListOccurrences(ctx, &athenav1.ListOccurrencesRequest{Schedule: "nothing"})
	if err != nil || len(empty.GetOccurrences()) != 0 {
		t.Fatalf("unknown schedule: %v %v", empty, err)
	}
	if _, err := b.jobs.ListOccurrences(ctx, &athenav1.ListOccurrencesRequest{Status: "bogus"}); err == nil {
		t.Fatal("bogus status succeeded")
	} else {
		requireCode(t, err, connect.CodeInvalidArgument)
	}
}

// TestCrashBetweenCommitAndDispatch proves R8.5: an event committed without
// dispatch (the process died in between) is delivered by the next dispatcher,
// and a redelivery applies the consumer effect once.
func TestCrashBetweenCommitAndDispatch(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	if err := events.Declare(ctx, f.db, dialect, events.Subscription{
		Consumer: "probe", EventType: "probe.crash", Mode: events.ModeAsync,
		Action: "record", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	id := emitQueueEvent(t, f.db, events.Envelope{EventType: "probe.crash", Producer: "probe",
		SubjectType: "probe", SubjectID: "p1", Payload: map[string]any{"n": 1}})
	// The commit landed but nothing ran: the crash.
	if claimed, err := events.Claimed(ctx, f.db, "probe", id, ""); err != nil || claimed {
		t.Fatalf("claimed before dispatch: %v %v", claimed, err)
	}
	var applied []string
	recovered := &events.Dispatcher{DB: f.db, Errors: dialect, Handlers: map[string]events.Handler{
		"probe": func(_ context.Context, _ *sql.Tx, event events.Envelope) error {
			applied = append(applied, event.ID)
			return nil
		},
	}}
	ran, err := recovered.DispatchNext(ctx)
	if err != nil || !ran {
		t.Fatalf("recovery dispatch: ran=%v err=%v", ran, err)
	}
	if len(applied) != 1 || applied[0] != id {
		t.Fatalf("applied: %v", applied)
	}
	if claimed, err := events.Claimed(ctx, f.db, "probe", id, ""); err != nil || !claimed {
		t.Fatalf("claimed after dispatch: %v %v", claimed, err)
	}
	// A redelivery of the same row skips the already-claimed effect.
	if _, err := f.db.ExecContext(ctx, `UPDATE athena_outbox SET status = 'pending',
		leased_by = NULL, leased_until = NULL WHERE event_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	ran, err = recovered.DispatchNext(ctx)
	if err != nil || !ran {
		t.Fatalf("redelivery dispatch: ran=%v err=%v", ran, err)
	}
	if len(applied) != 1 {
		t.Fatalf("redelivery applied again: %v", applied)
	}
}
