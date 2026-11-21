package server

import (
	"context"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"

	"connectrpc.com/connect/v2"
)

// layoutType saves a master with two text fields for layout tests.
func layoutType(t *testing.T, recordTypes recordTypeService) {
	t.Helper()
	saveRecordType(t, recordTypes, "page", &athenav1.RecordTypeField{Key: "title", Label: "Title", Type: "Text"}, &athenav1.RecordTypeField{Key: "body", Label: "Body", Type: "Text"})
}

func TestLayoutRoundTripThroughSave(t *testing.T) {
	recordTypes, _ := newServices(t)
	ctx := context.Background()
	layoutType(t, recordTypes)
	if _, err := recordTypes.db.ExecContext(ctx, `INSERT INTO athena_files (id, path, revision, hash, size, modified_by, created_at, modified_at)
		VALUES ('0198a2c4-7b1e-7000-8000-000000000001', 'images/hero.png', 1, 'abc', 10, 'ada', UTC_TIMESTAMP(3), UTC_TIMESTAMP(3))`); err != nil {
		t.Fatal(err)
	}
	saved, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{
		Key: "page", Label: "Page", Fields: []*athenav1.RecordTypeField{
			{Key: "title", Label: "Title", Type: "Text"},
			{Key: "body", Label: "Body", Type: "Text"},
		},
		Layout: &athenav1.FormLayout{Sections: []*athenav1.FormSection{
			{Title: "Main", Columns: 2, Visibility: `doc.title != ""`, Items: []*athenav1.FormLayoutItem{
				{Kind: &athenav1.FormLayoutItem_Field{Field: &athenav1.FormFieldItem{FieldKey: "title", Width: "half", HelpText: "The heading"}}},
				{Kind: &athenav1.FormLayoutItem_Text{Text: &athenav1.FormTextBlock{Text: "Hello"}}},
				{Kind: &athenav1.FormLayoutItem_Image{Image: &athenav1.FormImageBlock{Path: "images/hero.png"}}},
				{Kind: &athenav1.FormLayoutItem_Art{Art: &athenav1.FormArtBlock{Key: "geom:1"}}},
			}},
			{Title: "More", Columns: 1},
		}},
	}, ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if saved.GetRecordType().GetRevision() != 2 {
		t.Fatalf("revision = %d, want 2", saved.GetRecordType().GetRevision())
	}
	listed, err := recordTypes.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := listed.GetRecordTypes()[0].GetLayout()
	if got == nil || len(got.GetSections()) != 2 {
		t.Fatalf("layout = %+v", got)
	}
	// The unplaced body field appends to the last section at full width.
	last := got.GetSections()[1]
	if len(last.GetItems()) != 1 || last.GetItems()[0].GetField().GetFieldKey() != "body" || last.GetItems()[0].GetField().GetWidth() != "full" {
		t.Fatalf("last section = %+v", last)
	}
	first := got.GetSections()[0]
	if len(first.GetItems()) != 4 || first.GetColumns() != 2 || first.GetVisibility() != `doc.title != ""` {
		t.Fatalf("first section = %+v", first)
	}
	if first.GetItems()[2].GetImage().GetPath() != "images/hero.png" || first.GetItems()[3].GetArt().GetKey() != "geom:1" {
		t.Fatalf("blocks = %+v", first.GetItems())
	}
}

