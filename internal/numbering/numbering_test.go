package numbering

import (
	"testing"
	"time"
)

func TestPatternFormatAndScope(t *testing.T) {
	p, err := Parse("SINV-{yyyy}-{mm}-{#####}")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)
	if got := p.Format(at, 42); got != "SINV-2026-03-00042" {
		t.Fatalf("Format = %q", got)
	}
	if got := p.Format(at, 1234567); got != "SINV-2026-03-1234567" {
		t.Fatalf("the counter must grow past its width, got %q", got)
	}
	if a, b := p.Scope(at), p.Scope(at.AddDate(0, 1, 0)); a == b {
		t.Fatalf("a new month must open a new counter: %q", a)
	}
	y, _ := Parse("INV{yy}/{###}")
	if got := y.Format(at, 7); got != "INV26/007" {
		t.Fatalf("Format = %q", got)
	}
}

func TestParseRejectsBadPatterns(t *testing.T) {
	for _, pattern := range []string{"", "NOCOUNTER", "A-{#}-{##}", "A-{foo}-{#}", "A-{#", "A}-{#}", "A-{##########}", "A-{}"} {
		if _, err := Parse(pattern); err == nil {
			t.Errorf("Parse(%q) was accepted", pattern)
		}
	}
}
