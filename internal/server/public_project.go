package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect/v2"
)

// This file holds the shared public-projection machinery from ADR-0030: each
// public page reads only through publicRead with a fixed Record Type and a
// fixed field allowlist, over direct store queries. Projections never call
// RecordService, never evaluate Grants, and never read columns outside the
// allowlist.

// publicTextTypes are the field types a text slot accepts.
var publicTextTypes = map[string]bool{"Text": true, "Long text": true}

// publicListLimit caps every public listing. Catalogues stay small in the
// beta; paging arrives when a Pack outgrows it.
const publicListLimit = 200

// publicSlot is one allowlisted field: its key and the expected kind (text,
// checkbox, money, image, link or date).
type publicSlot struct {
	Key  string
	Kind string
}

// publicRow is one published record with only allowlisted values decoded. ID
// resolves links internally and is never rendered.
type publicRow struct {
	ID      string
	Updated time.Time
	Text    map[string]string
	Money   map[string]int64
	Images  map[string]imageReference
}

// publicFilter selects published rows: Slug matches one row exactly (the
// oldest ID wins when slugs collide), Token matches the verification_token
// column exactly (transcript verification only), Limit caps a listing.
type publicFilter struct {
	Slug  string
	Token string
	Limit int
}

// publicRead returns the published rows of typeKey, selecting only the
// allowlisted fields. Every required slot must exist with a usable type; a
// missing type, a kind other than master or document, or a missing required
// slot yields no rows (the type publishes nothing). Optional slots are read
// when present and correctly typed, skipped otherwise.
//
// A row is published when its is_published checkbox is set and the row is
// live (masters: not soft-deleted; documents: not cancelled). Table and
// column names come from the saved definition, never from the request.
func publicRead(ctx context.Context, db *sql.DB, typeKey string, required, optional []publicSlot, filter publicFilter) ([]publicRow, *recordTypeDefinition, error) {
	if db == nil {
		return nil, nil, nil
	}
	def, err := loadRecordTypeFrom(ctx, db, typeKey)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	if def.kind() != kindMaster && def.kind() != kindDocument {
		return nil, nil, nil
	}
	fields := fieldsByKey(def)
	usable := func(s publicSlot) bool {
		f, ok := fields[s.Key]
		if !ok {
			// Submittable documents reserve the name key: the kernel
			// names each document, and projections read that column.
			return s.Key == "name" && s.Kind == "text" && def.kind() == kindDocument && def.Submittable
		}
		switch s.Kind {
		case "text":
			return publicTextTypes[f.Type]
		case "checkbox":
			return f.Type == "Checkbox"
		case "money":
			return f.Type == "Money"
		case "image":
			return f.Type == "Image"
		case "link":
			return f.Type == "Link"
		case "date":
			return f.Type == "Date"
		}
		return false
	}
	for _, s := range required {
		if !usable(s) {
			return nil, nil, nil
		}
	}
	slots := append([]publicSlot{}, required...)
	for _, s := range optional {
		if usable(s) {
			slots = append(slots, s)
		}
	}
	columns := []string{dialect.QuoteIdent("id"), dialect.QuoteIdent("updated_at")}
	for _, s := range slots {
		columns = append(columns, dialect.QuoteIdent(s.Key))
	}
	where := dialect.QuoteIdent("is_published") + " = TRUE"
	if def.kind() == kindMaster {
		where += " AND " + dialect.QuoteIdent("deleted_at") + " IS NULL"
	} else if def.Submittable {
		// Only submittable documents carry docstate, and submittable can
		// only turn off while every row is a draft, so this filter is exact.
		where += " AND (" + dialect.QuoteIdent("docstate") + " IS NULL OR " + dialect.QuoteIdent("docstate") + " <> 2)"
	}
	var args []any
	if filter.Slug != "" {
		where += " AND " + dialect.QuoteIdent("slug") + " = ?"
		args = append(args, filter.Slug)
	}
	if filter.Token != "" {
		// The column name is fixed here, never from the request; the
		// caller pairs this filter with a verification_token slot, so
		// the required-slot check above guarantees the column exists.
		where += " AND " + dialect.QuoteIdent("verification_token") + " = ?"
		args = append(args, filter.Token)
	}
	limit := filter.Limit
	if limit <= 0 || limit > publicListLimit {
		limit = publicListLimit
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d", strings.Join(columns, ","), dialect.QuoteIdent(recordTypeTableName(def.Name)), where, dialect.QuoteIdent("id"), limit)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, internalError("read published "+typeKey, err)
	}
	defer rows.Close()
	var out []publicRow
	for rows.Next() {
		row := publicRow{Text: map[string]string{}, Money: map[string]int64{}, Images: map[string]imageReference{}}
		targets := []any{&row.ID, &row.Updated}
		values := make([]any, len(slots))
		for i, s := range slots {
			switch s.Kind {
			case "checkbox":
				values[i] = new(sql.NullBool)
			case "money":
				values[i] = new(sql.NullInt64)
			case "date":
				values[i] = new(sql.NullTime)
			default:
				values[i] = new(sql.NullString)
			}
			targets = append(targets, values[i])
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, nil, internalError("read published "+typeKey, err)
		}
		for i, s := range slots {
			switch s.Kind {
			case "checkbox":
				if v := values[i].(*sql.NullBool); v.Valid && v.Bool {
					row.Text[s.Key] = "true"
				}
			case "money":
				if v := values[i].(*sql.NullInt64); v.Valid {
					row.Money[s.Key] = v.Int64
				}
			case "date":
				if v := values[i].(*sql.NullTime); v.Valid {
					row.Text[s.Key] = v.Time.Format(time.DateOnly)
				}
			case "image":
				if v := values[i].(*sql.NullString); v.Valid && v.String != "" {
					var ref imageReference
					if err := json.Unmarshal([]byte(v.String), &ref); err != nil {
						return nil, nil, internalError("read image reference", err)
					}
					row.Images[s.Key] = ref
				}
			default:
				if v := values[i].(*sql.NullString); v.Valid {
					row.Text[s.Key] = v.String
				}
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, internalError("read published "+typeKey, err)
	}
	return out, &def, nil
}

// publicMoney renders minor units through the field's currency, as
// RecordService does. It returns "" when the field holds no currency.
func publicMoney(def *recordTypeDefinition, key string, minor int64) (text, currency string) {
	if def == nil {
		return "", ""
	}
	fields := fieldsByKey(*def)
	f, ok := fields[key]
	if !ok || f.Currency == "" {
		return "", ""
	}
	return moneyText(f, minor), f.Currency
}

// publicSnippet collapses whitespace and cuts text at max runes on a word
// boundary, for meta descriptions from projection fields.
func publicSnippet(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := r[:max]
	if i := strings.LastIndex(string(cut), " "); i > 0 {
		cut = cut[:i]
	}
	return string(cut)
}