func TestLayoutValidationErrors(t *testing.T) {
	field := func(key, width string) *athenav1.FormLayoutItem {
		return &athenav1.FormLayoutItem{Kind: &athenav1.FormLayoutItem_Field{Field: &athenav1.FormFieldItem{FieldKey: key, Width: width}}}
	}
	blockImage := func(path string) *athenav1.FormLayoutItem {
		return &athenav1.FormLayoutItem{Kind: &athenav1.FormLayoutItem_Image{Image: &athenav1.FormImageBlock{Path: path}}}
	}
	blockText := func(text string) *athenav1.FormLayoutItem {
		return &athenav1.FormLayoutItem{Kind: &athenav1.FormLayoutItem_Text{Text: &athenav1.FormTextBlock{Text: text}}}
	}
	blockArt := func(key string) *athenav1.FormLayoutItem {
		return &athenav1.FormLayoutItem{Kind: &athenav1.FormLayoutItem_Art{Art: &athenav1.FormArtBlock{Key: key}}}
	}
	sections := func(sections ...*athenav1.FormSection) *athenav1.FormLayout {
		return &athenav1.FormLayout{Sections: sections}
	}
	many := make([]*athenav1.FormSection, 51)
	for i := range many {
		many[i] = &athenav1.FormSection{Title: "S", Columns: 1}
	}
	cases := []struct {
		name   string
		layout *athenav1.FormLayout
		want   string
	}{
		{"unknown key", sections(&athenav1.FormSection{Title: "S", Columns: 1, Items: []*athenav1.FormLayoutItem{field("nope", "full")}}), "not a field"},
		{"duplicate key", sections(&athenav1.FormSection{Title: "S", Columns: 1, Items: []*athenav1.FormLayoutItem{field("title", "full"), field("title", "full")}}), "placed twice"},
		{"bad columns", sections(&athenav1.FormSection{Title: "S", Columns: 3}), "columns is 1 or 2"},
		{"missing columns", sections(&athenav1.FormSection{Title: "S"}), "columns is 1 or 2"},
		{"bad width", sections(&athenav1.FormSection{Title: "S", Columns: 1, Items: []*athenav1.FormLayoutItem{field("title", "wide")}}), "full or half"},
		{"non-boolean visibility", sections(&athenav1.FormSection{Title: "S", Columns: 1, Visibility: `doc.title`}), "must return true or false"},
		{"broken visibility", sections(&athenav1.FormSection{Title: "S", Columns: 1, Visibility: `doc.title ==`}), "visibility"},
		{"missing image", sections(
			&athenav1.FormSection{Title: "S", Columns: 1, Items: []*athenav1.FormLayoutItem{
				blockImage("images/gone.png"),
			}},
		), "not an uploaded image"},
		{"image outside images", sections(
			&athenav1.FormSection{Title: "S", Columns: 1, Items: []*athenav1.FormLayoutItem{
				blockImage("rules/x.png"),
			}},
		), "not an image under images/"},
		{"empty text", sections(
			&athenav1.FormSection{Title: "S", Columns: 1, Items: []*athenav1.FormLayoutItem{
				blockText("  "),
			}},
		), "needs its text"},
		{"empty art", sections(
			&athenav1.FormSection{Title: "S", Columns: 1, Items: []*athenav1.FormLayoutItem{
				blockArt(""),
			}},
		), "needs its registry key"},
		{"too many sections", sections(many...), "more than 50 sections"},
		{"long title", sections(&athenav1.FormSection{Title: strings.Repeat("t", 256), Columns: 1}), "longer than 255"},
		{"long help", sections(&athenav1.FormSection{Title: "S", Columns: 1, Items: []*athenav1.FormLayoutItem{{
			Kind: &athenav1.FormLayoutItem_Field{Field: &athenav1.FormFieldItem{FieldKey: "title", HelpText: strings.Repeat("h", 2001)}},
		}}}), "help text is longer than 2000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recordTypes, _ := newServices(t)
			ctx := context.Background()
			layoutType(t, recordTypes)
			_, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{
				Key: "page", Label: "Page", Fields: []*athenav1.RecordTypeField{
					{Key: "title", Label: "Title", Type: "Text"},
					{Key: "body", Label: "Body", Type: "Text"},
				},
				Layout: tc.layout,
			}, ExpectedRevision: 1})
			requireCode(t, err, connect.CodeInvalidArgument)
			if !strings.Contains(connectMessage(err), tc.want) {
				t.Fatalf("error = %q, want %q", connectMessage(err), tc.want)
			}
		})
	}
}

