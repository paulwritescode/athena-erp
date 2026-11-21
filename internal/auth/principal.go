package auth

import (
	"context"
	"sort"
	"strings"
	"time"
)

// RootUsername is the reserved workspace root account (ADR-0018). It is
// created at setup, cannot be invited, and always signs in.
const RootUsername = "administrator"

// IsRoot reports whether the username is the workspace root account.
func IsRoot(username string) bool {
	return strings.ToLower(strings.TrimSpace(username)) == RootUsername
}

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

// ValidAction reports whether action is a known Grant action.
func ValidAction(action string) bool { return contains(Actions, action) }

// System permissions are not tied to a Record Type.
const (
	// PermissionBuild saves, plans and deletes Record Types.
	PermissionBuild = "build"
	// PermissionAccessOwnership appoints owners and assigns platform Roles.
	PermissionAccessOwnership = "access:ownership"
	// PermissionAccessUsers manages users.
	PermissionAccessUsers = "access:users"
	// PermissionAccessRoles manages Roles.
	PermissionAccessRoles = "access:roles"
	// PermissionAccessAssign assigns Roles to users.
	PermissionAccessAssign = "access:assign"
	// PermissionAccessRecover resets passwords and recovers accounts.
	PermissionAccessRecover = "access:recover"
	// PermissionAccessSessions lists and revokes sessions.
	PermissionAccessSessions = "access:sessions"
	// PermissionAccessExplore explains access decisions.
	PermissionAccessExplore = "access:explore"
	// PermissionAudit verifies the Audit Log chain and reads the Access Log.
	PermissionAudit = "audit"
	// PermissionFilesRead lists and reads the Workspace files and their versions.
	PermissionFilesRead = "files:read"
	// PermissionFilesWrite creates, replaces, moves and deletes Workspace
	// files. It does not include reading them.
	PermissionFilesWrite = "files:write"
	// PermissionRulesRead lists Rules, their versions and the Rule Run log.
	PermissionRulesRead = "rules:read"
	// PermissionRulesWrite saves, deletes and simulates Rules. Treat it like build: a Rule runs on every matching save.
	PermissionRulesWrite = "rules:write"
	// PermissionLedgerRead lists the kernel's ledger masters.
	PermissionLedgerRead = "ledger:read"
	// PermissionLedgerWrite saves ledger masters, periods, rates, accounts,
	// determination mappings and Settings. Treat it like build.
	PermissionLedgerWrite = "ledger:write"
	PermissionAppearance  = "settings:appearance"
	// PermissionWorkspace edits the Workspace display name and logo.
	PermissionWorkspace = "settings:workspace"
	// PermissionSecurity changes the Workspace security settings (session
	// lifetimes and rate limits). Treat it like build: weaker settings
	// weaken every account.
	PermissionSecurity = "settings:security"
)

// SystemPermissions lists every system permission.
var SystemPermissions = []string{PermissionBuild, PermissionAudit, PermissionFilesRead, PermissionFilesWrite, PermissionRulesRead, PermissionRulesWrite,
	PermissionLedgerRead, PermissionLedgerWrite, PermissionAppearance, PermissionWorkspace, PermissionSecurity,
	PermissionAccessOwnership, PermissionAccessUsers, PermissionAccessRoles, PermissionAccessAssign, PermissionAccessRecover, PermissionAccessSessions, PermissionAccessExplore}

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
// It is loaded from the database on every request. A permission change
// ends the affected sessions, so the next call re-authenticates.
type Principal struct {
	UserID      string
	Username    string
	DisplayName string
	// Owner marks an instance owner. It grants nothing by itself: owners
	// hold permissions through their Assignments like everyone else. Owners
	// are created through the athena command line only.
	Owner bool
	// Root marks the workspace root account, the one bypass in the model
	// (ADR-0018): it holds every permission without Assignments, so access
	// can always be recovered and audited.
	Root bool
	// SystemPermissions unions the system permissions of the active
	// Assignments.
	SystemPermissions map[string]bool
	// Assignments carries the usable Grants with their Scopes and dates.
	// Every method below tests whole Assignments: an action on a row is
	// allowed only when one single Assignment covers it, and the results
	// of whole matching Assignments union.
	Assignments                        []Assignment
	SessionID                          string
	StepUpAt, IdleExpiresAt, ExpiresAt time.Time
	RateID                             string
	Remembered                         bool
	IdleDuration                       time.Duration
}

// HasSystem reports whether the principal holds a system permission.
func (p *Principal) HasSystem(permission string) bool {
	return p.Root || p.SystemPermissions[permission]
}

// Can reports whether some whole Assignment allows action on the Record
// Type. Row conditions and Scope values narrow the reachable rows later;
// Can only checks that an Assignment could allow the action at all. An
// Assignment whose Scope misses a required dimension allows nothing.
func (p *Principal) Can(recordType, action string) bool {
	if p.Root {
		return true
	}
	for _, a := range p.Assignments {
		if !a.usable() {
			continue
		}
		for _, g := range a.Grants {
			if g.covers(recordType) && g.Actions[action] {
				return true
			}
		}
	}
	return false
}

// CanAny reports whether the principal may do anything with the Record Type.
func (p *Principal) CanAny(recordType string) bool {
	return p.Can(recordType, ActionRead)
}

// Level is the highest field permission level the principal has on the
// Record Type, over the Grants of usable Assignments.
func (p *Principal) Level(recordType string) int {
	if p.Root {
		return MaxPermissionLevel
	}
	level := 0
	for _, a := range p.Assignments {
		if !a.usable() {
			continue
		}
		for _, g := range a.Grants {
			if g.covers(recordType) && len(g.Actions) > 0 && g.Level > level {
				level = g.Level
			}
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

// Scope returns one disjunct per whole Assignment that allows action on the
// Record Type: the Grant's row condition together with the Assignment's
// Scope. The principal reaches a row when some disjunct's condition holds
// and its Scope matches. unrestricted is true when some Assignment allows
// every Record. An empty list with unrestricted false means nothing allows
// the action.
func (p *Principal) Scope(recordType, action string) (disjuncts []ScopeDisjunct, unrestricted bool) {
	if p.Root {
		return nil, true
	}
	seen := map[string]bool{}
	for _, a := range p.Assignments {
		if !a.usable() {
			continue
		}
		for _, g := range a.Grants {
			if !g.covers(recordType) || !g.Actions[action] {
				continue
			}
			if g.Condition == "" && len(a.Scope) == 0 {
				return nil, true
			}
			d := ScopeDisjunct{Condition: g.Condition, Scope: a.Scope}
			key := g.Condition + "\x00" + a.Scope.canonical()
			if !seen[key] {
				seen[key] = true
				disjuncts = append(disjuncts, d)
			}
		}
	}
	sort.Slice(disjuncts, func(i, j int) bool {
		if disjuncts[i].Condition != disjuncts[j].Condition {
			return disjuncts[i].Condition < disjuncts[j].Condition
		}
		return disjuncts[i].Scope.canonical() < disjuncts[j].Scope.canonical()
	})
	return disjuncts, false
}

// MustSeparateDuties reports whether every whole Assignment that allows
// action on the Record Type demands that the creator of a Record not act on
// it.
func (p *Principal) MustSeparateDuties(recordType, action string) bool {
	if p.Root {
		return false
	}
	found := false
	for _, a := range p.Assignments {
		if !a.usable() {
			continue
		}
		for _, g := range a.Grants {
			if !g.covers(recordType) || !g.Actions[action] {
				continue
			}
			if !g.SeparateDuties {
				return false
			}
			found = true
		}
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
