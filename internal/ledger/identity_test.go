package ledger_test

import (
	"context"
	"errors"
	"testing"

	"athena-erp/internal/ledger"
)

func TestSaveIdentityDocRejectsStaleReads(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	zero := ledger.IdentityDoc{}
	first := ledger.IdentityDoc{DisplayName: "Acme"}
	if err := s.SaveIdentityDoc(ctx, zero, first, "ana"); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// A second save from the same stale read loses nothing silently.
	second := ledger.IdentityDoc{DisplayName: "Globex"}
	if err := s.SaveIdentityDoc(ctx, zero, second, "bob"); !errors.Is(err, ledger.ErrIdentityConflict) {
		t.Fatalf("stale save err = %v, want ErrIdentityConflict", err)
	}
	// The winner's generation stands, and a fresh read saves on top.
	read, err := s.GetIdentityDoc(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if read.DisplayName != "Acme" {
		t.Fatalf("latest = %q, want Acme", read.DisplayName)
	}
	third := read
	third.DisplayName = "Acme II"
	if err := s.SaveIdentityDoc(ctx, read, third, "ana"); err != nil {
		t.Fatalf("fresh save: %v", err)
	}
	// Old generations stay: the conflict wrote nothing, the two wins did.
	var generations int
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_setting_values WHERE owner_pack='athena' AND setting_type='brand.identity'`).Scan(&generations)
	if err != nil {
		t.Fatal(err)
	}
	if generations != 2 {
		t.Fatalf("generations = %d, want 2", generations)
	}
}
