package auth

import (
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	// This test pins the production cost; TestMain lowers it for the rest.
	previous := DefaultParams
	DefaultParams = Params{MemoryKiB: 64 * 1024, Time: 3, Threads: 4}
	t.Cleanup(func() { DefaultParams = previous })
	const password = "correct horse battery"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=4$") || strings.Contains(hash, password) {
		t.Fatalf("hash = %q", hash)
	}
	if ok, rehash := VerifyPassword(password, hash); !ok || rehash {
		t.Fatalf("verify = %v, rehash = %v", ok, rehash)
	}
	if ok, _ := VerifyPassword(password+"x", hash); ok {
		t.Fatal("a wrong password matched")
	}
	other, _ := HashPassword(password)
	if other == hash {
		t.Fatal("two hashes of one password share a salt")
	}
}

func TestWeakerHashMatchesAndAsksForRehash(t *testing.T) {
	// The rehash oracle needs the production default; TestMain lowers it.
	previous := DefaultParams
	DefaultParams = Params{MemoryKiB: 64 * 1024, Time: 3, Threads: 4}
	t.Cleanup(func() { DefaultParams = previous })
	hash, err := hashWith("correct horse battery", Params{MemoryKiB: 8 * 1024, Time: 1, Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	if ok, rehash := VerifyPassword("correct horse battery", hash); !ok || !rehash {
		t.Fatalf("verify = %v, rehash = %v", ok, rehash)
	}
}

func TestMalformedOrHostileHashNeverMatches(t *testing.T) {
	for _, hash := range []string{"", "plain", "$argon2i$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA", "$argon2id$v=19$m=999999999,t=3,p=4$c2FsdA$aGFzaA", "$argon2id$v=19$m=65536,t=3,p=4$$"} {
		if ok, _ := VerifyPassword("x", hash); ok {
			t.Fatalf("%q matched", hash)
		}
	}
}

func TestPasswordPolicy(t *testing.T) {
	if CheckPassword("alice", "short") == nil {
		t.Fatal("a short password was accepted")
	}
	if CheckPassword("alice-long-name", "ALICE-LONG-NAME") == nil {
		t.Fatal("the username was accepted as password")
	}
	if err := CheckPassword("alice", "a long enough passphrase"); err != nil {
		t.Fatal(err)
	}
}

func grant(recordType string, level int, condition string, actions ...string) Grant {
	g := Grant{RecordType: recordType, Level: level, Condition: condition, Actions: map[string]bool{}}
	for _, a := range actions {
		g.Actions[a] = true
	}
	return g
}

func assigned(grants ...Grant) *Principal {
	return &Principal{Assignments: []Assignment{{ID: "a", Active: true, Grants: grants}}}
}

func TestAssignmentsAllowByWholeAssignment(t *testing.T) {
	p := &Principal{}
	for _, action := range Actions {
		if p.Can("invoice", action) {
			t.Fatalf("%s allowed with no Assignment", action)
		}
	}
	if disjuncts, all := p.Scope("invoice", ActionRead); all || len(disjuncts) != 0 {
		t.Fatal("no Assignment gave a read scope")
	}
	p = assigned(grant("invoice", 1, "doc.owner == user.name", ActionRead), grant("invoice", 3, "", ActionCreate))
	if !p.Can("invoice", ActionRead) || !p.Can("invoice", ActionCreate) || p.Can("invoice", ActionDelete) || p.Can("order", ActionRead) {
		t.Fatal("Grants are not additive per action and Record Type")
	}
	if p.Level("invoice") != 3 || p.Level("order") != 0 {
		t.Fatalf("level = %d", p.Level("invoice"))
	}
	if disjuncts, all := p.Scope("invoice", ActionRead); all || len(disjuncts) != 1 || disjuncts[0].Condition != "doc.owner == user.name" {
		t.Fatalf("read scope = %v, %v", disjuncts, all)
	}
	p.Assignments = append(p.Assignments, Assignment{ID: "b", Active: true, Grants: []Grant{grant(AllRecordTypes, 0, "", ActionRead)}})
	if _, all := p.Scope("invoice", ActionRead); !all {
		t.Fatal("an unconditional Grant did not lift the row condition")
	}
	if (&Principal{Owner: true}).Can("anything", ActionDelete) {
		t.Fatal("the owner flag alone allowed deleting")
	}
	if (&Principal{Owner: true}).HasSystem(PermissionBuild) {
		t.Fatal("the owner flag alone allowed building")
	}
}

func TestAssignmentsOutsideTheirDatesAreAbsent(t *testing.T) {
	p := assigned(grant("invoice", 0, "", ActionRead))
	p.Assignments[0].Active = false
	if p.Can("invoice", ActionRead) {
		t.Fatal("an inactive Assignment allowed reading")
	}
	if disjuncts, all := p.Scope("invoice", ActionRead); all || len(disjuncts) != 0 {
		t.Fatal("an inactive Assignment gave a scope")
	}
	if p.Level("invoice") != 0 {
		t.Fatal("an inactive Assignment raised the level")
	}
}

func TestRequiredScopeDeniesAnUnscopedAssignment(t *testing.T) {
	p := &Principal{Assignments: []Assignment{{ID: "a", Active: true, RequiresScope: []string{"company"}, Grants: []Grant{grant("invoice", 0, "", ActionRead)}}}}
	if p.Can("invoice", ActionRead) {
		t.Fatal("an Assignment without the required Scope was allowed")
	}
	if disjuncts, all := p.Scope("invoice", ActionRead); all || len(disjuncts) != 0 {
		t.Fatal("an Assignment without the required Scope gave a scope")
	}
	p.Assignments[0].Scope = Scope{"company": {"acme"}}
	if !p.Can("invoice", ActionRead) {
		t.Fatal("a scoped Assignment was denied")
	}
	disjuncts, all := p.Scope("invoice", ActionRead)
	if all || len(disjuncts) != 1 || !disjuncts[0].Scope.Matches(map[string]string{"company": "acme"}) {
		t.Fatalf("scoped read = %v, %v", disjuncts, all)
	}
}

func TestScopeDisjunctsKeepAssignmentsWhole(t *testing.T) {
	// Read on company A plus write on company B must stay two separate
	// disjuncts, never write on A.
	p := &Principal{Assignments: []Assignment{
		{ID: "a", Active: true, Scope: Scope{"company": {"a"}}, Grants: []Grant{grant("invoice", 0, "", ActionRead)}},
		{ID: "b", Active: true, Scope: Scope{"company": {"b"}}, Grants: []Grant{grant("invoice", 0, "", ActionWrite)}},
	}}
	reads, all := p.Scope("invoice", ActionRead)
	if all || len(reads) != 1 || !reads[0].Scope.Matches(map[string]string{"company": "a"}) || reads[0].Scope.Matches(map[string]string{"company": "b"}) {
		t.Fatalf("read scope = %v, %v", reads, all)
	}
	writes, all := p.Scope("invoice", ActionWrite)
	if all || len(writes) != 1 || !writes[0].Scope.Matches(map[string]string{"company": "b"}) || writes[0].Scope.Matches(map[string]string{"company": "a"}) {
		t.Fatalf("write scope = %v, %v", writes, all)
	}
}

func TestSeparateDutiesNeedsEveryGrant(t *testing.T) {
	g := grant("invoice", 0, "", ActionSubmit)
	g.SeparateDuties = true
	p := assigned(g)
	if !p.MustSeparateDuties("invoice", ActionSubmit) {
		t.Fatal("separate duties not applied")
	}
	p.Assignments = append(p.Assignments, Assignment{ID: "b", Active: true, Grants: []Grant{grant("invoice", 0, "", ActionSubmit)}})
	if p.MustSeparateDuties("invoice", ActionSubmit) {
		t.Fatal("a Grant without separate duties should allow the creator")
	}
}

func TestScopeExpression(t *testing.T) {
	if got := (Scope{}).Expression(); got != "true" {
		t.Fatalf("empty Scope = %q", got)
	}
	if got := (Scope{"company": {"acme"}}).Expression(); got != `(doc.company == "acme")` {
		t.Fatalf("one value = %q", got)
	}
	if got := (Scope{"warehouse": {"w2", "w1"}, "company": {"acme"}}).Expression(); got != `(doc.company == "acme") && (doc.warehouse == "w1" || doc.warehouse == "w2")` {
		t.Fatalf("two dimensions = %q", got)
	}
	if got := (Scope{"company": {`a"b\c`}}).Expression(); got != `(doc.company == "a\"b\\c")` {
		t.Fatalf("escaped value = %q", got)
	}
	for _, scope := range []Scope{{"Company": {"acme"}}, {"company": {}}, {"co-mpany": {"acme"}}} {
		if got := scope.Expression(); got != "false" {
			t.Fatalf("scope %v = %q, want false", scope, got)
		}
	}
}

func TestScopeMatches(t *testing.T) {
	scope := Scope{"company": {"acme", "globex"}, "branch": {"north"}}
	if !scope.Matches(map[string]string{"company": "acme", "branch": "north", "other": "x"}) {
		t.Fatal("a matching row did not match")
	}
	if scope.Matches(map[string]string{"company": "initech", "branch": "north"}) {
		t.Fatal("a row outside the values matched")
	}
	if scope.Matches(map[string]string{"company": "acme"}) {
		t.Fatal("a row missing a dimension matched")
	}
	if !(Scope{}).Matches(map[string]string{"company": "acme"}) {
		t.Fatal("an empty Scope did not match")
	}
	if (Scope{"company": {}}).Matches(map[string]string{"company": "acme"}) {
		t.Fatal("an empty value list matched")
	}
}

// TestScopeUnionByWholeAssignmentProperty checks Property 3 over drawn
// assignment sets: the principal allows an action on a row if and only if at
// least one single active Assignment has a covering Grant, a true row
// condition and a matching Scope. Conditions stay opaque strings whose truth
// is drawn; what is tested is the union logic, not the expression language.
//
// Feature: athena-batch-2, Property 3: Scope union by whole Assignment.
func TestScopeUnionByWholeAssignmentProperty(t *testing.T) {
	dimensions := []string{"company", "branch"}
	values := []string{"a", "b", "c"}
	recordTypes := []string{"invoice", "order"}
	actions := []string{ActionRead, ActionWrite}
	conditions := []string{"", "c1", "c2"}
	pick := func(t *rapid.T, label string, xs []string) string {
		return xs[rapid.IntRange(0, len(xs)-1).Draw(t, label)]
	}
	subset := func(t *rapid.T, label string, xs []string, max int) []string {
		n := rapid.IntRange(0, max).Draw(t, label)
		var out []string
		seen := map[string]bool{}
		for len(out) < n {
			if s := pick(t, label, xs); !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
		return out
	}
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 3).Draw(t, "assignments")
		assignments := make([]Assignment, 0, n)
		for i := 0; i < n; i++ {
			scope := Scope{}
			for _, dimension := range subset(t, "scopeDims", dimensions, 2) {
				scope[dimension] = subset(t, "scopeValues", values, 2)
				if len(scope[dimension]) == 0 {
					scope[dimension] = []string{pick(t, "scopeValue", values)}
				}
			}
			m := rapid.IntRange(1, 2).Draw(t, "grants")
			grants := make([]Grant, 0, m)
			for j := 0; j < m; j++ {
				g := Grant{RecordType: pick(t, "grantType", recordTypes), Condition: pick(t, "grantCondition", conditions), Actions: map[string]bool{}}
				for _, action := range subset(t, "grantActions", actions, 2) {
					g.Actions[action] = true
				}
				if len(g.Actions) == 0 {
					g.Actions[pick(t, "grantAction", actions)] = true
				}
				grants = append(grants, g)
			}
			assignments = append(assignments, Assignment{
				Active:        rapid.Bool().Draw(t, "active"),
				Scope:         scope,
				RequiresScope: subset(t, "requires", dimensions, 2),
				Grants:        grants,
			})
		}
		recordType := pick(t, "recordType", recordTypes)
		action := pick(t, "action", actions)
		row := map[string]string{}
		for _, dimension := range dimensions {
			if rapid.Bool().Draw(t, "rowPresent") {
				row[dimension] = pick(t, "rowValue", values)
			}
		}
		truth := map[string]bool{}
		for _, condition := range conditions[1:] {
			truth[condition] = rapid.Bool().Draw(t, "conditionTrue")
		}
		holds := func(condition string) bool { return condition == "" || truth[condition] }

		// The naive model, written straight from Requirements 1.15: one
		// single Assignment must cover everything at once.
		naive := false
		for _, a := range assignments {
			if !a.Active || !a.Scope.Satisfies(a.RequiresScope) {
				continue
			}
			for _, g := range a.Grants {
				if g.covers(recordType) && g.Actions[action] && holds(g.Condition) && a.Scope.Matches(row) {
					naive = true
				}
			}
		}
		p := &Principal{Assignments: assignments}
		disjuncts, unrestricted := p.Scope(recordType, action)
		got := unrestricted
		if !got {
			for _, d := range disjuncts {
				if holds(d.Condition) && d.Scope.Matches(row) {
					got = true
					break
				}
			}
		}
		if got != naive {
			t.Fatalf("recordType=%s action=%s row=%v truth=%v assignments=%+v: scope allows %v, naive allows %v",
				recordType, action, row, truth, assignments, got, naive)
		}
		canNaive := false
		for _, a := range assignments {
			if !a.Active || !a.Scope.Satisfies(a.RequiresScope) {
				continue
			}
			for _, g := range a.Grants {
				if g.covers(recordType) && g.Actions[action] {
					canNaive = true
				}
			}
		}
		if p.Can(recordType, action) != canNaive {
			t.Fatalf("recordType=%s action=%s assignments=%+v: Can = %v, naive = %v",
				recordType, action, assignments, p.Can(recordType, action), canNaive)
		}
	})
}

