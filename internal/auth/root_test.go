// Root administrator tests (ADR-0018): setup seeding, the reserved name,
// always-sign-in, the evaluation bypass, suspension refusal, and the
// bootstrap exclusion.
package auth_test

import (
	"context"
	"errors"
	"testing"

	"athena-erp/internal/auth"
)

func TestEnsureRootSeedsFromTheDatabasePassword(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	const dbPassword = "the database password here"
	generated, err := svc.EnsureRoot(ctx, dbPassword)
	if err != nil {
		t.Fatal(err)
	}
	if generated != "" {
		t.Fatal("a usable database password was not used")
	}
	if _, err := svc.Login(ctx, "administrator", dbPassword, "seed"); err != nil {
		t.Fatalf("root sign-in: %v", err)
	}
	users, err := svc.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != "administrator" || !users[0].Owner || users[0].State != auth.UserStateActive {
		t.Fatalf("users = %+v", users)
	}
	generated, err = svc.EnsureRoot(ctx, "another database password")
	if err != nil || generated != "" {
		t.Fatalf("reseed = %q, %v; want idempotent", generated, err)
	}
	if _, err := svc.Login(ctx, "administrator", dbPassword, "seed"); err != nil {
		t.Fatalf("root sign-in after reseed: %v", err)
	}
}

func TestEnsureRootGeneratesWhenTheDatabasePasswordFailsPolicy(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	for _, dbPassword := range []string{"", "short"} {
		generated, err := svc.EnsureRoot(ctx, dbPassword)
		if err != nil {
			t.Fatal(err)
		}
		if generated == "" {
			t.Fatalf("no password was generated for %q", dbPassword)
		}
		if _, err := svc.Login(ctx, "administrator", generated, "seed"); err != nil {
			t.Fatalf("root sign-in with the generated password: %v", err)
		}
		if _, err := svc.DB.ExecContext(ctx, "DELETE FROM athena_users WHERE username = 'administrator'"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRootUsernameIsReserved(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	if _, _, err := svc.CreateUser(ctx, "Administrator", "", false, "seed"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("inviting administrator = %v, want ErrInvalid", err)
	}
	if _, _, err := svc.BootstrapOwner(ctx, "administrator", "", "seed"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("bootstrapping administrator = %v, want ErrInvalid", err)
	}
}

func TestRootAlwaysSignsIn(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	const dbPassword = "the database password here"
	if _, err := svc.EnsureRoot(ctx, dbPassword); err != nil {
		t.Fatal(err)
	}
	// Wrong passwords fail without counting: five of them lock nobody out.
	for i := 0; i < auth.MaxFailedAttempts; i++ {
		if _, err := svc.Login(ctx, "administrator", "wrong password here", "seed"); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("wrong root password %d = %v", i, err)
		}
	}
	if _, err := svc.Login(ctx, "administrator", dbPassword, "seed"); err != nil {
		t.Fatalf("root sign-in after wrong passwords: %v", err)
	}
	// Suspension is refused, so the account cannot be shut off.
	disabled := true
	if _, err := svc.UpdateUser(ctx, "administrator", auth.UserUpdate{Disabled: &disabled}); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("suspending root = %v, want ErrInvalid", err)
	}
}

func TestRootBypassesEvaluation(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	const dbPassword = "the database password here"
	if _, err := svc.EnsureRoot(ctx, dbPassword); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Login(ctx, "administrator", dbPassword, "seed")
	if err != nil {
		t.Fatal(err)
	}
	p := result.Principal
	if p == nil || !p.Root {
		t.Fatalf("root principal = %+v", p)
	}
	if !p.HasSystem(auth.PermissionBuild) || !p.Can("invoice", auth.ActionDelete) {
		t.Fatal("root was denied")
	}
	if p.Level("invoice") != auth.MaxPermissionLevel {
		t.Fatalf("root level = %d", p.Level("invoice"))
	}
	if _, unrestricted := p.Scope("invoice", auth.ActionRead); !unrestricted {
		t.Fatal("root scope is restricted")
	}
	if p.MustSeparateDuties("invoice", auth.ActionSubmit) {
		t.Fatal("root separates duties")
	}
}

func TestRootNeverCountsForTheGuard(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	// The startup pair, twice: a second boot must not assign the root.
	for i := 0; i < 2; i++ {
		if err := svc.EnsureSystemRoles(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.EnsureRoot(ctx, "the database password here"); err != nil {
			t.Fatal(err)
		}
	}
	assignments, err := svc.ListAssignments(ctx, "administrator")
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 0 {
		t.Fatalf("root assignments = %+v, want none", assignments)
	}
	if _, _, err := svc.BootstrapOwner(ctx, "owner", "", "seed"); err != nil {
		t.Fatal(err)
	}
	disabled := true
	if _, err := svc.UpdateUser(ctx, "owner", auth.UserUpdate{Disabled: &disabled}); !errors.Is(err, auth.ErrLastOwner) {
		t.Fatalf("disabling the only owner with root present = %v, want ErrLastOwner", err)
	}
}

func TestBootstrapIgnoresRoot(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	if _, err := svc.EnsureRoot(ctx, "the database password here"); err != nil {
		t.Fatal(err)
	}
	// Emergency recovery still works while no owner exists.
	_, token, err := svc.BootstrapOwner(ctx, "owner", "", "seed")
	if err != nil {
		t.Fatalf("bootstrap with only root present: %v", err)
	}
	const password = "an owner passphrase here"
	if _, err := svc.CompleteChallenge(ctx, "owner", auth.ChallengePurposeFirstLogin, token, password, "seed"); err != nil {
		t.Fatal(err)
	}
	login, err := svc.Login(ctx, "owner", password, "seed")
	if err != nil {
		t.Fatal(err)
	}
	ownerCtx := auth.WithPrincipal(ctx, login.Principal)
	if _, err := svc.TransferOwnership(ownerCtx, "administrator", password, false); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("transferring to root = %v, want ErrInvalid", err)
	}
}
