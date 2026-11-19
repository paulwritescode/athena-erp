package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/lifecycle"

	"github.com/google/uuid"
)

const (
	defaultLookupSize = 20
	maxLookupSize     = 50
)

// textFieldTypes can label a Record when no label field is set.
var textFieldTypes = map[string]bool{"Text": true, "Long text": true, "Email": true, "Phone": true}

// labelExpression is the SQL that yields the display label of a Record: the
// Record Type's label field, else the document name (when the kernel has
// named it), else the first required text field, else the first text field,
// else the Record ID. It never yields NULL or an empty string. Column names
// come from the saved definition only.
func labelExpression(d recordTypeDefinition) string {
	column := func(key string) string { return dialect.QuoteIdent(key) }
	text := ""
	for _, field := range d.Fields {
		if textFieldTypes[field.Type] && (text == "" || field.Required && !fieldsByKey(d)[text].Required) {
			text = field.Name
		}
	}
	var expr string
	switch {
	case d.LabelField != "":
		expr = column(d.LabelField)
	case d.Submittable && text != "":
		expr = fmt.Sprintf("CASE WHEN %s = %s THEN %s ELSE %s END", column("name"), column("id"), column(text), column("name"))
	case d.Submittable:
		expr = column("name")
	case text != "":
		expr = column(text)
	default:
		expr = column("id")
	}
	return fmt.Sprintf("COALESCE(NULLIF(%s, ''), %s)", expr, column("id"))
}

// lookupStateColumns select a Record's deletion flag and document state.
func lookupStateColumns(d recordTypeDefinition) (deleted, docstate string) {
	deleted, docstate = "0", "0"
	if d.kind() == kindMaster {
		deleted = "CASE WHEN `deleted_at` IS NULL THEN 0 ELSE 1 END"
	}
	if d.Submittable {
		docstate = "COALESCE(`docstate`, 0)"
	}
	return deleted, docstate
}

func (s recordService) LookupRecords(ctx context.Context, req *athenav1.LookupRecordsRequest) (*athenav1.LookupRecordsResponse, error) {
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := rowScopeFor(ctx, s.db, p, recordType, auth.ActionRead)
	if err != nil {
		return nil, err
	}
	var args []any
	scopeWhere := ""
	if !scope.unrestricted {
		scopeWhere = " WHERE " + scope.where
		args = append(args, scope.args...)
	}
	deleted, docstate := lookupStateColumns(recordType)
	inner := fmt.Sprintf("SELECT `id`, %s AS `label`, %s AS `is_deleted`, %s AS `doc_state` FROM %s%s", labelExpression(visibleDefinition(recordType, hiddenFields(p, recordType))), deleted, docstate, dialect.QuoteIdent(recordTypeTableName(recordType.Name)), scopeWhere)
	var where []string
	limit := 0
	if len(req.GetIds()) > 0 {
		if len(req.GetIds()) > maxLookupSize {
			return nil, invalid(fmt.Sprintf("look up at most %d Records at a time", maxLookupSize))
		}
		for _, id := range req.GetIds() {
			if _, err := uuid.Parse(id); err != nil {
				return nil, invalid("ids must be Record IDs")
			}
			args = append(args, id)
		}
		where = append(where, "`id` IN ("+strings.TrimSuffix(strings.Repeat("?,", len(req.GetIds())), ",")+")")
	} else {
		limit = int(req.GetPageSize())
		if limit <= 0 {
			limit = defaultLookupSize
		}
		limit = min(limit, maxLookupSize)
		// Only Records that can be chosen now: not soft-deleted, not cancelled.
		where = append(where, "`is_deleted` = 0", "`doc_state` <> ?")
		args = append(args, int32(lifecycle.Cancelled))
		if q := strings.TrimSpace(req.GetQuery()); q != "" {
			where = append(where, "`label` LIKE ?")
			args = append(args, "%"+likeEscaper.Replace(q)+"%")
		}
		if req.GetPageToken() != "" {
			label, id, err := decodeLookupCursor(req.GetPageToken(), recordType.Name)
			if err != nil {
				return nil, err
			}
			where = append(where, "(`label` > ? OR (`label` = ? AND `id` > ?))")
			args = append(args, label, label, id)
		}
	}
	query := "SELECT `id`, `label`, `is_deleted`, `doc_state` FROM (" + inner + ") AS `r` WHERE " + strings.Join(where, " AND ") + " ORDER BY `label`, `id`"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit+1)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, internalError("look up records", err)
	}
	defer rows.Close()
	res := &athenav1.LookupRecordsResponse{}
	for rows.Next() {
		var item athenav1.LookupItem
		var isDeleted, state int
		if err := rows.Scan(&item.Id, &item.Label, &isDeleted, &state); err != nil {
			return nil, internalError("read lookup", err)
		}
		switch {
		case isDeleted == 1:
			item.State = "deleted"
		case lifecycle.State(state) == lifecycle.Cancelled:
			item.State = "cancelled"
		}
		res.Items = append(res.Items, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("look up records", err)
	}
	if limit > 0 && len(res.Items) > limit {
		res.Items = res.Items[:limit]
		last := res.Items[limit-1]
		res.NextPageToken = encodeLookupCursor(recordType.Name, last.GetLabel(), last.GetId())
	}
	return res, nil
}

