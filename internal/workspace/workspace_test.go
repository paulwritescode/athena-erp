package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindHomeUpgradesTheLegacyMarkerOnce(t *testing.T) {
	dir := t.TempDir()
	body := "format_version: 1\ndatabase:\n  driver: mariadb\n  credentials_env: ATHENA_SERVER_URL\n"
	if err := os.WriteFile(filepath.Join(dir, legacyHomeFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "acme")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := FindHome(nested)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(dir); func() string { g, _ := filepath.EvalSymlinks(found); return g }() != want {
		t.Fatalf("home = %q, want %q", found, dir)
	}
	if exists(filepath.Join(dir, legacyHomeFile)) || !exists(filepath.Join(dir, HomeFile)) {
		t.Fatal("marker was not renamed")
	}
	got, err := os.ReadFile(filepath.Join(dir, HomeFile))
	if err != nil || string(got) != body {
		t.Fatalf("content changed: %q, %v", got, err)
	}
	if upgraded, err := UpgradeLegacyHome(dir); err != nil || upgraded {
		t.Fatalf("second upgrade = %v, %v", upgraded, err)
	}
}

func TestUpgradeLegacyHomeRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{legacyHomeFile, HomeFile} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := UpgradeLegacyHome(dir); err == nil {
		t.Fatal("expected an error when both markers exist")
	}
}
