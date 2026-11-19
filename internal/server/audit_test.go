package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/audit"
	"athena-erp/internal/lifecycle"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
)

func verifyChain(t *testing.T, s recordService) audit.Result {
	t.Helper()
	res, err := audit.Verify(context.Background(), s.db, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestAuditChainVerifiesAndDetectsEditsAndDeletions(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	if res := verifyChain(t, records); !res.OK || res.Rows != 0 {
		t.Fatalf("an empty chain must verify: %+v", res)
	}
	a := newCustomer(t, records, "Acme")
	b := newCustomer(t, records, "Bolt")
	if _, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "customer", Id: a.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"name": text("Acme Ltd")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "customer", Id: b.GetId(), ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if res := verifyChain(t, records); !res.OK || res.Rows != 4 || res.HeadSeq != 4 {
		t.Fatalf("verify = %+v", res)
	}
	anchor := &audit.Anchor{Seq: 2}
	if err := records.db.QueryRowContext(ctx, "SELECT hash FROM athena_audit_log WHERE seq = 2").Scan(&anchor.Hash); err != nil {
		t.Fatal(err)
	}
	if got, err := audit.Verify(ctx, records.db, anchor); err != nil || !got.OK {
		t.Fatalf("anchor verify = %+v, %v", got, err)
	}

	exec := func(statements ...string) {
		t.Helper()
		for _, statement := range statements {
			if _, err := records.db.ExecContext(ctx, statement); err != nil {
				t.Fatalf("%s: %v", statement, err)
			}
		}
	}
	exec("CREATE TABLE audit_copy AS SELECT * FROM athena_audit_log", "CREATE TABLE heads_copy AS SELECT * FROM athena_audit_heads")
	restore := func() {
		exec("DELETE FROM athena_audit_log", "INSERT INTO athena_audit_log SELECT * FROM audit_copy", "DELETE FROM athena_audit_heads", "INSERT INTO athena_audit_heads SELECT * FROM heads_copy")
		if res := verifyChain(t, records); !res.OK {
			t.Fatalf("restored chain = %+v", res)
		}
	}
	for _, c := range []struct {
		name, statement string
		seq             int64
	}{
		{"edited value", `UPDATE athena_audit_log SET changes = '[{"field":"name","new":"Forged"}]' WHERE seq = 3`, 3},
		{"edited actor", "UPDATE athena_audit_log SET actor = 'Someone else' WHERE seq = 2", 2},
		{"deleted middle row", "DELETE FROM athena_audit_log WHERE seq = 2", 3},
		{"rewritten prev_hash", "UPDATE athena_audit_log SET prev_hash = REPEAT('0', 64) WHERE seq = 3", 3},
		{"edited revision", "UPDATE athena_audit_log SET revision = revision + 1 WHERE seq = 1", 1},
	} {
		exec(c.statement)
		if got := verifyChain(t, records); got.OK || got.BrokenSeq != c.seq {
			t.Fatalf("%s: verify = %+v, want a break at row %d", c.name, got, c.seq)
		}
		restore()
	}
	exec("DELETE FROM athena_audit_log WHERE seq = 4")
	if got := verifyChain(t, records); got.OK || !strings.Contains(got.Reason, "removed from the end") {
		t.Fatalf("a deleted tail must be detected through the head: %+v", got)
	}
	restore()
	exec("UPDATE athena_audit_log SET hash = REPEAT('a', 64) WHERE seq = 2")
	if got, err := audit.Verify(ctx, records.db, anchor); err != nil || got.OK {
		t.Fatalf("a changed row must not satisfy its anchor: %+v, %v", got, err)
	}
}

func TestAuditRowsCarryValuesAndOneRequestIDLinksEverything(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	lifecycleFixture(t, recordTypes)
	order := newOrder(t, records, "A")
	updated, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "order", Id: order.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"title": text("B")}})
	if err != nil {
		t.Fatal(err)
	}
	history, err := records.ListRecordHistory(ctx, &athenav1.ListRecordHistoryRequest{RecordTypeKey: "order", Id: order.GetId()})
	if err != nil || len(history.GetEvents()) != 2 {
		t.Fatalf("history = %v, %v", history, err)
	}
	change := history.GetEvents()[0].GetChanges()[0]
	if change.GetField() != "title" || change.GetOldValue() != "A" || change.GetNewValue() != "B" || history.GetEvents()[0].GetSource() != sourceCLI {
		t.Fatalf("update row = %v", history.GetEvents()[0])
	}
	if history.GetEvents()[0].GetRequestId() != updated.GetRecord().GetRequestId() || history.GetEvents()[0].GetRequestId() == "" {
		t.Fatalf("audit request_id %q differs from the Record's %q", history.GetEvents()[0].GetRequestId(), updated.GetRecord().GetRequestId())
	}

	// One call, one request ID, on the Record, the Audit Log, the Document
	// Version and the Activity Timeline.
	callCtx := context.WithValue(ctx, requestKey{}, requestInfo{id: "0190a1b2-aaaa-7000-8000-000000000001", source: sourceAPI})
	res, err := records.SubmitRecord(callCtx, &athenav1.SubmitRecordRequest{RecordTypeKey: "order", Id: order.GetId(), ExpectedRevision: 2})
	if err != nil {
		t.Fatal(err)
	}
	const want = "0190a1b2-aaaa-7000-8000-000000000001"
	if res.GetRecord().GetRequestId() != want {
		t.Fatalf("record request_id = %q", res.GetRecord().GetRequestId())
	}
	for _, table := range []string{"athena_audit_log", "athena_document_versions", "athena_activity"} {
		var n int
		if err := records.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE record_id = ? AND request_id = ?", order.GetId(), want).Scan(&n); err != nil || n != 1 {
			t.Fatalf("%s rows with the request ID = %d (%v)", table, n, err)
		}
	}
	var source string
	if err := records.db.QueryRowContext(ctx, "SELECT source FROM athena_audit_log WHERE request_id = ?", want).Scan(&source); err != nil || source != sourceAPI {
		t.Fatalf("source = %q, %v", source, err)
	}
}

