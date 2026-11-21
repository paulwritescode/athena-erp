package auth_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"athena-erp/internal/auth"
	"athena-erp/internal/server"
	"athena-erp/internal/store"
	"pgregory.net/rapid"
)

// activeOwner invites an owner, completes the invite and returns the user
// with a session principal for delegated calls.
func activeOwner(t *testing.T, svc *auth.Service, username string) (auth.User, *auth.Principal) {
	t.Helper()
	ctx := context.Background()
	const password = "an owner passphrase here"
	user, token, err := svc.CreateUser(ctx, username, "", true, "seed")
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.CompleteChallenge(ctx, username, auth.ChallengePurposeFirstLogin, token, password, "seed")
	if err != nil {
		t.Fatal(err)
	}
	_ = user
	login, err := svc.Login(ctx, username, password, "seed")
	if err != nil {
		t.Fatal(err)
	}
	_ = result
	return user, login.Principal
}

func TestEnsureSystemRolesSeedsAndBackfills(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	if err := svc.EnsureSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureSystemRoles(ctx); err != nil {
		t.Fatal("seeding is not idempotent")
	}
	roles, err := svc.ListRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]auth.Role{}
	for _, r := range roles {
		byName[r.Name] = r
	}
	if len(roles) != 10 {
		t.Fatalf("%d roles seeded, want 10", len(roles))
	}
	owner, ok := byName[auth.RoleInstanceOwner]
	if !ok || !owner.System {
		t.Fatal("the Instance Owner role was not seeded as a system role")
	}
	held := map[string]bool{}
	for _, permission := range owner.SystemPermissions {
		held[permission] = true
	}
	for _, want := range []string{auth.PermissionAccessOwnership, auth.PermissionAccessUsers, auth.PermissionAccessAssign, auth.PermissionAccessRecover} {
		if !held[want] {
			t.Fatalf("the Instance Owner role lacks %s: %v", want, owner.SystemPermissions)
		}
	}
	for _, name := range []string{"Receiving Clerk", "Purchase User", "Accounts Payable Clerk", "Treasury Officer", "Accountant", "Auditor", "Report Viewer"} {
		template, ok := byName[name]
		if !ok {
			t.Fatalf("the %s template was not seeded", name)
		}
		if len(template.Grants) != 0 || len(template.SystemPermissions) != 0 || template.System {
			t.Fatalf("the %s template is not empty: %+v", name, template)
		}
	}
	// An owner flagged before the Role existed gains the Assignment.
	if _, _, err := svc.BootstrapOwner(ctx, "founder", "", "seed"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DB.ExecContext(ctx, "DELETE FROM athena_role_assignments WHERE user_id = (SELECT id FROM athena_users WHERE username = 'founder')"); err != nil {
		t.Fatal(err)
	}
	disabled := true
	if _, err := svc.UpdateUser(ctx, "founder", auth.UserUpdate{Disabled: &disabled}); err != nil {
		t.Fatalf("disabling a user with no ownership assignment: %v", err)
	}
	enabled := false
	if _, err := svc.UpdateUser(ctx, "founder", auth.UserUpdate{Disabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	assignments, err := svc.ListAssignments(ctx, "founder")
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 1 || assignments[0].RoleName != auth.RoleInstanceOwner {
		t.Fatalf("backfilled assignments = %+v", assignments)
	}
	if _, err := svc.UpdateUser(ctx, "founder", auth.UserUpdate{Disabled: &disabled}); !errors.Is(err, auth.ErrLastOwner) {
		t.Fatalf("disabling the last owner = %v, want ErrLastOwner", err)
	}
}

func TestBootstrapOwnerAssignsTheInstanceOwnerRole(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	const password = "a bootstrap passphrase"
	if _, _, err := svc.BootstrapOwner(ctx, "boss", "", "seed"); err != nil {
		t.Fatal(err)
	}
	assignments, err := svc.ListAssignments(ctx, "boss")
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 1 || assignments[0].RoleName != auth.RoleInstanceOwner {
		t.Fatalf("bootstrap assignments = %+v", assignments)
	}
	login, err := svc.Login(ctx, "boss", password, "seed")
	if err == nil {
		t.Fatal("an invited owner signed in")
	}
	_ = login
}

func TestLastOwnerGuardRefusesEveryDemotion(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	if err := svc.EnsureSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	_, adaToken, err := svc.BootstrapOwner(ctx, "ada", "", "seed")
	if err != nil {
		t.Fatal(err)
	}
	const password = "an owner passphrase here"
	if _, err := svc.CompleteChallenge(ctx, "ada", auth.ChallengePurposeFirstLogin, adaToken, password, "seed"); err != nil {
		t.Fatal(err)
	}
	_, graceToken, err := svc.CreateUser(ctx, "grace", "", true, "seed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteChallenge(ctx, "grace", auth.ChallengePurposeFirstLogin, graceToken, password, "seed"); err != nil {
		t.Fatal(err)
	}
	disabled := true
	if _, err := svc.UpdateUser(ctx, "ada", auth.UserUpdate{Disabled: &disabled}); err != nil {
		t.Fatalf("disabling one of two owners: %v", err)
	}
	if _, err := svc.UpdateUser(ctx, "grace", auth.UserUpdate{Disabled: &disabled}); !errors.Is(err, auth.ErrLastOwner) {
		t.Fatalf("disabling the last owner = %v, want ErrLastOwner", err)
	}
	enabled := false
	if _, err := svc.UpdateUser(ctx, "ada", auth.UserUpdate{Disabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	graceAssignments, err := svc.ListAssignments(ctx, "grace")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeAssignment(ctx, graceAssignments[0].ID); err != nil {
		t.Fatalf("revoking one of two ownership assignments: %v", err)
	}
	adaAssignments, err := svc.ListAssignments(ctx, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeAssignment(ctx, adaAssignments[0].ID); !errors.Is(err, auth.ErrLastOwner) {
		t.Fatalf("revoking the last ownership assignment = %v, want ErrLastOwner", err)
	}
	stripped := auth.Role{Name: auth.RoleInstanceOwner, SystemPermissions: []string{auth.PermissionAccessUsers}}
	if err := svc.SaveRole(ctx, stripped); !errors.Is(err, auth.ErrLastOwner) {
		t.Fatalf("stripping ownership from the role = %v, want ErrLastOwner", err)
	}
	if err := svc.DeleteRole(ctx, auth.RoleInstanceOwner); !errors.Is(err, auth.ErrLastOwner) {
		t.Fatalf("deleting the ownership role = %v, want ErrLastOwner", err)
	}
	// The failed demotions changed nothing: ada still owns the Workspace.
	adaAssignments, err = svc.ListAssignments(ctx, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if len(adaAssignments) != 1 || adaAssignments[0].RoleName != auth.RoleInstanceOwner {
		t.Fatalf("ada assignments after refused demotions = %+v", adaAssignments)
	}
}

func TestRevocationSticksAcrossSeeding(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	if err := svc.EnsureSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	_, adaToken, err := svc.BootstrapOwner(ctx, "ada", "", "seed")
	if err != nil {
		t.Fatal(err)
	}
	const password = "an owner passphrase here"
	if _, err := svc.CompleteChallenge(ctx, "ada", auth.ChallengePurposeFirstLogin, adaToken, password, "seed"); err != nil {
		t.Fatal(err)
	}
	_, graceToken, err := svc.CreateUser(ctx, "grace", "", true, "seed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteChallenge(ctx, "grace", auth.ChallengePurposeFirstLogin, graceToken, password, "seed"); err != nil {
		t.Fatal(err)
	}
	adaAssignments, err := svc.ListAssignments(ctx, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeAssignment(ctx, adaAssignments[0].ID); err != nil {
		t.Fatal(err)
	}
	users, err := svc.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u.Username == "ada" && u.Owner {
			t.Fatal("a revoked owner is still flagged")
		}
	}
	// Seeding again — as every startup does — must not resurrect the
	// revoked assignment.
	if err := svc.EnsureSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	adaAssignments, err = svc.ListAssignments(ctx, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if len(adaAssignments) != 0 {
		t.Fatalf("seeding resurrected ada's assignment: %+v", adaAssignments)
	}
}

func TestDelegationRefusesBeyondReach(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	if err := svc.EnsureSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	_, owner := activeOwner(t, svc, "owner")
	ownerCtx := auth.WithPrincipal(ctx, owner)
	manager, managerToken, err := svc.CreateUser(ctx, "manager", "", false, "seed")
	if err != nil {
		t.Fatal(err)
	}
	const managerPassword = "a manager passphrase here"
	if _, err := svc.CompleteChallenge(ctx, manager.Username, auth.ChallengePurposeFirstLogin, managerToken, managerPassword, "seed"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveAssignment(ownerCtx, "manager", auth.AssignmentInput{Role: auth.RoleSystemManager}); err != nil {
		t.Fatal(err)
	}
	clerk, clerkToken, err := svc.CreateUser(ctx, "clerk", "", false, "seed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteChallenge(ctx, clerk.Username, auth.ChallengePurposeFirstLogin, clerkToken, "a clerk passphrase here", "seed"); err != nil {
		t.Fatal(err)
	}
	managerLogin, err := svc.Login(ctx, "manager", managerPassword, "seed")
	if err != nil {
		t.Fatal(err)
	}
	managerCtx := auth.WithPrincipal(ctx, managerLogin.Principal)
	// Ownership and platform status are beyond a manager: only owners
	// appoint managers.
	if _, err := svc.SaveAssignment(managerCtx, "clerk", auth.AssignmentInput{Role: auth.RoleInstanceOwner}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("a manager assigned ownership: %v", err)
	}
	if _, err := svc.SaveAssignment(managerCtx, "clerk", auth.AssignmentInput{Role: auth.RoleSystemManager}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("a manager assigned the platform manager role: %v", err)
	}
	// A system permission the manager lacks is beyond delegation.
	if err := svc.SaveRole(managerCtx, auth.Role{Name: "Builder", SystemPermissions: []string{auth.PermissionBuild}}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("a manager granted build: %v", err)
	}
	// A Grant the manager lacks is beyond delegation.
	if _, err := svc.SaveAssignment(managerCtx, "clerk", auth.AssignmentInput{Role: "Receiving Clerk"}); err != nil {
		t.Fatalf("a manager assigning an empty template: %v", err)
	}
	if err := svc.SaveRole(ownerCtx, auth.Role{Name: "Writer", Grants: []auth.Grant{{RecordType: "invoice", Actions: map[string]bool{auth.ActionWrite: true}}}}); err != nil {
		t.Fatalf("an owner saving a grant: %v", err)
	}
	if _, err := svc.SaveAssignment(managerCtx, "clerk", auth.AssignmentInput{Role: "Writer"}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("a manager assigned a Grant beyond their own: %v", err)
	}
	// The owner holds ownership, so the same change succeeds for them.
	if _, err := svc.SaveAssignment(ownerCtx, "clerk", auth.AssignmentInput{Role: auth.RoleSystemManager}); err != nil {
		t.Fatalf("an owner assigning the manager role: %v", err)
	}
	// Scoped reach cannot broaden: the manager reads company A and cannot
	// grant company B or an unscoped read.
	if err := svc.SaveRole(ownerCtx, auth.Role{Name: "Reader", Grants: []auth.Grant{{RecordType: "invoice", Actions: map[string]bool{auth.ActionRead: true}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveAssignment(ownerCtx, "manager", auth.AssignmentInput{Role: "Reader", Scope: auth.Scope{"company": {"a"}}}); err != nil {
		t.Fatal(err)
	}
	managerLogin, err = svc.Login(ctx, "manager", managerPassword, "seed")
	if err != nil {
		t.Fatal(err)
	}
	managerCtx = auth.WithPrincipal(ctx, managerLogin.Principal)
	if _, err := svc.SaveAssignment(managerCtx, "clerk", auth.AssignmentInput{Role: "Reader", Scope: auth.Scope{"company": {"b"}}}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("a manager granted outside their Scope: %v", err)
	}
	if _, err := svc.SaveAssignment(managerCtx, "clerk", auth.AssignmentInput{Role: "Reader"}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("a manager granted an unscoped read from a Scope: %v", err)
	}
	if _, err := svc.SaveAssignment(managerCtx, "clerk", auth.AssignmentInput{Role: "Reader", Scope: auth.Scope{"company": {"a"}}}); err != nil {
		t.Fatalf("a manager granting within their Scope: %v", err)
	}
}

func TestStartResetNeedsTheActorPassword(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	if err := svc.EnsureSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	_, owner := activeOwner(t, svc, "owner")
	ownerCtx := auth.WithPrincipal(ctx, owner)
	clerk, clerkToken, err := svc.CreateUser(ctx, "clerk", "", false, "seed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteChallenge(ctx, clerk.Username, auth.ChallengePurposeFirstLogin, clerkToken, "a clerk passphrase here", "seed"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.StartReset(ownerCtx, "clerk", "wrong password here", "seed"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("a reset with a wrong actor password = %v, want ErrInvalidCredentials", err)
	}
	if _, _, err := svc.StartReset(ownerCtx, "clerk", "", "seed"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("a reset with no actor password = %v, want ErrInvalid", err)
	}
	if _, _, err := svc.StartReset(ownerCtx, "clerk", "an owner passphrase here", "seed"); err != nil {
		t.Fatalf("a reset with the actor password: %v", err)
	}
	// The command-line operator has no password and stays exempt.
	if _, _, err := svc.StartReset(ctx, "clerk", "", "seed"); err != nil {
		t.Fatalf("an operator reset: %v", err)
	}
}

// ownerDB resets the shared database and seeds the system roles. The serial
// phase calls it once and accumulates owners across iterations; the race
// phase calls it per iteration for exactly the owners it demotes.
func ownerDB(t *testing.T) *auth.Service {
	t.Helper()
	db := sharedDatabase(t)
	if _, err := store.Up(context.Background(), db, store.MariaDB{}, server.SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := &auth.Service{DB: db}
	if err := svc.EnsureSystemRoles(context.Background()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return svc
}

// naiveOwnerCount counts the set's owners through the public API only: an
// enabled, unsuspended user with an active Assignment to a Role granting
// ownership. It is the independent model for the guard property.
func naiveOwnerCount(t *rapid.T, svc *auth.Service, set []string) int {
	return countHolders(t, svc, set)
}

// naiveGlobalCount counts every owner in the database. The serial phase
// shares one database across iterations, so its assertions run against the
// global count.
func naiveGlobalCount(t *rapid.T, svc *auth.Service) int {
	return countHolders(t, svc, nil)
}

// countHolders counts owners through the public API only. A nil set counts
// every user; otherwise only the set. Users and Roles are read once per
// count; Assignments are read per counted user.
func countHolders(t *rapid.T, svc *auth.Service, set []string) int {
	ctx := context.Background()
	users, err := svc.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	roles, err := svc.ListRoles(ctx)
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	owning := map[string]bool{}
	for _, r := range roles {
		for _, permission := range r.SystemPermissions {
			if permission == auth.PermissionAccessOwnership {
				owning[r.ID] = true
			}
		}
	}
	wanted := map[string]bool{}
	for _, name := range set {
		wanted[name] = true
	}
	count := 0
	for _, u := range users {
		if set != nil && !wanted[u.Username] {
			continue
		}
		if u.Disabled || u.State == auth.UserStateSuspended {
			continue
		}
		assignments, err := svc.ListAssignments(ctx, u.Username)
		if err != nil {
			t.Fatalf("list assignments: %v", err)
		}
		for _, a := range assignments {
			if a.Active && owning[a.RoleID] {
				count++
				break
			}
		}
	}
	return count
}

// TestLastOwnerProperty checks Property 2 over drawn demotion sequences,
// serial and concurrent: after every step at least one owner remains, and a
// refused demotion changes nothing.
//
// Feature: athena-batch-2, Property 2: Last owner.
//
// It lives here rather than auth_test.go: that file is in package auth,
// which cannot import internal/server for migrations without a cycle. The
// serial phase shares one database and asserts against the global owner
// count, which is what the guard preserves. Each race iteration gets a fresh
// database so the raced set is exactly the owners it demotes. Hashing is
// lowered for the run: the property is about the guard, not Argon2id cost.
func TestLastOwnerProperty(t *testing.T) {
	previous := auth.DefaultParams
	auth.DefaultParams = auth.Params{MemoryKiB: 8 * 1024, Time: 1, Threads: 1}
	t.Cleanup(func() { auth.DefaultParams = previous })
	ctx := context.Background()

	makeOwner := func(t *rapid.T, svc *auth.Service, username string) {
		t.Helper()
		_, token, err := svc.CreateUser(ctx, username, "", true, "prop")
		if err != nil {
			t.Fatalf("invite %s: %v", username, err)
		}
		if _, err := svc.CompleteChallenge(ctx, username, auth.ChallengePurposeFirstLogin, token, fmt.Sprintf("owner passphrase %s", username), "prop"); err != nil {
			t.Fatalf("complete %s: %v", username, err)
		}
	}

	// The serial phase shares one database across iterations and asserts
	// against the global owner count, since the guard counts globally too.
	// Only the race phase needs a fresh database per iteration: its exact
	// survivor count requires the raced set to be the only owners.
	tt := t
	serialSvc := ownerDB(tt)
	var serialSeq int
	rapid.Check(t, func(t *rapid.T) {
		resting := naiveGlobalCount(t, serialSvc)
		n := rapid.IntRange(2, 3).Draw(t, "owners")
		serialSeq++
		set := make([]string, 0, n)
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("s8s%dx%d", serialSeq, i)
			makeOwner(t, serialSvc, name)
			set = append(set, name)
		}
		before := naiveGlobalCount(t, serialSvc)
		if before != resting+n {
			t.Fatalf("setup holds %d owners, want %d", before, resting+n)
		}
		steps := rapid.IntRange(1, 8).Draw(t, "steps")
		for step := 0; step < steps; step++ {
			target := set[rapid.IntRange(0, n-1).Draw(t, "target")]
			var err error
			if rapid.IntRange(0, 1).Draw(t, "op") == 0 {
				disabled := true
				_, err = serialSvc.UpdateUser(ctx, target, auth.UserUpdate{Disabled: &disabled})
			} else {
				assignments, listErr := serialSvc.ListAssignments(ctx, target)
				if listErr != nil {
					t.Fatalf("list assignments: %v", listErr)
				}
				if len(assignments) == 0 {
					continue
				}
				err = serialSvc.RevokeAssignment(ctx, assignments[rapid.IntRange(0, len(assignments)-1).Draw(t, "assignment")].ID)
			}
			after := naiveGlobalCount(t, serialSvc)
			if after < 1 {
				t.Fatalf("step %d left no owner (err = %v)", step, err)
			}
			if err == nil {
				if after > before {
					t.Fatalf("step %d added owners: %d -> %d", step, before, after)
				}
			} else if errors.Is(err, auth.ErrLastOwner) || errors.Is(err, auth.ErrNotFound) {
				if after != before {
					t.Fatalf("step %d refused with %v but changed owners: %d -> %d", step, err, before, after)
				}
			} else {
				t.Fatalf("step %d: unexpected error %v", step, err)
			}
			before = after
		}
	})

	var raceSeq int
	rapid.Check(t, func(t *rapid.T) {
		raceSvc := ownerDB(tt)
		n := rapid.IntRange(2, 3).Draw(t, "owners")
		raceSeq++
		set := make([]string, 0, n)
		revoke := make([]bool, n)
		assignment := make([]string, n)
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("s8c%dx%d", raceSeq, i)
			makeOwner(t, raceSvc, name)
			set = append(set, name)
			revoke[i] = rapid.Bool().Draw(t, "revoke")
			if revoke[i] {
				assignments, err := raceSvc.ListAssignments(ctx, name)
				if err != nil || len(assignments) == 0 {
					t.Fatalf("list assignments: %v", err)
				}
				assignment[i] = assignments[0].ID
			}
		}
		start := make(chan struct{})
		results := make(chan error, n)
		for i := 0; i < n; i++ {
			go func() {
				<-start
				if revoke[i] {
					results <- raceSvc.RevokeAssignment(ctx, assignment[i])
					return
				}
				disabled := true
				_, err := raceSvc.UpdateUser(ctx, set[i], auth.UserUpdate{Disabled: &disabled})
				results <- err
			}()
		}
		close(start)
		wins := 0
		for i := 0; i < n; i++ {
			switch err := <-results; {
			case err == nil:
				wins++
			case errors.Is(err, auth.ErrLastOwner):
			default:
				t.Fatalf("owners=%d: unexpected error %v", n, err)
			}
		}
		if wins != n-1 {
			t.Fatalf("owners=%d: %d demotions succeeded, want exactly %d", n, wins, n-1)
		}
		if got := naiveOwnerCount(t, raceSvc, set); got != 1 {
			t.Fatalf("owners=%d: %d remain, want exactly 1", n, got)
		}
	})
}
