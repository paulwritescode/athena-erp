package posting

import "testing"

// TestCanonicalDecimalText pins the hash normalization (review follow-up 4):
// equal values read alike whatever scale they carry.
func TestCanonicalDecimalText(t *testing.T) {
	for text, want := range map[string]string{
		"100":       "100",
		"100.00":    "100",
		"100.0":     "100",
		"25.50":     "25.5",
		"25.500000": "25.5",
		"0":         "0",
		"0.00":      "0",
		"0.000000":  "0",
		"-1.50":     "-1.5",
		"1.234":     "1.234",
		"1000000":   "1000000",
	} {
		if got := canonicalDecimalText(text); got != want {
			t.Fatalf("canonicalDecimalText(%q) = %q, want %q", text, got, want)
		}
	}
}
