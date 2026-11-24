package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"athena-erp/internal/secrets"
)

func TestSecretsCommands(t *testing.T) {
	db := sharedDSNEnv(t)
	run := func(command func([]string, io.Writer) error, args ...string) (string, error) {
		var out bytes.Buffer
		err := command(args, &out)
		return out.String(), err
	}
	if _, err := run(migrateCommand, "up"); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	out, err := run(secretsCommand, "save", "--key", "workspace", "--ref", "workspace")
	if err != nil || !strings.Contains(out, "saved reference workspace (file:workspace)") {
		t.Fatalf("save = %q, %v", out, err)
	}
	if _, err := run(secretsCommand, "save", "--key", "vaulted", "--provider", "vault", "--ref", "v"); err == nil {
		t.Fatal("save with a foreign provider succeeded, want an error")
	}
	keyDir := t.TempDir()
	material := func() string {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "key.bin")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	out, err = run(secretsCommand, "rotate", "--key", "workspace", "--from", material(), "--key-dir", keyDir)
	if err != nil || !strings.Contains(out, "rotated workspace to version 1 (first version)") {
		t.Fatalf("rotate bootstrap = %q, %v", out, err)
	}
	out, err = run(secretsCommand, "rotate", "--key", "workspace", "--from", material(), "--key-dir", keyDir)
	if err != nil || !strings.Contains(out, "rotated workspace to version 2 (version 1 retired)") {
		t.Fatalf("rotate = %q, %v", out, err)
	}
	ctx := context.Background()
	active, err := secrets.ActiveVersion(ctx, db, "workspace")
	if err != nil || active != 2 {
		t.Fatalf("active version is %d, %v, want 2, nil", active, err)
	}
	if _, err := os.Stat(filepath.Join(keyDir, "workspace", "v2")); err != nil {
		t.Fatalf("v2 key file missing: %v", err)
	}
	short := filepath.Join(t.TempDir(), "short.bin")
	if err := os.WriteFile(short, make([]byte, 16), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(secretsCommand, "rotate", "--key", "workspace", "--from", short, "--key-dir", keyDir); err == nil {
		t.Fatal("rotate with 16 bytes succeeded, want a length error")
	}
	if _, err := run(secretsCommand, "rotate", "--key", "ghost", "--from", material(), "--key-dir", keyDir); err == nil {
		t.Fatal("rotate of an unregistered key succeeded, want an error")
	}
	t.Setenv("ATHENA_SECRETS_DIR", "")
	if _, err := run(secretsCommand, "rotate", "--key", "workspace", "--from", material()); err == nil {
		t.Fatal("rotate with no key dir succeeded, want an error")
	}
	out, err = run(secretsCommand, "save", "--key", "relay", "--ref", "relays/mail", "--json")
	if err != nil || !strings.Contains(out, `"name": "relay"`) {
		t.Fatalf("save --json = %q, %v", out, err)
	}
}
