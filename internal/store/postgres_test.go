package store

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// The PostgreSQL adapter is implemented but untested against a server: these
// tests assert generated SQL strings, quoting and error classification only.

func TestPostgresBasics(t *testing.T) {
	var d Dialect = PostgreSQL{}
	if d.Name() != "postgres" {
		t.Fatalf("Name = %q", d.Name())
	}
	if d.MaxIdentLength() != 63 {
		t.Fatalf("MaxIdentLength = %d", d.MaxIdentLength())
	}
	if got := d.QuoteIdent(`we"ird`); got != `"we""ird"` {
		t.Fatalf("QuoteIdent = %q", got)
	}
	if got := d.BigIntType(); got != "BIGINT" {
		t.Fatalf("BigIntType = %q", got)
	}
	if got := d.TimestampType(); got != "TIMESTAMPTZ" {
		t.Fatalf("TimestampType = %q", got)
	}
	if got := d.JSONType(); got != "JSONB" {
		t.Fatalf("JSONType = %q", got)
	}
	if got := d.Now(); got != "CURRENT_TIMESTAMP" {
		t.Fatalf("Now = %q", got)
	}
	if got := d.ForUpdate(); got != "FOR UPDATE" {
		t.Fatalf("ForUpdate = %q", got)
	}
}

func TestPostgresColumnTypes(t *testing.T) {
	var d Dialect = PostgreSQL{}
	cases := map[string]string{
		"Text": "VARCHAR(255)", "Long text": "TEXT", "Email": "VARCHAR(320)", "Phone": "VARCHAR(40)",
		"Number": "DECIMAL(18,6)", "Currency": "DECIMAL(18,6)", "Quantity": "DECIMAL(18,6)", "Money": "BIGINT",
		"Date": "DATE", "Select": "VARCHAR(120)", "Enum": "VARCHAR(120)", "Link": "CHAR(36)", "Checkbox": "BOOLEAN",
	}
	for fieldType, want := range cases {
		if got, err := d.ColumnType(fieldType); err != nil || got != want {
			t.Fatalf("ColumnType(%q) = %q, %v", fieldType, got, err)
		}
	}
	if _, err := d.ColumnType("Nope"); err == nil {
		t.Fatal("an unknown field type was accepted")
	}
}

func TestPostgresDDL(t *testing.T) {
	var d Dialect = PostgreSQL{}
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"migrations", d.CreateMigrationsTable(), `CREATE TABLE IF NOT EXISTS "athena_migrations" (version INT NOT NULL PRIMARY KEY, name VARCHAR(100) NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`},
		{"record table", d.CreateRecordTypeTable("t", []Column{{Name: "title", Type: "VARCHAR(255)"}}),
			`CREATE TABLE IF NOT EXISTS "t" ("id" CHAR(36) NOT NULL PRIMARY KEY,"revision" BIGINT NOT NULL DEFAULT 1,"created_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,"updated_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,"title" VARCHAR(255) NULL)`},
		{"add revision", d.AddRevisionColumn("t"), `ALTER TABLE "t" ADD COLUMN IF NOT EXISTS "revision" BIGINT NOT NULL DEFAULT 1`},
		{"add column", d.AddColumn("t", Column{Name: "c", Type: "TEXT"}), `ALTER TABLE "t" ADD COLUMN IF NOT EXISTS "c" TEXT NULL`},
		{"make nullable", d.MakeNullable("t", Column{Name: "c", Type: "TEXT"}), `ALTER TABLE "t" ALTER COLUMN "c" TYPE TEXT, ALTER COLUMN "c" DROP NOT NULL`},
		{"drop column", d.DropColumn("t", "c"), `ALTER TABLE "t" DROP COLUMN IF EXISTS "c"`},
		{"add fk", d.AddForeignKey("t", "fk", "c", "u"), `ALTER TABLE "t" ADD CONSTRAINT "fk" FOREIGN KEY ("c") REFERENCES "u" ("id") ON DELETE RESTRICT`},
		{"drop fk", d.DropForeignKey("t", "fk"), `ALTER TABLE "t" DROP CONSTRAINT IF EXISTS "fk"`},
		{"add index", d.AddIndex("t", "ix", "c", false), `CREATE INDEX IF NOT EXISTS "ix" ON "t" ("c")`},
		{"add unique", d.AddIndex("t", "ix", "c", true), `CREATE UNIQUE INDEX IF NOT EXISTS "ix" ON "t" ("c")`},
		{"drop index", d.DropIndex("t", "ix"), `DROP INDEX IF EXISTS "ix"`},
		{"drop table", d.DropTable("t"), `DROP TABLE IF EXISTS "t"`},
		{"rename table", d.RenameTable("a", "b"), `ALTER TABLE "a" RENAME TO "b"`},
		{"rename column", d.RenameColumn("t", "a", "b"), `ALTER TABLE "t" RENAME COLUMN "a" TO "b"`},
		{"rename index", d.RenameIndex("t", "a", "b"), `ALTER INDEX "a" RENAME TO "b"`},
		{"upsert", d.Upsert("t", []string{"k", "v"}, "k", []string{"v"}), `INSERT INTO "t" ("k","v") VALUES (?,?) ON CONFLICT ("k") DO UPDATE SET "v" = EXCLUDED."v"`},
		{"upsert nothing", d.Upsert("t", []string{"k"}, "k", nil), `INSERT INTO "t" ("k") VALUES (?) ON CONFLICT ("k") DO NOTHING`},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("%s:\n got %q\nwant %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestPostgresPredicates(t *testing.T) {
	var d Dialect = PostgreSQL{}
	if got := d.NullSafeEqual("a", "b"); got != "(a IS NOT DISTINCT FROM b)" {
		t.Fatalf("NullSafeEqual = %q", got)
	}
	if got := d.BinaryString("c"); got != `c COLLATE "C"` {
		t.Fatalf("BinaryString = %q", got)
	}
	if got := d.DecimalParam(); got != "CAST(? AS DECIMAL(65,30))" {
		t.Fatalf("DecimalParam = %q", got)
	}
	if got := d.DateParam(); got != "CAST(? AS DATE)" {
		t.Fatalf("DateParam = %q", got)
	}
	number, err := d.IncompatibleValues("c", "Number", "")
	if err != nil || !strings.Contains(number, "!~") {
		t.Fatalf("number incompatibility = %q, %v", number, err)
	}
	date, err := d.IncompatibleValues("c", "Date", "")
	if err != nil || !strings.Contains(date, "0[1-9]|1[0-2]") {
		t.Fatalf("date incompatibility = %q, %v", date, err)
	}
	link, err := d.IncompatibleValues("c", "Link", "u")
	if err != nil || !strings.Contains(link, `"u"`) || !strings.Contains(link, "NOT IN") {
		t.Fatalf("link incompatibility = %q, %v", link, err)
	}
	if _, err := d.IncompatibleValues("c", "Nope", ""); err == nil {
		t.Fatal("an unknown field type was accepted")
	}
	// Enum values are checked against the field's list by the kernel, not by a column predicate.
	if _, err := d.IncompatibleValues("c", "Enum", ""); err == nil {
		t.Fatal("Enum has no column-only predicate")
	}
}

