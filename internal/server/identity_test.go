package server

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/files"
	"athena-erp/internal/ledger"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
	"connectrpc.com/connect/v2"
)

func testLogoPNG(t *testing.T, seed uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: seed, G: uint8(x), B: uint8(y), A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

const testLogoSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="8"><rect width="16" height="8" fill="red"/><script>alert(1)</script></svg>`

func TestWorkspaceIdentityPublicReadAndProtectedWrite(t *testing.T) {
	root := t.TempDir()
	f := newAccessFixtureAt(t, root)
	ctx := context.Background()
	anon := f.browser(t)

	got, err := anon.workspace.GetIdentity(ctx, &v1.GetIdentityRequest{})
	if err != nil {
		t.Fatalf("public GetIdentity: %v", err)
	}
	id := got.GetIdentity()
	if id.GetDisplayName() != "" || id.GetLogoLightUrl() != "" || id.GetLogoDarkUrl() != "" {
		t.Fatalf("fresh identity: %+v", id)
	}
	if id.GetFavicon_32Url() != "/brand/favicon-mark-32.png" || id.GetFavicon_180Url() != "/brand/favicon-mark-180.png" {
		t.Fatalf("fresh favicons: %+v", id)
	}

	_, err = anon.workspace.SetIdentity(ctx, &v1.SetIdentityRequest{DisplayName: "Acme"})
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestWorkspaceIdentityPermissionsAndLogos(t *testing.T) {
	root := t.TempDir()
	f := newAccessFixtureAt(t, root)
	ctx := context.Background()
	b := f.browser(t)

	f.role(t, "Clerk", []string{auth.PermissionLedgerRead})
	f.user(t, "clerk", "Clerk")
	b.login(t, "clerk")
	_, err := b.workspace.SetIdentity(ctx, &v1.SetIdentityRequest{DisplayName: "Acme"})
	if got := connect.CodeOf(err); got != connect.CodePermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied (%v)", got, err)
	}

	f.role(t, "Designer", []string{auth.PermissionWorkspace, auth.PermissionFilesRead})
	f.assign(t, "clerk", "Designer")
	b.login(t, "clerk")

	set := func(req *v1.SetIdentityRequest) *v1.Identity {
		t.Helper()
		res, err := b.workspace.SetIdentity(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		return res.GetIdentity()
	}
	get := func() *v1.Identity {
		t.Helper()
		res, err := b.workspace.GetIdentity(ctx, &v1.GetIdentityRequest{})
		if err != nil {
			t.Fatal(err)
		}
		return res.GetIdentity()
	}

	if id := set(&v1.SetIdentityRequest{DisplayName: "Acme"}); id.GetDisplayName() != "Acme" {
		t.Fatalf("name: %+v", id)
	}
	if _, err := b.workspace.SetIdentity(ctx, &v1.SetIdentityRequest{DisplayName: strings.Repeat("n", 81)}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("long name: %v", err)
	}
	if id := get(); id.GetDisplayName() != "Acme" {
		t.Fatalf("failed save changed the name: %+v", id)
	}

	png := testLogoPNG(t, 7)
	id := set(&v1.SetIdentityRequest{LogoLight: &v1.LogoUpload{Content: png, Filename: "logo.png"}})
	if !strings.HasPrefix(id.GetLogoLightUrl(), "/brand/logo-") || !strings.HasSuffix(id.GetLogoLightUrl(), ".png") {
		t.Fatalf("light URL: %q", id.GetLogoLightUrl())
	}
	if !strings.HasPrefix(id.GetFavicon_32Url(), "/brand/favicon-") || strings.Contains(id.GetFavicon_32Url(), "mark") {
		t.Fatalf("favicon should follow the PNG logo: %q", id.GetFavicon_32Url())
	}
	if _, err := os.Stat(filepath.Join(root, "brand", "logo-light.png")); err != nil {
		t.Fatalf("logo file: %v", err)
	}
	var versions int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM athena_file_versions WHERE path = ?", "brand/logo-light.png").Scan(&versions); err != nil || versions != 1 {
		t.Fatalf("versions = %d, err %v", versions, err)
	}
	var audits int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM athena_audit_log WHERE record_type_name = ? AND action = ?", "athena:file", "file_created").Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audits = %d, err %v", audits, err)
	}

	id = set(&v1.SetIdentityRequest{LogoLight: &v1.LogoUpload{Content: []byte(testLogoSVG), Filename: "logo.svg"}})
	if !strings.HasSuffix(id.GetLogoLightUrl(), ".svg") {
		t.Fatalf("replaced URL: %q", id.GetLogoLightUrl())
	}
	if _, err := os.Stat(filepath.Join(root, "brand", "logo-light.png")); !os.IsNotExist(err) {
		t.Fatalf("old PNG still on disk: %v", err)
	}
	stored, err := os.ReadFile(filepath.Join(root, "brand", "logo-light.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "script") {
		t.Fatalf("stored SVG not sanitized: %s", stored)
	}
	if !strings.Contains(id.GetFavicon_32Url(), "mark") {
		t.Fatalf("SVG logo should keep the mark favicon: %q", id.GetFavicon_32Url())
	}

	id = set(&v1.SetIdentityRequest{ClearLogoLight: true})
	if id.GetLogoLightUrl() != "" {
		t.Fatalf("cleared URL: %q", id.GetLogoLightUrl())
	}
	if _, err := os.Stat(filepath.Join(root, "brand", "logo-light.svg")); !os.IsNotExist(err) {
		t.Fatalf("cleared file still on disk: %v", err)
	}
	verRes, err := b.files.ListFileVersions(ctx, &v1.ListFileVersionsRequest{Path: "brand/logo-light.svg"})
	if err != nil || len(verRes.GetVersions()) == 0 {
		t.Fatalf("versions after delete: %v %+v", err, verRes)
	}

	if _, err := b.workspace.SetIdentity(ctx, &v1.SetIdentityRequest{
		LogoDark:      &v1.LogoUpload{Content: png, Filename: "d.png"},
		ClearLogoDark: true,
	}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("replace and clear: %v", err)
	}
	if _, err := b.workspace.SetIdentity(ctx, &v1.SetIdentityRequest{
		LogoDark: &v1.LogoUpload{Content: []byte("not a png"), Filename: "d.png"},
	}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("bad logo: %v", err)
	}
	if id := get(); id.GetLogoDarkUrl() != "" {
		t.Fatalf("failed save stored a logo: %+v", id)
	}
}

func TestFileServiceRefusesBrandWrites(t *testing.T) {
	root := t.TempDir()
	f := newAccessFixtureAt(t, root)
	ctx := context.Background()
	b := f.browser(t)
	f.role(t, "Editor", []string{auth.PermissionFilesRead, auth.PermissionFilesWrite, auth.PermissionWorkspace})
	f.user(t, "editor", "Editor")
	b.login(t, "editor")

	png := testLogoPNG(t, 3)
	set, err := b.workspace.SetIdentity(ctx, &v1.SetIdentityRequest{LogoLight: &v1.LogoUpload{Content: png, Filename: "logo.png"}})
	if err != nil {
		t.Fatal(err)
	}
	if set.GetIdentity().GetLogoLightUrl() == "" {
		t.Fatal("logo not stored")
	}

	if _, err := b.files.PutFile(ctx, &v1.PutFileRequest{Path: "brand/logo-light.png", Content: png}); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("put brand: %v", err)
	}
	if _, err := b.files.MoveFile(ctx, &v1.MoveFileRequest{From: "brand/logo-light.png", To: "rules/x.star"}); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("move from brand: %v", err)
	}
	if _, err := b.files.DeleteFile(ctx, &v1.DeleteFileRequest{Path: "brand/logo-light.png"}); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("delete brand: %v", err)
	}
	list, err := b.files.ListFiles(ctx, &v1.ListFilesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range list.GetFiles() {
		if strings.HasPrefix(info.GetPath(), "brand/") {
			t.Fatalf("brand listed: %q", info.GetPath())
		}
	}
	got, err := b.files.GetFile(ctx, &v1.GetFileRequest{Path: "brand/logo-light.png"})
	if err != nil || !bytes.Equal(got.GetContent(), png) {
		t.Fatalf("brand read: %v", err)
	}
}

func TestSetIdentityPruneFailureKeepsTheSave(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := &ledger.Service{DB: db, Dialect: dialect}
	ref := ledger.LogoRef{Ext: "png", Hash: strings.Repeat("a", 64)}
	if err := svc.SaveIdentityDoc(ctx, ledger.IdentityDoc{}, ledger.IdentityDoc{DisplayName: "Acme", Light: &ref}, "test"); err != nil {
		t.Fatal(err)
	}
	// A files store on a closed database: every prune read fails, while the
	// identity database stays healthy.
	dead := testdb.New(t)
	st, err := files.Open(t.TempDir(), dead, dialect)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_ = dead.Close()
	s := workspaceService{db: db, name: "Acme", files: st}
	res, err := s.SetIdentity(ctx, &v1.SetIdentityRequest{ClearLogoLight: true})
	if err != nil {
		t.Fatalf("prune failure failed the save: %v", err)
	}
	if res.GetIdentity().GetLogoLightUrl() != "" {
		t.Fatalf("logo still set: %+v", res.GetIdentity())
	}
	got, err := svc.GetIdentityDoc(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Light != nil || got.DisplayName != "Acme" {
		t.Fatalf("save did not commit: %+v", got)
	}
}
