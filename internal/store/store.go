// Package store holds the SQL that differs between database engines. Kernel
// code asks a Dialect for statements and classifications instead of writing
// engine syntax itself.
//
// Only MariaDB is tested. PostgreSQL is implemented but untested: its SQL
// strings are unit-tested without a server, and no parity is claimed.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
)

// ErrorKind classifies a database error independently of the engine.
type ErrorKind int

const (
	ErrOther ErrorKind = iota
	ErrDuplicateKey
	ErrDeadlock
	ErrSerializationFailure
	ErrLockTimeout
	ErrForeignKeyMissingParent
	ErrForeignKeyInUse
	ErrNotNull
	ErrValueTooLong
	ErrMissingTable
)

// Column is a column name with its rendered SQL type.
type Column struct {
	Name string
	Type string
}

// Querier is the part of *sql.DB, *sql.Conn and *sql.Tx that dialect lookups need.
type Querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Dialect generates engine-specific SQL. Every method that returns a
// statement takes unquoted identifiers that the caller has already validated.
type Dialect interface {
	// Name is the engine name used in logs and spans.
	Name() string
	// QuoteIdent quotes one identifier.
	QuoteIdent(name string) string
	// MaxIdentLength is the longest identifier this engine accepts.
	MaxIdentLength() int
	// ShortenIdent returns name unchanged when it fits MaxIdentLength, and a
	// deterministic hash-suffixed form otherwise.
	ShortenIdent(name string) string

	// ColumnType maps an Athena field type to a column type.
	ColumnType(fieldType string) (string, error)
	// BigIntType is the column type for 64-bit integers.
	BigIntType() string
	// TimestampType is the column type for UTC timestamps.
	TimestampType() string
	// JSONType is the column type for JSON documents.
	JSONType() string
	// Now is the SQL expression for the current UTC-based timestamp.
	Now() string

	// CreateRecordTypeTable creates a Record Type's table with the system
	// columns (id, revision, created_at, updated_at) followed by nullable
	// columns.
	CreateRecordTypeTable(table string, columns []Column) string
	// CreateMigrationsTable creates athena_migrations (version, name,
	// applied_at) when it is missing.
	CreateMigrationsTable() string
	// AddRevisionColumn adds the revision column to a table that lacks it.
	AddRevisionColumn(table string) string
	// AddColumn adds a nullable column when it is missing.
	AddColumn(table string, column Column) string
	// MakeNullable changes a column to the given type and allows NULL.
	MakeNullable(table string, column Column) string
	// AddIndex creates a single-column index when it is missing.
	AddIndex(table, index, column string, unique bool) string
	// DropIndex removes an index when it exists.
	DropIndex(table, index string) string
	// DropColumn removes a column when it exists.
	DropColumn(table, column string) string
	// AddForeignKey adds a restrictive foreign key to the target's id column.
	AddForeignKey(table, constraint, column, target string) string
	// DropForeignKey removes a constraint when it exists.
	DropForeignKey(table, constraint string) string
	// DropTable removes a table when it exists.
	DropTable(table string) string
	// RenameTable renames a table. Foreign keys that reference it follow the
	// new name but keep their constraint names.
	RenameTable(from, to string) string
	// RenameColumn renames a column and keeps its type, default and nullability.
	RenameColumn(table, from, to string) string
	// RenameIndex renames a secondary index.
	RenameIndex(table, from, to string) string

	// Upsert inserts a row, or updates updateColumns when keyColumn collides.
	Upsert(table string, columns []string, keyColumn string, updateColumns []string) string
	// ForUpdate is the row-lock clause appended to a SELECT inside a transaction.
	ForUpdate() string
	// NullSafeEqual, BinaryString, DecimalParam and DateParam are the pieces
	// the expression translator needs (see internal/expr.Dialect).
	NullSafeEqual(a, b string) string
	BinaryString(expr string) string
	DecimalParam() string
	DateParam() string

	// IncompatibleValues returns a condition that is true for stored values
	// in column that cannot be kept when the column becomes fieldType. For a
	// Link, linkTable is the target table.
	IncompatibleValues(column, fieldType, linkTable string) (string, error)

	// TableExists, ColumnExists, IndexExists, ColumnIsNullable and
	// ForeignKeyTarget read the catalogue of the current database.
	TableExists(ctx context.Context, q Querier, table string) (bool, error)
	ColumnExists(ctx context.Context, q Querier, table, column string) (bool, error)
	IndexExists(ctx context.Context, q Querier, table, index string) (bool, error)
	ColumnIsNullable(ctx context.Context, q Querier, table, column string) (bool, error)
	// ForeignKeyTarget returns the table a constraint references; found is
	// false when the constraint does not exist.
	ForeignKeyTarget(ctx context.Context, q Querier, table, constraint string) (target string, found bool, err error)

	// LockSchema takes the schema-wide lock for the current database. It
	// returns false when another session holds it after the wait. release
	// must be called on the same session.
	LockSchema(ctx context.Context, q Querier) (acquired bool, release func(), err error)

	// Rebind rewrites ? placeholders for the engine's driver. MariaDB
	// passes the statement through; PostgreSQL numbers them $1, $2, ...
	// for pgx. Callers apply it where statements meet arguments; during
	// the parity run, not before.
	Rebind(query string) string

	// Classify maps a driver error to an ErrorKind.
	Classify(err error) ErrorKind
}

// GeneratedIdentLimit is the longest identifier Athena generates on any
// engine. PostgreSQL stops at 63 bytes and MariaDB at 64, so a name that is
// valid here is valid on both. Dialect.MaxIdentLength stays the engine fact.
const GeneratedIdentLimit = 63

// TreePathColumnType holds a materialised tree path (`/root/child/`) on a
// Record Type table. Both engines accept this spelling; MariaDB is the only
// tested one.
const TreePathColumnType = "VARCHAR(2048)"

// ShortenIdentifier returns name unchanged when it is at most limit bytes.
// Otherwise it keeps a prefix and appends "_" and 12 hex characters of the
// SHA-256 of the full name, so the result is deterministic and exactly limit
// bytes long. limit must be at least 20.
func ShortenIdentifier(name string, limit int) string {
	if len(name) <= limit {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	suffix := "_" + hex.EncodeToString(sum[:6])
	return fmt.Sprintf("%s%s", name[:limit-len(suffix)], suffix)
}
