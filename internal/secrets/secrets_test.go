// Package secrets_test exercises secret references, key versions, the
// local file provider and field encryption against MariaDB. It runs as
// an external test package so it can migrate with
// server.SystemMigrations; the property test for field encryption lives
// in keys_test.go (batch-2 8.9, Property 16).
package secrets_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// writeKeyFile stores 32 random bytes at root/locator.
func writeKeyFile(t *testing.T, root, locator string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(locator))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, key, 0o600); err != nil {
		t.Fatal(err)
	}
}

// fixtureKey registers reference name with the file provider, records
// version 1 as active, and writes its key file.
func fixtureKey(t *testing.T, db *sql.DB, p *secrets.FileProvider, root, name string) {
	t.Helper()
	ctx := context.Background()
	if _, err := secrets.SaveRef(ctx, db, name, p.Name(), name); err != nil {
		t.Fatalf("save ref: %v", err)
	}
	if err := secrets.AddVersion(ctx, db, name, 1); err != nil {
		t.Fatalf("add version: %v", err)
	}
	writeKeyFile(t, root, name+"/v1")
}

func TestSaveGetAndDeleteRef(t *testing.T) {
	ctx := context.Background()
	db := migrate(t)
	ref, err := secrets.SaveRef(ctx, db, "mail-relay", "file", "relays/mail")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if ref.ID == "" || ref.Name != "mail-relay" || ref.Provider != "file" || ref.ProviderRef != "relays/mail" {
		t.Fatalf("saved ref is %+v, want the stored row back", ref)
	}
	updated, err := secrets.SaveRef(ctx, db, "mail-relay", "file", "relays/mail-v2")
	if err != nil {
		t.Fatalf("resave: %v", err)
	}
	if updated.ID != ref.ID || updated.ProviderRef != "relays/mail-v2" {
		t.Fatalf("resaved ref is %+v, want the same id with the new locator", updated)
	}
	got, err := secrets.GetRef(ctx, db, "mail-relay")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != updated {
		t.Fatalf("loaded ref is %+v, want %+v", got, updated)
	}
	if err := secrets.DeleteRef(ctx, db, "mail-relay"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := secrets.GetRef(ctx, db, "mail-relay"); err == nil {
		t.Fatal("get after delete succeeded, want not found")
	}
	if err := secrets.DeleteRef(ctx, db, "mail-relay"); err == nil {
		t.Fatal("second delete succeeded, want not found")
	}
}

func TestSaveRefRejectsBadNames(t *testing.T) {
	ctx := context.Background()
	db := migrate(t)
	for _, name := range []string{"", "has:colon", strings.Repeat("n", 141)} {
		if _, err := secrets.SaveRef(ctx, db, name, "file", "keys/x"); err == nil {
			t.Errorf("save %q succeeded, want a validation error", name)
		}
	}
	if _, err := secrets.SaveRef(ctx, db, "ok", "", "keys/x"); err == nil {
		t.Error("save with an empty provider succeeded, want a validation error")
	}
	if _, err := secrets.SaveRef(ctx, db, "ok", "file", ""); err == nil {
		t.Error("save with an empty locator succeeded, want a validation error")
	}
}

func TestKeyVersions(t *testing.T) {
	ctx := context.Background()
	db := migrate(t)
	if err := secrets.AddVersion(ctx, db, "workspace", 1); err != nil {
		t.Fatalf("add v1: %v", err)
	}
	if err := secrets.AddVersion(ctx, db, "workspace", 1); err == nil {
		t.Fatal("re-adding v1 succeeded, want a duplicate error")
	}
	if err := secrets.AddVersion(ctx, db, "workspace", 0); err == nil {
		t.Fatal("adding v0 succeeded, want a validation error")
	}
	active, err := secrets.ActiveVersion(ctx, db, "workspace")
	if err != nil || active != 1 {
		t.Fatalf("active version is %d, %v, want 1, nil", active, err)
	}
	if err := secrets.AddVersion(ctx, db, "workspace", 2); err != nil {
		t.Fatalf("add v2: %v", err)
	}
	if err := secrets.SetVersionStatus(ctx, db, "workspace", 1, secrets.VersionRetired); err != nil {
		t.Fatalf("retire v1: %v", err)
	}
	active, err = secrets.ActiveVersion(ctx, db, "workspace")
	if err != nil || active != 2 {
		t.Fatalf("active version is %d, %v, want 2, nil", active, err)
	}
	if err := secrets.SetVersionStatus(ctx, db, "workspace", 2, "bogus"); err == nil {
		t.Fatal("bogus status succeeded, want a validation error")
	}
	if err := secrets.SetVersionStatus(ctx, db, "workspace", 9, secrets.VersionRetired); err == nil {
		t.Fatal("retiring missing v9 succeeded, want not found")
	}
	if _, err := secrets.ActiveVersion(ctx, db, "missing"); err == nil {
		t.Fatal("active version of a missing key succeeded, want an error")
	}
}

func TestFileProvider(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	p := secrets.NewFileProvider(root)
	if p.Name() != "file" {
		t.Fatalf("provider name is %q, want file", p.Name())
	}
	writeKeyFile(t, root, "workspace/v1")
	key, err := p.Key(ctx, "workspace/v1")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key holds %d bytes, want 32", len(key))
	}
	if _, err := p.Key(ctx, "workspace/missing"); err == nil {
		t.Fatal("missing key file succeeded, want an error")
	}
	for _, locator := range []string{"", "/etc/passwd", "../escape", "sub/../../escape", ".."} {
		if _, err := p.Key(ctx, locator); err == nil {
			t.Errorf("locator %q succeeded, want a refusal", locator)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "short"), make([]byte, 31), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Key(ctx, "short"); err == nil {
		t.Fatal("31-byte key file succeeded, want a length error")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := migrate(t)
	root := t.TempDir()
	p := secrets.NewFileProvider(root)
	fixtureKey(t, db, p, root, "workspace")
	c := secrets.Context{Workspace: "acme", RecordType: "invoice", RecordID: "01911111-2222-7333-8444-555555555555", Field: "iban", KeyName: "workspace"}
	envelope, err := secrets.Encrypt(ctx, db, p, c, []byte("DE89370400440532013000"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(envelope, "athsec1:workspace:1:") {
		t.Fatalf("envelope %q lacks the key binding prefix", envelope)
	}
	if strings.Contains(envelope, "DE89370400440532013000") {
		t.Fatal("envelope carries the plaintext")
	}
	plaintext, err := secrets.Decrypt(ctx, db, p, secrets.Context{Workspace: "acme", RecordType: "invoice", RecordID: "01911111-2222-7333-8444-555555555555", Field: "iban"}, envelope)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(plaintext) != "DE89370400440532013000" {
		t.Fatalf("decrypted %q, want the IBAN back", plaintext)
	}
}

func TestDecryptRejectsACopiedValue(t *testing.T) {
	ctx := context.Background()
	db := migrate(t)
	root := t.TempDir()
	p := secrets.NewFileProvider(root)
	fixtureKey(t, db, p, root, "workspace")
	const iban = "DE89370400440532013000"
	envelope, err := secrets.Encrypt(ctx, db, p, secrets.Context{Workspace: "acme", RecordType: "invoice", RecordID: "01911111-2222-7333-8444-555555555555", Field: "iban", KeyName: "workspace"}, []byte(iban))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	for name, c := range map[string]secrets.Context{
		"other record": {Workspace: "acme", RecordType: "invoice", RecordID: "01999999-2222-7333-8444-555555555555", Field: "iban"},
		"other field":  {Workspace: "acme", RecordType: "invoice", RecordID: "01911111-2222-7333-8444-555555555555", Field: "bic"},
		"other type":   {Workspace: "acme", RecordType: "bill", RecordID: "01911111-2222-7333-8444-555555555555", Field: "iban"},
		"other space":  {Workspace: "globex", RecordType: "invoice", RecordID: "01911111-2222-7333-8444-555555555555", Field: "iban"},
	} {
		plaintext, err := secrets.Decrypt(ctx, db, p, c, envelope)
		if err == nil {
			t.Errorf("decrypt for %s succeeded with %q, want a binding failure", name, plaintext)
		} else if strings.Contains(err.Error(), iban) {
			t.Errorf("decrypt error for %s carries the plaintext: %v", name, err)
		}
	}
}

func TestDecryptRefusesRevokedAndUnknownVersions(t *testing.T) {
	ctx := context.Background()
	db := migrate(t)
	root := t.TempDir()
	p := secrets.NewFileProvider(root)
	fixtureKey(t, db, p, root, "workspace")
	c := secrets.Context{Workspace: "acme", RecordType: "invoice", RecordID: "01911111-2222-7333-8444-555555555555", Field: "iban", KeyName: "workspace"}
	envelope, err := secrets.Encrypt(ctx, db, p, c, []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := secrets.SetVersionStatus(ctx, db, "workspace", 1, secrets.VersionRetired); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if _, err := secrets.Decrypt(ctx, db, p, secrets.Context{Workspace: "acme", RecordType: "invoice", RecordID: "01911111-2222-7333-8444-555555555555", Field: "iban"}, envelope); err != nil {
		t.Fatalf("decrypt under a retired version failed: %v", err)
	}
	if err := secrets.SetVersionStatus(ctx, db, "workspace", 1, secrets.VersionRevoked); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := secrets.Decrypt(ctx, db, p, secrets.Context{Workspace: "acme", RecordType: "invoice", RecordID: "01911111-2222-7333-8444-555555555555", Field: "iban"}, envelope); err == nil {
		t.Fatal("decrypt under a revoked version succeeded, want a refusal")
	}
	tampered := strings.Replace(envelope, ":workspace:1:", ":workspace:9:", 1)
	if _, err := secrets.Decrypt(ctx, db, p, secrets.Context{Workspace: "acme", RecordType: "invoice", RecordID: "01911111-2222-7333-8444-555555555555", Field: "iban"}, tampered); err == nil {
		t.Fatal("decrypt of an unknown version succeeded, want an error")
	}
	for _, bad := range []string{"", "not-an-envelope", "athsec1:workspace:1:only-four", "athsec1:workspace:x:bm9uY2U:Y2lwaGVy"} {
		if _, err := secrets.Decrypt(ctx, db, p, secrets.Context{Workspace: "acme", RecordType: "invoice", RecordID: "01911111-2222-7333-8444-555555555555", Field: "iban"}, bad); err == nil {
			t.Errorf("decrypt of %q succeeded, want an error", bad)
		}
	}
}

func TestEncryptNeedsARegisteredRefAndProvider(t *testing.T) {
	ctx := context.Background()
	db := migrate(t)
	root := t.TempDir()
	p := secrets.NewFileProvider(root)
	fixtureKey(t, db, p, root, "workspace")
	c := secrets.Context{Workspace: "acme", RecordType: "invoice", RecordID: "01911111-2222-7333-8444-555555555555", Field: "iban", KeyName: "ghost"}
	if _, err := secrets.Encrypt(ctx, db, p, c, []byte("secret")); err == nil {
		t.Fatal("encrypt with an unregistered key succeeded, want an error")
	}
	if _, err := secrets.SaveRef(ctx, db, "vaulted", "vault", "vaulted"); err != nil {
		t.Fatalf("save foreign ref: %v", err)
	}
	if err := secrets.AddVersion(ctx, db, "vaulted", 1); err != nil {
		t.Fatalf("add foreign version: %v", err)
	}
	c.KeyName = "vaulted"
	if _, err := secrets.Encrypt(ctx, db, p, c, []byte("secret")); err == nil {
		t.Fatal("encrypt through the wrong provider succeeded, want a provider error")
	}
}

func TestSecretsErrorsCarryNoSecretMaterial(t *testing.T) {
	ctx := context.Background()
	db := migrate(t)
	root := t.TempDir()
	p := secrets.NewFileProvider(root)
	fixtureKey(t, db, p, root, "workspace")
	const value = "super-secret-iban-DE89370400440532013000"
	envelope, err := secrets.Encrypt(ctx, db, p, secrets.Context{Workspace: "acme", RecordType: "invoice", RecordID: "r1", Field: "iban", KeyName: "workspace"}, []byte(value))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	_, wrongErr := secrets.Decrypt(ctx, db, p, secrets.Context{Workspace: "acme", RecordType: "invoice", RecordID: "r2", Field: "iban"}, envelope)
	_, shortErr := p.Key(ctx, "short")
	errs := []error{wrongErr, shortErr}
	for _, err := range errs {
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if strings.Contains(err.Error(), value) || strings.Contains(err.Error(), envelope) {
			t.Errorf("error carries secret material: %v", err)
		}
	}
	if got := secrets.RedactValue([]byte(value)); got != secrets.Redacted {
		t.Errorf("RedactValue returned %q, want %q", got, secrets.Redacted)
	}
	if got := secrets.RedactString(value); got != secrets.Redacted {
		t.Errorf("RedactString returned %q, want %q", got, secrets.Redacted)
	}
}