// likeEscaper escapes the LIKE wildcards in a search text.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// A lookup pages by label then ID, so its cursor carries both. It shares the
// cursor shape with record lists but a different Record Type tag, so one kind
// of token is never accepted for the other.
func encodeLookupCursor(recordType, label, id string) string {
	return encodeCursor("lookup:"+recordType, label+"\x00"+id)
}

func decodeLookupCursor(token, recordType string) (label, id string, err error) {
	payload, decodeErr := base64.RawURLEncoding.DecodeString(token)
	var c cursor
	if decodeErr != nil || json.Unmarshal(payload, &c) != nil || c.RecordType != "lookup:"+recordType {
		return "", "", invalid("page_token is not valid for this lookup")
	}
	label, id, _ = strings.Cut(c.After, "\x00")
	if _, parseErr := uuid.Parse(id); parseErr != nil {
		return "", "", invalid("page_token is not valid for this lookup")
	}
	return label, id, nil
}

// checkLinkTargets makes every Link value being written point to an existing
// Record of the target Record Type that can still be chosen: not soft-deleted
// and not cancelled. A Record that already holds a link to one that was
// cancelled later keeps it, because callers pass only the values being
// written. The foreign key remains the last guard against a race.
func checkLinkTargets(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, columns []string, args []any) error {
	fields := fieldsByKey(recordType)
	for i, column := range columns {
		field := fields[strings.Trim(column, "`")]
		id, ok := args[i].(string)
		if field.Type != "Link" || !ok {
			continue
		}
		target, _, err := loadRecordType(ctx, tx, field.TargetRecordType)
		if err != nil || target == nil {
			return internalError("load linked Record Type", fmt.Errorf("target %q: %v", field.TargetRecordType, err))
		}
		deleted, docstate := lookupStateColumns(*target)
		var isDeleted, state int
		query := fmt.Sprintf("SELECT %s, %s FROM %s WHERE `id` = ?", deleted, docstate, dialect.QuoteIdent(recordTypeTableName(target.Name)))
		switch err := tx.QueryRowContext(ctx, query, id).Scan(&isDeleted, &state); {
		case errors.Is(err, sql.ErrNoRows):
			return invalid(fmt.Sprintf("%s must be an existing %s record", field.Label, target.Label))
		case err != nil:
			return internalError("check linked record", err)
		}
		if isDeleted == 1 || lifecycle.State(state) == lifecycle.Cancelled {
			return invalid(fmt.Sprintf("%s cannot point to a cancelled or deleted %s record", field.Label, target.Label))
		}
	}
	return nil
}
