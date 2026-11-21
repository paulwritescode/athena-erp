package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/audit"
	"athena-erp/internal/auth"

	"connectrpc.com/connect/v2"
)

// The permission gate (gate.go) checks that a caller may use an operation on a
// Record Type. The helpers here apply the rest of a Grant inside the handler,
// where the Records are: row conditions, separate duties and field levels.
// The CLI opens the database directly and acts as the operator, so it has no
// principal and no limits; a request that came through the API without a
// principal is refused, never treated as the operator.

// actorName names who makes a change: the signed-in username, or the
// operator for the athena command line.
func actorName(ctx context.Context) string {
	if p := auth.PrincipalFrom(ctx); p != nil {
		return p.Username
	}
	return localActor
}

// displayActor names who makes a change for people to read: the signed-in
// display name (falling back to the username), or the operator for the
// athena command line.
func displayActor(ctx context.Context) string {
	if p := auth.PrincipalFrom(ctx); p != nil {
		if p.DisplayName != "" {
			return p.DisplayName
		}
		return p.Username
	}
	return localActor
}

// accessFor returns the principal acting in ctx, or nil for the operator.
func accessFor(ctx context.Context) (*auth.Principal, error) {
	if p := auth.PrincipalFrom(ctx); p != nil {
		return p, nil
	}
	if _, source := requestFrom(ctx); source == sourceAPI {
		return nil, connect.NewError(connect.CodeUnauthenticated, "sign in to continue")
	}
	return nil, nil
}

// requireSystem returns the acting principal after checking a system
// permission. The gate already did so for API calls; this keeps a handler safe
// if it is ever reached another way.
func requireSystem(ctx context.Context, permission string) (*auth.Principal, error) {
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil && !p.HasSystem(permission) {
		return nil, connect.NewError(connect.CodePermissionDenied, "you do not have permission to do this")
	}
	return p, nil
}

// userNamePattern is the placeholder a row condition uses for the signed-in
// username. Usernames hold only letters, digits, dot, dash and underscore, so
// substituting one as a string literal cannot change the expression.
var userNamePattern = regexp.MustCompile(`\buser\.name\b`)

func bindUser(condition, username string) string {
	return userNamePattern.ReplaceAllLiteralString(condition, `"`+username+`"`)
}

// levelOf is the highest field permission level of p on the Record Type. The
// operator has every level.
func levelOf(p *auth.Principal, recordType string) int {
	if p == nil {
		return auth.MaxPermissionLevel
	}
	return p.Level(recordType)
}

// hiddenFields lists the fields of the Record Type above p's permission level.
func hiddenFields(p *auth.Principal, d recordTypeDefinition) map[string]bool {
	hidden := map[string]bool{}
	level := levelOf(p, d.Name)
	for _, field := range d.Fields {
		if field.PermissionLevel > level {
			hidden[field.Name] = true
		}
	}
	return hidden
}

// maskRecord removes the values of fields the principal may not see.
func maskRecord(hidden map[string]bool, record *athenav1.Record) {
	for key := range hidden {
		delete(record.GetValues(), key)
	}
}

// checkWritable refuses a request that sets a field above the principal's level.
func checkWritable(hidden map[string]bool, d recordTypeDefinition, values map[string]*athenav1.FieldValue) error {
	for _, field := range d.Fields {
		if _, set := values[field.Name]; set && hidden[field.Name] {
			return connect.NewError(connect.CodePermissionDenied, fmt.Sprintf("you do not have permission to change %s", field.Label))
		}
	}
	return nil
}

// rowScope is the SQL predicate that limits the Records a principal reaches
// with one action. unrestricted means no limit.
type rowScope struct {
	unrestricted bool
	where        string
	args         []any
}

