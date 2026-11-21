package store

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"pgregory.net/rapid"
)

func TestShortenIdentifierIsDeterministicAndBounded(t *testing.T) {
	const postgresLimit = GeneratedIdentLimit
	long := "athena_record_" + strings.Repeat("a", 50)
	a, b := ShortenIdentifier(long, postgresLimit), ShortenIdentifier(long, postgresLimit)
	if a != b {
		t.Fatalf("not deterministic: %q vs %q", a, b)
	}
	if len(a) != postgresLimit {
		t.Fatalf("length = %d, want %d", len(a), postgresLimit)
	}
	other := ShortenIdentifier("athena_record_"+strings.Repeat("a", 49)+"b", postgresLimit)
	if other == a {
		t.Fatal("different names must not collide into one shortened name")
	}
	if got := ShortenIdentifier("athena_record_orders", postgresLimit); got != "athena_record_orders" {
		t.Fatalf("short name changed to %q", got)
	}
}

// TestShortenIdentProperty checks Property 4 over drawn names: shortening is
// deterministic, bounded by each engine's limit, leaves short names alone,
// and keeps distinct inputs distinct at the 63-byte generation limit both
// engines share.
//
// Feature: athena-batch-2, Property 4: Identifier shortening.
func TestShortenIdentProperty(t *testing.T) {
	dialects := map[string]Dialect{"mariadb": MariaDB{}, "postgres": PostgreSQL{}}
	rapid.Check(t, func(t *rapid.T) {
		x := string(rapid.SliceOfN(rapid.Byte(), 0, 100).Draw(t, "first"))
		y := string(rapid.SliceOfN(rapid.Byte(), 0, 100).Draw(t, "second"))
		if x == y {
			return
		}
		a, b := ShortenIdentifier(x, GeneratedIdentLimit), ShortenIdentifier(y, GeneratedIdentLimit)
		if len(a) > GeneratedIdentLimit || len(b) > GeneratedIdentLimit {
			t.Fatalf("lengths %d, %d exceed %d", len(a), len(b), GeneratedIdentLimit)
		}
		if ShortenIdentifier(x, GeneratedIdentLimit) != a {
			t.Fatal("shortening is not deterministic")
		}
		if a == b {
			t.Fatalf("distinct inputs %q and %q collide into %q", x, y, a)
		}
		for name, dialect := range dialects {
			s := dialect.ShortenIdent(x)
			if len(s) > dialect.MaxIdentLength() {
				t.Fatalf("%s length %d exceeds %d", name, len(s), dialect.MaxIdentLength())
			}
			if dialect.ShortenIdent(x) != s {
				t.Fatalf("%s shortening is not deterministic", name)
			}
			if len(x) <= dialect.MaxIdentLength() && s != x {
				t.Fatalf("%s changed a short name to %q", name, s)
			}
		}
	})
}

func TestMariaDBShortenUsesItsOwnLimit(t *testing.T) {
	d := MariaDB{}
	exact := strings.Repeat("a", d.MaxIdentLength())
	if got := d.ShortenIdent(exact); got != exact {
		t.Fatalf("a name at the limit must stay unchanged, got %q", got)
	}
	if got := d.ShortenIdent(exact + "b"); len(got) != d.MaxIdentLength() {
		t.Fatalf("length = %d, want %d", len(got), d.MaxIdentLength())
	}
}

func TestMariaDBClassify(t *testing.T) {
	cases := map[uint16]ErrorKind{
		1062: ErrDuplicateKey, 1213: ErrDeadlock, 1205: ErrLockTimeout,
		1452: ErrForeignKeyMissingParent, 1451: ErrForeignKeyInUse,
		1048: ErrNotNull, 1364: ErrNotNull,
		1406: ErrValueTooLong, 1264: ErrValueTooLong, 1366: ErrValueTooLong,
		1146: ErrMissingTable,
	}
	d := MariaDB{}
	for number, want := range cases {
		err := fmt.Errorf("wrapped: %w", &mysql.MySQLError{Number: number})
		if got := d.Classify(err); got != want {
			t.Errorf("error %d = %d, want %d", number, got, want)
		}
	}
	if got := d.Classify(errors.New("plain")); got != ErrOther {
		t.Errorf("plain error = %d, want ErrOther", got)
	}
}

func TestTableExistsSQLPerDialect(t *testing.T) {
	if mariaDBTableExistsSQL != `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?)` {
		t.Errorf("MariaDB probe = %q", mariaDBTableExistsSQL)
	}
	if pgTableExistsSQL != `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_catalog = current_database() AND table_schema = current_schema() AND table_name = $1)` {
		t.Errorf("PostgreSQL probe = %q", pgTableExistsSQL)
	}
}

