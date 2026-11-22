package server

import (
	"context"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/posting"

	"connectrpc.com/connect/v2"
)

func TestPreviewPostingShowsPlansAndHash(t *testing.T) {
	f := newPostingFixture(t)
	ctx := context.Background()
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	bill := f.bill(t, "100")
	ledger := newLedgerService(f.db)
	preview, err := ledger.PreviewPosting(ctx, &athenav1.PreviewPostingRequest{RecordTypeKey: "bill", Id: bill.GetId()})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.GetRevision() != 1 || len(preview.GetPlans()) != 1 {
		t.Fatalf("preview = %+v", preview)
	}
	plan := preview.GetPlans()[0]
	if plan.GetRuleKey() != "bill_post" || plan.GetRuleVersion() != 1 || plan.GetBook() != "main" || plan.GetPostingDate() != "2026-06-15" {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.GetLines()) != 2 {
		t.Fatalf("lines = %+v", plan.GetLines())
	}
	first, second := plan.GetLines()[0], plan.GetLines()[1]
	if first.GetAccount() != "cash" || first.GetAccountLabel() != "Cash" || first.GetDebitBase() != "100" || first.GetCreditBase() != "0" {
		t.Fatalf("first leg = %+v", first)
	}
	if second.GetAccount() != "revenue" || second.GetAccountLabel() != "Revenue" || second.GetCreditBase() != "100" {
		t.Fatalf("second leg = %+v", second)
	}
	// A preview writes nothing to the books.
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM l_gl"); got != 0 {
		t.Fatalf("ledger rows = %d, want 0", got)
	}
	if got := queryInt(t, f.db, "SELECT COUNT(*) FROM athena_posting_groups"); got != 0 {
		t.Fatalf("posting groups = %d, want 0", got)
	}
	// The approved hash submits: preview and submit build the same plans.
	submitted, err := f.records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: "bill", Id: bill.GetId(), ExpectedRevision: 1, PostingPreviewHash: preview.GetPlanHash()})
	if err != nil {
		t.Fatalf("submit with the previewed hash: %v", err)
	}
	if submitted.GetRecord().GetRevision() != 2 {
		t.Fatalf("record = %+v", submitted.GetRecord())
	}
}

