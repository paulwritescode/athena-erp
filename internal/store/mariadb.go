package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// MariaDB is the only implemented Dialect. It is tested against MariaDB 11.8.
type MariaDB struct{}

var _ Dialect = MariaDB{}

// MariaDB error numbers; see the server error reference.
const (
	errDuplicateEntry   = 1062
	errLockWaitTimeout  = 1205
	errDeadlock         = 1213
	errRowIsReferenced  = 1451
	errNoReferencedRow  = 1452
	errBadNull          = 1048
	errNoDefault        = 1364
	errDataTooLong      = 1406
	errOutOfRange       = 1264
	errTruncatedWrongIn = 1366
)

// schemaLockWait is how many seconds GET_LOCK waits for another session.
const schemaLockWait = 10

func (MariaDB) Name() string { return "mariadb" }

func (MariaDB) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func (MariaDB) MaxIdentLength() int { return 64 }

func (d MariaDB) ShortenIdent(name string) string {
	return ShortenIdentifier(name, d.MaxIdentLength())
}

func (d MariaDB) ColumnType(fieldType string) (string, error) {
	switch fieldType {
	case "Text":
		return "VARCHAR(255)", nil
	case "Long text":
		return "TEXT", nil
	case "Email":
		return "VARCHAR(320)", nil
	case "Phone":
		return "VARCHAR(40)", nil
	case "Number", "Currency", "Quantity":
		return "DECIMAL(18,6)", nil
	case "Money":
		// Integer minor units; the currency is a field attribute.
		return d.BigIntType(), nil
	case "Date":
		return "DATE", nil
	case "Select", "Enum":
		return "VARCHAR(120)", nil
	case "Link":
		return "CHAR(36)", nil
	case "Checkbox":
		return "BOOLEAN", nil
	default:
		return "", fmt.Errorf("unsupported field type %q", fieldType)
	}
}

func (MariaDB) BigIntType() string { return "BIGINT" }

func (MariaDB) TimestampType() string { return "TIMESTAMP" }

func (MariaDB) JSONType() string { return "JSON" }

func (MariaDB) Now() string { return "CURRENT_TIMESTAMP" }

func (d MariaDB) CreateMigrationsTable() string {
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (version INT NOT NULL PRIMARY KEY, name VARCHAR(100) NOT NULL, applied_at TIMESTAMP NOT NULL DEFAULT %s) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci", d.QuoteIdent(MigrationsTable), d.Now())
}

func (d MariaDB) CreateRecordTypeTable(table string, columns []Column) string {
	defs := []string{
		d.QuoteIdent("id") + " CHAR(36) NOT NULL PRIMARY KEY",
		d.QuoteIdent("revision") + " BIGINT NOT NULL DEFAULT 1",
		d.QuoteIdent("created_at") + " TIMESTAMP NOT NULL DEFAULT " + d.Now(),
		// MariaDB refreshes the column on every update; other engines need a trigger.
		d.QuoteIdent("updated_at") + " TIMESTAMP NOT NULL DEFAULT " + d.Now() + " ON UPDATE " + d.Now(),
	}
	for _, column := range columns {
		defs = append(defs, d.QuoteIdent(column.Name)+" "+column.Type+" NULL")
	}
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci", d.QuoteIdent(table), strings.Join(defs, ","))
}

func (d MariaDB) AddRevisionColumn(table string) string {
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s BIGINT NOT NULL DEFAULT 1 AFTER %s", d.QuoteIdent(table), d.QuoteIdent("revision"), d.QuoteIdent("id"))
}

func (d MariaDB) AddColumn(table string, column Column) string {
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s NULL", d.QuoteIdent(table), d.QuoteIdent(column.Name), column.Type)
}

func (d MariaDB) MakeNullable(table string, column Column) string {
	return fmt.Sprintf("ALTER TABLE %s MODIFY %s %s NULL", d.QuoteIdent(table), d.QuoteIdent(column.Name), column.Type)
}