func TestMariaDBStatements(t *testing.T) {
	d := MariaDB{}
	if got := d.QuoteIdent("a`b"); got != "`a``b`" {
		t.Errorf("QuoteIdent = %s", got)
	}
	create := d.CreateRecordTypeTable("t", []Column{{Name: "name", Type: "VARCHAR(255)"}})
	for _, want := range []string{"CREATE TABLE IF NOT EXISTS `t`", "`id` CHAR(36) NOT NULL PRIMARY KEY", "`revision` BIGINT NOT NULL DEFAULT 1", "ON UPDATE CURRENT_TIMESTAMP", "`name` VARCHAR(255) NULL"} {
		if !strings.Contains(create, want) {
			t.Errorf("create table lacks %q: %s", want, create)
		}
	}
	checks := map[string]string{
		d.AddColumn("t", Column{Name: "c", Type: "DATE"}):     "ALTER TABLE `t` ADD COLUMN IF NOT EXISTS `c` DATE NULL",
		d.MakeNullable("t", Column{Name: "c", Type: "DATE"}):  "ALTER TABLE `t` MODIFY `c` DATE NULL",
		d.DropColumn("t", "c"):                                "ALTER TABLE `t` DROP COLUMN IF EXISTS `c`",
		d.DropForeignKey("t", "fk"):                           "ALTER TABLE `t` DROP FOREIGN KEY IF EXISTS `fk`",
		d.DropTable("t"):                                      "DROP TABLE IF EXISTS `t`",
		d.AddForeignKey("t", "fk", "c", "u"):                  "ALTER TABLE `t` ADD CONSTRAINT `fk` FOREIGN KEY (`c`) REFERENCES `u` (`id`) ON DELETE RESTRICT",
		d.Upsert("t", []string{"k", "v"}, "k", []string{"v"}): "INSERT INTO `t` (`k`,`v`) VALUES (?,?) ON DUPLICATE KEY UPDATE `v` = VALUES(`v`)",
		d.Upsert("t", []string{"k"}, "k", nil):                "INSERT INTO `t` (`k`) VALUES (?) ON DUPLICATE KEY UPDATE `k` = `k`",
		d.RenameTable("t", "u"):                               "RENAME TABLE `t` TO `u`",
		d.RenameColumn("t", "a", "b"):                         "ALTER TABLE `t` RENAME COLUMN `a` TO `b`",
		d.RenameIndex("t", "i", "j"):                          "ALTER TABLE `t` RENAME INDEX `i` TO `j`",
		d.AddRevisionColumn("t"):                              "ALTER TABLE `t` ADD COLUMN IF NOT EXISTS `revision` BIGINT NOT NULL DEFAULT 1 AFTER `id`",
	}
	for got, want := range checks {
		if got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	}
}

func TestMariaDBColumnTypesAndPredicates(t *testing.T) {
	d := MariaDB{}
	for _, fieldType := range []string{"Text", "Long text", "Email", "Phone", "Number", "Currency", "Date", "Select", "Link", "Checkbox", "Money", "Quantity"} {
		if _, err := d.ColumnType(fieldType); err != nil {
			t.Errorf("ColumnType(%s): %v", fieldType, err)
		}
		if _, err := d.IncompatibleValues("`c`", fieldType, "u"); err != nil {
			t.Errorf("IncompatibleValues(%s): %v", fieldType, err)
		}
	}
	if _, err := d.ColumnType("Enum"); err != nil {
		t.Errorf("ColumnType(Enum): %v", err)
	}
	// Enum values are checked against the field's list by the kernel, not by a column predicate.
	if _, err := d.IncompatibleValues("`c`", "Enum", ""); err == nil {
		t.Error("Enum has no column-only predicate")
	}
	if got, want := d.AddIndex("t", "ix", "c", true), "CREATE UNIQUE INDEX IF NOT EXISTS `ix` ON `t` (`c`)"; got != want {
		t.Errorf("AddIndex = %s", got)
	}
	if got, want := d.DropIndex("t", "ix"), "DROP INDEX IF EXISTS `ix` ON `t`"; got != want {
		t.Errorf("DropIndex = %s", got)
	}
	if _, err := d.ColumnType("Nope"); err == nil {
		t.Error("unknown type must fail")
	}
	if _, err := d.IncompatibleValues("`c`", "Nope", ""); err == nil {
		t.Error("unknown type must fail")
	}
}
