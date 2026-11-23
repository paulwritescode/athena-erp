// Package auth_test holds the invite and reset delivery tests (batch-2
// G8.11): issuing a Challenge enqueues an outbox delivery event where the
// user has a destination, and keeps the manual code path otherwise.
package auth_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"athena-erp/internal/auth"
)

func countDeliveryEvents(t *testing.T, db *sql.DB, eventType string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_events WHERE event_type = ?`, eventType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func outboxStatusFor(t *testing.T, db *sql.DB, eventID string) string {
	t.Helper()
	var status string
	if err := db.QueryRow(`SELECT status FROM athena_outbox WHERE event_id = ?`, eventID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestInviteEnqueuesDeliveryWhereDestinationExists(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	user, token, err := svc.CreateUserWithDelivery(ctx, "bert", "Bert", false, "hq-relay", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("no invite Challenge was returned in destination mode")
	}
	assertDelivery := func(eventType, kind string, generation int64) string {
		t.Helper()
		var id, producer, subjectType, subjectID, actor, correlation, payloadRaw string
		var revision int64
		if err := svc.DB.QueryRow(`SELECT id, producer, subject_type, subject_id, subject_revision,
			actor, correlation_id, CAST(payload AS CHAR) FROM athena_events
			WHERE event_type = ? ORDER BY sequence DESC LIMIT 1`, eventType).
			Scan(&id, &producer, &subjectType, &subjectID, &revision, &actor, &correlation, &payloadRaw); err != nil {
			t.Fatal(err)
		}
		if producer != "auth" || subjectType != "user" || subjectID != user.ID || revision != generation {
			t.Fatalf("event = %s %s/%s rev %d, want auth user/%s rev %d",
				producer, subjectType, subjectID, revision, user.ID, generation)
		}
		if actor != "" {
			t.Fatalf("actor = %q, want empty for the operator path", actor)
		}
		if correlation == "" {
			t.Fatal("the event carries no correlation id")
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["destination"] != "hq-relay" || payload["kind"] != kind {
			t.Fatalf("payload = %v, want the destination alias and kind %q", payload, kind)
		}
		if payload["challenge_id"] == nil || payload["challenge_id"] == "" {
			t.Fatalf("payload = %v, want a Challenge reference", payload)
		}
		if payload["expires_at"] == nil || payload["expires_at"] == "" {
			t.Fatalf("payload = %v, want an expiry", payload)
		}
		if strings.Contains(payloadRaw, token) {
			t.Fatal("the Challenge value is in the outbox payload; custody is Z4-undecided")
		}
		if status := outboxStatusFor(t, svc.DB, id); status != "pending" {
			t.Fatalf("outbox status = %q, want pending", status)
		}
		return payload["challenge_id"].(string)
	}
	firstChallenge := assertDelivery(auth.TypeInviteCreated, auth.DeliveryKindInvite, 0)
	if countDeliveryEvents(t, svc.DB, auth.TypeInviteCreated) != 1 {
		t.Fatal("the invite wrote more than one event")
	}
	// A resend enqueues again with the new Challenge reference.
	_, resendToken, err := svc.ResendInvite(ctx, "bert", "request-2")
	if err != nil {
		t.Fatal(err)
	}
	if resendToken == "" {
		t.Fatal("no resend Challenge was returned in destination mode")
	}
	secondChallenge := assertDelivery(auth.TypeInviteCreated, auth.DeliveryKindInvite, 1)
	if firstChallenge == secondChallenge {
		t.Fatal("the resend reused the Challenge reference")
	}
	// The returned code still completes the flow: delivery is additive.
	if _, err := svc.CompleteChallenge(ctx, "bert", auth.ChallengePurposeFirstLogin, resendToken, "a first login passphrase", "request-3"); err != nil {
		t.Fatalf("complete after delivery: %v", err)
	}
}

func TestInviteKeepsManualPathWithoutDestination(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	user, token, err := svc.CreateUser(ctx, "cara", "Cara", false, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("no invite Challenge was returned on the manual path")
	}
	if user.Delivery != auth.DeliveryManual {
		t.Fatalf("delivery = %q, want manual", user.Delivery)
	}
	if _, _, err := svc.ResendInvite(ctx, "cara", "request-2"); err != nil {
		t.Fatal(err)
	}
	if count := countDeliveryEvents(t, svc.DB, auth.TypeInviteCreated); count != 0 {
		t.Fatalf("%d invite events, want none on the manual path", count)
	}
	var outbox int
	if err := svc.DB.QueryRow(`SELECT COUNT(*) FROM athena_outbox`).Scan(&outbox); err != nil || outbox != 0 {
		t.Fatalf("outbox rows = %d, %v, want none on the manual path", outbox, err)
	}
}

func TestResetEnqueuesDeliveryWhereDestinationExists(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	const password = "a first login passphrase"
	_, token, err := svc.CreateUser(ctx, "dana", "Dana", false, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteChallenge(ctx, "dana", auth.ChallengePurposeFirstLogin, token, password, "request-2"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.StartReset(ctx, "dana", "", "request-3"); err != nil {
		t.Fatal(err)
	}
	if count := countDeliveryEvents(t, svc.DB, auth.TypeResetStarted); count != 0 {
		t.Fatalf("%d reset events, want none while manual", count)
	}
	destination := "hq-relay"
	updated, err := svc.UpdateUser(ctx, "dana", auth.UserUpdate{Delivery: &destination})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Delivery != destination {
		t.Fatalf("delivery = %q, want the alias", updated.Delivery)
	}
	_, resetToken, err := svc.StartReset(ctx, "dana", "", "request-4")
	if err != nil {
		t.Fatal(err)
	}
	if resetToken == "" {
		t.Fatal("no reset Challenge was returned in destination mode")
	}
	var kind, challengeRef, payloadRaw, eventID string
	var revision int64
	if err := svc.DB.QueryRow(`SELECT id, subject_revision, CAST(payload AS CHAR) FROM athena_events
		WHERE event_type = ?`, auth.TypeResetStarted).Scan(&eventID, &revision, &payloadRaw); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
		t.Fatal(err)
	}
	kind, _ = payload["kind"].(string)
	challengeRef, _ = payload["challenge_id"].(string)
	if kind != auth.DeliveryKindReset || payload["destination"] != destination || challengeRef == "" || revision != 2 {
		t.Fatalf("payload = %v rev %d, want kind reset with reference at generation 2", payload, revision)
	}
	if strings.Contains(payloadRaw, resetToken) {
		t.Fatal("the Challenge value is in the outbox payload; custody is Z4-undecided")
	}
	if status := outboxStatusFor(t, svc.DB, eventID); status != "pending" {
		t.Fatalf("outbox status = %q, want pending", status)
	}
	if _, err := svc.CompleteChallenge(ctx, "dana", auth.ChallengePurposeReset, resetToken, "a reset passphrase here", "request-5"); err != nil {
		t.Fatalf("complete reset after delivery: %v", err)
	}
	// Back to manual, a further reset enqueues nothing.
	manual := auth.DeliveryManual
	if _, err := svc.UpdateUser(ctx, "dana", auth.UserUpdate{Delivery: &manual}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.StartReset(ctx, "dana", "", "request-6"); err != nil {
		t.Fatal(err)
	}
	if count := countDeliveryEvents(t, svc.DB, auth.TypeResetStarted); count != 1 {
		t.Fatalf("%d reset events, want still 1 after returning to manual", count)
	}
}

func TestDeliveryChannelValidation(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	reserved := auth.DeliverySetup
	if _, _, err := svc.CreateUserWithDelivery(ctx, "erin", "Erin", false, reserved, "request-1"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("reserved channel at create = %v, want ErrInvalid", err)
	}
	if _, _, err := svc.CreateUser(ctx, "erin", "Erin", false, "request-2"); err != nil {
		t.Fatal(err)
	}
	blank := "   "
	tooLong := strings.Repeat("x", 21)
	for name, channel := range map[string]string{"blank": blank, "reserved": reserved, "too long": tooLong} {
		if _, err := svc.UpdateUser(ctx, "erin", auth.UserUpdate{Delivery: &channel}); !errors.Is(err, auth.ErrInvalid) {
			t.Fatalf("%s channel = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := svc.EnsureRoot(ctx, "a root passphrase here"); err != nil {
		t.Fatal(err)
	}
	alias := "hq-relay"
	if _, err := svc.UpdateUser(ctx, auth.RootUsername, auth.UserUpdate{Delivery: &alias}); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("root delivery change = %v, want ErrInvalid", err)
	}
}
