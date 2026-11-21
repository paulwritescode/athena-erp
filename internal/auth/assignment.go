package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Assignment is one Role given to one user with its own Scope, active-from
// date and expiry date. Access is evaluated per whole Assignment: an action
// on a row is allowed only when one single Assignment covers it with a Grant,
// a true row condition, a matching Scope and a sufficient field level
// (ADR-0017). Results of whole matching Assignments union; the Grants and
// Scopes of different Assignments never combine.
type Assignment struct {
	ID       string
	UserID   string
	RoleID   string
	RoleName string
	// Scope maps a dimension name to the allowed values, for example
	// {"company": ["acme"]}. A dimension names a Record Type field; a row
	// matches when every dimension's field holds one of its values.
	Scope Scope
	// ActiveFrom and ExpiresAt bound the Assignment to whole days. Nil
	// means unbounded. Active is computed at load: an Assignment outside
	// its dates is absent.
	ActiveFrom *time.Time
	ExpiresAt  *time.Time
	Active     bool
	Revision   int64
	// SystemPermissions, RequiresScope and Grants come from the Role.
	SystemPermissions []string
	RequiresScope     []string
	Grants            []Grant
}

// AssignmentActive reports whether the dates cover today. Both ends are
// inclusive: the Assignment works from its active-from day through its
// expiry day, compared as whole days in UTC.
func AssignmentActive(from, expires *time.Time, today time.Time) bool {
	y, m, d := today.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	if from != nil {
		fy, fm, fd := from.Date()
		if start.Before(time.Date(fy, fm, fd, 0, 0, 0, 0, time.UTC)) {
			return false
		}
	}
	if expires != nil {
		ey, em, ed := expires.Date()
		if start.After(time.Date(ey, em, ed, 0, 0, 0, 0, time.UTC)) {
			return false
		}
	}
	return true
}

// Scope maps dimension names to allowed values. The zero value matches every
// row.
type Scope map[string][]string

// Satisfies reports whether the Scope gives every required dimension at
// least one value. A Role that requires a Scope dimension denies an
// Assignment that gives none.
func (s Scope) Satisfies(required []string) bool {
	for _, dimension := range required {
		if len(s[dimension]) == 0 {
			return false
		}
	}
	return true
}

