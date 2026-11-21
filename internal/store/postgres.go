package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL implements Dialect without a running server behind it: the SQL
// strings are unit-tested, but nothing runs them (see Req 2, "implemented
// but untested"). Identifiers are double-quoted; string literals stay
// single-quoted; placeholders stay ? and Rebind numbers them at the
// execution boundary during the parity run.
type PostgreSQL struct{}

var _ Dialect = PostgreSQL{}

// PostgreSQL SQLSTATE codes; see the server error code reference.
const (
	pgUniqueViolation      = "23505"
	pgUndefinedTable       = "42P01"
	pgForeignKeyViolation  = "23503"
	pgNotNullViolation     = "23502"
	pgDeadlockDetected     = "40P01"
	pgSerializationFailure = "40001"
	pgLockNotAvailable     = "55P03"
	pgStringDataTruncated  = "22001"
	pgNumericOutOfRange    = "22003"
	pgInvalidTextRepr      = "22P02"
)

// schemaLockTries bounds the advisory-lock wait: 100 tries 100ms apart.
const schemaLockTries = 100

// schemaLockKey names the advisory lock. Advisory locks already live
// per-database, so unlike the MariaDB name it needs no database part.
const schemaLockKey = "hashtext('athena_schema')"

func (PostgreSQL) Name() string { return "postgres" }

func (PostgreSQL) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (PostgreSQL) MaxIdentLength() int { return 63 }

func (d PostgreSQL) ShortenIdent(name string) string {
	return ShortenIdentifier(name, d.MaxIdentLength())
}

func (d PostgreSQL) ColumnType(fieldType string) (string, error) {
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
		// CHAR(36) like MariaDB: blank-padded comparisons match UUIDs.
		return "CHAR(36)", nil
	case "Checkbox":
		return "BOOLEAN", nil
	default:
		return "", fmt.Errorf("unsupported field type %q", fieldType)
	}
}

func (PostgreSQL) BigIntType() string { return "BIGINT" }

func (PostgreSQL) TimestampType() string { return "TIMESTAMPTZ" }

func (PostgreSQL) JSONType() string { return "JSONB" }

func (PostgreSQL) Now() string { return "CURRENT_TIMESTAMP" }

func (d PostgreSQL) CreateMigrationsTable() string {
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (version INT NOT NULL PRIMARY KEY, name VARCHAR(100) NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT %s)", d.QuoteIdent(MigrationsTable), d.Now())
}

func (d PostgreSQL) CreateRecordTypeTable(table string, columns []Column) string {
	defs := []string{
		d.QuoteIdent("id") + " CHAR(36) NOT NULL PRIMARY KEY",
		d.QuoteIdent("revision") + " BIGINT NOT NULL DEFAULT 1",
		d.QuoteIdent("created_at") + " TIMESTAMPTZ NOT NULL DEFAULT " + d.Now(),
		// Unlike MariaDB there is no auto-refresh: the parity run adds an
		// update trigger or sets updated_at in every UPDATE.
		d.QuoteIdent("updated_at") + " TIMESTAMPTZ NOT NULL DEFAULT " + d.Now(),
	}
	for _, column := range columns {
		defs = append(defs, d.QuoteIdent(column.Name)+" "+column.Type+" NULL")
	}
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", d.QuoteIdent(table), strings.Join(defs, ","))
}

func (d PostgreSQL) AddRevisionColumn(table string) string {
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s BIGINT NOT NULL DEFAULT 1", d.QuoteIdent(table), d.QuoteIdent("revision"))
}

func (d PostgreSQL) AddColumn(table string, column Column) string {
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s NULL", d.QuoteIdent(table), d.QuoteIdent(column.Name), column.Type)
}

func (d PostgreSQL) MakeNullable(table string, column Column) string {
	return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s, ALTER COLUMN %s DROP NOT NULL", d.QuoteIdent(table), d.QuoteIdent(column.Name), column.Type, d.QuoteIdent(column.Name))
}

func (d PostgreSQL) DropColumn(table, column string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", d.QuoteIdent(table), d.QuoteIdent(column))
}

func (d PostgreSQL) AddForeignKey(table, constraint, column, target string) string {
	return fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s) ON DELETE RESTRICT", d.QuoteIdent(table), d.QuoteIdent(constraint), d.QuoteIdent(column), d.QuoteIdent(target), d.QuoteIdent("id"))
}

func (d PostgreSQL) DropForeignKey(table, constraint string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s", d.QuoteIdent(table), d.QuoteIdent(constraint))
}

func (d PostgreSQL) AddIndex(table, index, column string, unique bool) string {
	kind := "INDEX"
	if unique {
		kind = "UNIQUE INDEX"
	}
	return fmt.Sprintf("CREATE %s IF NOT EXISTS %s ON %s (%s)", kind, d.QuoteIdent(index), d.QuoteIdent(table), d.QuoteIdent(column))
}