func TestExplainReportsMatchesAndFailures(t *testing.T) {
	at := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	past := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	p := &Principal{Assignments: []Assignment{
		{ID: "read-a", Active: true, RoleName: "RO", Scope: Scope{"company": {"a"}}, Grants: []Grant{grant("invoice", 0, "", ActionRead)}},
		{ID: "write-b", Active: true, RoleName: "RW", Scope: Scope{"company": {"b"}}, Grants: []Grant{grant("invoice", 0, "", ActionWrite)}},
		{ID: "expired", Active: false, ExpiresAt: &past, RoleName: "RO", Grants: []Grant{grant("invoice", 0, "", ActionRead)}},
		{ID: "scoped", Active: true, RoleName: "RQ", RequiresScope: []string{"company"}, Grants: []Grant{grant("invoice", 0, "", ActionRead)}},
		{ID: "cond", Active: true, RoleName: "CO", Grants: []Grant{grant("invoice", 0, "doc.total > 100", ActionRead)}},
	}}
	row := map[string]string{"company": "a"}
	truth := map[string]bool{"doc.total > 100": true}
	out := Explain(p, "invoice", ActionRead, row, truth, at)
	if !out.Allowed || len(out.Assignments) != 5 {
		t.Fatalf("explanation = %+v", out)
	}
	byID := map[string]AssignmentVerdict{}
	for _, v := range out.Assignments {
		byID[v.AssignmentID] = v
	}
	if !byID["read-a"].Matched || len(byID["read-a"].Reasons) != 0 {
		t.Fatalf("read-a = %+v", byID["read-a"])
	}
	if byID["write-b"].Matched || len(byID["write-b"].Reasons) != 1 {
		t.Fatalf("write-b = %+v", byID["write-b"])
	}
	if byID["expired"].Matched || byID["expired"].Reasons[0] != "expired" {
		t.Fatalf("expired = %+v", byID["expired"])
	}
	if byID["scoped"].Matched || byID["scoped"].Reasons[0] != "scope misses the required company" {
		t.Fatalf("scoped = %+v", byID["scoped"])
	}
	if !byID["cond"].Matched {
		t.Fatalf("cond = %+v", byID["cond"])
	}
	truth["doc.total > 100"] = false
	out = Explain(p, "invoice", ActionRead, row, truth, at)
	for _, v := range out.Assignments {
		if v.AssignmentID == "cond" && (v.Matched || v.Reasons[0] != "row condition false") {
			t.Fatalf("cond with a false condition = %+v", v)
		}
	}
	out = Explain(p, "invoice", ActionWrite, map[string]string{"company": "a"}, truth, at)
	for _, v := range out.Assignments {
		if v.AssignmentID == "write-b" && (v.Matched || v.Reasons[0] != "scope does not match this record") {
			t.Fatalf("write-b on company a = %+v", v)
		}
	}
	if out.Allowed {
		t.Fatal("write on company a was allowed")
	}
	// Without a row, row checks stay untested and static verdicts stand.
	out = Explain(p, "invoice", ActionWrite, nil, nil, at)
	for _, v := range out.Assignments {
		if v.AssignmentID == "write-b" && !v.Matched {
			t.Fatalf("write-b without a row = %+v", v)
		}
	}
	if !out.Allowed {
		t.Fatal("write without a row was denied")
	}
}

