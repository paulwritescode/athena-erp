package server

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
)

// fieldRuleCase is one row of testdata/field_rules.json. The web tests read the
// same file, so the server and web/src/fieldRules.ts accept and reject the same
// values. A value of the form {repeat:X:N} stands for X repeated N times.
// Image rows carry an image object instead of a text value; a missing image
// sends null, which only an optional Image field accepts.
type fieldRuleCase struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	Required         bool   `json:"required"`
	TargetRecordType string `json:"target_record_type"`
	Value            string `json:"value"`
	Valid            bool   `json:"valid"`
	Image            *struct {
		Path    string `json:"path"`
		Hash    string `json:"hash"`
		AltText string `json:"alt_text"`
	} `json:"image"`
	// Attributes of the field under test; web/src/fieldRules.ts reads the same keys.
	MaxLength int      `json:"max_length"`
	Min       string   `json:"min"`
	Max       string   `json:"max"`
	Pattern   string   `json:"pattern"`
	Values    []string `json:"values"`
	Currency  string   `json:"currency"`
	Scale     *int     `json:"scale"`
	UoM       string   `json:"uom"`
}

func expandRepeat(t *testing.T, name, value string) string {
	t.Helper()
	if rest, ok := strings.CutPrefix(value, "{repeat:"); ok {
		unit, count, _ := strings.Cut(strings.TrimSuffix(rest, "}"), ":")
		n, err := strconv.Atoi(count)
		if err != nil {
			t.Fatalf("bad repeat in %q: %v", name, err)
		}
		return strings.Repeat(unit, n)
	}
	return value
}

func (c fieldRuleCase) text(t *testing.T) string {
	t.Helper()
	return expandRepeat(t, c.Name, c.Value)
}

func TestFieldRulesSharedTable(t *testing.T) {
	payload, err := os.ReadFile("testdata/field_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []fieldRuleCase
	if err := json.Unmarshal(payload, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			field := recordTypeField{Name: "f", Label: "Field", Type: c.Type, Required: c.Required, TargetRecordType: c.TargetRecordType,
				MaxLength: c.MaxLength, Min: c.Min, Max: c.Max, Pattern: c.Pattern, Currency: c.Currency, Scale: c.Scale, UoM: c.UoM}
			for _, v := range c.Values {
				field.Values = append(field.Values, enumValue{Value: v, Label: v})
			}
			recordType := recordTypeDefinition{Name: "r", Fields: []recordTypeField{field}}
			sent := &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: c.text(t)}}
			if c.Type == "Image" {
				sent = &athenav1.FieldValue{Kind: &athenav1.FieldValue_NullValue{NullValue: true}}
				if c.Image != nil {
					sent = &athenav1.FieldValue{Kind: &athenav1.FieldValue_ImageValue{ImageValue: &athenav1.ImageValue{
						Path: c.Image.Path, Hash: c.Image.Hash, AltText: expandRepeat(t, c.Name, c.Image.AltText),
					}}}
				}
			}
			values := map[string]*athenav1.FieldValue{"f": sent}
			_, _, err := bindValues(recordType, values, true)
			if (err == nil) != c.Valid {
				t.Fatalf("value valid = %v, want %v (error: %v)", err == nil, c.Valid, err)
			}
		})
	}
}