func (d PostgreSQL) DropIndex(table, index string) string {
	// Index names live outside tables; the table only documents intent.
	_ = table
	return "DROP INDEX IF EXISTS " + d.QuoteIdent(index)
}

func (d PostgreSQL) DropTable(table string) string {
	return "DROP TABLE IF EXISTS " + d.QuoteIdent(table)
}

func (d PostgreSQL) RenameTable(from, to string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME TO %s", d.QuoteIdent(from), d.QuoteIdent(to))
}

func (d PostgreSQL) RenameColumn(table, from, to string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", d.QuoteIdent(table), d.QuoteIdent(from), d.QuoteIdent(to))
}

func (d PostgreSQL) RenameIndex(table, from, to string) string {
	_ = table
	return fmt.Sprintf("ALTER INDEX %s RENAME TO %s", d.QuoteIdent(from), d.QuoteIdent(to))
}

func (d PostgreSQL) Upsert(table string, columns []string, keyColumn string, updateColumns []string) string {
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = d.QuoteIdent(column)
	}
	if len(updateColumns) == 0 {
		return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO NOTHING", d.QuoteIdent(table), strings.Join(quoted, ","), strings.TrimSuffix(strings.Repeat("?,", len(columns)), ","), d.QuoteIdent(keyColumn))
	}
	assignments := make([]string, len(updateColumns))
	for i, column := range updateColumns {
		assignments[i] = d.QuoteIdent(column) + " = EXCLUDED." + d.QuoteIdent(column)
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s", d.QuoteIdent(table), strings.Join(quoted, ","), strings.TrimSuffix(strings.Repeat("?,", len(columns)), ","), d.QuoteIdent(keyColumn), strings.Join(assignments, ", "))
}

func (PostgreSQL) ForUpdate() string { return "FOR UPDATE" }

// NullSafeEqual is the standard null-safe comparison.
func (PostgreSQL) NullSafeEqual(a, b string) string {
	return "(" + a + " IS NOT DISTINCT FROM " + b + ")"
}

// BinaryString forces byte comparison whatever the database collation. The
// translator calls it for string expressions only.
func (PostgreSQL) BinaryString(expr string) string { return expr + ` COLLATE "C"` }

// DecimalParam binds a text argument as an exact decimal; a bare string
// would compare with a DECIMAL column as a floating point number.
func (PostgreSQL) DecimalParam() string { return "CAST(? AS DECIMAL(65,30))" }

// DateParam binds a YYYY-MM-DD text argument as a DATE.
func (PostgreSQL) DateParam() string { return "CAST(? AS DATE)" }

func (d PostgreSQL) IncompatibleValues(column, fieldType, linkTable string) (string, error) {
	text := "CAST(" + column + " AS TEXT)"
	switch fieldType {
	case "Number", "Currency", "Quantity":
		return text + ` !~ '^-?[0-9]{1,12}(\.[0-9]{1,6})?$'`, nil
	case "Money":
		// Minor units are not comparable with other types, so nothing converts.
		return "1 = 1", nil
	case "Date":
		// A bad cast aborts the query in PostgreSQL, so impossible dates
		// are rejected by shape (month and day ranges) instead of by
		// casting. A February 30 still passes the preview; the ALTER
		// itself validates every value and fails safe.
		return text + ` !~ '^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$'`, nil
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

// pgTableExistsSQL is the catalogue probe TableExists runs, kept as a
// constant so tests assert the exact SQL per dialect. pgx binds by number,
// so the marker is $1, not ?.
const pgTableExistsSQL = `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_catalog = current_database() AND table_schema = current_schema() AND table_name = $1)`

func (PostgreSQL) TableExists(ctx context.Context, q Querier, table string) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx, pgTableExistsSQL, table).Scan(&exists)
	return exists, err
}

func (PostgreSQL) ColumnExists(ctx context.Context, q Querier, table, column string) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_catalog = current_database() AND table_schema = current_schema() AND table_name = ? AND column_name = ?)`, table, column).Scan(&exists)
	return exists, err
}

func (PostgreSQL) IndexExists(ctx context.Context, q Querier, table, index string) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_catalog = current_database() AND table_schema = current_schema() AND table_name = ? AND index_name = ?)`, table, index).Scan(&exists)
	return exists, err
}