func (d MariaDB) DropColumn(table, column string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", d.QuoteIdent(table), d.QuoteIdent(column))
}

func (d MariaDB) AddForeignKey(table, constraint, column, target string) string {
	return fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s) ON DELETE RESTRICT", d.QuoteIdent(table), d.QuoteIdent(constraint), d.QuoteIdent(column), d.QuoteIdent(target), d.QuoteIdent("id"))
}

func (d MariaDB) DropForeignKey(table, constraint string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP FOREIGN KEY IF EXISTS %s", d.QuoteIdent(table), d.QuoteIdent(constraint))
}

func (d MariaDB) AddIndex(table, index, column string, unique bool) string {
	kind := "INDEX"
	if unique {
		kind = "UNIQUE INDEX"
	}
	return fmt.Sprintf("CREATE %s IF NOT EXISTS %s ON %s (%s)", kind, d.QuoteIdent(index), d.QuoteIdent(table), d.QuoteIdent(column))
}

func (d MariaDB) DropIndex(table, index string) string {
	return fmt.Sprintf("DROP INDEX IF EXISTS %s ON %s", d.QuoteIdent(index), d.QuoteIdent(table))
}

func (d MariaDB) DropTable(table string) string {
	return "DROP TABLE IF EXISTS " + d.QuoteIdent(table)
}

func (d MariaDB) RenameTable(from, to string) string {
	return fmt.Sprintf("RENAME TABLE %s TO %s", d.QuoteIdent(from), d.QuoteIdent(to))
}

func (d MariaDB) RenameColumn(table, from, to string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", d.QuoteIdent(table), d.QuoteIdent(from), d.QuoteIdent(to))
}

func (d MariaDB) RenameIndex(table, from, to string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME INDEX %s TO %s", d.QuoteIdent(table), d.QuoteIdent(from), d.QuoteIdent(to))
}

func (d MariaDB) Upsert(table string, columns []string, keyColumn string, updateColumns []string) string {
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = d.QuoteIdent(column)
	}
	assignments := make([]string, len(updateColumns))
	for i, column := range updateColumns {
		assignments[i] = d.QuoteIdent(column) + " = VALUES(" + d.QuoteIdent(column) + ")"
	}
	if len(assignments) == 0 {
		// A no-op assignment on the key keeps the statement valid.
		assignments = []string{d.QuoteIdent(keyColumn) + " = " + d.QuoteIdent(keyColumn)}
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON DUPLICATE KEY UPDATE %s", d.QuoteIdent(table), strings.Join(quoted, ","), strings.TrimSuffix(strings.Repeat("?,", len(columns)), ","), strings.Join(assignments, ", "))
}

func (MariaDB) ForUpdate() string { return "FOR UPDATE" }

// Rebind passes the statement through: the MySQL driver binds ? directly.
func (MariaDB) Rebind(query string) string { return query }

// NullSafeEqual is MariaDB's null-safe equality operator.
func (MariaDB) NullSafeEqual(a, b string) string { return "(" + a + " <=> " + b + ")" }

// BinaryString uses the code-point collation without space padding, so
// comparisons match Go string comparison. Column collation is
// utf8mb4_unicode_ci, which would ignore case and accents.
func (MariaDB) BinaryString(expr string) string { return expr + " COLLATE utf8mb4_nopad_bin" }

// DecimalParam binds a text argument as DECIMAL(65,30); a bare string would
// compare with a DECIMAL column as a floating point number.
func (MariaDB) DecimalParam() string { return "CAST(? AS DECIMAL(65,30))" }

// DateParam binds a YYYY-MM-DD text argument as a DATE.
func (MariaDB) DateParam() string { return "CAST(? AS DATE)" }

