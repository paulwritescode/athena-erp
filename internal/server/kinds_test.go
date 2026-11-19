package server

import (
	"context"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"

	"connectrpc.com/connect/v2"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func intp(n int32) *int32 { return &n }

// saveType saves a new Record Type with every property given.
func saveType(t *testing.T, s recordTypeService, recordType *athenav1.RecordType) error {
	t.Helper()
	_, err := s.SaveRecordType(context.Background(), &athenav1.SaveRecordTypeRequest{RecordType: recordType})
	return err
}

func mustSave(t *testing.T, s recordTypeService, recordType *athenav1.RecordType) {
	t.Helper()
	if err := saveType(t, s, recordType); err != nil {
		t.Fatalf("save %s: %v", recordType.GetKey(), err)
	}
}

func create(t *testing.T, s recordService, key string, values map[string]*athenav1.FieldValue) (*athenav1.Record, error) {
	t.Helper()
	res, err := s.CreateRecord(context.Background(), &athenav1.CreateRecordRequest{RecordTypeKey: key, Values: values})
	if err != nil {
		return nil, err
	}
	return res.GetRecord(), nil
}

func TestMasterSoftDeleteNeverRemovesTheRow(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	record := newCustomer(t, records, "Acme")
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	var deleted, total int
	if err := records.db.QueryRowContext(ctx, "SELECT COUNT(`deleted_at`), COUNT(*) FROM p_workspace__customer").Scan(&deleted, &total); err != nil {
		t.Fatal(err)
	}
	if deleted != 1 || total != 1 {
		t.Fatalf("deleted rows = %d of %d; the row must stay, marked", deleted, total)
	}
	list, _ := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "customer"})
	if len(list.GetRecords()) != 0 {
		t.Fatal("a soft-deleted record is still listed")
	}
	_, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 2, Values: map[string]*athenav1.FieldValue{"name": text("x")}})
	requireCode(t, err, connect.CodeNotFound)
	history, _ := records.ListRecordHistory(ctx, &athenav1.ListRecordHistoryRequest{RecordTypeKey: "customer", Id: record.GetId()})
	if history.GetEvents()[0].GetAction() != "deleted" {
		t.Fatalf("history = %v", history.GetEvents())
	}
}

func TestSystemFieldsAreSetByTheKernel(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	saveRecordType(t, recordTypes, "customer", customerFields...)
	record := newCustomer(t, records, "Acme")
	if record.GetCreatedBy() != localActor || record.GetOwner() != localActor || record.GetDefVersion() != 1 || record.GetRequestId() == "" {
		t.Fatalf("system fields = %v", record)
	}
	created := record.GetRequestId()
	updated, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"name": text("Acme Ltd")}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetRecord().GetRequestId() == created || updated.GetRecord().GetModifiedBy() != localActor {
		t.Fatalf("an update must carry its own request ID: %v", updated.GetRecord())
	}
	// Clients cannot write system fields through values.
	_, err = create(t, records, "customer", map[string]*athenav1.FieldValue{"name": text("x"), "owner": text("someone")})
	requireCode(t, err, connect.CodeInvalidArgument)
	// def_version follows the Record Type's revision.
	withEmail := append(append([]*athenav1.RecordTypeField{}, customerFields...), &athenav1.RecordTypeField{Key: "email", Label: "Email", Type: "Email"})
	if _, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "customer", Label: "customer", Fields: withEmail}, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	second := newCustomer(t, records, "Second")
	if second.GetDefVersion() != 2 {
		t.Fatalf("def_version = %d, want 2", second.GetDefVersion())
	}
}

func TestSetOnceRejectsChangeAfterFirstValue(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "item", Label: "Item", Fields: []*athenav1.RecordTypeField{
		{Key: "name", Label: "Name", Type: "Text", Required: true},
		{Key: "code", Label: "Code", Type: "Text", SetOnce: true},
	}})
	record, err := create(t, records, "item", map[string]*athenav1.FieldValue{"name": text("A")})
	if err != nil {
		t.Fatal(err)
	}
	update := func(revision int64, code string) error {
		_, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "item", Id: record.GetId(), ExpectedRevision: revision, Values: map[string]*athenav1.FieldValue{"code": text(code)}})
		return err
	}
	if err := update(1, "X1"); err != nil {
		t.Fatalf("first value: %v", err)
	}
	requireCode(t, update(2, "X2"), connect.CodeInvalidArgument)
	if err := update(2, "X1"); err != nil {
		t.Fatalf("saving the same value again must be allowed: %v", err)
	}
}

