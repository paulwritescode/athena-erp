package ledger_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"athena-erp/internal/ledger"

	"pgregory.net/rapid"
)

func mustAccount(t *testing.T, s *ledger.Service, name, parent string, group bool) {
	t.Helper()
	if _, err := s.SaveAccountType(context.Background(), ledger.AccountType{Name: "asset", Label: "Asset", NormalBalance: "debit"}); err != nil && !errors.Is(err, ledger.ErrExists) {
		t.Fatal(err)
	}
	_, err := s.SaveAccount(context.Background(), ledger.Account{Name: name, Label: name, Type: "asset", Parent: parent, IsGroup: group}, 0)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAccountTreeRules(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustCompany(t, s, "acme")
	if _, err := s.SaveAccountType(ctx, ledger.AccountType{Name: "asset", Label: "Asset", NormalBalance: "sideways"}); err == nil {
		t.Fatal("bad normal balance accepted")
	}
	mustAccount(t, s, "assets", "", true)
	mustAccount(t, s, "cash", "assets", false)
	if _, err := s.SaveAccount(ctx, ledger.Account{Name: "deep", Label: "d", Type: "asset", Parent: "cash"}, 0); err == nil {
		t.Fatal("leaf parent accepted")
	}
	loop, err := s.SaveAccount(ctx, ledger.Account{Name: "loop", Label: "l", Type: "asset", Parent: "assets", IsGroup: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	assets, err := s.GetAccount(ctx, "assets")
	if err != nil {
		t.Fatal(err)
	}
	assets.Parent = loop.Name
	if _, err := s.SaveAccount(ctx, assets, 1); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle: %v", err)
	}
	_ = loop
	accounts, err := s.ListAccounts(ctx, "acme")
	if err != nil || len(accounts) != 3 {
		t.Fatalf("accounts: %v %v", accounts, err)
	}
}

func TestDeterminationResolve(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustAccount(t, s, "assets", "", true)
	mustAccount(t, s, "cash", "assets", false)
	mustAccount(t, s, "bank", "assets", false)
	if _, err := s.SaveDetermination(ctx, ledger.Determination{Label: "group target", Account: "assets"}, 0); err == nil {
		t.Fatal("group target accepted")
	}
	def, err := s.SaveDetermination(ctx, ledger.Determination{Label: "default cash", Account: "cash"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveDetermination(ctx, ledger.Determination{Label: "acme bank", Account: "bank", Match: map[string]string{"company": "acme"}}, 0); err != nil {
		t.Fatal(err)
	}
	got, err := s.ResolveAccount(ctx, map[string]string{"company": "acme"}, "2026-06-15")
	if err != nil || got.Account != "bank" {
		t.Fatalf("specific wins: %+v %v", got, err)
	}
	got, err = s.ResolveAccount(ctx, map[string]string{"company": "globex"}, "2026-06-15")
	if err != nil || got.Account != "cash" {
		t.Fatalf("catch-all: %+v %v", got, err)
	}
	// A second catch-all ties the first and names both.
	if _, err := s.SaveDetermination(ctx, ledger.Determination{Label: "second cash", Account: "cash"}, 0); err != nil {
		t.Fatal(err)
	}
	_, err = s.ResolveAccount(ctx, map[string]string{"company": "globex"}, "2026-06-15")
	if !errors.Is(err, ledger.ErrAmbiguous) || !strings.Contains(err.Error(), "default cash") || !strings.Contains(err.Error(), "second cash") {
		t.Fatalf("tie: %v", err)
	}
	// Disabling one breaks the tie.
	def.Disabled = true
	if _, err := s.SaveDetermination(ctx, def, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveAccount(ctx, map[string]string{"company": "globex"}, "2026-06-15"); err != nil {
		t.Fatalf("after disable: %v", err)
	}
}

func TestDeterminationEffectiveRanges(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustAccount(t, s, "assets", "", true)
	mustAccount(t, s, "cash", "assets", false)
	mustAccount(t, s, "bank", "assets", false)
	for _, tc := range []struct {
		name     string
		from, to string
	}{
		{"bad from", "2026-13-01", ""},
		{"bad to", "", "not-a-date"},
		{"impossible day", "2026-02-30", ""},
		{"empty range", "2026-07-01", "2026-07-01"},
		{"reversed range", "2026-08-01", "2026-07-01"},
	} {
		if _, err := s.SaveDetermination(ctx, ledger.Determination{Label: tc.name, Account: "cash", EffectiveFrom: tc.from, EffectiveTo: tc.to}, 0); err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
	}
	if _, err := s.SaveDetermination(ctx, ledger.Determination{Label: "fallback", Account: "cash"}, 0); err != nil {
		t.Fatal(err)
	}
	dated, err := s.SaveDetermination(ctx, ledger.Determination{
		Label: "dated bank", Account: "bank", Match: map[string]string{"company": "acme"},
		EffectiveFrom: "2026-07-01", EffectiveTo: "2026-08-01",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetDetermination(ctx, dated.ID)
	if err != nil || got.EffectiveFrom != "2026-07-01" || got.EffectiveTo != "2026-08-01" {
		t.Fatalf("get range: %+v %v", got, err)
	}
	acme := map[string]string{"company": "acme"}
	globex := map[string]string{"company": "globex"}
	// Before the range only the fallback holds.
	resolved, err := s.ResolveAccount(ctx, acme, "2026-06-15")
	if err != nil || resolved.Account != "cash" {
		t.Fatalf("before range: %+v %v", resolved, err)
	}
	// From is inclusive: on July 1 the dated mapping wins and reports its revision.
	resolved, err = s.ResolveAccount(ctx, acme, "2026-07-01")
	if err != nil || resolved.Account != "bank" || resolved.Revision != 1 {
		t.Fatalf("range start: %+v %v", resolved, err)
	}
	// To is exclusive: on August 1 the dated mapping has expired.
	resolved, err = s.ResolveAccount(ctx, acme, "2026-08-01")
	if err != nil || resolved.Account != "cash" {
		t.Fatalf("range end: %+v %v", resolved, err)
	}
	// Outside the company the dated mapping never matches, even mid-range.
	resolved, err = s.ResolveAccount(ctx, globex, "2026-07-15")
	if err != nil || resolved.Account != "cash" {
		t.Fatalf("other company: %+v %v", resolved, err)
	}
	// Open-ended ranges hold forever after their start.
	open, err := s.SaveDetermination(ctx, ledger.Determination{
		Label: "open bank", Account: "bank", Match: map[string]string{"company": "globex", "book": "tax"},
		EffectiveFrom: "2026-07-01",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = open
	resolved, err = s.ResolveAccount(ctx, map[string]string{"company": "globex", "book": "tax"}, "2030-01-01")
	if err != nil || resolved.Account != "bank" {
		t.Fatalf("open range: %+v %v", resolved, err)
	}
	if _, err := s.ResolveAccount(ctx, acme, "2026-07-15x"); err == nil {
		t.Fatal("bad as-of accepted")
	}
	if _, err := s.ResolveAccount(ctx, acme, ""); err == nil {
		t.Fatal("empty as-of accepted")
	}
}

// TestDeterminationProperty checks the determination half of Property 7 over
// drawn mappings and contexts: the unique most-specific match wins, ties fail
// naming every tied mapping, and nothing matching fails distinctly.
//
// Feature: athena-batch-2, Property 7: Determination specificity and Setting
// resolution.
func TestDeterminationProperty(t *testing.T) {
	dims := []string{"c", "b"}
	values := []string{"x", "y"}
	rapid.Check(t, func(t *rapid.T) {
		var mappings []ledger.Determination
		for i := 0; i < rapid.IntRange(1, 5).Draw(t, "mappings"); i++ {
			match := map[string]string{}
			for j := 0; j < rapid.IntRange(0, 2).Draw(t, "pairs"); j++ {
				match[dims[rapid.IntRange(0, 1).Draw(t, "dim")]] = values[rapid.IntRange(0, 1).Draw(t, "value")]
			}
			mappings = append(mappings, ledger.Determination{
				Label: fmt.Sprintf("m%d", i), Account: []string{"a", "b"}[i%2],
				Match:    match,
				Disabled: rapid.IntRange(0, 4).Draw(t, "disabled") == 0,
			})
		}
		context_ := map[string]string{
			"c": values[rapid.IntRange(0, 1).Draw(t, "c")],
			"b": values[rapid.IntRange(0, 1).Draw(t, "b")],
		}
		// Independent oracle: subset match, then most pairs.
		var matched []ledger.Determination
		for _, mapping := range mappings {
			if mapping.Disabled {
				continue
			}
			ok := true
			for key, value := range mapping.Match {
				if context_[key] != value {
					ok = false
				}
			}
			if ok {
				matched = append(matched, mapping)
			}
		}
		best := -1
		for _, mapping := range matched {
			best = max(best, len(mapping.Match))
		}
		var winners []ledger.Determination
		for _, mapping := range matched {
			if len(mapping.Match) == best {
				winners = append(winners, mapping)
			}
		}
		got, err := ledger.MatchDeterminations(mappings, context_, "2026-06-15")
		switch {
		case len(winners) == 0:
			if !errors.Is(err, ledger.ErrNoMatch) {
				t.Fatalf("no match: %+v %v", got, err)
			}
		case len(winners) == 1:
			if err != nil || got.Account != winners[0].Account {
				t.Fatalf("unique winner: %+v %v", got, err)
			}
		default:
			if !errors.Is(err, ledger.ErrAmbiguous) {
				t.Fatalf("tie: %+v %v", got, err)
			}
			for _, winner := range winners {
				if !strings.Contains(err.Error(), winner.Label) {
					t.Fatalf("tie names %v, want %s", err, winner.Label)
				}
			}
		}
	})
}

// TestSettingResolutionProperty checks the settings half of Property 7 over
// drawn generation sets: resolution is deterministic, scalar and replace
// results equal one winner, append results contain every input item,
// intersect results stay within every input, and only generations at or
// before the instant take part.
func TestSettingResolutionProperty(t *testing.T) {
	chains := []string{"", "c:x", "c:y", "b:x", "b:y", "c:x/b:y"}
	items := []string{"a", "b", "c"}
	rapid.Check(t, func(t *rapid.T) {
		kind := ledger.KindScalar
		rule := ""
		if rapid.IntRange(0, 1).Draw(t, "list") == 1 {
			kind = ledger.KindList
			rule = []string{ledger.RuleReplace, ledger.RuleAppend, ledger.RuleIntersect}[rapid.IntRange(0, 2).Draw(t, "rule")]
		}
		var generations []ledger.SettingValue
		for i := 0; i < rapid.IntRange(1, 4).Draw(t, "generations"); i++ {
			var value any = fmt.Sprintf("v%d", rapid.IntRange(0, 2).Draw(t, "scalar"))
			if kind == ledger.KindList {
				var list []any
				for _, item := range items {
					if rapid.IntRange(0, 1).Draw(t, "member") == 1 {
						list = append(list, item)
					}
				}
				value = list
			}
			generations = append(generations, ledger.SettingValue{
				ScopeChain:    chains[rapid.IntRange(0, len(chains)-1).Draw(t, "chain")],
				EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, rapid.IntRange(0, 10).Draw(t, "instant")),
				Revision:      int64(rapid.IntRange(1, 2).Draw(t, "revision")),
				Value:         value,
				Status:        []string{ledger.StatusPublished, ledger.StatusDraft}[rapid.IntRange(0, 1).Draw(t, "status")],
			})
		}
		scope := map[string]string{
			"c": []string{"x", "y"}[rapid.IntRange(0, 1).Draw(t, "c")],
			"b": []string{"x", "y"}[rapid.IntRange(0, 1).Draw(t, "b")],
		}
		at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, rapid.IntRange(0, 12).Draw(t, "at"))
		first, firstErr := ledger.ResolveGenerations(kind, rule, generations, scope, at)
		second, secondErr := ledger.ResolveGenerations(kind, rule, generations, scope, at)
		if fmt.Sprintf("%v|%v", first.Value, firstErr) != fmt.Sprintf("%v|%v", second.Value, secondErr) {
			t.Fatal("resolution is not deterministic")
		}
		if firstErr != nil {
			if !errors.Is(firstErr, ledger.ErrNoGeneration) && !errors.Is(firstErr, ledger.ErrAmbiguous) {
				t.Fatalf("error = %v", firstErr)
			}
			return
		}
		for _, winner := range first.Winners {
			if winner.Status != ledger.StatusPublished {
				t.Fatal("a draft took part")
			}
			if winner.EffectiveFrom.After(at) {
				t.Fatal("a future generation took part")
			}
		}
		encode := func(value any) string {
			raw, _ := json.Marshal(value)
			return string(raw)
		}
		switch {
		case kind == ledger.KindScalar || rule == ledger.RuleReplace:
			found := false
			for _, winner := range first.Winners {
				if encode(winner.Value) == encode(first.Value) {
					found = true
				}
			}
			if !found || len(first.Winners) != 1 {
				t.Fatalf("result %v is not one winner %+v", first.Value, first.Winners)
			}
		case rule == ledger.RuleAppend:
			for _, winner := range first.Winners {
				for _, item := range winner.Value.([]any) {
					if !strings.Contains(encode(first.Value), encode(item)) {
						t.Fatalf("append result %v lost %v", first.Value, item)
					}
				}
			}
		case rule == ledger.RuleIntersect:
			result, _ := first.Value.([]any)
			for _, item := range result {
				for _, winner := range first.Winners {
					found := false
					for _, candidate := range winner.Value.([]any) {
						found = found || encode(candidate) == encode(item)
					}
					if !found {
						t.Fatalf("intersect result %v has %v outside a winner", result, item)
					}
				}
			}
		}
	})
}