// rowScopeFor compiles one disjunct per whole Assignment that allows action:
// the Grant's row condition together with the Assignment's Scope. A disjunct
// that no longer compiles (a field was removed) allows nothing, so a stale
// Grant fails closed.
func rowScopeFor(ctx context.Context, q querier, p *auth.Principal, d recordTypeDefinition, action string) (rowScope, error) {
	if p == nil {
		return rowScope{unrestricted: true}, nil
	}
	disjuncts, unrestricted := p.Scope(d.Name, action)
	if unrestricted {
		return rowScope{unrestricted: true}, nil
	}
	var parts []string
	var args []any
	for _, disjunct := range disjuncts {
		source := andExpr(bindUser(disjunct.Condition, p.Username), disjunct.Scope.Expression())
		predicate, err := filterPredicate(ctx, q, d, source)
		if err != nil {
			continue
		}
		parts = append(parts, predicate.Where)
		args = append(args, predicate.Args...)
	}
	if len(parts) == 0 {
		return rowScope{where: "1 = 0"}, nil
	}
	return rowScope{where: "(" + strings.Join(parts, ") OR (") + ")", args: args}, nil
}

// andExpr conjoins two filter expressions, either of which may be empty.
func andExpr(condition, scope string) string {
	condition, scope = strings.TrimSpace(condition), strings.TrimSpace(scope)
	if condition == "" {
		return scope
	}
	if scope == "" || scope == "true" {
		return condition
	}
	return "(" + condition + ") && (" + scope + ")"
}

// authorizeRecord checks that p may take action on one existing Record. A
// Record outside the read scope looks missing; one that is readable but outside
// the scope of the action is refused.
func authorizeRecord(ctx context.Context, q querier, p *auth.Principal, d recordTypeDefinition, id, action string) error {
	actions := []string{auth.ActionRead}
	if action != auth.ActionRead {
		actions = append(actions, action)
	}
	for _, a := range actions {
		scope, err := rowScopeFor(ctx, q, p, d, a)
		if err != nil {
			return err
		}
		if scope.unrestricted {
			continue
		}
		rows, err := q.QueryContext(ctx, "SELECT 1 FROM "+dialect.QuoteIdent(recordTypeTableName(d.Name))+" WHERE `id` = ? AND ("+scope.where+") LIMIT 1", append([]any{id}, scope.args...)...)
		if err != nil {
			return internalError("check record access", err)
		}
		found := rows.Next()
		rows.Close()
		if !found {
			if a == auth.ActionRead {
				return connect.NewError(connect.CodeNotFound, "the record does not exist")
			}
			return connect.NewError(connect.CodePermissionDenied, fmt.Sprintf("you do not have permission to %s this record", action))
		}
	}
	return nil
}

// authorizeProposed rechecks an updated Record after its new values are
// written in the transaction: the proposed row must satisfy the Assignments
// too, so moving a Record out of scope is refused. A miss is always a
// denial, never a missing Record: the row exists, the change is refused.
func authorizeProposed(ctx context.Context, q querier, p *auth.Principal, d recordTypeDefinition, id, action string) error {
	if err := authorizeRecord(ctx, q, p, d, id, action); err != nil {
		var ce *connect.Error
		if errors.As(err, &ce) && (ce.Code() == connect.CodeNotFound || ce.Code() == connect.CodePermissionDenied) {
			return connect.NewError(connect.CodePermissionDenied, "you do not have permission to change this record this way")
		}
		return err
	}
	return nil
}

// checkSeparateDuties refuses an action on a Record the principal created,
// when every Grant that allows the action demands separate duties.
func checkSeparateDuties(p *auth.Principal, d recordTypeDefinition, action string, record *athenav1.Record) error {
	if p != nil && p.MustSeparateDuties(d.Name, action) && record.GetCreatedBy() == p.Username {
		return connect.NewError(connect.CodePermissionDenied, fmt.Sprintf("you created this record, so someone else must %s it", action))
	}
	return nil
}

// maskChanges hides the values of Audit Log changes to fields above the
// principal's level. The stored row, and so its hash, is untouched.
func maskChanges(hidden map[string]bool, changes []audit.Change) []*athenav1.FieldChange {
	out := make([]*athenav1.FieldChange, 0, len(changes))
	for _, change := range changes {
		if hidden[change.Field] {
			out = append(out, &athenav1.FieldChange{Field: change.Field, Masked: true})
			continue
		}
		out = append(out, &athenav1.FieldChange{Field: change.Field, OldValue: change.Old, NewValue: change.New})
	}
	return out
}