func TestUniqueAndRequiredAreEnforcedOnTheServer(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "item", Label: "Item", Fields: []*athenav1.RecordTypeField{
		{Key: "code", Label: "Code", Type: "Text", Required: true, Unique: true},
		{Key: "note", Label: "Note", Type: "Text", Index: true},
	}})
	first, err := create(t, records, "item", map[string]*athenav1.FieldValue{"code": text("A")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = create(t, records, "item", map[string]*athenav1.FieldValue{"code": text("A")})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = create(t, records, "item", map[string]*athenav1.FieldValue{})
	requireCode(t, err, connect.CodeInvalidArgument)
	second, err := create(t, records, "item", map[string]*athenav1.FieldValue{"code": text("B")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "item", Id: second.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"code": text("A")}})
	requireCode(t, err, connect.CodeInvalidArgument)
	// A deleted Master keeps its value reserved; the index still refuses it.
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "item", Id: first.GetId(), ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	_, err = create(t, records, "item", map[string]*athenav1.FieldValue{"code": text("A")})
	requireCode(t, err, connect.CodeInvalidArgument)
	var unique, plain bool
	table := recordTypeTableName("item")
	unique, _ = dialect.IndexExists(ctx, records.db, table, fieldIndexName("uq", "item", "code"))
	plain, _ = dialect.IndexExists(ctx, records.db, table, fieldIndexName("ix", "item", "note"))
	if !unique || !plain {
		t.Fatalf("indexes: unique %v, plain %v", unique, plain)
	}
}

func TestMakingAFieldUniqueIsRefusedWhileValuesRepeat(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	fields := []*athenav1.RecordTypeField{{Key: "code", Label: "Code", Type: "Text"}}
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "item", Label: "Item", Fields: fields})
	for range 2 {
		if _, err := create(t, records, "item", map[string]*athenav1.FieldValue{"code": text("same")}); err != nil {
			t.Fatal(err)
		}
	}
	unique := []*athenav1.RecordTypeField{{Key: "code", Label: "Code", Type: "Text", Unique: true}}
	_, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "item", Label: "Item", Fields: unique}, ExpectedRevision: 1})
	requireCode(t, err, connect.CodeFailedPrecondition)
}

