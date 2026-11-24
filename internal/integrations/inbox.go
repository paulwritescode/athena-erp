// Package integrations is the kernel's integration surface (batch-2
// G15, ADR-0035, Proposed): a verified provider inbox, content-addressed
// attachments, scoped search, comments and import/export, all through
// the same permission gate as Core reads and writes. This file is the
// inbox: it verifies a webhook's HMAC signature with secrets-held keys
// (the previous key stays valid for a bounded window after rotation),
// checks the merchant, amount and identity claims, dedupes by provider
// event id through the bus inbox, and maps each event to one
// Operation_ID. The emitted bus event and its inbox row are the
// evidence; the raw-body archive table lands with migration 25. There
// is deliberately no "mark paid" path from a provider callback.
package integrations

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"athena-erp/internal/events"
	"athena-erp/internal/secrets"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// TypeProviderReceived is the bus event per accepted provider webhook.
const TypeProviderReceived = "provider.received"

// ConsumerProviderInbox is the inbox consumer that dedupes provider
// events: one row per provider plus provider event id.
const ConsumerProviderInbox = "provider-inbox"

// DefaultRotationWindow bounds previous-key acceptance after a
// rotation: signatures under the retired key verify while the active
// version is younger than this.
const DefaultRotationWindow = 72 * time.Hour

// Inbox verifies provider webhooks. Secrets holds the HMAC key
// material; Window bounds previous-key acceptance (zero means the
// default). Now is the clock; nil means time.Now in UTC.
type Inbox struct {
	Secrets secrets.Provider
	Window  time.Duration
	Now     func() time.Time
}

// IngestRequest is one provider webhook delivery: the provider and
// its event id, the secrets reference holding the HMAC keys, the raw
// body and its hex HMAC-SHA256 signature (a "sha256=" prefix is
// accepted and stripped).
type IngestRequest struct {
	Provider  string
	EventID   string
	KeyName   string
	Body      []byte
	Signature string
}

// window returns the configured rotation window.
func (in *Inbox) window() time.Duration {
	if in.Window <= 0 {
		return DefaultRotationWindow
	}
	return in.Window
}

// now returns the inbox clock.
func (in *Inbox) now() time.Time {
	if in.Now != nil {
		return in.Now()
	}
	return time.Now().UTC()
}

// Verify checks a webhook's signature under the key window and returns
// the verifying version: the active version first, then the previous
// retired one while the rotation is younger than the window. It also
// checks the merchant, amount and identity claims in the body. Errors
// name the provider and event only, never the body or key material.
func (in *Inbox) Verify(ctx context.Context, q secrets.Querier, req IngestRequest) (int64, error) {
	if err := validateRequest(req); err != nil {
		return 0, err
	}
	if err := validateClaims(req.Body); err != nil {
		return 0, fmt.Errorf("integrations: provider %s event %s: %w", req.Provider, req.EventID, err)
	}
	want, err := decodeSignature(req.Signature)
	if err != nil {
		return 0, fmt.Errorf("integrations: provider %s event %s has no valid signature", req.Provider, req.EventID)
	}
	active, err := secrets.ActiveVersionInfo(ctx, q, req.KeyName)
	if err != nil {
		return 0, fmt.Errorf("integrations: provider %s event %s: %w", req.Provider, req.EventID, err)
	}
	key, err := secrets.KeyForVersion(ctx, q, in.Secrets, req.KeyName, active.Version)
	if err != nil {
		return 0, fmt.Errorf("integrations: provider %s event %s: %w", req.Provider, req.EventID, err)
	}
	if checkHMAC(key, req.Body, want) {
		return active.Version, nil
	}
	if active.Version > 1 && in.now().Sub(active.CreatedAt) < in.window() {
		previous, err := secrets.KeyForVersion(ctx, q, in.Secrets, req.KeyName, active.Version-1)
		if err == nil && checkHMAC(previous, req.Body, want) {
			return active.Version - 1, nil
		}
	}
	return 0, fmt.Errorf("integrations: provider %s event %s has no valid signature", req.Provider, req.EventID)
}