// Matches reports whether the row falls inside the Scope. Row maps field
// names to values; every dimension with values must hold one of them. A
// dimension the row lacks never matches, and an empty Scope matches every
// row.
func (s Scope) Matches(row map[string]string) bool {
	for dimension, values := range s {
		if len(values) == 0 {
			return false
		}
		got, ok := row[dimension]
		if !ok {
			return false
		}
		matched := false
		for _, want := range values {
			if got == want {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// dimensionPattern is the shape of a Scope dimension: a Record Type field
// key. Anything else cannot name a field, so it matches nothing.
var dimensionPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// escapeCELString quotes a value as a CEL string literal.
func escapeCELString(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			out.WriteString(`\\`)
		case '"':
			out.WriteString(`\"`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&out, `\u%04x`, r)
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
	return out.String()
}

// Expression renders the Scope as a filter expression over doc fields: every
// dimension must hold one of its values. Dimensions compare as strings; a
// dimension on a non-text field never matches. An empty Scope renders "true".
// An invalid dimension or an empty value list renders "false", denying the
// disjunct it guards.
func (s Scope) Expression() string {
	if len(s) == 0 {
		return "true"
	}
	dimensions := make([]string, 0, len(s))
	for dimension := range s {
		dimensions = append(dimensions, dimension)
	}
	sort.Strings(dimensions)
	parts := make([]string, 0, len(dimensions))
	for _, dimension := range dimensions {
		values := s[dimension]
		if !dimensionPattern.MatchString(dimension) || len(values) == 0 {
			return "false"
		}
		quoted := make([]string, 0, len(values))
		for _, value := range values {
			quoted = append(quoted, "doc."+dimension+" == "+escapeCELString(value))
		}
		sort.Strings(quoted)
		parts = append(parts, "("+strings.Join(quoted, " || ")+")")
	}
	return strings.Join(parts, " && ")
}

// canonical renders the Scope deterministically for sorting and tests.
func (s Scope) canonical() string {
	if len(s) == 0 {
		return ""
	}
	dimensions := make([]string, 0, len(s))
	for dimension := range s {
		dimensions = append(dimensions, dimension)
	}
	sort.Strings(dimensions)
	var out strings.Builder
	for _, dimension := range dimensions {
		values := append([]string{}, s[dimension]...)
		sort.Strings(values)
		fmt.Fprintf(&out, "%s=%s;", dimension, strings.Join(values, ","))
	}
	return out.String()
}

// ScopeDisjunct is one whole Assignment's contribution to a Scope: the
// Grant's row condition together with the Assignment's Scope. A row matches
// when both hold; the results of whole matching disjuncts union.
type ScopeDisjunct struct {
	Condition string
	Scope     Scope
}

// usable reports whether the Assignment can allow anything: it is inside its
// dates and its Scope satisfies the Role's required dimensions.
func (a Assignment) usable() bool {
	return a.Active && a.Scope.Satisfies(a.RequiresScope)
}

// AssignmentInput describes an Assignment to save. An empty ID creates one;
// otherwise the Assignment is replaced when its revision still matches
// ExpectedRevision.
type AssignmentInput struct {
	ID               string
	Role             string
	Scope            Scope
	ActiveFrom       *time.Time
	ExpiresAt        *time.Time
	ExpectedRevision int64
}

// Assignment bounds keep a Scope small enough to compile into SQL.
const (
	maxScopeDimensions = 32
	maxScopeValues     = 256
	maxScopeValueBytes = 256
)

// parseScope checks stored Scope JSON. A Scope that cannot be read matches
// nothing, so the Assignment is skipped by the caller.
func parseScope(payload []byte) (Scope, error) {
	scope := Scope{}
	if err := json.Unmarshal(payload, &scope); err != nil {
		return nil, err
	}
	return scope, nil
}

// checkScope validates an Assignment Scope for saving.
func checkScope(scope Scope) error {
	if len(scope) > maxScopeDimensions {
		return fmt.Errorf("%w: a Scope holds at most %d dimensions", ErrInvalid, maxScopeDimensions)
	}
	for dimension, values := range scope {
		if !dimensionPattern.MatchString(dimension) {
			return fmt.Errorf("%w: Scope dimension %q is not a field key", ErrInvalid, dimension)
		}
		if len(values) > maxScopeValues {
			return fmt.Errorf("%w: Scope dimension %q holds at most %d values", ErrInvalid, dimension, maxScopeValues)
		}
		for _, value := range values {
			if len(value) == 0 || len(value) > maxScopeValueBytes {
				return fmt.Errorf("%w: a Scope value needs 1 to %d bytes", ErrInvalid, maxScopeValueBytes)
			}
		}
	}
	return nil
}

// day truncates a time to its UTC day for DATE columns and comparisons.
func day(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// SaveAssignment creates an Assignment or replaces one whose revision still
// matches. It refuses a change beyond the actor's delegation and a change
// that would leave no active owner, both in the same transaction as the
// write. A clash with a concurrent access change retries the transaction.
// It returns the stored Assignment with the Role's Grants.
func (s *Service) SaveAssignment(ctx context.Context, username string, in AssignmentInput) (Assignment, error) {
	var out Assignment
	err := retryGuarded(func() error {
		var err error
		out, err = s.saveAssignmentOnce(ctx, username, in)
		return err
	})
	return out, err
}

func (s *Service) saveAssignmentOnce(ctx context.Context, username string, in AssignmentInput) (Assignment, error) {
	scope := in.Scope
	if scope == nil {
		scope = Scope{}
	}
	if err := checkScope(scope); err != nil {
		return Assignment{}, err
	}
	var from, expires any
	if in.ActiveFrom != nil {
		from = day(*in.ActiveFrom)
	}
	if in.ExpiresAt != nil {
		expires = day(*in.ExpiresAt)
	}
	if from != nil && expires != nil && from.(time.Time).After(expires.(time.Time)) {
		return Assignment{}, fmt.Errorf("%w: the active-from date is after the expiry date", ErrInvalid)
	}
	payload, err := json.Marshal(scope)
	if err != nil {
		return Assignment{}, fmt.Errorf("save assignment: %w", err)
	}
	tx, err := beginGuardedTx(ctx, s.DB)
	if err != nil {
		return Assignment{}, fmt.Errorf("save assignment: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	today := s.now().Format("2006-01-02")
	before, err := beginOwnershipGuard(ctx, tx, today)
	if err != nil {
		return Assignment{}, err
	}
	user, err := s.userByName(ctx, tx, username, "")
	if err != nil {
		return Assignment{}, err
	}
	role, err := s.roleByName(ctx, tx, in.Role)
	if err != nil {
		return Assignment{}, err
	}
	if err := checkDelegation(PrincipalFrom(ctx), role, scope); err != nil {
		return Assignment{}, err
	}
	out := Assignment{UserID: user.ID, RoleID: role.ID, RoleName: role.Name, Scope: scope, Revision: 1,
		SystemPermissions: role.SystemPermissions, RequiresScope: role.RequiresScope, Grants: role.Grants}
	if in.ActiveFrom != nil {
		at := from.(time.Time)
		out.ActiveFrom = &at
	}
	if in.ExpiresAt != nil {
		at := expires.(time.Time)
		out.ExpiresAt = &at
	}
	if in.ID == "" {
		out.ID = newID()
		if _, err := tx.ExecContext(ctx, "INSERT INTO athena_role_assignments (id, user_id, role_id, scope, active_from, expires_at, revision) VALUES (?,?,?,?,?,?,1)",
			out.ID, user.ID, out.RoleID, payload, from, expires); err != nil {
			return Assignment{}, fmt.Errorf("save assignment: %w", err)
		}
	} else {
		res, err := tx.ExecContext(ctx, "UPDATE athena_role_assignments SET role_id = ?, scope = ?, active_from = ?, expires_at = ?, revision = revision + 1 WHERE id = ? AND user_id = ? AND revision = ?",
			out.RoleID, payload, from, expires, in.ID, user.ID, in.ExpectedRevision)
		if err != nil {
			return Assignment{}, fmt.Errorf("save assignment: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return Assignment{}, fmt.Errorf("save assignment: %w", err)
		}
		if n == 0 {
			var exists bool
			if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM athena_role_assignments WHERE id = ? AND user_id = ?)", in.ID, user.ID).Scan(&exists); err != nil {
				return Assignment{}, fmt.Errorf("save assignment: %w", err)
			}
			if !exists {
				return Assignment{}, fmt.Errorf("%w: assignment", ErrNotFound)
			}
			return Assignment{}, ErrConflict
		}
		out.ID = in.ID
		out.Revision = in.ExpectedRevision + 1
	}
	if err := syncOwnerFlag(ctx, tx, user.ID); err != nil {
		return Assignment{}, err
	}
	if err := endOwnershipGuard(ctx, tx, today, before); err != nil {
		return Assignment{}, err
	}
	if err := notifyAccess(ctx, tx, user.ID); err != nil {
		return Assignment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Assignment{}, fmt.Errorf("save assignment: %w", err)
	}
	out.Active = AssignmentActive(out.ActiveFrom, out.ExpiresAt, s.now())
	return out, nil
}

// RevokeAssignment deletes one Assignment. It refuses to leave no active
// owner. Revoking only removes access, so it needs no delegation check.
func (s *Service) RevokeAssignment(ctx context.Context, id string) error {
	return retryGuarded(func() error { return s.revokeAssignmentOnce(ctx, id) })
}

func (s *Service) revokeAssignmentOnce(ctx context.Context, id string) error {
	tx, err := beginGuardedTx(ctx, s.DB)
	if err != nil {
		return fmt.Errorf("revoke assignment: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	today := s.now().Format("2006-01-02")
	before, err := beginOwnershipGuard(ctx, tx, today)
	if err != nil {
		return err
	}
	var userID string
	if err := tx.QueryRowContext(ctx, "SELECT user_id FROM athena_role_assignments WHERE id = ?", id).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: assignment", ErrNotFound)
		}
		return fmt.Errorf("revoke assignment: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM athena_role_assignments WHERE id = ?", id); err != nil {
		return fmt.Errorf("revoke assignment: %w", err)
	}
	if err := syncOwnerFlag(ctx, tx, userID); err != nil {
		return err
	}
	if err := endOwnershipGuard(ctx, tx, today, before); err != nil {
		return err
	}
	if err := notifyAccess(ctx, tx, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// ListAssignments returns the user's Assignments with their Roles' Grants,
// each marked with whether its dates cover today.
func (s *Service) ListAssignments(ctx context.Context, username string) ([]Assignment, error) {
	user, err := s.userByName(ctx, s.DB, username, "")
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT a.id, a.role_id, r.name, a.scope, a.active_from, a.expires_at, a.revision, r.system_permissions, r.requires_scope
		FROM athena_role_assignments a JOIN athena_roles r ON r.id = a.role_id WHERE a.user_id = ? ORDER BY r.name, a.id`, user.ID)
	if err != nil {
		return nil, fmt.Errorf("list assignments: %w", err)
	}
	defer rows.Close()
	var out []Assignment
	byID := map[string]int{}
	now := s.now()
	for rows.Next() {
		var a Assignment
		var scopePayload, permissionsPayload, requiresPayload []byte
		var from, expires sql.NullTime
		a.UserID = user.ID
		if err := rows.Scan(&a.ID, &a.RoleID, &a.RoleName, &scopePayload, &from, &expires, &a.Revision, &permissionsPayload, &requiresPayload); err != nil {
			return nil, fmt.Errorf("read assignment: %w", err)
		}
		scope, err := parseScope(scopePayload)
		if err != nil {
			continue
		}
		a.Scope = scope
		var requires []string
		if err := json.Unmarshal(requiresPayload, &requires); err != nil {
			continue
		}
		a.RequiresScope = requires
		_ = json.Unmarshal(permissionsPayload, &a.SystemPermissions)
		if from.Valid {
			at := from.Time
			a.ActiveFrom = &at
		}
		if expires.Valid {
			at := expires.Time
			a.ExpiresAt = &at
		}
		a.Active = AssignmentActive(a.ActiveFrom, a.ExpiresAt, now)
		byID[a.ID] = len(out)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list assignments: %w", err)
	}
	grants, err := s.DB.QueryContext(ctx, `SELECT a.id, g.record_type, g.actions, g.permission_level, g.row_condition, g.separate_duties
		FROM athena_grants g JOIN athena_role_assignments a ON a.role_id = g.role_id WHERE a.user_id = ? ORDER BY g.id`, user.ID)
	if err != nil {
		return nil, fmt.Errorf("list assignment grants: %w", err)
	}
	defer grants.Close()
	for grants.Next() {
		var id string
		var g Grant
		var actions []byte
		var condition sql.NullString
		if err := grants.Scan(&id, &g.RecordType, &actions, &g.Level, &condition, &g.SeparateDuties); err != nil {
			return nil, fmt.Errorf("read assignment grant: %w", err)
		}
		var list []string
		if err := json.Unmarshal(actions, &list); err != nil {
			return nil, fmt.Errorf("read assignment grant: %w", err)
		}
		g.Actions = map[string]bool{}
		for _, action := range list {
			g.Actions[action] = true
		}
		g.Condition = condition.String
		if i, ok := byID[id]; ok {
			out[i].Grants = append(out[i].Grants, g)
		}
	}
	return out, grants.Err()
}

// roleByID loads one Role with its Grants.
func (s *Service) roleByID(ctx context.Context, id string) (Role, error) {
	return roleByIDTx(ctx, s.DB, id)
}

// RoleHolder is one user's hold on a Role for an action: whether the dates,
// state and Scope currently allow it, with the Scope and dates for display.
type RoleHolder struct {
	Username    string
	DisplayName string
	State       string
	Usable      bool
	Scope       Scope
	ActiveFrom  *time.Time
	ExpiresAt   *time.Time
}

// ListRoleHolders returns the users holding the Role with a Grant covering
// the action on the Record Type, by username. Row conditions and Scope
// matching still apply per Record; Usable covers dates, state and required
// Scope only.
func (s *Service) ListRoleHolders(ctx context.Context, role, recordType, action string) ([]RoleHolder, error) {
	if !ValidAction(action) {
		return nil, fmt.Errorf("%w: unknown action %q", ErrInvalid, action)
	}
	r, err := s.roleByName(ctx, s.DB, role)
	if err != nil {
		return nil, err
	}
	covers := false
	for _, g := range r.Grants {
		if (g.RecordType == recordType || g.RecordType == AllRecordTypes) && g.Actions[action] {
			covers = true
			break
		}
	}
	if !covers {
		return nil, nil
	}
	now := s.now()
	rows, err := s.DB.QueryContext(ctx, `SELECT u.username, u.display_name, u.disabled, u.state, a.scope, a.active_from, a.expires_at
		FROM athena_role_assignments a JOIN athena_users u ON u.id = a.user_id JOIN athena_roles r ON r.id = a.role_id
		WHERE r.name = ? ORDER BY u.username, a.id`, role)
	if err != nil {
		return nil, fmt.Errorf("list role holders: %w", err)
	}
	defer rows.Close()
	var out []RoleHolder
	for rows.Next() {
		var h RoleHolder
		var disabled bool
		var payload []byte
		var from, expires sql.NullTime
		if err := rows.Scan(&h.Username, &h.DisplayName, &disabled, &h.State, &payload, &from, &expires); err != nil {
			return nil, fmt.Errorf("read role holder: %w", err)
		}
		scope, err := parseScope(payload)
		if err != nil {
			continue
		}
		h.Scope = scope
		if from.Valid {
			at := from.Time
			h.ActiveFrom = &at
		}
		if expires.Valid {
			at := expires.Time
			h.ExpiresAt = &at
		}
		h.Usable = !disabled && h.State == UserStateActive && AssignmentActive(h.ActiveFrom, h.ExpiresAt, now) && h.Scope.Satisfies(r.RequiresScope)
		out = append(out, h)
	}
	return out, rows.Err()
}

// roleByName loads one Role with its Grants by name.
func (s *Service) roleByName(ctx context.Context, db execQuerier, name string) (Role, error) {
	var id string
	if err := db.QueryRowContext(ctx, "SELECT id FROM athena_roles WHERE name = ?", name).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Role{}, fmt.Errorf("%w: Role %q", ErrNotFound, name)
		}
		return Role{}, fmt.Errorf("read role: %w", err)
	}
	return roleByIDTx(ctx, db, id)
}

func roleByIDTx(ctx context.Context, db execQuerier, id string) (Role, error) {
	var r Role
	var permissions, requires []byte
	err := db.QueryRowContext(ctx, "SELECT id, name, description, system_permissions, requires_scope, system FROM athena_roles WHERE id = ?", id).
		Scan(&r.ID, &r.Name, &r.Description, &permissions, &requires, &r.System)
	if err != nil {
		return Role{}, fmt.Errorf("read role: %w", err)
	}
	_ = json.Unmarshal(permissions, &r.SystemPermissions)
	_ = json.Unmarshal(requires, &r.RequiresScope)
	rows, err := db.QueryContext(ctx, "SELECT record_type, actions, permission_level, row_condition, separate_duties FROM athena_grants WHERE role_id = ? ORDER BY id", id)
	if err != nil {
		return Role{}, fmt.Errorf("read role grants: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		g, err := scanGrant(rows)
		if err != nil {
			return Role{}, err
		}
		r.Grants = append(r.Grants, g)
	}
	return r, rows.Err()
}
