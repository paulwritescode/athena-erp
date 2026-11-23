package ledger_test

import (
	"context"
	"strings"
	"testing"
)

func TestAccountTreePaths(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustAccount(t, s, "assets", "", true)
	mustAccount(t, s, "liquid", "assets", true)
	mustAccount(t, s, "cash", "liquid", false)
	checkTree(t, s, "athena_accounts", "assets", "/assets/", 0)
	checkTree(t, s, "athena_accounts", "liquid", "/assets/liquid/", 1)
	checkTree(t, s, "athena_accounts", "cash", "/assets/liquid/cash/", 2)
	// Reparenting moves the whole subtree; the old parent keeps its path.
	mustAccount(t, s, "funds", "", true)
	liquid, err := s.GetAccount(ctx, "liquid")
	if err != nil {
		t.Fatal(err)
	}
	liquid.Parent = "funds"
	if _, err := s.SaveAccount(ctx, liquid, liquid.Revision); err != nil {
		t.Fatal(err)
	}
	checkTree(t, s, "athena_accounts", "liquid", "/funds/liquid/", 1)
	checkTree(t, s, "athena_accounts", "cash", "/funds/liquid/cash/", 2)
	checkTree(t, s, "athena_accounts", "assets", "/assets/", 0)
	// A label-only change leaves every path alone.
	cash, err := s.GetAccount(ctx, "cash")
	if err != nil {
		t.Fatal(err)
	}
	cash.Label = "Petty cash"
	if _, err := s.SaveAccount(ctx, cash, cash.Revision); err != nil {
		t.Fatal(err)
	}
	checkTree(t, s, "athena_accounts", "cash", "/funds/liquid/cash/", 2)
	// A refused cycle writes nothing: funds sits above cash, so it cannot
	// move under it.
	cash.IsGroup = true
	if _, err := s.SaveAccount(ctx, cash, 2); err != nil {
		t.Fatal(err)
	}
	funds, err := s.GetAccount(ctx, "funds")
	if err != nil {
		t.Fatal(err)
	}
	funds.Parent = "cash"
	if _, err := s.SaveAccount(ctx, funds, funds.Revision); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle: %v", err)
	}
	checkTree(t, s, "athena_accounts", "funds", "/funds/", 0)
}