func TestPreviewPostingWithoutRulesReturnsEmpty(t *testing.T) {
	f := newPostingFixture(t)
	ctx := context.Background()
	mustSave(t, f.types, &athenav1.RecordType{Key: "memo", Label: "Memo", Kind: "document", Class: "non_posting", Submittable: true, Fields: []*athenav1.RecordTypeField{
		{Key: "body", Label: "Body", Type: "Text"},
	}})
	record, err := create(t, f.records, "memo", map[string]*athenav1.FieldValue{"body": text("hello")})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := newLedgerService(f.db).PreviewPosting(ctx, &athenav1.PreviewPostingRequest{RecordTypeKey: "memo", Id: record.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := posting.HashPlans(nil)
	if err != nil {
		t.Fatal(err)
	}
	if preview.GetRevision() != 1 || len(preview.GetPlans()) != 0 || preview.GetPlanHash() != empty {
		t.Fatalf("preview = %+v", preview)
	}
}

func TestPreviewPostingRefusesAnUnpostableVoucher(t *testing.T) {
	f := newPostingFixture(t)
	ctx := context.Background()
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	bill := f.bill(t, "")
	_, err := newLedgerService(f.db).PreviewPosting(ctx, &athenav1.PreviewPostingRequest{RecordTypeKey: "bill", Id: bill.GetId()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(err.Error(), "no amount") {
		t.Fatalf("error = %v", err)
	}
}

func TestPostingReadsNeedLedgerReadAndRecordGrant(t *testing.T) {
	f := newPostingFixture(t)
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	bill := f.bill(t, "100")
	ledger := newLedgerService(f.db)
	req := &athenav1.PreviewPostingRequest{RecordTypeKey: "bill", Id: bill.GetId()}
	// No ledger:read: denied before the voucher is even looked at.
	bare := auth.WithPrincipal(context.Background(), &auth.Principal{Username: "ana"})
	if _, err := ledger.PreviewPosting(bare, req); err == nil {
		t.Fatalf("preview without ledger:read passed")
	} else {
		requireCode(t, err, connect.CodePermissionDenied)
	}
	// Ledger:read without a Grant on bills: the voucher stays hidden.
	reader := auth.WithPrincipal(context.Background(), &auth.Principal{Username: "ana", SystemPermissions: map[string]bool{auth.PermissionLedgerRead: true}})
	for name, call := range map[string]func() error{
		"preview": func() error {
			_, err := ledger.PreviewPosting(reader, req)
			return err
		},
		"status": func() error {
			_, err := ledger.GetPostingStatus(reader, &athenav1.GetPostingStatusRequest{RecordTypeKey: "bill", Id: bill.GetId()})
			return err
		},
		"log": func() error {
			_, err := ledger.ListPostingLog(reader, &athenav1.ListPostingLogRequest{RecordTypeKey: "bill", Id: bill.GetId()})
			return err
		},
		"rows": func() error {
			_, err := ledger.ListLedgerRows(reader, &athenav1.ListLedgerRowsRequest{RecordTypeKey: "bill", Id: bill.GetId()})
			return err
		},
	} {
		if err := call(); err == nil {
			t.Fatalf("%s without a record Grant passed", name)
		} else {
			requireCode(t, err, connect.CodeNotFound)
		}
	}
}

func TestGetPostingStatusFollowsSubmitAndCancel(t *testing.T) {
	f := newPostingFixture(t)
	ctx := context.Background()
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	bill := f.bill(t, "100")
	ledger := newLedgerService(f.db)
	status, err := ledger.GetPostingStatus(ctx, &athenav1.GetPostingStatusRequest{RecordTypeKey: "bill", Id: bill.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.GetGroups()) != 0 {
		t.Fatalf("groups before submit = %+v", status.GetGroups())
	}
	submitted := submit(t, f.records, "bill", bill)
	status, err = ledger.GetPostingStatus(ctx, &athenav1.GetPostingStatusRequest{RecordTypeKey: "bill", Id: bill.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.GetGroups()) != 1 {
		t.Fatalf("groups = %+v", status.GetGroups())
	}
	group := status.GetGroups()[0]
	if group.GetRevision() != 2 || group.GetEvent() != "submit" || group.GetBook() != "main" || group.GetRowCount() != 2 || group.GetPostedAt() == "" || group.GetReversed() {
		t.Fatalf("group = %+v", group)
	}
	if _, err := cancel(f.records, "bill", submitted); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	status, err = ledger.GetPostingStatus(ctx, &athenav1.GetPostingStatusRequest{RecordTypeKey: "bill", Id: bill.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.GetGroups()) != 2 {
		t.Fatalf("groups = %+v", status.GetGroups())
	}
	if first := status.GetGroups()[0]; !first.GetReversed() || first.GetReversedBy() == "" {
		t.Fatalf("submit group = %+v: cancel must mark it reversed", first)
	}
	if second := status.GetGroups()[1]; second.GetEvent() != "cancel" || second.GetReversed() {
		t.Fatalf("cancel group = %+v", second)
	}
}

func TestListPostingLogPagesNewestFirst(t *testing.T) {
	f := newPostingFixture(t)
	ctx := context.Background()
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	bill := f.bill(t, "100")
	ledger := newLedgerService(f.db)
	submitted := submit(t, f.records, "bill", bill)
	if _, err := cancel(f.records, "bill", submitted); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	page, err := ledger.ListPostingLog(ctx, &athenav1.ListPostingLogRequest{RecordTypeKey: "bill", Id: bill.GetId(), PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetEntries()) != 1 || page.GetNextPageToken() == "" {
		t.Fatalf("page = %+v", page)
	}
	if first := page.GetEntries()[0]; first.GetEvent() != "cancel" || first.GetOutcome() != posting.OutcomePosted || first.GetCode() != posting.CodeOK || first.GetPlanHash() == "" || first.GetActor() == "" || first.GetCreatedAt() == "" {
		t.Fatalf("newest entry = %+v", first)
	}
	rest, err := ledger.ListPostingLog(ctx, &athenav1.ListPostingLogRequest{RecordTypeKey: "bill", Id: bill.GetId(), PageToken: page.GetNextPageToken()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.GetEntries()) != 1 || rest.GetEntries()[0].GetEvent() != "submit" || rest.GetNextPageToken() != "" {
		t.Fatalf("second page = %+v", rest)
	}
	filtered, err := ledger.ListPostingLog(ctx, &athenav1.ListPostingLogRequest{RecordTypeKey: "bill", Id: bill.GetId(), Outcome: "rejected"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.GetEntries()) != 0 {
		t.Fatalf("rejected entries = %+v", filtered.GetEntries())
	}
	if _, err := ledger.ListPostingLog(ctx, &athenav1.ListPostingLogRequest{RecordTypeKey: "bill", Id: bill.GetId(), Outcome: "bogus"}); err == nil {
		t.Fatalf("an unknown outcome filter passed")
	} else {
		requireCode(t, err, connect.CodeInvalidArgument)
	}
}

func TestListLedgerRowsPagesInPostingOrder(t *testing.T) {
	f := newPostingFixture(t)
	ctx := context.Background()
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	bill := f.bill(t, "100")
	ledger := newLedgerService(f.db)
	submitted := submit(t, f.records, "bill", bill)
	if _, err := cancel(f.records, "bill", submitted); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	page, err := ledger.ListLedgerRows(ctx, &athenav1.ListLedgerRowsRequest{RecordTypeKey: "bill", Id: bill.GetId(), PageSize: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetRows()) != 3 || page.GetNextPageToken() == "" {
		t.Fatalf("page = %+v", page)
	}
	first := page.GetRows()[0]
	if first.GetCompany() != "acme" || first.GetBook() != "main" || first.GetAccount() != "cash" || first.GetPeriod() == "" || first.GetPostingDate() != "2026-06-15" {
		t.Fatalf("first row = %+v", first)
	}
	if first.GetDebitBase() != "100.000000" || first.GetRevision() != 2 || first.GetEvent() != "submit" || first.GetPostingGroupId() == "" || first.GetIsReversal() {
		t.Fatalf("first row = %+v", first)
	}
	rest, err := ledger.ListLedgerRows(ctx, &athenav1.ListLedgerRowsRequest{RecordTypeKey: "bill", Id: bill.GetId(), PageToken: page.GetNextPageToken()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.GetRows()) != 1 || rest.GetNextPageToken() != "" {
		t.Fatalf("second page = %+v", rest)
	}
	if last := rest.GetRows()[0]; !last.GetIsReversal() || last.GetReversedRowId() == "" || last.GetEvent() != "cancel" {
		t.Fatalf("last row = %+v", last)
	}
}

func TestSubmitRefusesWhenRulesVanishedAfterPreview(t *testing.T) {
	f := newPostingFixture(t)
	ctx := context.Background()
	if _, err := f.saveRule(t, "bill_post", billRuleV1, true, 0); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	bill := f.bill(t, "100")
	preview, err := newLedgerService(f.db).PreviewPosting(ctx, &athenav1.PreviewPostingRequest{RecordTypeKey: "bill", Id: bill.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f.rules.GetRule(ctx, &athenav1.GetRuleRequest{Key: "bill_post"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rules.DeleteRule(ctx, &athenav1.DeleteRuleRequest{Key: "bill_post", ExpectedVersion: stored.GetRule().GetVersion()}); err != nil {
		t.Fatal(err)
	}
	_, err = f.records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: "bill", Id: bill.GetId(), ExpectedRevision: 1, PostingPreviewHash: preview.GetPlanHash()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if !strings.Contains(err.Error(), "changed since it was previewed") {
		t.Fatalf("error = %v", err)
	}
}
