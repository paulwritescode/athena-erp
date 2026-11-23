package ledger_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"athena-erp/internal/ledger"
)

// treePosition reads the stored path and depth of one master row. The
// structs do not carry them yet; the columns are the contract under test.
func treePosition(t *testing.T, s *ledger.Service, table, name string) (string, int) {
	t.Helper()
	var path sql.NullString
	var depth sql.NullInt64
	err := s.DB.QueryRowContext(context.Background(), "SELECT tree_path, tree_depth FROM "+table+" WHERE name = ?", name).Scan(&path, &depth)
	if err != nil {
		t.Fatalf("%s %s: %v", table, name, err)
	}
	if !path.Valid || !depth.Valid {
		t.Fatalf("%s %s has no tree position", table, name)
	}
	return path.String, int(depth.Int64)
}

func checkTree(t *testing.T, s *ledger.Service, table, name, wantPath string, wantDepth int) {
	t.Helper()
	path, depth := treePosition(t, s, table, name)
	if path != wantPath || depth != wantDepth {
		t.Fatalf("%s %s = %s depth %d, want %s depth %d", table, name, path, depth, wantPath, wantDepth)
	}
}

func TestBranchTreePaths(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustCompany(t, s, "acme")
	if _, err := s.SaveBranch(ctx, ledger.Branch{Name: "north", Label: "North", Company: "acme", IsGroup: true}, 0); err != nil {
		t.Fatal(err)
	}
	metro, err := s.SaveBranch(ctx, ledger.Branch{Name: "metro", Label: "Metro", Company: "acme", Parent: "north", IsGroup: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	downtown, err := s.SaveBranch(ctx, ledger.Branch{Name: "downtown", Label: "Downtown", Company: "acme", Parent: "metro"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkTree(t, s, "athena_branches", "north", "/north/", 0)
	checkTree(t, s, "athena_branches", "metro", "/north/metro/", 1)
	checkTree(t, s, "athena_branches", "downtown", "/north/metro/downtown/", 2)
	// Reparenting moves the whole subtree; the old parent keeps its path.
	if _, err := s.SaveBranch(ctx, ledger.Branch{Name: "south", Label: "South", Company: "acme", IsGroup: true}, 0); err != nil {
		t.Fatal(err)
	}
	metro.Parent = "south"
	if _, err := s.SaveBranch(ctx, metro, 1); err != nil {
		t.Fatal(err)
	}
	checkTree(t, s, "athena_branches", "metro", "/south/metro/", 1)
	checkTree(t, s, "athena_branches", "downtown", "/south/metro/downtown/", 2)
	checkTree(t, s, "athena_branches", "north", "/north/", 0)
	// A label-only change leaves every path alone.
	downtown.Label = "Downtown Core"
	if _, err := s.SaveBranch(ctx, downtown, 1); err != nil {
		t.Fatal(err)
	}
	checkTree(t, s, "athena_branches", "downtown", "/south/metro/downtown/", 2)
	// A refused cycle writes nothing: south sits above downtown, so it
	// cannot move under it.
	downtown.IsGroup = true
	if _, err := s.SaveBranch(ctx, downtown, 2); err != nil {
		t.Fatal(err)
	}
	south, err := s.GetBranch(ctx, "south")
	if err != nil {
		t.Fatal(err)
	}
	south.Parent = "downtown"
	if _, err := s.SaveBranch(ctx, south, south.Revision); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle: %v", err)
	}
	checkTree(t, s, "athena_branches", "south", "/south/", 0)
	checkTree(t, s, "athena_branches", "north", "/north/", 0)
}

func TestDepartmentTreePaths(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustCompany(t, s, "acme")
	if _, err := s.SaveBranch(ctx, ledger.Branch{Name: "north", Label: "N", Company: "acme"}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveDepartment(ctx, ledger.Department{Name: "ops", Label: "Ops", Company: "acme", IsGroup: true}, 0); err != nil {
		t.Fatal(err)
	}
	sales, err := s.SaveDepartment(ctx, ledger.Department{Name: "sales", Label: "Sales", Company: "acme", Branch: "north", Parent: "ops"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkTree(t, s, "athena_departments", "ops", "/ops/", 0)
	checkTree(t, s, "athena_departments", "sales", "/ops/sales/", 1)
	if _, err := s.SaveDepartment(ctx, ledger.Department{Name: "support", Label: "Support", Company: "acme", IsGroup: true}, 0); err != nil {
		t.Fatal(err)
	}
	sales.Parent = "support"
	if _, err := s.SaveDepartment(ctx, sales, 1); err != nil {
		t.Fatal(err)
	}
	checkTree(t, s, "athena_departments", "sales", "/support/sales/", 1)
	checkTree(t, s, "athena_departments", "ops", "/ops/", 0)
	got, err := s.GetDepartment(ctx, "sales")
	if err != nil || got.Branch != "north" {
		t.Fatalf("branch link lost: %+v %v", got, err)
	}
}

func TestCostCentreTreePaths(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustCompany(t, s, "acme")
	if _, err := s.SaveCostCentre(ctx, ledger.CostCentre{Name: "cc_root", Label: "Root", Company: "acme", IsGroup: true}, 0); err != nil {
		t.Fatal(err)
	}
	sub, err := s.SaveCostCentre(ctx, ledger.CostCentre{Name: "cc_sub", Label: "Sub", Company: "acme", Parent: "cc_root"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkTree(t, s, "athena_cost_centres", "cc_root", "/cc_root/", 0)
	checkTree(t, s, "athena_cost_centres", "cc_sub", "/cc_root/cc_sub/", 1)
	if _, err := s.SaveCostCentre(ctx, ledger.CostCentre{Name: "cc_other", Label: "Other", Company: "acme", IsGroup: true}, 0); err != nil {
		t.Fatal(err)
	}
	sub.Parent = "cc_other"
	if _, err := s.SaveCostCentre(ctx, sub, 1); err != nil {
		t.Fatal(err)
	}
	checkTree(t, s, "athena_cost_centres", "cc_sub", "/cc_other/cc_sub/", 1)
	checkTree(t, s, "athena_cost_centres", "cc_root", "/cc_root/", 0)
}