func TestLayoutHidesFieldsAboveTheUserLevel(t *testing.T) {
	f := newAccessFixture(t)
	f.notes(t)
	ctx := context.Background()
	_, err := f.types.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{
		Key: "note", Label: "Note", Fields: noteFields,
		Layout: &athenav1.FormLayout{Sections: []*athenav1.FormSection{
			{Title: "Main", Columns: 2, Items: []*athenav1.FormLayoutItem{
				{Kind: &athenav1.FormLayoutItem_Field{Field: &athenav1.FormFieldItem{FieldKey: "title", Width: "half"}}},
				{Kind: &athenav1.FormLayoutItem_Text{Text: &athenav1.FormTextBlock{Text: "Hello"}}},
			}},
			{Title: "Pay", Columns: 1, Items: []*athenav1.FormLayoutItem{
				{Kind: &athenav1.FormLayoutItem_Field{Field: &athenav1.FormFieldItem{FieldKey: "salary"}}},
			}},
		}},
	}, ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	f.role(t, "clerk", nil, grantOf("note", 1, "", auth.ActionRead))
	f.user(t, "clerk", "clerk")
	b := f.browser(t)
	b.login(t, "clerk")
	listed, err := b.types.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	note := listed.GetRecordTypes()[0]
	for _, field := range note.GetFields() {
		if field.GetKey() == "salary" {
			t.Fatalf("fields = %+v, salary must stay hidden", note.GetFields())
		}
	}
	sections := note.GetLayout().GetSections()
	if len(sections) != 1 || sections[0].GetTitle() != "Main" {
		t.Fatalf("sections = %+v, want only Main", sections)
	}
	items := sections[0].GetItems()
	if len(items) != 2 || items[0].GetField().GetFieldKey() != "title" || items[1].GetText().GetText() != "Hello" {
		t.Fatalf("items = %+v, want title and the text block", items)
	}
}

func TestLayoutOnlySaveSkipsDDL(t *testing.T) {
	recordTypes, _ := newServices(t)
	ctx := context.Background()
	layoutType(t, recordTypes)
	columns := func() []string {
		rows, err := recordTypes.db.QueryContext(ctx, `SELECT COLUMN_NAME FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'p_workspace__page' ORDER BY ORDINAL_POSITION`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var column string
			if err := rows.Scan(&column); err != nil {
				t.Fatal(err)
			}
			out = append(out, column)
		}
		return out
	}
	before := columns()
	plan, err := recordTypes.PlanRecordTypeSave(ctx, &athenav1.PlanRecordTypeSaveRequest{RecordType: &athenav1.RecordType{
		Key: "page", Label: "Page", Fields: []*athenav1.RecordTypeField{
			{Key: "title", Label: "Title", Type: "Text"},
			{Key: "body", Label: "Body", Type: "Text"},
		},
		Layout: &athenav1.FormLayout{Sections: []*athenav1.FormSection{{Title: "Main", Columns: 2}}},
	}, ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.GetSteps()) != 0 || plan.GetRequiresConfirmation() {
		t.Fatalf("plan = %+v, want no steps and no confirmation", plan)
	}
	saved, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{
		Key: "page", Label: "Page", Fields: []*athenav1.RecordTypeField{
			{Key: "title", Label: "Title", Type: "Text"},
			{Key: "body", Label: "Body", Type: "Text"},
		},
		Layout: &athenav1.FormLayout{Sections: []*athenav1.FormSection{{Title: "Main", Columns: 2}}},
	}, ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if saved.GetRecordType().GetRevision() != 2 {
		t.Fatalf("revision = %d, want 2", saved.GetRecordType().GetRevision())
	}
	if after := columns(); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Fatalf("columns changed: %v -> %v", before, after)
	}
	listed, err := recordTypes.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := listed.GetRecordTypes()[0].GetLayout()
	if len(got.GetSections()) != 1 || len(got.GetSections()[0].GetItems()) != 2 {
		t.Fatalf("layout = %+v, want both fields appended", got)
	}
}
