package files_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"athena-erp/internal/audit"
	"athena-erp/internal/files"
	"athena-erp/internal/server"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
)

const customerFile = `api: athena/v1
record_type: customer
label: Customer
kind: master
fields:
  - name: title
    type: Text
`

type fixture struct {
	db   *sql.DB
	dir  string
	s    *files.Store
	meta files.Meta
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, store.MariaDB{}, server.SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	dir := t.TempDir()
	s, err := files.Open(dir, db, store.MariaDB{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return &fixture{db: db, dir: dir, s: s, meta: files.Meta{Actor: "ada", Source: "api", RequestID: "0198a2c4-7b1e-7000-8000-000000000001"}}
}

func (f *fixture) put(t *testing.T, path, content, expected string) (files.Info, error) {
	t.Helper()
	return f.s.Put(context.Background(), path, []byte(content), files.Expect{Hash: expected}, f.meta)
}

func (f *fixture) disk(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCleanPathTable(t *testing.T) {
	var table struct {
		Valid   []string          `json:"valid"`
		Invalid map[string]string `json:"invalid"`
	}
	data, err := os.ReadFile("testdata/paths.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &table); err != nil {
		t.Fatal(err)
	}
	for _, p := range table.Valid {
		if got, err := files.CleanPath(p); err != nil || got != p {
			t.Errorf("CleanPath(%q) = %q, %v; want it accepted", p, got, err)
		}
	}
	for p, why := range table.Invalid {
		if _, err := files.CleanPath(p); !errors.Is(err, files.ErrInvalidPath) {
			t.Errorf("CleanPath(%q) (%s) = %v; want ErrInvalidPath", p, why, err)
		}
	}
}

func TestEveryOperationRejectsAnEscapingPathAndWritesNothing(t *testing.T) {
	f := newFixture(t)
	outside := t.TempDir()
	ctx := context.Background()
	bad := []string{"/etc/passwd", "../x.yaml", "record_types/../../x.yaml", "record_types/%2e%2e/x.yaml", "Record_Types/x.yaml", "record_types\\..\\x.yaml", ".env", ".git/config", filepath.Join(outside, "x.yaml")}
	if _, err := f.put(t, "record_types/ok.yaml", strings.ReplaceAll(customerFile, "customer", "ok"), ""); err != nil {
		t.Fatal(err)
	}
	for _, p := range bad {
		if _, err := f.put(t, p, "x: 1\n", ""); !errors.Is(err, files.ErrInvalidPath) {
			t.Errorf("Put(%q) = %v", p, err)
		}
		if _, _, err := f.s.Get(ctx, p, 0); !errors.Is(err, files.ErrInvalidPath) {
			t.Errorf("Get(%q) = %v", p, err)
		}
		if err := f.s.Delete(ctx, p, files.Expect{}, f.meta); !errors.Is(err, files.ErrInvalidPath) {
			t.Errorf("Delete(%q) = %v", p, err)
		}
		if _, err := f.s.Move(ctx, p, "record_types/moved.yaml", files.Expect{}, f.meta); !errors.Is(err, files.ErrInvalidPath) {
			t.Errorf("Move from %q = %v", p, err)
		}
		if _, err := f.s.Move(ctx, "record_types/ok.yaml", p, files.Expect{}, f.meta); !errors.Is(err, files.ErrInvalidPath) {
			t.Errorf("Move to %q = %v", p, err)
		}
		if _, err := f.s.Versions(ctx, p); !errors.Is(err, files.ErrInvalidPath) {
			t.Errorf("Versions(%q) = %v", p, err)
		}
	}
	for _, dir := range []string{outside, filepath.Dir(f.dir)} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.Name() == "x.yaml" || e.Name() == "moved.yaml" {
				t.Fatalf("a file reached %s", dir)
			}
		}
	}
	if got := f.disk(t, "record_types/ok.yaml"); !strings.Contains(got, "record_type: ok") {
		t.Fatal("the existing file changed")
	}
}

func TestSymbolicLinksAreRefusedEverywhere(t *testing.T) {
	f := newFixture(t)
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.yaml")
	if err := os.WriteFile(secret, []byte("api: secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(f.dir, "record_types"), 0o755))
	must(os.Symlink(secret, filepath.Join(f.dir, "record_types", "leak.yaml")))
	must(os.Symlink(outside, filepath.Join(f.dir, "rules")))
	must(os.Symlink("../record_types", filepath.Join(f.dir, "generated")))
	ctx := context.Background()
	for _, p := range []string{"record_types/leak.yaml", "rules/secret.yaml", "generated/leak.yaml"} {
		if _, _, err := f.s.Get(ctx, p, 0); !errors.Is(err, files.ErrInvalidPath) && !errors.Is(err, files.ErrNotFound) {
			t.Errorf("Get(%q) = %v", p, err)
		}
		if _, err := f.put(t, p, "api: x\n", ""); err == nil {
			t.Errorf("Put(%q) succeeded", p)
		}
		if err := f.s.Delete(ctx, p, files.Expect{}, f.meta); err == nil {
			t.Errorf("Delete(%q) succeeded", p)
		}
	}
	if _, err := f.s.Move(ctx, "record_types/leak.yaml", "packs/a/leak.yaml", files.Expect{}, f.meta); err == nil {
		t.Fatal("moved a symbolic link")
	}
	if data, _ := os.ReadFile(secret); string(data) != "api: secret\n" {
		t.Fatal("the file behind a symbolic link changed")
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.yaml")); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 1 {
		t.Fatalf("a file was written outside the Workspace: %v", entries)
	}
	list, err := f.s.List(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("List = %v, %v; want nothing", list, err)
	}
}

func TestHiddenAndRuntimeFilesAreNeverListedOrReadable(t *testing.T) {
	f := newFixture(t)
	write := func(rel, content string) {
		path := filepath.Join(f.dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".env.local", "ATHENA_SERVER_URL=secret\n")
	write(".git/config", "[core]\n")
	write(".athena/state.yaml", "x: 1\n")
	write("athena.yaml", "id: x\n")
	write("record_types/.env", "KEY=secret\n")
	write("record_types/.athena-tmp-1", "partial")
	write("storage/uploads/a.txt", "upload")
	write("record_types/Upper.yaml", "x: 1\n")
	write("record_types/binary.yaml", "a\x00b")
	write("record_types/big.yaml", strings.Repeat("a", files.MaxFileBytes+1))
	write("rules/visible.star", "def f(): pass\n")
	list, err := f.s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Path != "rules/visible.star" {
		t.Fatalf("List = %+v; want only rules/visible.star", list)
	}
	for _, p := range []string{".env.local", ".git/config", ".athena/state.yaml", "athena.yaml", "record_types/.env", "storage/uploads/a.txt"} {
		if _, _, err := f.s.Get(context.Background(), p, 0); !errors.Is(err, files.ErrInvalidPath) {
			t.Errorf("Get(%q) = %v; want ErrInvalidPath", p, err)
		}
	}
	for _, p := range []string{"record_types/binary.yaml", "record_types/big.yaml"} {
		if _, _, err := f.s.Get(context.Background(), p, 0); err == nil {
			t.Errorf("Get(%q) read a refused file", p)
		}
	}
}

func TestSizeAndTypeLimits(t *testing.T) {
	f := newFixture(t)
	if _, err := f.put(t, "rules/big.star", strings.Repeat("a", files.MaxFileBytes+1), ""); !errors.Is(err, files.ErrTooLarge) {
		t.Errorf("oversized = %v", err)
	}
	if _, err := f.put(t, "rules/ok.star", strings.Repeat("a", files.MaxFileBytes), ""); err != nil {
		t.Errorf("a file at the limit = %v", err)
	}
	for name, content := range map[string]string{"nul": "a\x00b", "latin1": "caf\xe9"} {
		if _, err := f.put(t, "rules/"+name+".star", content, ""); !errors.Is(err, files.ErrNotText) {
			t.Errorf("%s = %v", name, err)
		}
	}
	if _, err := f.put(t, "rules/tool.exe", "MZ", ""); !errors.Is(err, files.ErrInvalidPath) {
		t.Errorf("a type outside the list = %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "rules", "big.star")); err == nil {
		t.Error("a refused file was written")
	}
}

func TestRecordTypeFilesAreValidatedWithLineNumbers(t *testing.T) {
	f := newFixture(t)
	bad := `api: athena/v1
record_type: Customer
label: Customer
kind: widget
colour: red
fields:
  - name: title
    type: Text
  - name: title
    type: Text
`
	_, err := f.put(t, "record_types/customer.yaml", bad, "")
	var verr *files.ValidationError
	if !errors.Is(err, files.ErrInvalidFile) || !errors.As(err, &verr) {
		t.Fatalf("Put = %v; want a ValidationError", err)
	}
	lines := map[int]bool{}
	for _, issue := range verr.Issues {
		lines[issue.Line] = true
	}
	for _, line := range []int{2, 4, 5, 9} {
		if !lines[line] {
			t.Errorf("no issue on line %d: %v", line, verr)
		}
	}
	if _, err := os.Stat(filepath.Join(f.dir, "record_types", "customer.yaml")); err == nil {
		t.Fatal("an invalid Record Type file was written")
	}
	if _, err := f.put(t, "record_types/other.yaml", customerFile, ""); !errors.Is(err, files.ErrInvalidFile) || !strings.Contains(err.Error(), "must match the file name") {
		t.Errorf("a name that differs from the file = %v", err)
	}
	if _, err := f.put(t, "rules/broken.rule.yaml", "a: [1, 2\n", ""); !errors.Is(err, files.ErrInvalidFile) {
		t.Errorf("broken YAML = %v", err)
	}
	if _, err := f.put(t, "record_types/customer.yaml", customerFile, ""); err != nil {
		t.Fatalf("a valid Record Type file = %v", err)
	}
}

func TestStaleHashFailsAndLeavesTheFileUnchanged(t *testing.T) {
	f := newFixture(t)
	first, err := f.put(t, "rules/a.star", "one\n", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.put(t, "rules/a.star", "two\n", first.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.put(t, "rules/a.star", "three\n", first.Hash); !errors.Is(err, files.ErrStale) {
		t.Fatalf("stale Put = %v", err)
	}
	if _, err := f.put(t, "rules/a.star", "four\n", ""); !errors.Is(err, files.ErrExists) {
		t.Fatalf("create over an existing file = %v", err)
	}
	if err := f.s.Delete(context.Background(), "rules/a.star", files.Expect{Hash: first.Hash}, f.meta); !errors.Is(err, files.ErrStale) {
		t.Fatalf("stale Delete = %v", err)
	}
	if _, err := f.s.Move(context.Background(), "rules/a.star", "rules/b.star", files.Expect{Hash: first.Hash}, f.meta); !errors.Is(err, files.ErrStale) {
		t.Fatalf("stale Move = %v", err)
	}
	if got := f.disk(t, "rules/a.star"); got != "two\n" {
		t.Fatalf("the file changed to %q", got)
	}
	if second.Revision != 2 {
		t.Fatalf("revision = %d", second.Revision)
	}
}

func TestAnInterruptedWriteLeavesTheOldContent(t *testing.T) {
	f := newFixture(t)
	first, err := f.put(t, "rules/a.star", "old\n", "")
	if err != nil {
		t.Fatal(err)
	}
	f.s.SetFault(func(stage string) error {
		if stage == "before_rename" {
			return errors.New("disk failure")
		}
		return nil
	})
	if _, err := f.put(t, "rules/a.star", "new\n", first.Hash); err == nil {
		t.Fatal("the write succeeded")
	}
	if got := f.disk(t, "rules/a.star"); got != "old\n" {
		t.Fatalf("content = %q; want the old content", got)
	}
	entries, _ := os.ReadDir(filepath.Join(f.dir, "rules"))
	if len(entries) != 1 {
		t.Fatalf("a temporary file stayed behind: %v", entries)
	}
	f.s.SetFault(nil)
	versions, err := f.s.Versions(context.Background(), "rules/a.star")
	if err != nil || len(versions) != 1 {
		t.Fatalf("the failed write left a version: %v, %v", versions, err)
	}
	// A failed create leaves no file and no history.
	f.s.SetFault(func(string) error { return errors.New("disk failure") })
	if _, err := f.put(t, "rules/b.star", "x\n", ""); err == nil {
		t.Fatal("the create succeeded")
	}
	if _, err := os.Stat(filepath.Join(f.dir, "rules", "b.star")); err == nil {
		t.Fatal("a failed create left a file")
	}
}

func TestVersionHistoryIsPreservedAcrossEditsMovesAndDeletes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, _ := f.put(t, "rules/a.star", "one\n", "")
	b, err := f.put(t, "rules/a.star", "two\n", a.Hash)
	if err != nil {
		t.Fatal(err)
	}
	// An edit made outside Athena is kept before the next change replaces it.
	if err := os.WriteFile(filepath.Join(f.dir, "rules", "a.star"), []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, _ := f.s.List(ctx)
	if len(list) != 1 || list[0].Recorded {
		t.Fatalf("an external edit should show as not recorded: %+v", list)
	}
	outsideHash := files.Hash([]byte("outside\n"))
	if _, err := f.put(t, "rules/a.star", "three\n", b.Hash); !errors.Is(err, files.ErrStale) {
		t.Fatalf("a hash from before the outside edit = %v", err)
	}
	if _, err := f.put(t, "rules/a.star", "three\n", outsideHash); err != nil {
		t.Fatal(err)
	}
	moved, err := f.s.Move(ctx, "rules/a.star", "rules/renamed.star", files.Expect{Hash: files.Hash([]byte("three\n"))}, f.meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.Delete(ctx, "rules/renamed.star", files.Expect{Hash: moved.Hash}, f.meta); err != nil {
		t.Fatal(err)
	}
	versions, err := f.s.Versions(ctx, "rules/renamed.star")
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, v := range versions {
		actions = append(actions, v.Action)
	}
	if got := strings.Join(actions, ","); got != "deleted,moved,updated,external,updated,created" {
		t.Fatalf("history = %s", got)
	}
	// The history follows the file across the rename, from the old path too.
	if again, err := f.s.Versions(ctx, "rules/a.star"); err != nil || len(again) != 2 && len(again) != 6 {
		t.Fatalf("history by old path = %d versions, %v", len(again), err)
	}
	for revision, want := range map[int64]string{1: "one\n", 2: "two\n", 3: "outside\n", 4: "three\n"} {
		info, content, err := f.s.Get(ctx, "rules/renamed.star", revision)
		if err != nil || string(content) != want || info.Revision != revision {
			t.Errorf("revision %d = %q, %v; want %q", revision, content, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(f.dir, "rules", "renamed.star")); err == nil {
		t.Fatal("the deleted file is still on disk")
	}
	// The path can be created again; it starts a new history.
	if _, err := f.put(t, "rules/renamed.star", "again\n", ""); err != nil {
		t.Fatal(err)
	}
}

func TestEveryChangeWritesOneAuditEntryWithoutContent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	const marker = "TOPSECRETCONTENT"
	a, _ := f.put(t, "rules/a.star", "# "+marker+" one\n", "")
	b, _ := f.put(t, "rules/a.star", "# "+marker+" two\n", a.Hash)
	m, _ := f.s.Move(ctx, "rules/a.star", "rules/b.star", files.Expect{Hash: b.Hash}, f.meta)
	if err := f.s.Delete(ctx, "rules/b.star", files.Expect{Hash: m.Hash}, f.meta); err != nil {
		t.Fatal(err)
	}
	rows, err := f.db.Query("SELECT action, actor, source, request_id, changes FROM athena_audit_log WHERE record_type_name = ? ORDER BY seq", files.AuditType)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var actions []string
	for rows.Next() {
		var action, actor, source, requestID, changes string
		if err := rows.Scan(&action, &actor, &source, &requestID, &changes); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, action)
		if actor != "ada" || source != "api" || requestID != f.meta.RequestID {
			t.Errorf("%s: actor %q source %q request %q", action, actor, source, requestID)
		}
		if strings.Contains(changes, marker) {
			t.Errorf("%s: the audit entry holds file content: %s", action, changes)
		}
		if !strings.Contains(changes, `"path"`) || !strings.Contains(changes, `"hash"`) {
			t.Errorf("%s: no path or hash in %s", action, changes)
		}
	}
	if got := strings.Join(actions, ","); got != "file_created,file_updated,file_moved,file_deleted" {
		t.Fatalf("audit actions = %s", got)
	}
	// A refused or unchanged write adds nothing, and the chain still verifies.
	_, _ = f.put(t, "rules/c.star", "a\x00", "")
	result, err := audit.Verify(ctx, f.db, nil)
	if err != nil || !result.OK {
		t.Fatalf("chain = %+v, %v", result, err)
	}
	if result.Rows != 4 {
		t.Fatalf("audit rows = %d, want 4", result.Rows)
	}
}

func TestBackingMapsFilesToWhatTheyHold(t *testing.T) {
	for path, want := range map[string]string{
		"record_types/customer.yaml":                             "record_type:customer",
		"rules/post_invoice.star":                                "rule:post_invoice",
		"rules/post_invoice.rule.yaml":                           "rule:post_invoice",
		"packs/farming/modules/crops/reports/profit.report.yaml": "report:profit",
		"packs/farming/modules/crops/record_types/crop.yaml":     "record_type:crop",
		"generated/customer_gen.go":                              "generated_source:customer",
		"packs/farming/pack.yaml":                                ":",
		"generated/hooks.go":                                     ":",
	} {
		kind, key := files.Backing(path)
		if got := kind + ":" + key; got != want {
			t.Errorf("Backing(%q) = %s, want %s", path, got, want)
		}
	}
}

// testImagePNG encodes a w-by-h image for upload tests.
func testImagePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 0, 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestPutImageRoundTrip(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	content := testImagePNG(t, 12, 8)
	result, err := f.s.PutImage(ctx, "images/logo.png", content, files.Expect{}, f.meta)
	if err != nil {
		t.Fatal(err)
	}
	if result.Width != 12 || result.Height != 8 || result.Info.Path != "images/logo.png" || result.Info.Revision != 1 {
		t.Fatalf("stored = %+v", result)
	}
	got, read, err := f.s.Get(ctx, "images/logo.png", 0)
	if err != nil || !bytes.Equal(read, content) {
		t.Fatalf("read back = %d bytes, %v", len(read), err)
	}
	if got.Hash != result.Info.Hash {
		t.Fatal("the stored hash does not match the read hash")
	}
	versions, err := f.s.Versions(ctx, "images/logo.png")
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions = %d, %v", len(versions), err)
	}
	// A stale guard fails without touching the file.
	if _, err := f.s.PutImage(ctx, "images/logo.png", content, files.Expect{Hash: strings.Repeat("0", 64)}, f.meta); !errors.Is(err, files.ErrStale) {
		t.Fatalf("stale write = %v", err)
	}
	// Images outside images/, and text inside it, are refused.
	if _, err := f.s.PutImage(ctx, "rules/logo.png", content, files.Expect{}, f.meta); err == nil {
		t.Fatal("an image outside images/ was accepted")
	}
	if _, err := f.s.Put(ctx, "images/note.txt", []byte("hello\n"), files.Expect{}, f.meta); err == nil {
		t.Fatal("text inside images/ was accepted")
	}
}
