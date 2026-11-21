package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/ledger"
	"athena-erp/internal/notifications"
	"connectrpc.com/connect/v2"
)

func inboxUser(t *testing.T, f *accessFixture, name string) *auth.Principal {
	t.Helper()
	res, err := f.users.Login(context.Background(), name, testPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	return res.Principal
}

func assertInbox(t *testing.T, f *accessFixture, id string, want ...notifications.Kind) []*v1.Notification {
	t.Helper()
	page, err := notifications.List(context.Background(), f.db, id, 100, "", false)
	if err != nil {
		t.Fatal(err)
	}
	rows := page.GetNotifications()
	if len(rows) != len(want) {
		t.Fatalf("inbox size %d, expected kinds %v, got %v", len(rows), want, rows)
	}
	for i, kind := range want {
		if rows[i].Kind != string(kind) {
			t.Fatalf("item %d kind %s, want %s", i, rows[i].Kind, kind)
		}
	}
	return rows
}

func TestInboxAuthEvents(t *testing.T) {
	f := newAccessFixture(t)
	f.user(t, "host")
	host := inboxUser(t, f, "host")
	ctx := context.Background()
	acting := auth.WithPrincipal(ctx, host)
	u, token, err := f.users.CreateUser(acting, "guest", "Guest", false, "")
	if err != nil {
		t.Fatal(err)
	}
	joined, err := f.users.CompleteChallenge(ctx, u.Username, auth.ChallengePurposeFirstLogin, token, testPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	invites := assertInbox(t, f, host.UserID, notifications.InviteAccepted)
	if invites[0].ActorUserId != u.ID || invites[0].Text != "Guest accepted your invitation." {
		t.Fatal(invites[0])
	}
	assertInbox(t, f, u.ID)
	f.role(t, "Clerk", nil)
	assignment, err := f.users.SaveAssignment(ctx, u.Username, auth.AssignmentInput{Role: "Clerk"})
	if err != nil {
		t.Fatal(err)
	}
	f.role(t, "Clerk", []string{auth.PermissionLedgerRead})
	if err := f.users.RevokeAssignment(ctx, assignment.ID); err != nil {
		t.Fatal(err)
	}
	assertInbox(t, f, u.ID, notifications.AccessChanged, notifications.AccessChanged, notifications.AccessChanged)
	if err := f.users.RevokeSession(acting, host.UserID, joined.Principal.SessionID, true); err != nil {
		t.Fatal(err)
	}
	events := assertInbox(t, f, u.ID, notifications.SessionEnded, notifications.AccessChanged, notifications.AccessChanged, notifications.AccessChanged)
	if events[0].ActorUserId != host.UserID {
		t.Fatal(events[0])
	}
	for i := 1; i <= auth.MaxFailedAttempts; i++ {
		if _, err := f.users.Login(ctx, u.Username, "a wrong password", ""); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatal(err)
		}
		if i == 2 {
			assertInbox(t, f, u.ID, notifications.SessionEnded, notifications.AccessChanged, notifications.AccessChanged, notifications.AccessChanged)
		}
	}
	events = assertInbox(t, f, u.ID, notifications.AccountLocked, notifications.SignInFailed, notifications.SignInFailed, notifications.SessionEnded, notifications.AccessChanged, notifications.AccessChanged, notifications.AccessChanged)
	if !strings.Contains(events[2].Text, "3 unsuccessful") {
		t.Fatal(events[2])
	}
}

func TestNotificationPagingAndOwnership(t *testing.T) {
	f := newAccessFixture(t)
	f.user(t, "alice")
	f.user(t, "bob")
	ctx := context.Background()
	alice, bob := inboxUser(t, f, "alice"), inboxUser(t, f, "bob")
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, id := range []string{alice.UserID, bob.UserID, alice.UserID, alice.UserID} {
		if err := notifications.Write(ctx, tx, id, notifications.Event{Kind: notifications.AccessChanged, SubjectType: "user", SubjectID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec("UPDATE athena_notifications SET created_at='2026-01-01 00:00:00'"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	b := f.browser(t)
	_, err = b.inbox.List(ctx, &v1.ListNotificationsRequest{})
	requireCode(t, err, connect.CodeUnauthenticated)
	b.login(t, "alice")
	p1, err := b.inbox.List(ctx, &v1.ListNotificationsRequest{PageSize: 2, UnreadOnly: true})
	if err != nil || len(p1.GetNotifications()) != 2 || p1.GetNextPageToken() == "" {
		t.Fatalf("first page: %v %v", p1, err)
	}
	p2, err := b.inbox.List(ctx, &v1.ListNotificationsRequest{PageSize: 2, PageToken: p1.NextPageToken, UnreadOnly: true})
	if err != nil || len(p2.GetNotifications()) != 1 || p2.GetNextPageToken() != "" {
		t.Fatalf("second page: %v %v", p2, err)
	}
	seen := map[string]bool{}
	for _, n := range append(p1.Notifications, p2.Notifications...) {
		if n.SubjectId != alice.UserID || seen[n.Id] {
			t.Fatal(n)
		}
		seen[n.Id] = true
	}
	foreign := assertInbox(t, f, bob.UserID, notifications.AccessChanged)[0]
	for i, want := range []int64{1, 0} {
		m, err := b.inbox.MarkRead(ctx, &v1.MarkReadRequest{Ids: []string{p1.Notifications[0].Id, foreign.Id}})
		if err != nil || m.GetChanged() != want {
			t.Fatalf("mark %d: %v %v", i, m, err)
		}
	}
	unread, err := b.inbox.List(ctx, &v1.ListNotificationsRequest{UnreadOnly: true})
	if err != nil || len(unread.GetNotifications()) != 2 {
		t.Fatalf("unread: %v %v", unread, err)
	}
	count, err := b.inbox.UnreadCount(ctx, &v1.UnreadCountRequest{})
	if err != nil || count.GetCount() != 2 {
		t.Fatalf("count: %v %v", count, err)
	}
	all, err := b.inbox.MarkAllRead(ctx, &v1.MarkAllReadRequest{})
	if err != nil || all.GetChanged() != 2 {
		t.Fatalf("mark all: %v %v", all, err)
	}
	if assertInbox(t, f, bob.UserID, notifications.AccessChanged)[0].ReadAt != "" {
		t.Fatal("changed another user's inbox")
	}
}

func TestRuleStopsSaveAndNotifiesActor(t *testing.T) {
	f := newRuleFixture(t)
	f.save(t, "qty_positive", "validation", "validate", positiveQty, 10)
	p := &auth.Principal{UserID: newRequestID(), Username: "worker", Root: true}
	ctx := auth.WithPrincipal(context.Background(), p)
	_, err := f.records.CreateRecord(ctx, &v1.CreateRecordRequest{RecordTypeKey: "invoice", Values: map[string]*v1.FieldValue{"qty": text("0"), "title": text("private document")}})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if f.countInvoices(t) != 0 {
		t.Fatal("the stopped save created a document")
	}
	page, err := notifications.List(ctx, f.db, p.UserID, 5, "", false)
	if err != nil || len(page.GetNotifications()) != 1 {
		t.Fatalf("inbox: %v %v", page, err)
	}
	n := page.Notifications[0]
	if n.Kind != "save_stopped" || n.SubjectType != "invoice" || n.ActorUserId != p.UserID || strings.Contains(n.String(), "private document") {
		t.Fatal(n)
	}
}

func TestPeriodCloseNotifiesCurrentLedgerWriters(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	f.role(t, "Writer", []string{auth.PermissionLedgerWrite})
	f.user(t, "writer", "Writer")
	f.user(t, "reader")
	f.user(t, "former")
	writer := inboxUser(t, f, "writer")
	reader := inboxUser(t, f, "reader")
	former := inboxUser(t, f, "former")
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	if _, err := f.users.SaveAssignment(ctx, "former", auth.AssignmentInput{Role: "Writer", ExpiresAt: &yesterday}); err != nil {
		t.Fatal(err)
	}
	s := ledger.Service{DB: f.db, Dialect: dialect}
	if _, err := s.SaveCompany(ctx, ledger.Company{Name: "acme", Label: "Acme"}, 0); err != nil {
		t.Fatal(err)
	}
	p, err := s.SavePeriod(ctx, ledger.Period{Company: "acme", Name: "nov", Label: "November", StartDate: "2026-11-01", EndDate: "2026-11-30"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPeriodStatus(auth.WithPrincipal(ctx, writer), p.Company, p.Book, p.Name, ledger.PeriodSoftClosed, writer.Username, p.Revision); err != nil {
		t.Fatal(err)
	}
	n := assertInbox(t, f, writer.UserID, notifications.PeriodClosed, notifications.AccessChanged)[0]
	if n.SubjectId != "acme/main/nov" || n.ActorUserId != writer.UserID {
		t.Fatal(n)
	}
	assertInbox(t, f, reader.UserID)
	assertInbox(t, f, former.UserID, notifications.AccessChanged)
	// A caller rolling back a hard close also rolls back its notification.
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	scoped := ledger.Service{DB: tx, Dialect: dialect}
	if _, err := scoped.SetPeriodStatus(ctx, p.Company, p.Book, p.Name, ledger.PeriodHardClosed, "operator", p.Revision+1); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertInbox(t, f, writer.UserID, notifications.PeriodClosed, notifications.AccessChanged)
}
