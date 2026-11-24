// Package integrations_test exercises the provider inbox against
// MariaDB. It runs as an external test package so it can migrate with
// server.SystemMigrations; the property test for provider event
// verification lives in inbox_test.go (batch-2 15.3, Property 26).
package integrations_test

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"athena-erp/internal/integrations"
	"athena-erp/internal/secrets"
	"athena-erp/internal/server"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
)

// migrate returns a database with every system migration applied.
func migrate(t *testing.T) *sql.DB {
	t.Helper()
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, store.MariaDB{}, server.SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// keyFixture registers a secrets reference with the numbered versions
// active and writes their key files, returning the material by version.
func keyFixture(t *testing.T, db *sql.DB, p *secrets.FileProvider, root, name string, versions ...int64) map[int64][]byte {
	t.Helper()
	ctx := context.Background()
	if _, err := secrets.SaveRef(ctx, db, name, p.Name(), name); err != nil {
		t.Fatalf("save ref: %v", err)
	}
	material := map[int64][]byte{}
	for _, v := range versions {
		if err := secrets.AddVersion(ctx, db, name, v); err != nil {
			t.Fatalf("add version: %v", err)
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			t.Fatal(err)
		}
		full := filepath.Join(root, name, "v"+strconv.FormatInt(v, 10))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, key, 0o600); err != nil {
			t.Fatal(err)
		}
		material[v] = key
	}
	return material
}

