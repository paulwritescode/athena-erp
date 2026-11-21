package server

import (
	"context"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"

	"connectrpc.com/connect/v2"
)

const imageTestHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func imageType(t *testing.T, recordTypes recordTypeService) {
	t.Helper()
	saveRecordType(t, recordTypes, "item", &athenav1.RecordTypeField{Key: "photo", Label: "Photo", Type: "Image"})
}

func imageValue(path, hash, alt string) *athenav1.FieldValue {
	return &athenav1.FieldValue{Kind: &athenav1.FieldValue_ImageValue{ImageValue: &athenav1.ImageValue{Path: path, Hash: hash, AltText: alt}}}
}

func TestImageFieldRoundTrip(t *testing.T) {
	recordTypes, records := newServices(t)
	ctx := context.Background()
	imageType(t, recordTypes)
	created, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "item",
		Values: map[string]*athenav1.FieldValue{"photo": imageValue("images/logo.png", imageTestHash, "The company logo")}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "item", Id: created.GetRecord().GetId()})
	if err != nil {
		t.Fatal(err)
	}
	value := got.GetRecord().GetValues()["photo"].GetImageValue()
	if value.GetPath() != "images/logo.png" || value.GetHash() != imageTestHash || value.GetAltText() != "The company logo" {
		t.Fatalf("image value = %+v", value)
	}
}

func TestImageFieldValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value *athenav1.FieldValue
		want  string
	}{
		{"missing alt", imageValue("images/logo.png", imageTestHash, "  "), "needs alt text"},
		{"bad hash", imageValue("images/logo.png", "xyz", "Logo"), "64-character hash"},
		{"outside images", imageValue("rules/logo.png", imageTestHash, "Logo"), "under images/"},
		{"text file", imageValue("images/note.txt", imageTestHash, "Logo"), "under images/"},
		{"wrong kind", &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: "images/logo.png"}}, "must be an image reference"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recordTypes, records := newServices(t)
			ctx := context.Background()
			imageType(t, recordTypes)
			_, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "item",
				Values: map[string]*athenav1.FieldValue{"photo": tc.value}})
			requireCode(t, err, connect.CodeInvalidArgument)
			if !strings.Contains(connectMessage(err), tc.want) {
				t.Fatalf("error = %q, want %q", connectMessage(err), tc.want)
			}
		})
	}
}

func TestImageFieldMissingFileReads(t *testing.T) {
	recordTypes, records := newServices(t)
	ctx := context.Background()
	imageType(t, recordTypes)
	// The reference is stored without checking the file exists; reads serve
	// what was stored, and GetFile reports a deleted file.
	created, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "item",
		Values: map[string]*athenav1.FieldValue{"photo": imageValue("images/gone.png", imageTestHash, "Gone")}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "item", Id: created.GetRecord().GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetRecord().GetValues()["photo"].GetImageValue().GetPath() != "images/gone.png" {
		t.Fatalf("image value = %+v", got.GetRecord().GetValues()["photo"])
	}
}

func TestDeleteRecordKeepsFileVersions(t *testing.T) {
	recordTypes, records := newServices(t)
	ctx := context.Background()
	imageType(t, recordTypes)
	if _, err := recordTypes.db.ExecContext(ctx, `INSERT INTO athena_files (id, path, revision, hash, size, modified_by, created_at, modified_at)
		VALUES ('0198a2c4-7b1e-7000-8000-000000000002', 'images/logo.png', 1, ?, 10, 'ada', UTC_TIMESTAMP(3), UTC_TIMESTAMP(3))`, imageTestHash); err != nil {
		t.Fatal(err)
	}
	if _, err := recordTypes.db.ExecContext(ctx, `INSERT INTO athena_file_versions (file_id, revision, path, action, hash, size, content, actor, source, request_id, created_at)
		VALUES ('0198a2c4-7b1e-7000-8000-000000000002', 1, 'images/logo.png', 'created', ?, 10, 'x', 'ada', 'test', NULL, UTC_TIMESTAMP(3))`, imageTestHash); err != nil {
		t.Fatal(err)
	}
	created, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "item",
		Values: map[string]*athenav1.FieldValue{"photo": imageValue("images/logo.png", imageTestHash, "Logo")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "item", Id: created.GetRecord().GetId(), ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	var files, versions int
	if err := recordTypes.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_files WHERE path = 'images/logo.png'`).Scan(&files); err != nil {
		t.Fatal(err)
	}
	if err := recordTypes.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_file_versions WHERE path = 'images/logo.png'`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if files != 1 || versions != 1 {
		t.Fatalf("files = %d, versions = %d, want both kept", files, versions)
	}
}

func TestValueTextRendersImagePath(t *testing.T) {
	path := valueText(imageValue("images/logo.png", imageTestHash, "Logo"))
	if path == nil || *path != "images/logo.png" {
		t.Fatalf("valueText = %+v", path)
	}
	if valueText(&athenav1.FieldValue{}) != nil {
		t.Fatal("an empty value must render nil")
	}
}

func TestFormulaOnImageRejected(t *testing.T) {
	recordTypes, _ := newServices(t)
	ctx := context.Background()
	_, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{
		Key: "item", Label: "Item", Fields: []*athenav1.RecordTypeField{
			{Key: "name", Label: "Name", Type: "Text"},
			{Key: "photo", Label: "Photo", Type: "Image", Formula: `doc.name`},
		},
	}})
	requireCode(t, err, connect.CodeInvalidArgument)
	if !strings.Contains(connectMessage(err), "cannot be calculated") {
		t.Fatalf("error = %q", connectMessage(err))
	}
}
