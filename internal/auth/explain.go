package auth

import (
	"context"
	"fmt"
	"time"
)

// AssignmentVerdict reports why one Assignment allows an action or why it
// fails. Reasons are fixed phrases, one per failed check.
type AssignmentVerdict struct {
	AssignmentID string
	Role         string
	Matched      bool
	Reasons      []string
	// SeparateDutiesConflict marks a submit Grant that separates duties
	// while the user also holds the creator duty: submitting their own
	// Record would be refused. It never flips Matched by itself.
	SeparateDutiesConflict bool
}

// Explanation is the whole verdict for one user, action and Record Type:
// whether anything allows it, and what each Assignment contributed.
type Explanation struct {
	Allowed     bool
	Assignments []AssignmentVerdict
}

// ExplainAccess explains whether username may take action on recordType. Row
// carries one Record's field values for Scope matching, and truth carries
// each distinct Grant condition's value on that Record; both come from the
// caller, which evaluates conditions. A nil row means no Record was given:
// row conditions and Scopes stay untested, and Matched means the Assignment
// reaches the type subject to the row checks.
func (s *Service) ExplainAccess(ctx context.Context, username, action, recordType string, row map[string]string, truth map[string]bool, at time.Time) (Explanation, error) {
	if !ValidAction(action) {
		return Explanation{}, fmt.Errorf("%w: unknown action %q", ErrInvalid, action)
	}
	user, err := s.userByName(ctx, s.DB, username, "")
	if err != nil {
		return Explanation{}, err
	}
	principal, err := s.loadPrincipal(ctx, user.ID)
	if err != nil {
		return Explanation{}, err
	}
	return Explain(principal, recordType, action, row, truth, at), nil
}

// Explain tests every Assignment of the principal against the action. Whole
// Assignments are tested independently: an action is allowed when one single
// Assignment covers the Grant, the row condition, the Scope and the dates
// together.
func Explain(p *Principal, recordType, action string, row map[string]string, truth map[string]bool, at time.Time) Explanation {
	out := Explanation{}
	for _, a := range p.Assignments {
		v := AssignmentVerdict{AssignmentID: a.ID, Role: a.RoleName}
		if !a.Active {
			v.Reasons = append(v.Reasons, dateReason(a, at))
		}
		for _, dimension := range a.RequiresScope {
			if len(a.Scope[dimension]) == 0 {
				v.Reasons = append(v.Reasons, "scope misses the required "+dimension)
			}
		}
		var covering []Grant
		for _, g := range a.Grants {
			if g.covers(recordType) && g.Actions[action] {
				covering = append(covering, g)
			}
		}
		if len(covering) == 0 {
			v.Reasons = append(v.Reasons, "no Grant for "+action+" on "+recordType)
		} else {
			if row != nil {
				// The verdict follows the best covering grant: one needing
				// no failing condition. Any other choice would report a
				// failure the Assignment does not have.
				grant := covering[0]
				for _, g := range covering {
					if g.Condition == "" || truth[g.Condition] {
						grant = g
						break
					}
				}
				if grant.Condition != "" && !truth[grant.Condition] {
					v.Reasons = append(v.Reasons, "row condition false")
				}
				if !a.Scope.Matches(row) {
					v.Reasons = append(v.Reasons, "scope does not match this record")
				}
			}
			if action == ActionSubmit && p.Can(recordType, ActionCreate) {
				for _, g := range covering {
					if g.SeparateDuties {
						v.SeparateDutiesConflict = true
						break
					}
				}
			}
		}
		v.Matched = len(v.Reasons) == 0
		if v.Matched {
			out.Allowed = true
		}
		out.Assignments = append(out.Assignments, v)
	}
	return out
}

// dateReason names why an Assignment outside its dates is absent.
func dateReason(a Assignment, at time.Time) string {
	today := day(at)
	if a.ActiveFrom != nil && today.Before(day(*a.ActiveFrom)) {
		return "not yet active"
	}
	if a.ExpiresAt != nil && today.After(day(*a.ExpiresAt)) {
		return "expired"
	}
	return "outside its dates"
}