func TestDefaultReadOnlyAndRangeAttributes(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "item", Label: "Item", Fields: []*athenav1.RecordTypeField{
		{Key: "status", Label: "Status", Type: "Text", DefaultValue: "new", ReadOnly: true},
		{Key: "qty", Label: "Qty", Type: "Number", Min: "1", Max: "10"},
		{Key: "code", Label: "Code", Type: "Text", Pattern: "[A-Z]{3}-[0-9]+", MaxLength: 8},
		{Key: "active", Label: "Active", Type: "Checkbox", DefaultValue: "true"},
	}})
	record, err := create(t, records, "item", map[string]*athenav1.FieldValue{"qty": text("5")})
	if err != nil {
		t.Fatal(err)
	}
	if record.GetValues()["status"].GetStringValue() != "new" || !record.GetValues()["active"].GetBoolValue() {
		t.Fatalf("defaults were not applied: %v", record.GetValues())
	}
	for name, values := range map[string]map[string]*athenav1.FieldValue{
		"read only":    {"status": text("done")},
		"below min":    {"qty": text("0.5")},
		"above max":    {"qty": text("11")},
		"pattern":      {"code": text("abc-1")},
		"pattern full": {"code": text("ABC-1x")},
		"max length":   {"code": text("ABC-12345")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := create(t, records, "item", values)
			requireCode(t, err, connect.CodeInvalidArgument)
		})
	}
	if _, err := create(t, records, "item", map[string]*athenav1.FieldValue{"qty": text("10"), "code": text("ABC-12"), "status": text("new")}); err != nil {
		t.Fatalf("a valid record with the default echoed back: %v", err)
	}
	_, err = records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "item", Id: record.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"status": text("done")}})
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestMoneyIsStoredAsMinorUnitsAndRejectsExtraDecimals(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "invoice", Label: "Invoice", Fields: []*athenav1.RecordTypeField{
		{Key: "total", Label: "Total", Type: "Money", Currency: "USD", Scale: intp(2), Min: "0"},
		{Key: "yen", Label: "Yen", Type: "Money", Currency: "JPY", Scale: intp(0)},
		{Key: "weight", Label: "Weight", Type: "Quantity", Uom: "kg", Scale: intp(3)},
		{Key: "state", Label: "State", Type: "Enum", EnumValues: []*athenav1.EnumValue{{Value: "open", Label: "Open"}, {Value: "paid", Label: "Paid"}}},
	}})
	record, err := create(t, records, "invoice", map[string]*athenav1.FieldValue{"total": text("12.3"), "yen": text("500"), "weight": text("1.25"), "state": text("open")})
	if err != nil {
		t.Fatal(err)
	}
	if got := record.GetValues()["total"].GetStringValue(); got != "12.30" {
		t.Fatalf("total = %q, want 12.30", got)
	}
	var minor int64
	if err := records.db.QueryRowContext(ctx, "SELECT `total` FROM p_workspace__invoice").Scan(&minor); err != nil || minor != 1230 {
		t.Fatalf("stored minor units = %d, %v", minor, err)
	}
	for name, values := range map[string]map[string]*athenav1.FieldValue{
		"three decimals": {"total": text("1.234")},
		"negative":       {"total": text("-1")},
		"not a number":   {"total": text("$5")},
		"yen fraction":   {"yen": text("5.5")},
		"overflow":       {"total": text("99999999999999999999")},
		"quantity scale": {"weight": text("1.2345")},
		"unknown choice": {"state": text("void")},
		"choice in caps": {"state": text("OPEN")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := create(t, records, "invoice", values)
			requireCode(t, err, connect.CodeInvalidArgument)
		})
	}
	// Unchanged money does not count as a change.
	res, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "invoice", Id: record.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"total": text("12.30")}})
	if err != nil || res.GetRecord().GetRevision() != 1 {
		t.Fatalf("unchanged money update: %v %v", res, err)
	}
}

func TestChangingMoneyCurrencyClearsValuesOnlyWithConfirmation(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "invoice", Label: "Invoice", Fields: []*athenav1.RecordTypeField{{Key: "total", Label: "Total", Type: "Money", Currency: "USD", Scale: intp(2)}}})
	if _, err := create(t, records, "invoice", map[string]*athenav1.FieldValue{"total": text("5")}); err != nil {
		t.Fatal(err)
	}
	next := &athenav1.RecordType{Key: "invoice", Label: "Invoice", Fields: []*athenav1.RecordTypeField{{Key: "total", Label: "Total", Type: "Money", Currency: "EUR", Scale: intp(2)}}}
	_, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: next, ExpectedRevision: 1})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if _, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: next, ExpectedRevision: 1, ConfirmDataLoss: true}); err != nil {
		t.Fatal(err)
	}
	list, _ := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "invoice"})
	if !list.GetRecords()[0].GetValues()["total"].GetNullValue() {
		t.Fatal("stored minor units survived a currency change")
	}
}

