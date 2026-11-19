package server

import (
	"context"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
	"connectrpc.com/connect/v2"
)

func newServices(t *testing.T) (recordTypeService, recordService) {
	t.Helper()
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return recordTypeService{db: db}, recordService{db: db}
}

func text(v string) *athenav1.FieldValue {
	return &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: v}}
}

func requireCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %v error, got nil", want)
	}
	if got := connect.CodeOf(err); got != want {
		t.Fatalf("code = %v, want %v (%v)", got, want, err)
	}
}

// saveRecordType saves a new Record Type with the given fields and returns its revision.
func saveRecordType(t *testing.T, s recordTypeService, key string, fields ...*athenav1.RecordTypeField) int64 {
	t.Helper()
	res, err := s.SaveRecordType(context.Background(), &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: key, Label: key, Fields: fields}})
	if err != nil {
		t.Fatalf("save Record Type %s: %v", key, err)
	}
	return res.GetRecordType().GetRevision()
}