// maskSnapshot removes hidden field values from a Document Version snapshot.
// children maps a child Record Type to its own hidden fields.
func maskSnapshot(doc map[string]any, hidden map[string]bool, children map[string]map[string]bool) {
	if values, ok := doc["values"].(map[string]any); ok {
		for key := range hidden {
			delete(values, key)
		}
	}
	lines, _ := doc["children"].(map[string]any)
	for name, items := range lines {
		maskLines(items, children[name])
	}
}

func maskLines(items any, hidden map[string]bool) {
	list, _ := items.([]any)
	for _, item := range list {
		if line, ok := item.(map[string]any); ok {
			if values, ok := line["values"].(map[string]any); ok {
				for key := range hidden {
					delete(values, key)
				}
			}
		}
	}
}

// maskDiff removes hidden field values from a Document Version diff.
func maskDiff(diff map[string]any, hidden map[string]bool, children map[string]map[string]bool) {
	for key := range hidden {
		delete(diff, key)
	}
	for key, entry := range diff {
		name, ok := strings.CutPrefix(key, "children.")
		if !ok {
			continue
		}
		if sides, ok := entry.(map[string]any); ok {
			maskLines(sides["from"], children[name])
			maskLines(sides["to"], children[name])
		}
	}
}

// hiddenWithChildren returns the hidden fields of d and of each of its child
// Record Types, for masking document snapshots.
func hiddenWithChildren(ctx context.Context, q querier, p *auth.Principal, d recordTypeDefinition) (map[string]bool, map[string]map[string]bool, error) {
	hidden := hiddenFields(p, d)
	if p == nil {
		return hidden, nil, nil
	}
	children, err := childRecordTypes(ctx, q, d.Name)
	if err != nil {
		return nil, nil, err
	}
	childHidden := map[string]map[string]bool{}
	for _, child := range children {
		if h := hiddenFields(p, child); len(h) > 0 {
			childHidden[child.Name] = h
		}
	}
	return hidden, childHidden, nil
}

// maskVersion removes hidden values from a stored snapshot and diff.
func maskVersion(snapshot, diff []byte, hidden map[string]bool, children map[string]map[string]bool) ([]byte, []byte, error) {
	var doc, changes map[string]any
	if err := json.Unmarshal(snapshot, &doc); err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(diff, &changes); err != nil {
		return nil, nil, err
	}
	maskSnapshot(doc, hidden, children)
	maskDiff(changes, hidden, children)
	maskedSnapshot, err := json.Marshal(doc)
	if err != nil {
		return nil, nil, err
	}
	maskedDiff, err := json.Marshal(changes)
	return maskedSnapshot, maskedDiff, err
}

// visibleDefinition returns d without the fields in hidden, so an expression,
// filter or label built from it cannot read a value the caller may not see.
func visibleDefinition(d recordTypeDefinition, hidden map[string]bool) recordTypeDefinition {
	if len(hidden) == 0 {
		return d
	}
	fields := make([]recordTypeField, 0, len(d.Fields))
	for _, field := range d.Fields {
		if !hidden[field.Name] {
			fields = append(fields, field)
		}
	}
	d.Fields = fields
	if hidden[d.LabelField] {
		d.LabelField = ""
	}
	if d.Layout != nil {
		d.Layout = visibleLayout(d.Layout, hidden)
	}
	return d
}

// visibleLayout drops the layout blocks of hidden fields, and the sections
// left with nothing in them, so a filtered definition never names a field
// the caller may not see.
func visibleLayout(layout *formLayout, hidden map[string]bool) *formLayout {
	out := &formLayout{}
	for _, section := range layout.Sections {
		kept := formSection{Title: section.Title, Description: section.Description, Columns: section.Columns, Visibility: section.Visibility}
		for _, item := range section.Items {
			if item.Field != nil && hidden[item.Field.FieldKey] {
				continue
			}
			kept.Items = append(kept.Items, item)
		}
		if len(kept.Items) > 0 {
			out.Sections = append(out.Sections, kept)
		}
	}
	if len(out.Sections) == 0 {
		return nil
	}
	return out
}