func (d MariaDB) IncompatibleValues(column, fieldType, linkTable string) (string, error) {
	text := "CAST(" + column + " AS CHAR)"
	switch fieldType {
	case "Number", "Currency", "Quantity":
		return text + ` NOT REGEXP '^-?[0-9]{1,12}(\\.[0-9]{1,6})?$'`, nil
	case "Money":
		// Minor units are not comparable with other types, so nothing converts.
		return "1 = 1", nil
	case "Date":
		return text + ` NOT REGEXP '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' OR STR_TO_DATE(` + text + `, '%Y-%m-%d') IS NULL`, nil
	case "Checkbox":
		return text + ` NOT IN ('0', '1')`, nil
	case "Link":
		return column + " NOT IN (SELECT " + d.QuoteIdent("id") + " FROM " + d.QuoteIdent(linkTable) + ")", nil
	case "Long text":
		return "CHAR_LENGTH(" + text + ") > 65535", nil
	case "Email":
		return "CHAR_LENGTH(" + text + ") > 320 OR " + text + " NOT LIKE '%_@_%'", nil
	}
	limit := map[string]int{"Text": 255, "Phone": 40, "Select": 120}[fieldType]
	if limit == 0 {
		return "", fmt.Errorf("unsupported field type %q", fieldType)
	}
	return fmt.Sprintf("CHAR_LENGTH(%s) > %d", text, limit), nil
}

func (MariaDB) TableExists(ctx context.Context, q Querier, table string) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?)`, table).Scan(&exists)
	return exists, err
}

func (MariaDB) ColumnExists(ctx context.Context, q Querier, table, column string) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?)`, table, column).Scan(&exists)
	return exists, err
}

func (MariaDB) IndexExists(ctx context.Context, q Querier, table, index string) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?)`, table, index).Scan(&exists)
	return exists, err
}

func (MariaDB) ColumnIsNullable(ctx context.Context, q Querier, table, column string) (bool, error) {
	var nullable string
	err := q.QueryRowContext(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, table, column).Scan(&nullable)
	return nullable != "NO", err
}

func (MariaDB) ForeignKeyTarget(ctx context.Context, q Querier, table, constraint string) (string, bool, error) {
	var target string
	err := q.QueryRowContext(ctx, `SELECT referenced_table_name FROM information_schema.referential_constraints WHERE constraint_schema = DATABASE() AND table_name = ? AND constraint_name = ?`, table, constraint).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return target, err == nil, err
}

// LockSchema uses a named lock. MariaDB named locks are server-wide, so the
// database name is part of the lock name.
func (MariaDB) LockSchema(ctx context.Context, q Querier) (bool, func(), error) {
	var got sql.NullInt64
	if err := q.QueryRowContext(ctx, fmt.Sprintf(`SELECT GET_LOCK(CONCAT('athena_schema_', DATABASE()), %d)`, schemaLockWait)).Scan(&got); err != nil {
		return false, nil, err
	}
	if !got.Valid || got.Int64 != 1 {
		return false, nil, nil
	}
	release := func() {
		var released sql.NullInt64
		_ = q.QueryRowContext(context.Background(), `SELECT RELEASE_LOCK(CONCAT('athena_schema_', DATABASE()))`).Scan(&released)
	}
	return true, release, nil
}

// Classify maps MariaDB error numbers. MariaDB reports serialization
// conflicts as deadlocks, so ErrSerializationFailure is never returned.
func (MariaDB) Classify(err error) ErrorKind {
	var dbErr *mysql.MySQLError
	if !errors.As(err, &dbErr) {
		return ErrOther
	}
	switch dbErr.Number {
	case errDuplicateEntry:
		return ErrDuplicateKey
	case errDeadlock:
		return ErrDeadlock
	case errLockWaitTimeout:
		return ErrLockTimeout
	case errNoReferencedRow:
		return ErrForeignKeyMissingParent
	case errRowIsReferenced:
		return ErrForeignKeyInUse
	case errBadNull, errNoDefault:
		return ErrNotNull
	case errDataTooLong, errOutOfRange, errTruncatedWrongIn:
		return ErrValueTooLong
	}
	return ErrOther
}