func TestRecordTypeKindRules(t *testing.T) {
	ctx := context.Background()
	recordTypes, _ := newServices(t)
	field := []*athenav1.RecordTypeField{{Key: "name", Label: "Name", Type: "Text"}}
	for name, recordType := range map[string]*athenav1.RecordType{
		"document without class": {Key: "order", Label: "Order", Kind: "document", Fields: field},
		"unknown class":          {Key: "order", Label: "Order", Kind: "document", Class: "other", Fields: field},
		"class on master":        {Key: "order", Label: "Order", Kind: "master", Class: "work", Fields: field},
		"kernel kind":            {Key: "entry", Label: "Entry", Kind: "ledger", Fields: field},
		"unknown kind":           {Key: "entry", Label: "Entry", Kind: "thing", Fields: field},
		"child without parent":   {Key: "line", Label: "Line", Kind: "child", Fields: field},
		"child of nothing":       {Key: "line", Label: "Line", Kind: "child", ParentRecordTypeKey: "ghost", Fields: field},
		"parent on master":       {Key: "order", Label: "Order", ParentRecordTypeKey: "x", Fields: field},
	} {
		t.Run(name, func(t *testing.T) {
			requireCode(t, saveType(t, recordTypes, recordType), connect.CodeInvalidArgument)
		})
	}
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "order", Label: "Order", Kind: "document", Class: "posting", Fields: field})
	_, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "order", Label: "Order", Kind: "master", Fields: field}, ExpectedRevision: 1})
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestSettingHoldsOneUndeletableRecord(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "stock_settings", Label: "Stock settings", Kind: "setting", Fields: []*athenav1.RecordTypeField{{Key: "policy", Label: "Policy", Type: "Text"}}})
	record, err := create(t, records, "stock_settings", map[string]*athenav1.FieldValue{"policy": text("fifo")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = create(t, records, "stock_settings", map[string]*athenav1.FieldValue{"policy": text("lifo")})
	requireCode(t, err, connect.CodeFailedPrecondition)
	_, err = records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "stock_settings", Id: record.GetId(), ExpectedRevision: 1})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if _, err := records.UpdateRecord(ctx, &athenav1.UpdateRecordRequest{RecordTypeKey: "stock_settings", Id: record.GetId(), ExpectedRevision: 1, Values: map[string]*athenav1.FieldValue{"policy": text("lifo")}}); err != nil {
		t.Fatalf("a setting is editable: %v", err)
	}
}

func TestChildRecordsFollowTheirParent(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "order", Label: "Order", Kind: "document", Class: "non_posting", Fields: []*athenav1.RecordTypeField{{Key: "ref", Label: "Ref", Type: "Text"}}})
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "order_line", Label: "Order line", Kind: "child", ParentRecordTypeKey: "order", Fields: []*athenav1.RecordTypeField{{Key: "item", Label: "Item", Type: "Text"}}})
	order, err := create(t, records, "order", map[string]*athenav1.FieldValue{"ref": text("SO-1")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = create(t, records, "order_line", map[string]*athenav1.FieldValue{"item": text("x")})
	requireCode(t, err, connect.CodeInvalidArgument)
	_, err = records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "order_line", ParentId: "0190a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b", Values: map[string]*athenav1.FieldValue{"item": text("x")}})
	requireCode(t, err, connect.CodeInvalidArgument)
	var lines []*athenav1.Record
	for _, item := range []string{"a", "b"} {
		res, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "order_line", ParentId: order.GetId(), Values: map[string]*athenav1.FieldValue{"item": text(item)}})
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, res.GetRecord())
	}
	if lines[0].GetIdx() != 1 || lines[1].GetIdx() != 2 || lines[1].GetParentId() != order.GetId() {
		t.Fatalf("positions = %d, %d", lines[0].GetIdx(), lines[1].GetIdx())
	}
	list, err := records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: "order_line", ParentId: order.GetId()})
	if err != nil || len(list.GetRecords()) != 2 {
		t.Fatalf("list by parent: %v %v", list, err)
	}
	// The parent cannot be removed while children point at it.
	_, err = records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "order", Id: order.GetId(), ExpectedRevision: 1})
	requireCode(t, err, connect.CodeFailedPrecondition)
	_, err = recordTypes.DeleteRecordType(ctx, &athenav1.DeleteRecordTypeRequest{Key: "order", ExpectedRevision: 1, DeleteRecords: true})
	requireCode(t, err, connect.CodeFailedPrecondition)
}

func TestMasterWithChildRecordsCannotBeDeleted(t *testing.T) {
	ctx := context.Background()
	recordTypes, records := newServices(t)
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "account", Label: "Account", Kind: "master", Fields: []*athenav1.RecordTypeField{{Key: "title", Label: "Title", Type: "Text"}}})
	mustSave(t, recordTypes, &athenav1.RecordType{Key: "contact", Label: "Contact", Kind: "child", ParentRecordTypeKey: "account", Fields: []*athenav1.RecordTypeField{{Key: "person", Label: "Person", Type: "Text"}}})
	account, err := create(t, records, "account", map[string]*athenav1.FieldValue{"title": text("Acme")})
	if err != nil {
		t.Fatal(err)
	}
	res, err := records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "contact", ParentId: account.GetId(), Values: map[string]*athenav1.FieldValue{"person": text("Ann")}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "account", Id: account.GetId(), ExpectedRevision: account.GetRevision()})
	requireCode(t, err, connect.CodeFailedPrecondition)
	child := res.GetRecord()
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "contact", Id: child.GetId(), ExpectedRevision: child.GetRevision()}); err != nil {
		t.Fatal(err)
	}
	if _, err := records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "account", Id: account.GetId(), ExpectedRevision: account.GetRevision()}); err != nil {
		t.Fatalf("parent delete after the child is gone: %v", err)
	}
}

