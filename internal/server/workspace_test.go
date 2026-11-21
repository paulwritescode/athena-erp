package server

import (
	"context"
	"errors"
	"testing"
	"time"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/ledger"
	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"
	"google.golang.org/protobuf/proto"
)

func TestWorkspaceAppearancePublicReadAndProtectedWrite(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	b := f.browser(t)
	read := func() *v1.GetAppearanceResponse {
		t.Helper()
		r, err := b.workspace.GetAppearance(ctx, &v1.GetAppearanceRequest{})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := read(); r.Appearance.Preset != "ink" {
		t.Fatal(r)
	}
	red := &v1.SetAppearanceRequest{Appearance: &v1.Appearance{Preset: "red"}}
	_, err := b.workspace.SetAppearance(ctx, red)
	requireCode(t, err, connect.CodeUnauthenticated)
	f.role(t, "Books", []string{auth.PermissionLedgerWrite})
	f.user(t, "builder", "Books")
	b.login(t, "builder")
	_, err = b.workspace.SetAppearance(ctx, red)
	requireCode(t, err, connect.CodePermissionDenied)
	f.role(t, "Designer", []string{auth.PermissionAppearance})
	f.assign(t, "builder", "Designer")
	if _, err := b.workspace.SetAppearance(ctx, red); err != nil {
		t.Fatal(err)
	}
	bad := &v1.SetAppearanceRequest{Appearance: &v1.Appearance{Preset: "custom", Accent: "oklch(.9 .02 70)", ChartColors: []string{"", "oklch(.6 .1 50)"}}}
	_, err = b.workspace.SetAppearance(ctx, bad)
	requireCode(t, err, connect.CodeInvalidArgument)
	var failure *connect.Error
	if !errors.As(err, &failure) || len(failure.Details()) != 1 {
		t.Fatalf("missing contrast detail: %v", err)
	}
	message, err := connectproto.UnmarshalErrorDetail(failure.Details()[0])
	if err != nil {
		t.Fatal(err)
	}
	detail, ok := message.(*v1.AppearanceContrastFailure)
	if !ok || detail.SuggestedAccent == "" {
		t.Fatalf("unexpected detail: %v", message)
	}
	if read().Appearance.Preset != "red" {
		t.Fatal("failed save changed the preset")
	}
	bad.Appearance.Accent = detail.SuggestedAccent
	saved, err := b.workspace.SetAppearance(ctx, bad)
	if err != nil {
		t.Fatal(err)
	}
	anon := f.browser(t)
	got, err := anon.workspace.GetAppearance(ctx, &v1.GetAppearanceRequest{})
	if err != nil || !proto.Equal(got.GetAppearance(), bad.Appearance) || saved.DarkAccent != got.DarkAccent || got.DarkAccent != detail.DarkAccent || got.LightAccent == "" {
		t.Fatalf("round trip: %v %v", got, err)
	}
	g5 := ledger.Service{DB: f.db, Dialect: dialect}
	if _, err := g5.ResolveSetting(ctx, "athena", "brand.accent", nil, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := g5.SaveSettingValue(ctx, ledger.SettingValue{OwnerPack: "ATHENA", Type: "BRAND.ACCENT", EffectiveFrom: time.Now(), Value: "red"}); err == nil {
		t.Fatal("generic settings writer modified the reserved setting")
	}
}
