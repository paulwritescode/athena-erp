package server

import (
	"context"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
	"connectrpc.com/connect/v2"
)

func newServices(t *testing.T) (recordService, entryService) {
	t.Helper()
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return recordService{db: db}, entryService{db: db}
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

// saveRecord saves a new Record with the given fields and returns its revision.
func saveRecord(t *testing.T, s recordService, key string, fields ...*athenav1.RecordField) int64 {
	t.Helper()
	res, err := s.SaveRecord(context.Background(), &athenav1.SaveRecordRequest{Record: &athenav1.Record{Key: key, Label: key, Fields: fields}})
	if err != nil {
		t.Fatalf("save Record %s: %v", key, err)
	}
	return res.GetRecord().GetRevision()
}
