package ledger_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"athena-erp/internal/ledger"

	"pgregory.net/rapid"
)

// TestOverlapsProperty checks Property 6 over drawn ranges: the overlap
// predicate is symmetric and reflexive, adjacent ranges never overlap, ranges
// sharing an endpoint always do, and the predicate agrees with day-by-day set
// overlap.
//
// Feature: athena-batch-2, Property 6: Period non-overlap.
func TestOverlapsProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		a, b := rapid.IntRange(0, 60).Draw(t, "a"), rapid.IntRange(0, 60).Draw(t, "b")
		c, d := rapid.IntRange(0, 60).Draw(t, "c"), rapid.IntRange(0, 60).Draw(t, "d")
		start1, end1 := min(a, b), max(a, b)
		start2, end2 := min(c, d), max(c, d)
		s1, e1 := day(start1), day(end1)
		s2, e2 := day(start2), day(end2)
		got := ledger.Overlaps(s1, e1, s2, e2)
		if ledger.Overlaps(s2, e2, s1, e1) != got {
			t.Fatal("overlap is not symmetric")
		}
		if !ledger.Overlaps(s1, e1, s1, e1) {
			t.Fatal("overlap is not reflexive")
		}
		want := false
		for day := 0; day <= 60; day++ {
			if day >= start1 && day <= end1 && day >= start2 && day <= end2 {
				want = true
			}
		}
		if got != want {
			t.Fatalf("overlap(%s..%s, %s..%s) = %v, want %v", s1, e1, s2, e2, got, want)
		}
	})
}

// TestPeriodSaveEnforcesNonOverlap checks the enforcement half of Property 6:
// a save succeeds exactly when its dates overlap nothing already saved in its
// calendar.
func TestPeriodSaveEnforcesNonOverlap(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	calendars := 0
	rapid.Check(t, func(t *rapid.T) {
		calendars++
		company := fmt.Sprintf("prop%d", calendars)
		if _, err := s.SaveCompany(ctx, ledger.Company{Name: company, Label: company}, 0); err != nil {
			t.Fatal(err)
		}
		book := []string{"main", "tax"}[rapid.IntRange(0, 1).Draw(t, "book")]
		type span struct{ start, end string }
		var saved []span
		for i := 0; i < rapid.IntRange(0, 3).Draw(t, "existing"); i++ {
			start, length := rapid.IntRange(0, 60).Draw(t, "start"), rapid.IntRange(0, 30).Draw(t, "length")
			period := ledger.Period{Company: company, Book: book, Name: fmt.Sprintf("p%d", i), Label: "P", StartDate: day(start), EndDate: day(start + length)}
			_, err := s.SavePeriod(ctx, period, 0)
			overlaps := false
			for _, other := range saved {
				if ledger.Overlaps(period.StartDate, period.EndDate, other.start, other.end) {
					overlaps = true
				}
			}
			if overlaps != (err != nil) {
				t.Fatalf("seed %s..%s: err = %v", period.StartDate, period.EndDate, err)
			}
			if err == nil {
				saved = append(saved, span{period.StartDate, period.EndDate})
			} else if !strings.Contains(err.Error(), "overlaps") {
				t.Fatalf("seed error = %v", err)
			}
		}
		start, length := rapid.IntRange(0, 60).Draw(t, "newStart"), rapid.IntRange(0, 30).Draw(t, "newLength")
		period := ledger.Period{Company: company, Book: book, Name: "new", Label: "N", StartDate: day(start), EndDate: day(start + length)}
		_, err := s.SavePeriod(ctx, period, 0)
		overlaps := false
		for _, other := range saved {
			if ledger.Overlaps(period.StartDate, period.EndDate, other.start, other.end) {
				overlaps = true
			}
		}
		if overlaps != (err != nil) {
			t.Fatalf("save %s..%s over %d saved: err = %v", period.StartDate, period.EndDate, len(saved), err)
		}
	})
}

func day(offset int) string {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, offset).Format("2006-01-02")
}
