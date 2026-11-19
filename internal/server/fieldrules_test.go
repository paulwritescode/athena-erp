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
type fieldRuleCase struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	Required         bool   `json:"required"`
	TargetRecordType string `json:"target_record_type"`
	Value            string `json:"value"`
	Valid            bool   `json:"valid"`
}

func (c fieldRuleCase) text(t *testing.T) string {
	t.Helper()
	if rest, ok := strings.CutPrefix(c.Value, "{repeat:"); ok {
		unit, count, _ := strings.Cut(strings.TrimSuffix(rest, "}"), ":")
		n, err := strconv.Atoi(count)
		if err != nil {
			t.Fatalf("bad repeat in %q: %v", c.Name, err)
		}
		return strings.Repeat(unit, n)
	}
	return c.Value
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
			field := recordTypeField{Name: "f", Label: "Field", Type: c.Type, Required: c.Required, TargetRecordType: c.TargetRecordType}
			recordType := recordTypeDefinition{Name: "r", Fields: []recordTypeField{field}}
			values := map[string]*athenav1.FieldValue{"f": {Kind: &athenav1.FieldValue_StringValue{StringValue: c.text(t)}}}
			_, _, err := bindValues(recordType, values, true)
			if (err == nil) != c.Valid {
				t.Fatalf("value valid = %v, want %v (error: %v)", err == nil, c.Valid, err)
			}
		})
	}
}
