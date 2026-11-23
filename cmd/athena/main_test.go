package main

import (
	"strings"
	"testing"
)

// The CLI banner carries the ASCII tree above the product name. The mark
// stays within eight lines of plain ASCII (design §9.6).
func TestUsageBannerShowsMarkAndVersion(t *testing.T) {
	banner := usageBanner()
	if !strings.Contains(banner, "Athena ERP "+version) {
		t.Fatalf("banner %q names neither the product nor %s", banner, version)
	}
	lines := strings.Split(asciiMark, "\n")
	if len(lines) > 8 {
		t.Fatalf("mark has %d lines, at most 8 allowed", len(lines))
	}
	for _, line := range lines {
		for _, r := range line {
			if r < 0x20 || r > 0x7e {
				t.Fatalf("mark line %q is not plain ASCII", line)
			}
		}
		if strings.HasSuffix(line, " ") {
			t.Fatalf("mark line %q has trailing space", line)
		}
	}
}

// The version output keeps the banner above the version lines; the last
// line stays the bare version F8 owns.
func TestVersionPrintsBannerAboveVersion(t *testing.T) {
	lines := strings.Split(versionBanner(), "\n")
	if got := lines[len(lines)-1]; got != version {
		t.Fatalf("version last line = %q, want %q", got, version)
	}
	if got := strings.Join(lines[:len(lines)-1], "\n"); got != asciiMark {
		t.Fatalf("version banner differs from the usage mark:\n%s", got)
	}
}
