package server

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/expr"
	"athena-erp/internal/files"
)

// Form layout of a Record Type: sections of fields, text, images and art.
// The layout lives with the definition, so a layout-only save bumps the
// revision and journals an operation without touching the table.
type formLayout struct {
	Sections []formSection `json:"sections,omitempty"`
}

type formSection struct {
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Columns     int32            `json:"columns,omitempty"`
	Visibility  string           `json:"visibility,omitempty"`
	Items       []formLayoutItem `json:"items,omitempty"`
}

// Exactly one of the pointers is set, matching the proto oneof.
type formLayoutItem struct {
	Field *formFieldItem  `json:"field,omitempty"`
	Text  *formTextBlock  `json:"text,omitempty"`
	Image *formImageBlock `json:"image,omitempty"`
	Art   *formArtBlock   `json:"art,omitempty"`
}

type formFieldItem struct {
	FieldKey string `json:"field_key,omitempty"`
	Width    string `json:"width,omitempty"`
	HelpText string `json:"help_text,omitempty"`
}

type formTextBlock struct {
	Text string `json:"text,omitempty"`
}

type formImageBlock struct {
	Path string `json:"path,omitempty"`
}

type formArtBlock struct {
	Key string `json:"key,omitempty"`
}

const (
	maxLayoutSections = 50
	maxLayoutItems    = 500
	maxLayoutText     = 10000
	maxArtKey         = 200
)

// layoutFromProto maps a form layout, appending fields the layout does not
// place to the last section at full width. A layout with no sections is no
// layout: it normalizes to nil.
func layoutFromProto(layout *athenav1.FormLayout, fields []recordTypeField) *formLayout {
	if layout == nil || len(layout.GetSections()) == 0 {
		return nil
	}
	out := &formLayout{}
	for _, section := range layout.GetSections() {
		mapped := formSection{
			Title:       strings.TrimSpace(section.GetTitle()),
			Description: strings.TrimSpace(section.GetDescription()),
			Columns:     section.GetColumns(),
			Visibility:  strings.TrimSpace(section.GetVisibility()),
		}
		for _, item := range section.GetItems() {
			mapped.Items = append(mapped.Items, layoutItemFromProto(item))
		}
		out.Sections = append(out.Sections, mapped)
	}
	placed := map[string]bool{}
	for _, section := range out.Sections {
		for _, item := range section.Items {
			if item.Field != nil {
				placed[item.Field.FieldKey] = true
			}
		}
	}
	last := &out.Sections[len(out.Sections)-1]
	for _, field := range fields {
		if !placed[field.Name] {
			last.Items = append(last.Items, formLayoutItem{Field: &formFieldItem{FieldKey: field.Name, Width: "full"}})
		}
	}
	return out
}

func layoutItemFromProto(item *athenav1.FormLayoutItem) formLayoutItem {
	switch kind := item.GetKind().(type) {
	case *athenav1.FormLayoutItem_Field:
		return formLayoutItem{Field: &formFieldItem{FieldKey: kind.Field.GetFieldKey(), Width: kind.Field.GetWidth(), HelpText: kind.Field.GetHelpText()}}
	case *athenav1.FormLayoutItem_Text:
		return formLayoutItem{Text: &formTextBlock{Text: kind.Text.GetText()}}
	case *athenav1.FormLayoutItem_Image:
		return formLayoutItem{Image: &formImageBlock{Path: strings.TrimSpace(kind.Image.GetPath())}}
	case *athenav1.FormLayoutItem_Art:
		return formLayoutItem{Art: &formArtBlock{Key: strings.TrimSpace(kind.Art.GetKey())}}
	}
	return formLayoutItem{}
}

// layoutToProto renders a stored layout. A nil layout stays nil.
func layoutToProto(layout *formLayout) *athenav1.FormLayout {
	if layout == nil {
		return nil
	}
	out := &athenav1.FormLayout{}
	for _, section := range layout.Sections {
		mapped := &athenav1.FormSection{Title: section.Title, Description: section.Description, Columns: section.Columns, Visibility: section.Visibility}
		for _, item := range section.Items {
			mapped.Items = append(mapped.Items, layoutItemToProto(item))
		}
		out.Sections = append(out.Sections, mapped)
	}
	return out
}

func layoutItemToProto(item formLayoutItem) *athenav1.FormLayoutItem {
	switch {
	case item.Field != nil:
		return &athenav1.FormLayoutItem{Kind: &athenav1.FormLayoutItem_Field{Field: &athenav1.FormFieldItem{FieldKey: item.Field.FieldKey, Width: item.Field.Width, HelpText: item.Field.HelpText}}}
	case item.Text != nil:
		return &athenav1.FormLayoutItem{Kind: &athenav1.FormLayoutItem_Text{Text: &athenav1.FormTextBlock{Text: item.Text.Text}}}
	case item.Image != nil:
		return &athenav1.FormLayoutItem{Kind: &athenav1.FormLayoutItem_Image{Image: &athenav1.FormImageBlock{Path: item.Image.Path}}}
	case item.Art != nil:
		return &athenav1.FormLayoutItem{Kind: &athenav1.FormLayoutItem_Art{Art: &athenav1.FormArtBlock{Key: item.Art.Key}}}
	}
	return &athenav1.FormLayoutItem{}
}

