package server

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/files"

	"connectrpc.com/connect/v2"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const widgetFile = `api: athena/v1
record_type: widget
label: Widget
kind: master
fields:
  - name: title
    type: Text
`

func TestFileServiceDeniesWithoutTheSystemPermissionAndLogsIt(t *testing.T) {
	dir := t.TempDir()
	f := newAccessFixtureAt(t, dir)
	f.role(t, "reader", []string{auth.PermissionFilesRead})
	f.role(t, "writer", []string{auth.PermissionFilesWrite})
	f.role(t, "builder", []string{auth.PermissionBuild})
	f.user(t, "nobody")
	f.user(t, "rita", "reader")
	f.user(t, "walt", "writer")
	f.user(t, "bob", "builder")
	ctx := context.Background()
	put := &athenav1.PutFileRequest{Path: "rules/a.star", Content: []byte("x\n")}

	for _, name := range []string{"nobody", "rita", "bob"} {
		b := f.browser(t)
		b.login(t, name)
		_, err := b.files.PutFile(ctx, put)
		requireCode(t, err, connect.CodePermissionDenied)
		_, err = b.files.MoveFile(ctx, &athenav1.MoveFileRequest{From: "rules/a.star", To: "rules/b.star"})
		requireCode(t, err, connect.CodePermissionDenied)
		_, err = b.files.DeleteFile(ctx, &athenav1.DeleteFileRequest{Path: "rules/a.star"})
		requireCode(t, err, connect.CodePermissionDenied)
	}
	for _, name := range []string{"nobody", "walt", "bob"} {
		b := f.browser(t)
		b.login(t, name)
		_, err := b.files.ListFiles(ctx, &athenav1.ListFilesRequest{})
		requireCode(t, err, connect.CodePermissionDenied)
		_, err = b.files.GetFile(ctx, &athenav1.GetFileRequest{Path: "rules/a.star"})
		requireCode(t, err, connect.CodePermissionDenied)
		_, err = b.files.ListFileVersions(ctx, &athenav1.ListFileVersionsRequest{Path: "rules/a.star"})
		requireCode(t, err, connect.CodePermissionDenied)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a denied call wrote %v", entries)
	}
	// 3 users x 3 writes, and 3 users x 3 reads.
	if denials := f.accessEvents(t, auth.EventDenied); len(denials) != 18 {
		t.Fatalf("denials = %d, want 18", len(denials))
	}
	denial := f.accessEvents(t, auth.EventDenied)[0]
	if denial.Procedure == "" || denial.RequestID == "" || (denial.Action != auth.PermissionFilesRead && denial.Action != auth.PermissionFilesWrite) {
		t.Fatalf("denial row = %+v", denial)
	}

	w := f.browser(t)
	w.login(t, "walt")
	if _, err := w.files.PutFile(ctx, put); err != nil {
		t.Fatalf("a user with files:write could not write: %v", err)
	}
	r := f.browser(t)
	r.login(t, "rita")
	if res, err := r.files.GetFile(ctx, &athenav1.GetFileRequest{Path: "rules/a.star"}); err != nil || string(res.GetContent()) != "x\n" {
		t.Fatalf("a user with files:read could not read: %v", err)
	}
	// Signed-out calls are refused too.
	_, err := f.browser(t).files.ListFiles(ctx, &athenav1.ListFilesRequest{})
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestFileServiceOverTheAPI(t *testing.T) {
	dir := t.TempDir()
	f := newAccessFixtureAt(t, dir)
	f.role(t, "editor", []string{auth.PermissionFilesRead, auth.PermissionFilesWrite})
	f.user(t, "ed", "editor")
	mustSave(t, f.types, &athenav1.RecordType{Key: "widget", Label: "Widget", Fields: []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text"}}})
	mustSave(t, f.types, &athenav1.RecordType{Key: "lonely", Label: "Lonely", Fields: []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text"}}})
	b := f.browser(t)
	b.login(t, "ed")
	ctx := context.Background()

	created, err := b.files.PutFile(ctx, &athenav1.PutFileRequest{Path: "record_types/widget.yaml", Content: []byte(widgetFile)})
	if err != nil {
		t.Fatal(err)
	}
	info := created.GetInfo()
	if info.GetRevision() != 1 || info.GetModifiedBy() != "ed" || !info.GetRecorded() || info.GetHash() != files.Hash([]byte(widgetFile)) {
		t.Fatalf("info = %+v", info)
	}
	_, err = b.files.PutFile(ctx, &athenav1.PutFileRequest{Path: "record_types/widget.yaml", Content: []byte(widgetFile + "# edit\n"), ExpectedHash: "stale"})
	requireCode(t, err, connect.CodeAborted)
	_, err = b.files.PutFile(ctx, &athenav1.PutFileRequest{Path: "record_types/widget.yaml", Content: []byte(widgetFile)})
	requireCode(t, err, connect.CodeAlreadyExists)
	_, err = b.files.PutFile(ctx, &athenav1.PutFileRequest{Path: "../escape.yaml", Content: []byte("x: 1\n")})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = b.files.PutFile(ctx, &athenav1.PutFileRequest{Path: "record_types/bad.yaml", Content: []byte("record_type: bad\n")})
	requireCode(t, err, connect.CodeInvalidArgument)
	if !strings.Contains(connectMessage(err), "line 1") && !strings.Contains(connectMessage(err), "missing required key") {
		t.Fatalf("validation message = %q", connectMessage(err))
	}
	_, err = b.files.GetFile(ctx, &athenav1.GetFileRequest{Path: "rules/none.star"})
	requireCode(t, err, connect.CodeNotFound)
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.yaml")); err == nil {
		t.Fatal("a path outside the Workspace was written")
	}

	if _, err := b.files.PutFile(ctx, &athenav1.PutFileRequest{Path: "rules/post.star", Content: []byte("# rule\n")}); err != nil {
		t.Fatal(err)
	}
	list, err := b.files.ListFiles(ctx, &athenav1.ListFilesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	backing := map[string]string{}
	for _, file := range list.GetFiles() {
		backing[file.GetPath()] = file.GetBacking().GetKind() + ":" + file.GetBacking().GetKey()
		if file.GetPath() == "record_types/widget.yaml" && !file.GetBacking().GetSaved() {
			t.Error("widget.yaml should map to the saved widget Record Type")
		}
		if file.GetPath() == "rules/post.star" && file.GetBacking().GetSaved() {
			t.Error("a Rule is never saved yet")
		}
	}
	if backing["record_types/widget.yaml"] != "record_type:widget" || backing["rules/post.star"] != "rule:post" {
		t.Fatalf("backing = %v", backing)
	}
	if got := strings.Join(list.GetRecordTypesWithoutFile(), ","); got != "lonely" {
		t.Fatalf("record types without a file = %q", got)
	}
	if list.GetMaxFileBytes() != files.MaxFileBytes || len(list.GetFolders()) != 5 {
		t.Fatalf("limits = %d, %v", list.GetMaxFileBytes(), list.GetFolders())
	}

	moved, err := b.files.MoveFile(ctx, &athenav1.MoveFileRequest{From: "rules/post.star", To: "rules/posted.star", ExpectedHash: files.Hash([]byte("# rule\n"))})
	if err != nil || moved.GetInfo().GetPath() != "rules/posted.star" {
		t.Fatalf("move = %v, %v", moved, err)
	}
	if _, err := b.files.DeleteFile(ctx, &athenav1.DeleteFileRequest{Path: "rules/posted.star", ExpectedHash: moved.GetInfo().GetHash()}); err != nil {
		t.Fatal(err)
	}
	versions, err := b.files.ListFileVersions(ctx, &athenav1.ListFileVersionsRequest{Path: "rules/posted.star"})
	if err != nil || len(versions.GetVersions()) != 3 {
		t.Fatalf("versions = %v, %v", versions, err)
	}
	old, err := b.files.GetFile(ctx, &athenav1.GetFileRequest{Path: "rules/posted.star", Revision: 1})
	if err != nil || string(old.GetContent()) != "# rule\n" {
		t.Fatalf("revision 1 = %v, %v", old, err)
	}

	var actor, source string
	if err := f.db.QueryRow("SELECT actor, source FROM athena_audit_log WHERE record_type_name = ? ORDER BY seq LIMIT 1", files.AuditType).Scan(&actor, &source); err != nil {
		t.Fatal(err)
	}
	if actor != "ed" || source != "api" {
		t.Fatalf("audit actor %q source %q", actor, source)
	}
}

func TestFileServiceNeedsAWorkspaceDirectory(t *testing.T) {
	f := newAccessFixture(t)
	f.role(t, "editor", []string{auth.PermissionFilesRead})
	f.user(t, "ed", "editor")
	b := f.browser(t)
	b.login(t, "ed")
	_, err := b.files.ListFiles(context.Background(), &athenav1.ListFilesRequest{})
	requireCode(t, err, connect.CodeFailedPrecondition)
}

func TestFileSpansAndLogsNeverCarryContentOrPaths(t *testing.T) {
	dir := t.TempDir()
	f := newAccessFixtureAt(t, dir)
	f.role(t, "editor", []string{auth.PermissionFilesRead, auth.PermissionFilesWrite})
	f.user(t, "ed", "editor")
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	var logged bytes.Buffer
	original := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(original) })

	b := f.browser(t)
	b.login(t, "ed")
	ctx := context.Background()
	const marker = "FILE-CONTENT-MARKER-7731"
	const path = "rules/very_private_name.star"
	res, err := b.files.PutFile(ctx, &athenav1.PutFileRequest{Path: path, Content: []byte("# " + marker + "\n")})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = b.files.GetFile(ctx, &athenav1.GetFileRequest{Path: path})
	_, _ = b.files.PutFile(ctx, &athenav1.PutFileRequest{Path: path, Content: []byte("# " + marker + " 2\n"), ExpectedHash: "stale"})
	_, _ = b.files.ListFiles(ctx, &athenav1.ListFilesRequest{})
	_, _ = b.files.DeleteFile(ctx, &athenav1.DeleteFileRequest{Path: path, ExpectedHash: res.GetInfo().GetHash()})

	operations := map[string]bool{}
	for _, span := range exporter.GetSpans() {
		var all []string
		for _, attribute := range span.Attributes {
			all = append(all, string(attribute.Key)+"="+attribute.Value.Emit())
			if attribute.Key == "athena.operation" {
				operations[attribute.Value.Emit()] = true
			}
		}
		all = append(all, span.Status.Description, span.Name)
		for _, text := range all {
			if strings.Contains(text, marker) || strings.Contains(text, "very_private_name") {
				t.Fatalf("span %s carries a file name or content: %s", span.Name, text)
			}
		}
	}
	for _, want := range []string{"file_put", "file_get", "file_list", "file_delete"} {
		if !operations[want] {
			t.Errorf("no span for %s; have %v", want, operations)
		}
	}
	if strings.Contains(logged.String(), marker) {
		t.Fatal("the process log carries file content")
	}
}
