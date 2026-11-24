package main

import (
	"bytes"
	"encoding/json"
	"testing"

	v1 "athena-erp/gen/athena/v1"
)

func TestAppearanceAndInboxOperatorCommands(t *testing.T) {
	db := sharedDSNEnv(t)
	var out bytes.Buffer
	if err := workspaceCommand([]string{"appearance", "set", "--accent", "oklch(0.4 0.05 50)", "--chart5", "oklch(0.65 0.03 130)"}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := workspaceCommand([]string{"appearance", "show"}, &out); err != nil {
		t.Fatal(err)
	}
	var a v1.GetAppearanceResponse
	if err := json.Unmarshal(out.Bytes(), &a); err != nil || len(a.GetAppearance().GetChartColors()) != 5 {
		t.Fatalf("appearance: %v %s", err, out.String())
	}
	ledgerRun(t, "company", "save", "--name", "acme", "--label", "Acme")
	ledgerRun(t, "period", "save", "--company", "acme", "--name", "nov", "--label", "November", "--start", "2026-11-01", "--end", "2026-11-30")
	ledgerRun(t, "period", "close", "--company", "acme", "--name", "nov", "--expected-revision", "1")
	out.Reset()
	if err := notificationsCommand([]string{"list", "--unread", "--user", "administrator"}, &out); err != nil {
		t.Fatal(err)
	}
	var list v1.ListNotificationsResponse
	if err := json.Unmarshal(out.Bytes(), &list); err != nil || len(list.Notifications) != 1 {
		t.Fatalf("list: %v %s", err, out.String())
	}
	if err := notificationsCommand([]string{"read", list.Notifications[0].Id, "--user", "administrator"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := notificationsCommand([]string{"read-all"}, &out); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM athena_notifications WHERE read_at IS NULL").Scan(&count); err != nil || count != 0 {
		t.Fatalf("unread %d, err %v", count, err)
	}
}
