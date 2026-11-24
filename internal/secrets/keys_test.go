package secrets_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"pgregory.net/rapid"

	"athena-erp/internal/secrets"
)

// TestFieldEncryptionRoundTripAndBinding checks Property 16 over drawn
// keys, contexts and plaintexts: every envelope decrypts to its
// plaintext under the exact context it was sealed for, fails under any
// one-field change of that context, and never repeats ciphertext for
// the same plaintext (fresh nonce per seal).
//
// Feature: athena-batch-2, Property 16: Field encryption round trip and binding.
func TestFieldEncryptionRoundTripAndBinding(t *testing.T) {
	db := migrate(t)
	root := t.TempDir()
	p := secrets.NewFileProvider(root)
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		for _, table := range []string{"athena_secret_refs", "athena_key_versions"} {
			if _, err := db.Exec(`DELETE FROM ` + table); err != nil {
				t.Fatal(err)
			}
		}
		keyName := "k-" + rapid.StringMatching(`[a-z0-9-]{1,10}`).Draw(t, "key")
		top := rapid.IntRange(1, 3).Draw(t, "versions")
		c := secrets.Context{
			Workspace:  "w-" + rapid.StringMatching(`[a-z0-9-]{1,10}`).Draw(t, "workspace"),
			RecordType: "t-" + rapid.StringMatching(`[a-z0-9-]{1,10}`).Draw(t, "record_type"),
			RecordID:   "r-" + rapid.StringMatching(`[a-z0-9-]{1,10}`).Draw(t, "record_id"),
			Field:      "f-" + rapid.StringMatching(`[a-z0-9-]{1,10}`).Draw(t, "field"),
			KeyName:    keyName,
		}
		plaintext := rapid.SliceOfN(rapid.Byte(), 0, 64).Draw(t, "plaintext")
		if _, err := secrets.SaveRef(ctx, db, keyName, p.Name(), keyName); err != nil {
			t.Fatalf("save ref: %v", err)
		}
		for v := int64(1); v <= int64(top); v++ {
			if err := secrets.AddVersion(ctx, db, keyName, v); err != nil {
				t.Fatalf("add version: %v", err)
			}
			writeKeyBytes(t, root, keyName+"/v"+strconv.FormatInt(v, 10))
		}
		envelope, err := secrets.Encrypt(ctx, db, p, c, plaintext)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		again, err := secrets.Encrypt(ctx, db, p, c, plaintext)
		if err != nil {
			t.Fatalf("re-encrypt: %v", err)
		}
		if envelope == again {
			t.Fatal("two seals of one plaintext share ciphertext, want a fresh nonce per seal")
		}
		open := secrets.Context{Workspace: c.Workspace, RecordType: c.RecordType, RecordID: c.RecordID, Field: c.Field}
		back, err := secrets.Decrypt(ctx, db, p, open, envelope)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if string(back) != string(plaintext) {
			t.Fatal("round trip changed the plaintext")
		}
		moved := open
		switch rapid.IntRange(0, 3).Draw(t, "moved_field") {
		case 0:
			moved.Workspace = differ(t, open.Workspace, "moved_workspace")
		case 1:
			moved.RecordType = differ(t, open.RecordType, "moved_type")
		case 2:
			moved.RecordID = differ(t, open.RecordID, "moved_id")
		case 3:
			moved.Field = differ(t, open.Field, "moved_field_value")
		}
		if back, err := secrets.Decrypt(ctx, db, p, moved, envelope); err == nil {
			t.Fatalf("decrypt under a moved context succeeded with %q, want a binding failure", back)
		}
	})
}

// writeKeyBytes stores 32 rapid-drawn bytes at root/locator. It takes
// the rapid T because testing.TB helpers cannot run inside a check.
func writeKeyBytes(t *rapid.T, root, locator string) {
	full := filepath.Join(root, filepath.FromSlash(locator))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	key := rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(t, "key_material")
	if err := os.WriteFile(full, key, 0o600); err != nil {
		t.Fatal(err)
	}
}

// differ draws a value guaranteed to differ from orig.
func differ(t *rapid.T, orig, label string) string {
	alt := "x-" + rapid.StringMatching(`[a-z0-9-]{1,10}`).Draw(t, label)
	if alt == orig {
		alt += "-moved"
	}
	return alt
}