func (PostgreSQL) ColumnIsNullable(ctx context.Context, q Querier, table, column string) (bool, error) {
	var nullable string
	err := q.QueryRowContext(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_catalog = current_database() AND table_schema = current_schema() AND table_name = ? AND column_name = ?`, table, column).Scan(&nullable)
	return nullable != "NO", err
}

func (PostgreSQL) ForeignKeyTarget(ctx context.Context, q Querier, table, constraint string) (string, bool, error) {
	var target string
	err := q.QueryRowContext(ctx, `SELECT ccu.table_name FROM information_schema.referential_constraints rc
		JOIN information_schema.table_constraints tc ON tc.constraint_name = rc.constraint_name AND tc.constraint_schema = rc.constraint_schema
		JOIN information_schema.constraint_column_usage ccu ON ccu.constraint_name = rc.unique_constraint_name AND ccu.constraint_schema = rc.unique_constraint_schema
		WHERE rc.constraint_schema = current_schema() AND tc.table_name = ? AND rc.constraint_name = ?`, table, constraint).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return target, err == nil, err
}

// LockSchema takes a session advisory lock, retrying for about ten seconds.
// Unlike MariaDB named locks it is released only explicitly or at session
// end, never by timeout.
func (PostgreSQL) LockSchema(ctx context.Context, q Querier) (bool, func(), error) {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	for i := 0; i < schemaLockTries; i++ {
		var got bool
		if err := q.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(`+schemaLockKey+`)`).Scan(&got); err != nil {
			return false, nil, err
		}
		if got {
			release := func() {
				var released bool
				_ = q.QueryRowContext(context.Background(), `SELECT pg_advisory_unlock(`+schemaLockKey+`)`).Scan(&released)
			}
			return true, release, nil
		}
		timer.Reset(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			return false, nil, ctx.Err()
		case <-timer.C:
		}
	}
	return false, nil, nil
}

// Classify maps SQLSTATE codes. Error detail text is English-only, so an
// unrecognized foreign-key violation stays ErrOther rather than guessing.
func (PostgreSQL) Classify(err error) ErrorKind {
	var dbErr *pgconn.PgError
	if !errors.As(err, &dbErr) {
		return ErrOther
	}
	switch dbErr.Code {
	case pgUniqueViolation:
		return ErrDuplicateKey
	case pgUndefinedTable:
		return ErrMissingTable
	case pgDeadlockDetected:
		return ErrDeadlock
	case pgSerializationFailure:
		return ErrSerializationFailure
	case pgLockNotAvailable:
		return ErrLockTimeout
	case pgForeignKeyViolation:
		switch {
		case strings.Contains(dbErr.Detail, "is still referenced"):
			return ErrForeignKeyInUse
		case strings.Contains(dbErr.Detail, "is not present"):
			return ErrForeignKeyMissingParent
		}
		return ErrOther
	case pgNotNullViolation:
		return ErrNotNull
	case pgStringDataTruncated, pgNumericOutOfRange, pgInvalidTextRepr:
		return ErrValueTooLong
	}
	return ErrOther
}

// Rebind numbers ? placeholders as $1, $2, ... for pgx, skipping string
// literals, quoted identifiers and comments. Inputs never contain $.
func (PostgreSQL) Rebind(query string) string {
	var out strings.Builder
	out.Grow(len(query) + 8)
	number := 0
	i, n := 0, len(query)
	emit := func(from, to int) { out.WriteString(query[from:to]) }
	for i < n {
		c := query[i]
		switch {
		case c == '?':
			// ??, ?| and ?& are JSON operators; anything else is a
			// placeholder. Inputs never use the lone ? exists operator.
			if i+1 < n && (query[i+1] == '?' || query[i+1] == '|' || query[i+1] == '&') {
				emit(i, i+2)
				i += 2
			} else {
				number++
				fmt.Fprintf(&out, "$%d", number)
				i++
			}
		case c == '\'':
			j := i + 1
			escaped := i > 0 && (query[i-1] == 'E' || query[i-1] == 'e')
			for j < n {
				if escaped && query[j] == '\\' {
					j += 2
					continue
				}
				if query[j] == '\'' {
					if j+1 < n && query[j+1] == '\'' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			emit(i, j)
			i = j
		case c == '"':
			j := i + 1
			for j < n {
				if query[j] == '"' {
					if j+1 < n && query[j+1] == '"' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			emit(i, j)
			i = j
		case c == '-' && i+1 < n && query[i+1] == '-':
			j := i + 2
			for j < n && query[j] != '\n' {
				j++
			}
			emit(i, j)
			i = j
		case c == '/' && i+1 < n && query[i+1] == '*':
			j, depth := i+2, 1
			for j < n && depth > 0 {
				if j+1 < n && query[j] == '/' && query[j+1] == '*' {
					depth++
					j += 2
				} else if j+1 < n && query[j] == '*' && query[j+1] == '/' {
					depth--
					j += 2
				} else {
					j++
				}
			}
			emit(i, j)
			i = j
		default:
			emit(i, i+1)
			i++
		}
	}
	return out.String()
}
