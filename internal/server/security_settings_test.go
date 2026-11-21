package server

import (
	"context"
	"strings"
	"testing"
	"time"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"connectrpc.com/connect/v2"
)

func securitySettingsByKey(t *testing.T, settings []*v1.SecuritySetting) map[string]*v1.SecuritySetting {
	t.Helper()
	out := map[string]*v1.SecuritySetting{}
	for _, s := range settings {
		out[s.GetKey()] = s
	}
	if len(out) != 8 {
		t.Fatalf("settings count = %d, want 8", len(out))
	}
	return out
}

func TestSecuritySettingsGetPermissions(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	b := f.browser(t)
	_, err := b.auth.GetSecuritySettings(ctx, &v1.GetSecuritySettingsRequest{})
	requireCode(t, err, connect.CodeUnauthenticated)

	f.role(t, "Viewer", nil)
	f.user(t, "plain", "Viewer")
	b.login(t, "plain")
	_, err = b.auth.GetSecuritySettings(ctx, &v1.GetSecuritySettingsRequest{})
	requireCode(t, err, connect.CodePermissionDenied)

	f.role(t, "SessionViewer", []string{auth.PermissionAccessSessions})
	f.user(t, "viewer", "SessionViewer")
	b.login(t, "viewer")
	res, err := b.auth.GetSecuritySettings(ctx, &v1.GetSecuritySettingsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	byKey := securitySettingsByKey(t, res.GetSettings())
	idle := byKey[auth.IdleMinutes]
	if idle.GetValue() != 30 || idle.GetDefaultValue() != 30 || idle.GetMinValue() != 5 ||
		idle.GetMaxValue() != 480 || idle.GetUnit() != "minutes" || idle.GetDescription() == "" {
		t.Fatalf("idle setting: %+v", idle)
	}

	f.role(t, "SecurityAdmin", []string{auth.PermissionSecurity})
	f.user(t, "admin", "SecurityAdmin")
	b.login(t, "admin")
	if _, err := b.auth.GetSecuritySettings(ctx, &v1.GetSecuritySettingsRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestSecuritySettingsSetBoundsAndAtomicity(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	f.role(t, "SecurityAdmin", []string{auth.PermissionSecurity})
	f.user(t, "admin", "SecurityAdmin")
	b := f.browser(t)
	b.login(t, "admin")
	set := func(settings map[string]int32) error {
		_, err := b.auth.SetSecuritySettings(ctx, &v1.SetSecuritySettingsRequest{Settings: settings})
		return err
	}
	for name, settings := range map[string]map[string]int32{
		"unknown key": {auth.IdleMinutes: 45, "security.nope": 1},
		"below min":   {auth.IdleMinutes: 4},
		"above max":   {auth.IdleMinutes: 481},
	} {
		if err := set(settings); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("%s: code = %v, want InvalidArgument", name, connect.CodeOf(err))
		}
	}
	if err := set(map[string]int32{}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty: code = %v, want InvalidArgument", connect.CodeOf(err))
	}
	// The rejected batch changed nothing: the valid key kept its default.
	got, err := b.auth.GetSecuritySettings(ctx, &v1.GetSecuritySettingsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if v := securitySettingsByKey(t, got.GetSettings())[auth.IdleMinutes].GetValue(); v != 30 {
		t.Fatalf("idle = %d after rejected batches, want 30", v)
	}
	res, err := b.auth.SetSecuritySettings(ctx, &v1.SetSecuritySettingsRequest{Settings: map[string]int32{
		auth.IdleMinutes: 45, auth.SessionWrites: 30,
	}})
	if err != nil {
		t.Fatal(err)
	}
	byKey := securitySettingsByKey(t, res.GetSettings())
	if byKey[auth.IdleMinutes].GetValue() != 45 || byKey[auth.SessionWrites].GetValue() != 30 {
		t.Fatalf("partial update not applied: %+v", res.GetSettings())
	}
	rows := f.accessEvents(t, auth.EventSecuritySettingsChanged)
	if len(rows) != 1 || rows[0].Username != "admin" {
		t.Fatalf("audit rows: %+v", rows)
	}
	if detail := rows[0].Detail; !strings.Contains(detail, auth.IdleMinutes+": 30 -> 45") ||
		!strings.Contains(detail, auth.SessionWrites+": 60 -> 30") {
		t.Fatalf("audit detail: %q", detail)
	}
}

func TestSecuritySettingsSetPermissionAndStepUp(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	f.role(t, "Builder", []string{auth.PermissionBuild})
	f.user(t, "builder", "Builder")
	b := f.browser(t)
	b.login(t, "builder")
	_, err := b.auth.SetSecuritySettings(ctx, &v1.SetSecuritySettingsRequest{Settings: map[string]int32{auth.IdleMinutes: 45}})
	requireCode(t, err, connect.CodePermissionDenied)

	f.role(t, "SecurityAdmin", []string{auth.PermissionSecurity})
	f.user(t, "admin", "SecurityAdmin")
	b.login(t, "admin")
	p, err := f.users.Authenticate(ctx, b.cookie())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("UPDATE athena_sessions SET step_up_at=? WHERE id=?", time.Now().UTC().Add(-16*time.Minute), p.SessionID); err != nil {
		t.Fatal(err)
	}
	_, err = b.auth.SetSecuritySettings(ctx, &v1.SetSecuritySettingsRequest{Settings: map[string]int32{auth.IdleMinutes: 45}})
	requireCode(t, err, connect.CodeFailedPrecondition)
	if failureOf(t, err).GetReason() != "step_up_required" {
		t.Fatalf("missing step-up reason: %v", err)
	}
	up, err := b.auth.StepUp(ctx, &v1.StepUpRequest{Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	*b.csrf = up.GetSession().GetCsrfToken()
	if _, err := b.auth.SetSecuritySettings(ctx, &v1.SetSecuritySettingsRequest{Settings: map[string]int32{auth.IdleMinutes: 45}}); err != nil {
		t.Fatal(err)
	}
}

func TestSecuritySettingsCLISharesTheWritePath(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()
	// The CLI's single-key call writes through the same audited batch.
	if err := f.users.SetSecurity(ctx, auth.IdleMinutes, 45); err != nil {
		t.Fatal(err)
	}
	rows := f.accessEvents(t, auth.EventSecuritySettingsChanged)
	if len(rows) != 1 || !strings.Contains(rows[0].Detail, auth.IdleMinutes+": 30 -> 45") {
		t.Fatalf("audit rows: %+v", rows)
	}
	if rows[0].Username != "" {
		t.Fatalf("operator row carries a username: %+v", rows[0])
	}
}