func TestPostgresRebind(t *testing.T) {
	var d Dialect = PostgreSQL{}
	cases := map[string]string{
		"SELECT ?, ?":                        "SELECT $1, $2",
		"SELECT 'it''s ?', \"a?b\"":          "SELECT 'it''s ?', \"a?b\"",
		"SELECT 1 -- a ? comment\n, ?":       "SELECT 1 -- a ? comment\n, $1",
		"SELECT /* ? */ ?":                   "SELECT /* ? */ $1",
		"SELECT /* outer /* inner ? */ */ ?": "SELECT /* outer /* inner ? */ */ $1",
		`SELECT E'a\n?' , ?`:                 `SELECT E'a\n?' , $1`,
		"SELECT data ?? 'k', ?":              "SELECT data ?? 'k', $1",
		"SELECT data ?| '{a}', ?":            "SELECT data ?| '{a}', $1",
		"SELECT data ?& '{a}', ?":            "SELECT data ?& '{a}', $1",
		"SELECT 1":                           "SELECT 1",
	}
	for in, want := range cases {
		if got := d.Rebind(in); got != want {
			t.Fatalf("Rebind(%q) = %q, want %q", in, got, want)
		}
	}
	var m Dialect = MariaDB{}
	if got := m.Rebind("SELECT ?, ?"); got != "SELECT ?, ?" {
		t.Fatalf("MariaDB Rebind = %q", got)
	}
}

func TestPostgresClassify(t *testing.T) {
	var d Dialect = PostgreSQL{}
	cases := []struct {
		code   string
		detail string
		want   ErrorKind
	}{
		{"23505", "", ErrDuplicateKey},
		{"40P01", "", ErrDeadlock},
		{"40001", "", ErrSerializationFailure},
		{"55P03", "", ErrLockTimeout},
		{"23503", `Key (id)=(1) is still referenced from table "t".`, ErrForeignKeyInUse},
		{"23503", `Key (c)=(1) is not present in table "u".`, ErrForeignKeyMissingParent},
		{"23503", "", ErrOther},
		{"23502", "", ErrNotNull},
		{"22001", "", ErrValueTooLong},
		{"22003", "", ErrValueTooLong},
		{"22P02", "", ErrValueTooLong},
		{"42P01", "", ErrMissingTable},
		{"XX000", "", ErrOther},
	}
	for _, tc := range cases {
		if got := d.Classify(&pgconn.PgError{Code: tc.code, Detail: tc.detail}); got != tc.want {
			t.Fatalf("Classify(%q) = %v, want %v", tc.code, got, tc.want)
		}
	}
	if got := d.Classify(nil); got != ErrOther {
		t.Fatalf("Classify(nil) = %v", got)
	}
}
