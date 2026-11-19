package auth

import (
	"context"
	"sort"
)

// Actions a Grant can allow on a Record Type.
const (
	ActionRead   = "read"
	ActionCreate = "create"
	ActionWrite  = "write"
	ActionDelete = "delete"
	ActionSubmit = "submit"
	ActionCancel = "cancel"
	ActionAmend  = "amend"
	// ActionAudit reads the Audit Log rows, Document Versions and Activity
	// Timeline of a Record. It is separate from read because those hold old
	// values (spec 21).
	ActionAudit = "audit"
)

// Actions lists every Grant action.
var Actions = []string{ActionRead, ActionCreate, ActionWrite, ActionDelete, ActionSubmit, ActionCancel, ActionAmend, ActionAudit}

// System permissions are not tied to a Record Type.
const (
	// PermissionBuild saves, plans and deletes Record Types.
	PermissionBuild = "build"
	// PermissionAccess manages users, Roles and Grants.
	PermissionAccess = "access"
	// PermissionAudit verifies the Audit Log chain and reads the Access Log.
	PermissionAudit = "audit"
	// PermissionFilesRead lists and reads the Workspace files and their versions.
	PermissionFilesRead = "files:read"
	// PermissionFilesWrite creates, replaces, moves and deletes Workspace
	// files. It does not include reading them.
	PermissionFilesWrite = "files:write"
)

// SystemPermissions lists every system permission.
var SystemPermissions = []string{PermissionBuild, PermissionAccess, PermissionAudit, PermissionFilesRead, PermissionFilesWrite}

// AllRecordTypes is the Grant target that covers every Record Type.
const AllRecordTypes = "*"

// MaxPermissionLevel is the highest field permission level (0 to 9).
const MaxPermissionLevel = 9

// Grant allows actions on one Record Type (or all of them). Grants are
// additive: an action is allowed when any Grant of any of the user's Roles
// allows it. With no Grant, everything is denied.
type Grant struct {
	RecordType string
	Actions    map[string]bool
	// Level is the highest field permission level the Grant reaches. A field
	// above the user's highest Level on the Record Type is hidden and cannot
	// be written.
	Level int
	// Condition limits the Grant to Records for which the expression is true.
	// Empty means every Record.
	Condition string
	// SeparateDuties stops a user from submitting a Record they created.
	SeparateDuties bool
}

func (g Grant) covers(recordType string) bool {
	return g.RecordType == AllRecordTypes || g.RecordType == recordType
}

// Principal is the signed-in user as the kernel sees them for one request.
// It is loaded from the database on every request, so a Role change takes
// effect at once.
type Principal struct {
	UserID      string
	Username    string
	DisplayName string
	// Owner is the instance owner. It holds every permission and ignores
	// Grants. Owners are created through the athena command line only.
	Owner             bool
	SystemPermissions map[string]bool
	Grants            []Grant
	SessionID         string
}

// HasSystem reports whether the principal holds a system permission.
func (p *Principal) HasSystem(permission string) bool {
	return p.Owner || p.SystemPermissions[permission]
}

// Can reports whether any Grant allows action on the Record Type.
func (p *Principal) Can(recordType, action string) bool {
	if p.Owner {
		return true
	}
	for _, g := range p.Grants {
		if g.covers(recordType) && g.Actions[action] {
			return true
		}
	}
	return false
}

// CanAny reports whether the principal may do anything with the Record Type.
func (p *Principal) CanAny(recordType string) bool {
	return p.Can(recordType, ActionRead)
}

// Level is the highest field permission level the principal has on the
// Record Type.
func (p *Principal) Level(recordType string) int {
	if p.Owner {
		return MaxPermissionLevel
	}
	level := 0
	for _, g := range p.Grants {
		if g.covers(recordType) && len(g.Actions) > 0 && g.Level > level {
			level = g.Level
		}
	}
	return level
}

// Actions lists the actions the principal may take on the Record Type.
func (p *Principal) Actions(recordType string) []string {
	var out []string
	for _, action := range Actions {
		if p.Can(recordType, action) {
			out = append(out, action)
		}
	}
	return out
}

// Scope returns the row conditions that limit action on the Record Type.
// unrestricted is true when some Grant allows every Record. Otherwise the
// principal reaches the Records for which any of conditions is true; an empty
// list with unrestricted false means no Grant allows the action.
func (p *Principal) Scope(recordType, action string) (conditions []string, unrestricted bool) {
	if p.Owner {
		return nil, true
	}
	seen := map[string]bool{}
	for _, g := range p.Grants {
		if !g.covers(recordType) || !g.Actions[action] {
			continue
		}
		if g.Condition == "" {
			return nil, true
		}
		if !seen[g.Condition] {
			seen[g.Condition] = true
			conditions = append(conditions, g.Condition)
		}
	}
	sort.Strings(conditions)
	return conditions, false
}

// MustSeparateDuties reports whether every Grant that allows action on the
// Record Type demands that the creator of a Record not act on it.
func (p *Principal) MustSeparateDuties(recordType, action string) bool {
	if p.Owner {
		return false
	}
	found := false
	for _, g := range p.Grants {
		if !g.covers(recordType) || !g.Actions[action] {
			continue
		}
		if !g.SeparateDuties {
			return false
		}
		found = true
	}
	return found
}

type principalKey struct{}

// WithPrincipal returns ctx carrying the signed-in principal.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the principal in ctx, or nil.
func PrincipalFrom(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}