// validateLayout checks a layout against its fields: section and item caps,
// columns, widths, field keys placed at most once, text, recorded images,
// art keys and visibility expressions. A nil layout is valid.
func validateLayout(ctx context.Context, conn sqlConn, d recordTypeDefinition) error {
	layout := d.Layout
	if layout == nil {
		return nil
	}
	if len(layout.Sections) > maxLayoutSections {
		return invalid(fmt.Sprintf("the layout holds more than %d sections", maxLayoutSections))
	}
	keys := map[string]bool{}
	for _, field := range d.Fields {
		keys[field.Name] = true
	}
	placed := map[string]bool{}
	items := 0
	environment, err := newExprEnvironment(ctx, conn, d, nil)
	if err != nil {
		return err
	}
	for i := range layout.Sections {
		section := &layout.Sections[i]
		name := section.Title
		if name == "" {
			name = fmt.Sprintf("section %d", i+1)
		}
		if utf8.RuneCountInString(section.Title) > 255 {
			return invalid(fmt.Sprintf("%s: the title is longer than 255 characters", name))
		}
		if utf8.RuneCountInString(section.Description) > 2000 {
			return invalid(fmt.Sprintf("%s: the description is longer than 2000 characters", name))
		}
		if section.Columns != 1 && section.Columns != 2 {
			return invalid(fmt.Sprintf("%s: columns is 1 or 2", name))
		}
		if err := checkVisibility(environment, name, section.Visibility); err != nil {
			return err
		}
		for _, item := range section.Items {
			items++
			if items > maxLayoutItems {
				return invalid(fmt.Sprintf("the layout holds more than %d blocks", maxLayoutItems))
			}
			switch {
			case item.Field != nil:
				if !keys[item.Field.FieldKey] {
					return invalid(fmt.Sprintf("%s: %q is not a field of this Record Type", name, item.Field.FieldKey))
				}
				if placed[item.Field.FieldKey] {
					return invalid(fmt.Sprintf("%s: %q is placed twice", name, item.Field.FieldKey))
				}
				placed[item.Field.FieldKey] = true
				if width := item.Field.Width; width != "" && width != "full" && width != "half" {
					return invalid(fmt.Sprintf("%s: %q must be full or half width", name, item.Field.FieldKey))
				}
				if utf8.RuneCountInString(item.Field.HelpText) > 2000 {
					return invalid(fmt.Sprintf("%s: the help text is longer than 2000 characters", name))
				}
			case item.Text != nil:
				if strings.TrimSpace(item.Text.Text) == "" {
					return invalid(fmt.Sprintf("%s: a text block needs its text", name))
				}
				if utf8.RuneCountInString(item.Text.Text) > maxLayoutText {
					return invalid(fmt.Sprintf("%s: a text block is longer than %d characters", name, maxLayoutText))
				}
			case item.Image != nil:
				if err := checkLayoutImage(ctx, conn, name, item.Image.Path); err != nil {
					return err
				}
			case item.Art != nil:
				if strings.TrimSpace(item.Art.Key) == "" {
					return invalid(fmt.Sprintf("%s: an art block needs its registry key", name))
				}
				if utf8.RuneCountInString(item.Art.Key) > maxArtKey {
					return invalid(fmt.Sprintf("%s: an art key is longer than %d characters", name, maxArtKey))
				}
			default:
				return invalid(fmt.Sprintf("%s: a block needs its content", name))
			}
		}
	}
	return nil
}

// checkVisibility type-checks one section visibility expression: CEL that
// returns true or false over the Record's fields. Empty always shows.
// Display-only like depends_on: today() is allowed, the server never
// enforces the result.
func checkVisibility(environment *exprEnvironment, name, source string) error {
	if strings.TrimSpace(source) == "" {
		return nil
	}
	if utf8.RuneCountInString(source) > maxExpressionLength {
		return invalid(fmt.Sprintf("%s: visibility is longer than %d characters", name, maxExpressionLength))
	}
	program, err := environment.env.Compile(source)
	if err != nil {
		return invalid(fmt.Sprintf("%s: visibility: %v", name, err))
	}
	if program.Result != expr.Bool {
		return invalid(fmt.Sprintf("%s: visibility must return true or false, not %s", name, program.Result))
	}
	return nil
}

// checkLayoutImage verifies a static image block names a recorded image
// file. Content was sniffed at upload, so the recorded extension proves the
// type; no bytes are read here.
func checkLayoutImage(ctx context.Context, conn sqlConn, name, path string) error {
	if !files.IsImagePath(path) {
		return invalid(fmt.Sprintf("%s: %q is not an image under images/", name, path))
	}
	var recorded bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM athena_files WHERE path = ?)`, path).Scan(&recorded); err != nil {
		return internalError("check image file", err)
	}
	if !recorded {
		return invalid(fmt.Sprintf("%s: %q is not an uploaded image", name, path))
	}
	return nil
}
