package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/audit"
	"athena-erp/internal/numbering"
	"athena-erp/internal/store"

	"connectrpc.com/connect/v2"
	"github.com/google/uuid"
)

// maxNameLength is the longest document name, matching the Text column.
const maxNameLength = 140

// labelFieldTypes are the field types that can label a Record in a picker.
var labelFieldTypes = map[string]bool{"Text": true, "Long text": true, "Email": true, "Phone": true, "Select": true, "Enum": true, "Date": true}

// validateNaming checks the naming and label settings of a definition.
func validateNaming(d recordTypeDefinition) error {
	if d.NamingSeries != "" {
		if !d.Submittable {
			return invalid("a naming series needs a submittable document")
		}
		if _, err := numbering.Parse(d.NamingSeries); err != nil {
			return invalid(err.Error())
		}
	}
	if d.GapFree && d.NamingSeries == "" {
		return invalid("gap-free numbering needs a naming series")
	}
	if d.AllowManualName && !d.Submittable {
		return invalid("manual names need a submittable document")
	}
	if d.LabelField != "" {
		field, ok := fieldsByKey(d)[d.LabelField]
		if !ok {
			return invalid(fmt.Sprintf("the label field %q is not a field of this Record Type", d.LabelField))
		}
		if !labelFieldTypes[field.Type] {
			return invalid(fmt.Sprintf("%s cannot label Records; use a text, choice or date field", field.Label))
		}
	}
	return nil
}

// assignSeriesName gives a draft the next name of its series when it is
// submitted. A gap-free series draws its number inside tx, so a rolled-back
// submit returns it. Other series draw in their own transaction. A document
// that already has a name of its own (a manual name or an amendment) keeps it:
// a name never changes once it is set. It returns "" when nothing was
// assigned.
func (s recordService) assignSeriesName(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, current *athenav1.Record) (string, error) {
	if recordType.NamingSeries == "" || current.GetName() != current.GetId() || current.GetAmendedFrom() != "" {
		return "", nil
	}
	pattern, err := numbering.Parse(recordType.NamingSeries)
	if err != nil {
		return "", internalError("read naming series", err)
	}
	now := time.Now().UTC()
	scope := pattern.Scope(now)
	var n int64
	if recordType.GapFree {
		if err = numbering.Ensure(ctx, s.db, dialect, recordType.Name, scope); err != nil {
			return "", internalError("take the next number", err)
		}
		n, err = numbering.Next(ctx, tx, dialect, recordType.Name, scope)
	} else {
		n, err = numbering.NextIndependent(ctx, s.db, dialect, recordType.Name, scope)
	}
	if err != nil {
		return "", internalError("take the next number", err)
	}
	name := pattern.Format(now, n)
	if _, err := tx.ExecContext(ctx, "UPDATE "+dialect.QuoteIdent(recordTypeTableName(recordType.Name))+" SET `name` = ? WHERE `id` = ?", name, current.GetId()); err != nil {
		if dialect.Classify(err) == store.ErrDuplicateKey {
			return "", connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("the next number %s is already used by another document; the series cannot continue until that name is changed or removed", name))
		}
		return "", writeError("name document", err)
	}
	return name, nil
}

// manualName validates a name the caller supplied on create and returns it.
func manualName(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, requested string) (string, error) {
	name := strings.TrimSpace(requested)
	if name == "" {
		return "", nil
	}
	if !recordType.AllowManualName {
		return "", invalid(fmt.Sprintf("%s does not allow a manual name", recordType.Label))
	}
	if len(name) > maxNameLength {
		return "", invalid(fmt.Sprintf("a name has at most %d characters", maxNameLength))
	}
	if _, err := uuid.Parse(name); err == nil {
		return "", invalid("a manual name cannot look like a Record ID")
	}
	var taken bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+dialect.QuoteIdent(recordTypeTableName(recordType.Name))+" WHERE `name` = ?)", name).Scan(&taken); err != nil {
		return "", internalError("check name", err)
	}
	if taken {
		return "", invalid(fmt.Sprintf("the name %s is already used by another document", name))
	}
	return name, nil
}

// auditNameOverride logs a manual name next to the create row.
func auditNameOverride(ctx context.Context, tx *sql.Tx, recordTypeName, id, name string) error {
	return writeAudit(ctx, tx, recordTypeName, id, 1, "name_override", []audit.Change{{Field: "name", New: &name}})
}