// Ingest verifies a webhook, records it once and maps it to its
// Operation_ID. A redelivered event returns the same Operation_ID with
// duplicate set and records nothing new. The span carries ids and the
// outcome only.
func (in *Inbox) Ingest(ctx context.Context, db *sql.DB, classifier events.Classifier, req IngestRequest) (operationID string, duplicate bool, err error) {
	ctx, span := otel.Tracer("athena/integrations").Start(ctx, "athena.integrations.ingest")
	defer span.End()
	outcome := "ok"
	defer func() { span.SetAttributes(attribute.String("athena.outcome", outcome)) }()
	operationID, duplicate, err = in.ingest(ctx, db, classifier, req)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		return "", false, err
	}
	span.SetAttributes(attribute.String("athena.provider", req.Provider),
		attribute.String("athena.operation_id", operationID),
		attribute.Bool("athena.duplicate", duplicate))
	return operationID, duplicate, nil
}

func (in *Inbox) ingest(ctx context.Context, db *sql.DB, classifier events.Classifier, req IngestRequest) (string, bool, error) {
	version, err := in.Verify(ctx, db, req)
	if err != nil {
		return "", false, err
	}
	operationID := MapOperationID(req.Provider, req.EventID)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback() }()
	claimed, err := events.Claim(ctx, tx, classifier, ConsumerProviderInbox, req.Provider, req.EventID, "")
	if err != nil {
		return "", false, err
	}
	if !claimed {
		if err := tx.Rollback(); err != nil {
			return "", false, err
		}
		return operationID, true, nil
	}
	_, err = events.Emit(ctx, tx, events.Envelope{
		EventType:   TypeProviderReceived,
		Producer:    req.Provider,
		SubjectType: "provider_event",
		SubjectID:   req.Provider + ":" + req.EventID,
		OperationID: operationID,
		Payload:     map[string]any{"provider": req.Provider, "provider_event_id": req.EventID, "key_version": version},
	})
	if err != nil {
		return "", false, err
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return operationID, false, nil
}

// MapOperationID deterministically maps a provider event to its
// Operation_ID. Equal inputs map equal: redeliveries share the id of
// their first sighting.
func MapOperationID(provider, eventID string) string {
	sum := sha256.Sum256([]byte(provider + "\x00" + eventID))
	return "inbox-" + hex.EncodeToString(sum[:])
}

// validateRequest checks the routable fields: a short provider name
// and event id that fit the inbox key columns, and a key reference.
func validateRequest(req IngestRequest) error {
	if req.Provider == "" || utf8.RuneCountInString(req.Provider) > 36 {
		return fmt.Errorf("integrations: provider name must be 1 to 36 characters")
	}
	if req.EventID == "" || utf8.RuneCountInString(req.EventID) > 200 {
		return fmt.Errorf("integrations: provider event id must be 1 to 200 characters")
	}
	if req.KeyName == "" {
		return fmt.Errorf("integrations: a secrets key name is required")
	}
	return nil
}

// validateClaims checks the webhook's merchant, amount and identity
// claims are present and shaped: non-empty merchant and identity
// strings and a present amount. There is no merchant registry in
// beta, so verification means presence and shape, never a lookup.
func validateClaims(body []byte) error {
	var claims map[string]any
	if err := json.Unmarshal(body, &claims); err != nil {
		return fmt.Errorf("claims are not JSON")
	}
	for _, field := range []string{"merchant", "identity"} {
		value, ok := claims[field].(string)
		if !ok || value == "" {
			return fmt.Errorf("claim %q must be a non-empty string", field)
		}
	}
	switch amount := claims["amount"].(type) {
	case string:
		if amount == "" {
			return fmt.Errorf("claim \"amount\" must be present")
		}
	case float64:
	default:
		return fmt.Errorf("claim \"amount\" must be present")
	}
	return nil
}

// decodeSignature strips an optional "sha256=" prefix and hex-decodes
// the signature.
func decodeSignature(signature string) ([]byte, error) {
	trimmed := strings.TrimPrefix(signature, "sha256=")
	if trimmed == "" {
		return nil, fmt.Errorf("empty")
	}
	return hex.DecodeString(trimmed)
}

// checkHMAC reports whether want is the HMAC-SHA256 of body under key,
// in constant time.
func checkHMAC(key, body, want []byte) bool {
	got := hmac.New(sha256.New, key)
	got.Write(body)
	return subtle.ConstantTimeCompare(got.Sum(nil), want) == 1
}
