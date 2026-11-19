package store

import (
	"context"
	"testing"

	"athena-erp/internal/testdb"
)

// TestMariaDBCatalogueAndLock runs the generated statements against MariaDB.
func TestMariaDBCatalogueAndLock(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	d := MariaDB{}

	for _, statement := range []string{
		d.CreateRecordTable("parent", []Column{{Name: "name", Type: "VARCHAR(255)"}}),
		d.CreateRecordTable("child", []Column{{Name: "parent_id", Type: "CHAR(36)"}}),
		d.AddForeignKey("child", "fk_child_parent", "parent_id", "parent"),
		d.AddColumn("child", Column{Name: "note", Type: "TEXT"}),
		d.AddColumn("child", Column{Name: "note", Type: "TEXT"}), // repeating is safe
	} {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if ok, err := d.TableExists(ctx, conn, "child"); err != nil || !ok {
		t.Fatalf("TableExists(child) = %v, %v", ok, err)
	}
	if ok, err := d.TableExists(ctx, conn, "missing"); err != nil || ok {
		t.Fatalf("TableExists(missing) = %v, %v", ok, err)
	}
	if nullable, err := d.ColumnIsNullable(ctx, conn, "child", "note"); err != nil || !nullable {
		t.Fatalf("ColumnIsNullable = %v, %v", nullable, err)
	}
	if target, found, err := d.ForeignKeyTarget(ctx, conn, "child", "fk_child_parent"); err != nil || !found || target != "parent" {
		t.Fatalf("ForeignKeyTarget = %q, %v, %v", target, found, err)
	}
	if _, found, err := d.ForeignKeyTarget(ctx, conn, "child", "nope"); err != nil || found {
		t.Fatalf("ForeignKeyTarget(nope) found = %v, err = %v", found, err)
	}

	// A foreign key violation is classified without reading driver numbers.
	_, err = conn.ExecContext(ctx, "INSERT INTO `child` (`id`, `parent_id`) VALUES (UUID(), UUID())")
	if got := d.Classify(err); got != ErrForeignKeyMissingParent {
		t.Fatalf("Classify(missing parent) = %d, err = %v", got, err)
	}

	acquired, release, err := d.LockSchema(ctx, conn)
	if err != nil || !acquired {
		t.Fatalf("LockSchema = %v, %v", acquired, err)
	}
	release()
}
