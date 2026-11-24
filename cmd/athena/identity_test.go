package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/testdb"
	"athena-erp/internal/workspace"
)

func identityTestPNG(t *testing.T) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 9, G: uint8(x), B: uint8(y), A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "logo.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWorkspaceIdentityCommands(t *testing.T) {
	slug := newTestWorkspace(t)
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := workspaceCommand(append([]string{"identity"}, append(args, "--workspace", slug)...), &out)
		return out.String(), err
	}
	show := func() *v1.GetIdentityResponse {
		t.Helper()
		out, err := run("show")
		if err != nil {
			t.Fatal(err)
		}
		var res v1.GetIdentityResponse
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatalf("show: %v %s", err, out)
		}
		return &res
	}
	if got := show(); got.GetIdentity().GetDisplayName() == "" {
		t.Fatalf("fresh identity has no default name: %+v", got.GetIdentity())
	}
	if _, err := run("set", "--name", "Acme"); err != nil {
		t.Fatal(err)
	}
	if got := show(); got.GetIdentity().GetDisplayName() != "Acme" {
		t.Fatalf("name: %+v", got.GetIdentity())
	}
	out, err := run("set", "--logo-light", identityTestPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	var set v1.SetIdentityResponse
	if err := json.Unmarshal([]byte(out), &set); err != nil {
		t.Fatal(err)
	}
	if url := set.GetIdentity().GetLogoLightUrl(); !strings.HasPrefix(url, "/brand/logo-") || !strings.HasSuffix(url, ".png") {
		t.Fatalf("logo URL: %q", url)
	}
	bad := filepath.Join(t.TempDir(), "bad.png")
	if err := os.WriteFile(bad, []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run("set", "--logo-dark", bad); err == nil || !strings.Contains(err.Error(), "logo") {
		t.Fatalf("bad logo: %v", err)
	}
	if got := show(); got.GetIdentity().GetLogoDarkUrl() != "" {
		t.Fatalf("failed save stored a logo: %+v", got.GetIdentity())
	}
}

func TestNewLogoFlag(t *testing.T) {
	// Private database: this test provisions a real workspace database
	// (dropped in cleanup); the main database is only its admin handle.
	db, cfg := testdb.NewWithConfig(t)
	server := cfg.Clone()
	server.DBName = ""
	home := t.TempDir()
	if err := workspace.InitHome(home, server.FormatDSN()); err != nil {
		t.Fatal(err)
	}
	t.Chdir(home)
	png := identityTestPNG(t)
	// The database name must start with test_ for the app user to create it.
	name := "test b5 logo " + strings.ReplaceAll(filepath.Base(t.TempDir()), "_", "")
	var out bytes.Buffer
	if err := newCommand([]string{"--logo", png, name}, &out); err != nil {
		t.Fatal(err)
	}
	var slug string
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := workspace.LoadManifest(filepath.Join(home, entry.Name()))
		if err != nil {
			continue
		}
		slug = manifest.Slug
		t.Cleanup(func() { _, _ = db.Exec("DROP DATABASE IF EXISTS `" + manifest.Database.Name + "`") })
	}
	if slug == "" {
		t.Fatalf("no workspace created in %s", home)
	}
	var show bytes.Buffer
	if err := workspaceCommand([]string{"identity", "show", "--workspace", slug}, &show); err != nil {
		t.Fatal(err)
	}
	var res v1.GetIdentityResponse
	if err := json.Unmarshal(show.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.GetIdentity().GetDisplayName() != name || res.GetIdentity().GetLogoLightUrl() == "" {
		t.Fatalf("identity: %+v", res.GetIdentity())
	}
}

func TestNewLogoMissingFileFailsFast(t *testing.T) {
	newTestHome(t, workspace.DriverMariaDB, "athena:secret@tcp(127.0.0.1:1)/")
	var out bytes.Buffer
	err := newCommand([]string{"--logo", filepath.Join(t.TempDir(), "missing.png"), "Acme"}, &out)
	if err == nil || !strings.Contains(err.Error(), "missing.png") {
		t.Fatalf("err = %v, want the missing file before any database work", err)
	}
}