func TestExplainShowsSeparateDutiesConflicts(t *testing.T) {
	at := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	submit := grant("invoice", 0, "", ActionSubmit)
	submit.SeparateDuties = true
	p := &Principal{Assignments: []Assignment{
		{ID: "a", Active: true, Grants: []Grant{submit, grant("invoice", 0, "", ActionCreate)}},
	}}
	out := Explain(p, "invoice", ActionSubmit, nil, nil, at)
	if !out.Allowed || !out.Assignments[0].SeparateDutiesConflict {
		t.Fatalf("explanation = %+v", out)
	}
	p.Assignments[0].Grants = []Grant{submit}
	out = Explain(p, "invoice", ActionSubmit, nil, nil, at)
	if out.Assignments[0].SeparateDutiesConflict {
		t.Fatalf("conflict without the creator duty = %+v", out)
	}
}

func TestAssignmentActiveBoundaries(t *testing.T) {
	today := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	at := func(y int, m time.Month, d int) *time.Time {
		t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		return &t
	}
	if !AssignmentActive(nil, nil, today) {
		t.Fatal("an unbounded Assignment is not active")
	}
	if !AssignmentActive(at(2026, 6, 15), at(2026, 6, 15), today) {
		t.Fatal("an Assignment for today only is not active")
	}
	if AssignmentActive(at(2026, 6, 16), nil, today) {
		t.Fatal("an Assignment starting tomorrow is active")
	}
	if AssignmentActive(nil, at(2026, 6, 14), today) {
		t.Fatal("an Assignment that expired yesterday is active")
	}
}

func TestCSRFTokenIsBoundToTheSession(t *testing.T) {
	token := CSRFToken("session-a")
	if !CheckCSRF("session-a", token) || CheckCSRF("session-b", token) || CheckCSRF("session-a", "") {
		t.Fatal("CSRF check is not bound to the session")
	}
}

func TestValidateRole(t *testing.T) {
	bad := []Role{
		{Name: ""},
		{Name: "R", SystemPermissions: []string{"root"}},
		{Name: "R", Grants: []Grant{grant("invoice", 0, "")}},
		{Name: "R", Grants: []Grant{grant("invoice", 10, "", ActionRead)}},
		{Name: "R", Grants: []Grant{grant("invoice", 0, "", "fly")}},
		{Name: "R", Grants: []Grant{grant(AllRecordTypes, 0, "doc.x == 1", ActionRead)}},
	}
	for _, r := range bad {
		if ValidateRole(r) == nil {
			t.Fatalf("role %+v was accepted", r)
		}
	}
	if err := ValidateRole(Role{Name: "Clerk", SystemPermissions: []string{PermissionBuild}, Grants: []Grant{grant("invoice", 2, "", ActionRead)}}); err != nil {
		t.Fatal(err)
	}
}
