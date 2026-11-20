package ledger_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"athena-erp/internal/ledger"
)

func mustSetting(t *testing.T, s *ledger.Service, kind, rule string) {
	t.Helper()
	_, err := s.SaveSetting(context.Background(), ledger.Setting{OwnerPack: "core", Type: "grace_days", Label: "Grace", Kind: kind, ListRule: rule}, 0)
	if err != nil {
		t.Fatal(err)
	}
}

func future(days int) time.Time { return time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour) }

func TestSettingLifecycle(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	if _, err := s.SaveSetting(ctx, ledger.Setting{OwnerPack: "core", Type: "x", Label: "X", Kind: "scalar", ListRule: "append"}, 0); err == nil {
		t.Fatal("scalar with list rule accepted")
	}
	if _, err := s.SaveSetting(ctx, ledger.Setting{OwnerPack: "core", Type: "x", Label: "X", Kind: "list"}, 0); err == nil {
		t.Fatal("list without rule accepted")
	}
	mustSetting(t, s, ledger.KindScalar, "")
	first, err := s.SaveSettingValue(ctx, ledger.SettingValue{OwnerPack: "core", Type: "grace_days", EffectiveFrom: future(1), Value: "5"})
	if err != nil || first.Status != ledger.StatusDraft || first.Revision != 1 {
		t.Fatalf("draft: %+v %v", first, err)
	}
	past := first
	past.EffectiveFrom = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s.SaveSettingValue(ctx, past); err == nil || !strings.Contains(err.Error(), "prospectively") {
		t.Fatalf("backdated: %v", err)
	}
	published, err := s.PublishSettingValue(ctx, "core", "grace_days", "", first.EffectiveFrom, 1, "ana")
	if err != nil || published.Status != ledger.StatusPublished {
		t.Fatalf("publish: %+v %v", published, err)
	}
	if err := s.DiscardSettingValue(ctx, "core", "grace_days", "", first.EffectiveFrom, 1); err == nil {
		t.Fatal("published generation discarded")
	}
	second, err := s.SaveSettingValue(ctx, ledger.SettingValue{OwnerPack: "core", Type: "grace_days", EffectiveFrom: future(2), Value: "7"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DiscardSettingValue(ctx, "core", "grace_days", "", second.EffectiveFrom, second.Revision); err != nil {
		t.Fatalf("discard draft: %v", err)
	}
}

func TestSettingResolve(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustSetting(t, s, ledger.KindScalar, "")
	publish := func(chain string, days int, value string) {
		t.Helper()
		draft, err := s.SaveSettingValue(ctx, ledger.SettingValue{OwnerPack: "core", Type: "grace_days", ScopeChain: chain, EffectiveFrom: future(days), Value: value})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.PublishSettingValue(ctx, "core", "grace_days", draft.ScopeChain, draft.EffectiveFrom, draft.Revision, "ana"); err != nil {
			t.Fatal(err)
		}
	}
	publish("", 1, "5")
	publish("company:acme", 1, "7")
	publish("company:acme", 10, "9")
	resolved, err := s.ResolveSetting(ctx, "core", "grace_days", map[string]string{"company": "acme"}, future(2))
	if err != nil || resolved.Value != "7" {
		t.Fatalf("specific wins: %+v %v", resolved, err)
	}
	resolved, err = s.ResolveSetting(ctx, "core", "grace_days", map[string]string{"company": "acme"}, future(11))
	if err != nil || resolved.Value != "9" {
		t.Fatalf("later instant: %+v %v", resolved, err)
	}
	resolved, err = s.ResolveSetting(ctx, "core", "grace_days", map[string]string{"company": "globex"}, future(2))
	if err != nil || resolved.Value != "5" {
		t.Fatalf("global fallback: %+v %v", resolved, err)
	}
	if _, err := s.ResolveSetting(ctx, "core", "grace_days", map[string]string{}, time.Now().UTC()); !errors.Is(err, ledger.ErrNoGeneration) {
		t.Fatalf("before any generation: %v", err)
	}
	// A second one-segment chain ties company:acme.
	publish("branch:north", 1, "3")
	_, err = s.ResolveSetting(ctx, "core", "grace_days", map[string]string{"company": "acme", "branch": "north"}, future(2))
	if !errors.Is(err, ledger.ErrAmbiguous) {
		t.Fatalf("tie: %v", err)
	}
}

func TestSettingListMergeAndPreview(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustSetting(t, s, ledger.KindList, ledger.RuleAppend)
	if _, err := s.SaveSettingValue(ctx, ledger.SettingValue{OwnerPack: "core", Type: "grace_days", EffectiveFrom: future(1), Value: "nope"}); err == nil {
		t.Fatal("scalar value for a list accepted")
	}
	publish := func(chain string, days int, value []any) ledger.SettingValue {
		t.Helper()
		draft, err := s.SaveSettingValue(ctx, ledger.SettingValue{OwnerPack: "core", Type: "grace_days", ScopeChain: chain, EffectiveFrom: future(days), Value: value})
		if err != nil {
			t.Fatal(err)
		}
		published, err := s.PublishSettingValue(ctx, "core", "grace_days", draft.ScopeChain, draft.EffectiveFrom, draft.Revision, "ana")
		if err != nil {
			t.Fatal(err)
		}
		return published
	}
	publish("", 1, []any{"a", "b"})
	publish("company:acme", 1, []any{"b", "c"})
	resolved, err := s.ResolveSetting(ctx, "core", "grace_days", map[string]string{"company": "acme"}, future(2))
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := resolved.Value.([]any); len(list) != 3 {
		t.Fatalf("append merge: %v", resolved.Value)
	}
	changes, err := s.DiffPreview(ctx, "core", "grace_days", map[string]string{"company": "acme"}, ledger.SettingValue{
		OwnerPack: "core", Type: "grace_days", ScopeChain: "company:acme", EffectiveFrom: future(5), Value: []any{"z"},
	})
	if err != nil || len(changes) != 1 {
		t.Fatalf("preview: %+v %v", changes, err)
	}
	before, _ := changes[0].Before.([]any)
	after, _ := changes[0].After.([]any)
	if len(before) != 3 || len(after) != 3 || after[2] != "z" {
		t.Fatalf("preview diff: %v -> %v", before, after)
	}
}