func TestKindsAndSystemColumnsMatchTheDocumentedTable(t *testing.T) {
	want := map[string]string{
		kindMaster:   "created_by modified_by def_version request_id owner deleted_at",
		kindDocument: "created_by modified_by def_version request_id owner",
		kindChild:    "created_by modified_by def_version request_id parent_id idx",
		kindSetting:  "created_by modified_by def_version request_id",
	}
	for kind, columns := range want {
		var names []string
		for _, column := range systemColumns(kind) {
			names = append(names, column.Name)
			if !reservedColumns[column.Name] {
				t.Errorf("%s is not reserved", column.Name)
			}
		}
		if got := strings.Join(names, " "); got != columns {
			t.Errorf("%s columns = %q, want %q", kind, got, columns)
		}
	}
}

func TestMigrationGivesLegacyRecordTypesTheMasterKind(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{Target: 5}); err != nil {
		t.Fatal(err)
	}
	// A Record Type saved by the previous build: no kind, no system columns.
	for _, statement := range []string{
		"CREATE TABLE p_workspace__legacy (id CHAR(36) NOT NULL PRIMARY KEY, revision BIGINT NOT NULL DEFAULT 1, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, name VARCHAR(255) NULL)",
		"INSERT INTO p_workspace__legacy (id, name) VALUES ('0190a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b', 'kept')",
		`INSERT INTO athena_record_types (name, label, definition, revision) VALUES ('legacy', 'Legacy', '{"name":"legacy","label":"Legacy","description":"","fields":[{"name":"name","label":"Name","type":"Text","required":false}]}', 3)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	records := recordService{db: db}
	recordType, err := records.recordType(ctx, "legacy")
	if err != nil || recordType.kind() != kindMaster || recordType.Kind != kindMaster {
		t.Fatalf("definition = %+v, %v", recordType, err)
	}
	got, err := records.GetRecord(ctx, &athenav1.GetRecordRequest{RecordTypeKey: "legacy", Id: "0190a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b"})
	if err != nil {
		t.Fatal(err)
	}
	record := got.GetRecord()
	if record.GetValues()["name"].GetStringValue() != "kept" || record.GetCreatedBy() != localActor || record.GetOwner() != localActor || record.GetDefVersion() != 3 {
		t.Fatalf("legacy record after migration = %v", record)
	}
}

// Spans carry ids, counts and outcome codes only (AGENTS.md telemetry rule).
func TestKernelSpansNeverCarryRecordValues(t *testing.T) {
	recordTypes, records := newServices(t)
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	saveRecordType(t, recordTypes, "customer", customerFields...)
	const secret = "s3cret-customer-name"
	record := newCustomer(t, records, secret)
	_, _ = records.UpdateRecord(context.Background(), &athenav1.UpdateRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 9, Values: map[string]*athenav1.FieldValue{"name": text(secret)}})
	_, _ = records.DeleteRecord(context.Background(), &athenav1.DeleteRecordRequest{RecordTypeKey: "customer", Id: record.GetId(), ExpectedRevision: 1})
	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("spans = %d, want create, update and delete", len(spans))
	}
	outcomes := map[string]string{}
	for _, span := range spans {
		for _, attribute := range span.Attributes {
			if strings.Contains(attribute.Value.Emit(), secret) {
				t.Fatalf("span %s attribute %s carries a record value", span.Name, attribute.Key)
			}
			if attribute.Key == "athena.outcome" {
				outcomes[span.Name] = attribute.Value.Emit()
			}
		}
		if strings.Contains(span.Status.Description, secret) {
			t.Fatalf("span %s status carries a record value", span.Name)
		}
	}
	if outcomes["athena.create_record"] != "ok" || outcomes["athena.update_record"] != "aborted" {
		t.Fatalf("outcomes = %v", outcomes)
	}
}