// sign returns the hex HMAC-SHA256 of body under key.
func sign(key, body []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

var webhookBody = []byte(`{"merchant":"acme","amount":"12.30","identity":"cus-1"}`)

func TestIngest(t *testing.T) {
	ctx := context.Background()
	db := migrate(t)
	root := t.TempDir()
	p := secrets.NewFileProvider(root)
	keys := keyFixture(t, db, p, root, "pay", 1)
	inbox := &integrations.Inbox{Secrets: p}
	req := integrations.IngestRequest{Provider: "acme-pay", EventID: "evt-1", KeyName: "pay", Body: webhookBody, Signature: sign(keys[1], webhookBody)}
	opID, duplicate, err := inbox.Ingest(ctx, db, store.MariaDB{}, req)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if duplicate {
		t.Fatal("first ingest reports duplicate, want false")
	}
	if opID != integrations.MapOperationID("acme-pay", "evt-1") {
		t.Fatalf("operation id is %q, want the mapped id", opID)
	}
	var eventType, operationID, got string
	if err := db.QueryRowContext(ctx, `SELECT event_type, operation_id, CAST(payload AS CHAR) FROM athena_events`).Scan(&eventType, &operationID, &got); err != nil {
		t.Fatalf("evidence event: %v", err)
	}
	if eventType != integrations.TypeProviderReceived || operationID != opID {
		t.Fatalf("evidence event is %q %q, want provider.received with the mapped id", eventType, operationID)
	}
	if !strings.Contains(got, `"key_version":1`) {
		t.Fatalf("evidence payload %s lacks the verifying version", got)
	}
	again, duplicate, err := inbox.Ingest(ctx, db, store.MariaDB{}, req)
	if err != nil {
		t.Fatalf("reingest: %v", err)
	}
	if !duplicate || again != opID {
		t.Fatalf("reingest returns %q duplicate=%v, want the same id as duplicate", again, duplicate)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_events`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("event count = %d, %v, want exactly one recorded event", count, err)
	}
}

func TestIngestKeyWindow(t *testing.T) {
	ctx := context.Background()
	db := migrate(t)
	root := t.TempDir()
	p := secrets.NewFileProvider(root)
	keys := keyFixture(t, db, p, root, "pay", 1)
	if err := secrets.AddVersion(ctx, db, "pay", 2); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(root, "pay", "v2")
	second := make([]byte, 32)
	if _, err := rand.Read(second); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, second, 0o600); err != nil {
		t.Fatal(err)
	}
	keys[2] = second
	if err := secrets.SetVersionStatus(ctx, db, "pay", 1, secrets.VersionRetired); err != nil {
		t.Fatal(err)
	}
	inbox := &integrations.Inbox{Secrets: p}
	signed := func(key []byte, id string) integrations.IngestRequest {
		return integrations.IngestRequest{Provider: "acme-pay", EventID: id, KeyName: "pay", Body: webhookBody, Signature: sign(key, webhookBody)}
	}
	if v, err := inbox.Verify(ctx, db, signed(keys[2], "evt-new")); err != nil || v != 2 {
		t.Fatalf("active-key verify = v%d, %v, want v2, nil", v, err)
	}
	if v, err := inbox.Verify(ctx, db, signed(keys[1], "evt-old")); err != nil || v != 1 {
		t.Fatalf("previous-key verify = v%d, %v, want v1, nil", v, err)
	}
	// Age the rotation past the window: the previous key stops verifying.
	if _, err := db.ExecContext(ctx, `UPDATE athena_key_versions SET created_at = ? WHERE key_name = 'pay' AND version = 2`, time.Now().UTC().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	tight := &integrations.Inbox{Secrets: p, Window: time.Hour}
	if _, err := tight.Verify(ctx, db, signed(keys[1], "evt-old")); err == nil {
		t.Fatal("previous-key verify after the window succeeded, want an error")
	}
	if v, err := tight.Verify(ctx, db, signed(keys[2], "evt-new")); err != nil || v != 2 {
		t.Fatalf("active-key verify after the window = v%d, %v, want v2, nil", v, err)
	}
}

func TestDedupeIgnoresTheKey(t *testing.T) {
	ctx := context.Background()
	db := migrate(t)
	root := t.TempDir()
	p := secrets.NewFileProvider(root)
	keys := keyFixture(t, db, p, root, "pay", 1, 2)
	inbox := &integrations.Inbox{Secrets: p}
	first := integrations.IngestRequest{Provider: "acme-pay", EventID: "evt-9", KeyName: "pay", Body: webhookBody, Signature: sign(keys[1], webhookBody)}
	opID, duplicate, err := inbox.Ingest(ctx, db, store.MariaDB{}, first)
	if err != nil || duplicate {
		t.Fatalf("first ingest = %q duplicate=%v, %v", opID, duplicate, err)
	}
	second := first
	second.Signature = sign(keys[2], webhookBody)
	again, duplicate, err := inbox.Ingest(ctx, db, store.MariaDB{}, second)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if !duplicate || again != opID {
		t.Fatalf("same event under another key returns %q duplicate=%v, want the same id as duplicate", again, duplicate)
	}
}

func TestIngestRejects(t *testing.T) {
	ctx := context.Background()
	db := migrate(t)
	root := t.TempDir()
	p := secrets.NewFileProvider(root)
	keys := keyFixture(t, db, p, root, "pay", 1)
	inbox := &integrations.Inbox{Secrets: p}
	good := integrations.IngestRequest{Provider: "acme-pay", EventID: "evt-1", KeyName: "pay", Body: webhookBody, Signature: sign(keys[1], webhookBody)}
	badBody := append([]byte(nil), webhookBody...)
	badBody[len(badBody)-2]++
	cases := map[string]integrations.IngestRequest{
		"bad signature":    {Provider: "acme-pay", EventID: "evt-1", KeyName: "pay", Body: webhookBody, Signature: sign(keys[1], badBody)},
		"tampered body":    {Provider: "acme-pay", EventID: "evt-1", KeyName: "pay", Body: badBody, Signature: sign(keys[1], webhookBody)},
		"empty body":       {Provider: "acme-pay", EventID: "evt-1", KeyName: "pay", Body: nil, Signature: sign(keys[1], webhookBody)},
		"not json":         {Provider: "acme-pay", EventID: "evt-1", KeyName: "pay", Body: []byte("nope"), Signature: "00"},
		"missing merchant": {Provider: "acme-pay", EventID: "evt-1", KeyName: "pay", Body: []byte(`{"amount":"1","identity":"i"}`), Signature: "00"},
		"missing amount":   {Provider: "acme-pay", EventID: "evt-1", KeyName: "pay", Body: []byte(`{"merchant":"m","identity":"i"}`), Signature: "00"},
		"missing identity": {Provider: "acme-pay", EventID: "evt-1", KeyName: "pay", Body: []byte(`{"merchant":"m","amount":"1"}`), Signature: "00"},
		"empty provider":   {Provider: "", EventID: "evt-1", KeyName: "pay", Body: webhookBody, Signature: sign(keys[1], webhookBody)},
		"empty event":      {Provider: "acme-pay", EventID: "", KeyName: "pay", Body: webhookBody, Signature: sign(keys[1], webhookBody)},
		"long provider":    {Provider: strings.Repeat("p", 37), EventID: "evt-1", KeyName: "pay", Body: webhookBody, Signature: sign(keys[1], webhookBody)},
		"long event":       {Provider: "acme-pay", EventID: strings.Repeat("e", 201), KeyName: "pay", Body: webhookBody, Signature: sign(keys[1], webhookBody)},
		"unknown key":      {Provider: "acme-pay", EventID: "evt-1", KeyName: "ghost", Body: webhookBody, Signature: sign(keys[1], webhookBody)},
	}
	for name, req := range cases {
		if _, _, err := inbox.Ingest(ctx, db, store.MariaDB{}, req); err == nil {
			t.Errorf("%s: ingest succeeded, want an error", name)
		} else if strings.Contains(err.Error(), string(webhookBody)) || strings.Contains(err.Error(), req.Signature) {
			t.Errorf("%s: error carries request material: %v", name, err)
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_events`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("event count = %d, %v, want no recorded events", count, err)
	}
	if _, _, err := inbox.Ingest(ctx, db, store.MariaDB{}, good); err != nil {
		t.Fatalf("good ingest after rejects: %v", err)
	}
}

func TestMapOperationID(t *testing.T) {
	first := integrations.MapOperationID("acme-pay", "evt-1")
	if first != integrations.MapOperationID("acme-pay", "evt-1") {
		t.Fatal("mapping is not deterministic")
	}
	if !strings.HasPrefix(first, "inbox-") || len(first) > 140 {
		t.Fatalf("mapping %q must start with inbox- and fit 140 characters", first)
	}
	if first == integrations.MapOperationID("acme-pay", "evt-2") || first == integrations.MapOperationID("other", "evt-1") {
		t.Fatal("distinct events share an Operation_ID")
	}
}