func TestRolledBackChangeLeavesNoAuditRow(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	lifecycleFixture(t, recordTypes)
	records.hooks = lifecycle.NewRegistry()
	records.hooks.On(lifecycle.OnSubmit, "order", func(context.Context, *lifecycle.Event) error { return errors.New("posting failed") })
	order := newOrder(t, records, "R")
	before := verifyChain(t, records)
	if _, err := records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: "order", Id: order.GetId(), ExpectedRevision: 1}); err == nil {
		t.Fatal("the hook must reject the submit")
	}
	after := verifyChain(t, records)
	if !after.OK || after.HeadSeq != before.HeadSeq || after.HeadHash != before.HeadHash {
		t.Fatalf("a failed submit moved the chain: before %+v, after %+v", before, after)
	}
}

func TestVerifyAuditLogService(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	newCustomer(t, records, "Acme")
	system := systemService{db: records.db}
	res, err := system.VerifyAuditLog(ctx, &athenav1.VerifyAuditLogRequest{})
	if err != nil || !res.GetOk() || res.GetRowsChecked() != 1 || len(res.GetHeadHash()) != 64 {
		t.Fatalf("verify = %v, %v", res, err)
	}
	if _, err := system.VerifyAuditLog(ctx, &athenav1.VerifyAuditLogRequest{Anchor: "nonsense"}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("a bad anchor must be refused: %v", err)
	}
	if _, err := records.db.ExecContext(ctx, "UPDATE athena_audit_log SET actor = 'x'"); err != nil {
		t.Fatal(err)
	}
	if res, err := system.VerifyAuditLog(ctx, &athenav1.VerifyAuditLogRequest{}); err != nil || res.GetOk() || res.GetBrokenSeq() != 1 {
		t.Fatalf("tampered verify = %v, %v", res, err)
	}
}

// The interceptor accepts a client's UUID request ID, replaces anything else,
// and returns the ID in the response header.
func TestVerifyDuringConcurrentWritesReportsNoTampering(t *testing.T) {
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	newCustomer(t, records, "Seed")
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := records.CreateRecord(context.Background(), &athenav1.CreateRecordRequest{RecordTypeKey: "customer", Values: map[string]*athenav1.FieldValue{"name": text("Writer")}}); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for i := 0; i < 30; i++ {
		if res := verifyChain(t, records); !res.OK {
			close(stop)
			<-done
			t.Fatalf("an intact chain failed verification during writes: %+v", res)
		}
	}
	close(stop)
	<-done
}

func TestRequestInterceptorSetsAndEchoesTheRequestID(t *testing.T) {
	rpc := connect.NewServer(requestInterceptor())
	athenav1connect.RegisterSystemServiceHandler(rpc, systemService{})
	mux := http.NewServeMux()
	connecthttp.Mount(mux, rpc)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	call := func(sent string) string {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/athena.v1.SystemService/GetSystemInfo", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		if sent != "" {
			req.Header.Set(requestIDHeader, sent)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		return res.Header.Get(requestIDHeader)
	}
	const mine = "0190a1b2-bbbb-7000-8000-000000000002"
	if got := call(mine); got != mine {
		t.Fatalf("a client ID must be kept, got %q", got)
	}
	if got := call(""); len(got) != 36 {
		t.Fatalf("a missing ID must be generated, got %q", got)
	}
	if got := call("not a uuid; DROP TABLE"); len(got) != 36 || strings.Contains(got, "DROP") {
		t.Fatalf("an invalid ID must be replaced, got %q", got)
	}
}
