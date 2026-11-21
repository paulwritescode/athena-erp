// Package auth_test holds the database-backed Challenge tests. It is an
// external test package because applying the system migrations needs
// internal/server, which imports internal/auth.
package auth_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"athena-erp/internal/auth"
	"athena-erp/internal/server"
	"athena-erp/internal/store"
	"pgregory.net/rapid"
)

func challengeService(t *testing.T) (*auth.Service, *time.Time) {
	t.Helper()
	db := sharedDatabase(t)
	if _, err := store.Up(context.Background(), db, store.MariaDB{}, server.SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &auth.Service{DB: db, Now: func() time.Time { return now }}, &now
}

func TestInviteCompleteAndLogin(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	const password = "a first login passphrase"

	user, token, err := svc.CreateUser(ctx, "ada", "Ada", false, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if user.State != auth.UserStateInvited {
		t.Fatalf("state = %q, want invited", user.State)
	}
	if token == "" {
		t.Fatal("no invite Challenge was returned")
	}
	if _, err := svc.Login(ctx, "ada", password, "request-2"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("an invited user signed in: %v", err)
	}
	result, err := svc.CompleteChallenge(ctx, "ada", auth.ChallengePurposeFirstLogin, token, password, "request-3")
	if err != nil {
		t.Fatal(err)
	}
	if result.Token == "" || result.Principal == nil || result.Principal.Username != "ada" {
		t.Fatalf("completion returned %+v", result)
	}
	if _, err := svc.Login(ctx, "ada", password, "request-4"); err != nil {
		t.Fatalf("sign-in after completion: %v", err)
	}
	if _, err := svc.CompleteChallenge(ctx, "ada", auth.ChallengePurposeFirstLogin, token, "another long passphrase", "request-5"); !errors.Is(err, auth.ErrChallengeRefused) {
		t.Fatalf("a consumed Challenge completed again: %v", err)
	}
}

func TestChallengeValueNeverStoredOrLogged(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	user, token, err := svc.CreateUser(ctx, "bob", "", false, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	var digest string
	if err := svc.DB.QueryRowContext(ctx, "SELECT digest FROM athena_challenges WHERE user_id = ?", user.ID).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	if digest != auth.ChallengeDigest(token) || strings.Contains(digest, token) {
		t.Fatal("the Challenge row does not hold exactly the digest")
	}
	if _, err := svc.CompleteChallenge(ctx, "bob", auth.ChallengePurposeFirstLogin, "wrong-token-value", "a long enough passphrase", "request-2"); !errors.Is(err, auth.ErrChallengeRefused) {
		t.Fatalf("a wrong Challenge value completed: %v", err)
	}
	rows, err := svc.ListAccessLog(ctx, 50, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if strings.Contains(r.Detail, token) || strings.Contains(r.Detail, "wrong-token-value") {
			t.Fatalf("the Access Log holds a Challenge value: %+v", r)
		}
	}
	if len(rows) == 0 {
		t.Fatal("no Access Log rows were written")
	}
}

func TestWrongChallengeValuesBurnTheChallenge(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	const password = "a first login passphrase"
	if _, _, err := svc.CreateUser(ctx, "carol", "", false, "request-1"); err != nil {
		t.Fatal(err)
	}
	_, token, err := svc.ResendInvite(ctx, "carol", "request-2")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < auth.MaxChallengeAttempts; i++ {
		if _, err := svc.CompleteChallenge(ctx, "carol", auth.ChallengePurposeFirstLogin, "wrong-token-value", password, "request-3"); !errors.Is(err, auth.ErrChallengeRefused) {
			t.Fatalf("wrong value %d completed: %v", i, err)
		}
	}
	if _, err := svc.CompleteChallenge(ctx, "carol", auth.ChallengePurposeFirstLogin, token, password, "request-4"); !errors.Is(err, auth.ErrChallengeRefused) {
		t.Fatalf("the Challenge survived %d wrong values: %v", auth.MaxChallengeAttempts, err)
	}
}

func TestResendInviteSupersedesTheEarlierChallenge(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	const password = "a first login passphrase"
	if _, first, err := svc.CreateUser(ctx, "dave", "", false, "request-1"); err != nil {
		t.Fatal(err)
	} else if _, second, err := svc.ResendInvite(ctx, "dave", "request-2"); err != nil {
		t.Fatal(err)
	} else {
		if _, err := svc.CompleteChallenge(ctx, "dave", auth.ChallengePurposeFirstLogin, first, password, "request-3"); !errors.Is(err, auth.ErrChallengeRefused) {
			t.Fatalf("the superseded Challenge completed: %v", err)
		}
		if _, err := svc.CompleteChallenge(ctx, "dave", auth.ChallengePurposeFirstLogin, second, password, "request-4"); err != nil {
			t.Fatalf("the re-sent Challenge did not complete: %v", err)
		}
	}
}

func TestResetRevokesSessionsAndNeedsTheChallenge(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	const password = "an active user passphrase"
	const replacement = "a replacement passphrase"
	user, token, err := svc.CreateUser(ctx, "erin", "", false, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteChallenge(ctx, user.Username, auth.ChallengePurposeFirstLogin, token, password, "request-2"); err != nil {
		t.Fatal(err)
	}
	login, err := svc.Login(ctx, "erin", password, "request-3")
	if err != nil {
		t.Fatal(err)
	}
	_, firstReset, err := svc.StartReset(ctx, "erin", "", "request-4")
	if err != nil {
		t.Fatal(err)
	}
	_, resetToken, err := svc.StartReset(ctx, "erin", "", "request-5")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteChallenge(ctx, "erin", auth.ChallengePurposeReset, firstReset, replacement, "request-5b"); !errors.Is(err, auth.ErrChallengeRefused) {
		t.Fatalf("the superseded reset Challenge completed: %v", err)
	}
	if _, err := svc.Authenticate(ctx, login.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("a session survived the reset: %v", err)
	}
	if _, err := svc.Login(ctx, "erin", password, "request-6"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("the old password signed in during a reset: %v", err)
	}
	if _, err := svc.CompleteChallenge(ctx, "erin", auth.ChallengePurposeReset, resetToken, replacement, "request-7"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, "erin", replacement, "request-8"); err != nil {
		t.Fatalf("sign-in after the reset: %v", err)
	}
}

func TestExpiredChallengeIsRefused(t *testing.T) {
	svc, now := challengeService(t)
	ctx := context.Background()
	if _, _, err := svc.CreateUser(ctx, "frank", "", false, "request-1"); err != nil {
		t.Fatal(err)
	}
	_, token, err := svc.ResendInvite(ctx, "frank", "request-2")
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(auth.InviteLifetime + time.Hour)
	if _, err := svc.CompleteChallenge(ctx, "frank", auth.ChallengePurposeFirstLogin, token, "a first login passphrase", "request-3"); !errors.Is(err, auth.ErrChallengeRefused) {
		t.Fatalf("an expired Challenge completed: %v", err)
	}
}

func TestBootstrapOwnerRefusesASecondOwner(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	if _, _, err := svc.BootstrapOwner(ctx, "owner", "", "request-1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.BootstrapOwner(ctx, "second", "", "request-2"); !errors.Is(err, auth.ErrExists) {
		t.Fatalf("a second bootstrap = %v, want ErrExists", err)
	}
}

func TestActivationCodeIsShortAndSingleUse(t *testing.T) {
	svc, _ := challengeService(t)
	ctx := context.Background()
	user, token, err := svc.CreateUser(ctx, "gina", "", false, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteChallenge(ctx, user.Username, auth.ChallengePurposeFirstLogin, token, "an active user passphrase", "request-2"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.IssueActivationCode(ctx, "gina", "request-3"); err != nil {
		t.Fatal(err)
	}
	_, code, err := svc.IssueActivationCode(ctx, "gina", "request-4")
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 8 {
		t.Fatalf("activation code = %q, want 8 characters", code)
	}
	for _, c := range code {
		if !strings.ContainsRune(auth.ActivationAlphabet, c) {
			t.Fatalf("activation code = %q, want only unambiguous characters", code)
		}
	}
	if _, err := svc.VerifyActivationCode(ctx, "gina", code, "request-5"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyActivationCode(ctx, "gina", code, "request-6"); !errors.Is(err, auth.ErrChallengeRefused) {
		t.Fatalf("a consumed activation code verified again: %v", err)
	}
}

// TestChallengeSingleUseProperty checks Property 1 over drawn races: for any
// Challenge and any number of concurrent completions, exactly one succeeds
// and every later attempt is refused.
//
// Feature: athena-batch-2, Property 1: Challenge single use.
//
// It lives here rather than auth_test.go: that file is in package auth, which
// cannot import internal/server for migrations without a cycle. rapid runs 100
// iterations by default, the spec minimum. Hashing is lowered for the run:
// the property is about concurrency, not Argon2id cost.
func TestChallengeSingleUseProperty(t *testing.T) {
	db := sharedDatabase(t)
	if _, err := store.Up(context.Background(), db, store.MariaDB{}, server.SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	previous := auth.DefaultParams
	auth.DefaultParams = auth.Params{MemoryKiB: 8 * 1024, Time: 1, Threads: 1}
	t.Cleanup(func() { auth.DefaultParams = previous })
	svc := &auth.Service{DB: db}
	ctx := context.Background()
	var seq int
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(2, 6).Draw(t, "completions")
		useReset := rapid.Bool().Draw(t, "reset")
		mixWrong := rapid.Bool().Draw(t, "wrongValueMixed")
		seeds := rapid.SliceOfN(rapid.IntRange(0, 1<<30), n, n).Draw(t, "passwords")
		seq++
		username := fmt.Sprintf("prop%d", seq)
		password := func(i int) string { return fmt.Sprintf("correct horse battery %d %d", seq, seeds[i]) }

		_, invite, err := svc.CreateUser(ctx, username, "", false, "prop")
		if err != nil {
			t.Fatalf("invite: %v", err)
		}
		purpose, token := auth.ChallengePurposeFirstLogin, invite
		if useReset {
			if _, err := svc.CompleteChallenge(ctx, username, purpose, invite, fmt.Sprintf("seed passphrase %d", seq), "prop"); err != nil {
				t.Fatalf("seed completion: %v", err)
			}
			if _, token, err = svc.StartReset(ctx, username, "", "prop"); err != nil {
				t.Fatalf("seed reset: %v", err)
			}
			purpose = auth.ChallengePurposeReset
		}
		start := make(chan struct{})
		results := make(chan error, n)
		for i := 0; i < n; i++ {
			presented, pw := token, password(i)
			if mixWrong && i == 0 {
				presented = "definitely the wrong value"
			}
			go func() {
				<-start
				_, err := svc.CompleteChallenge(ctx, username, purpose, presented, pw, "prop")
				results <- err
			}()
		}
		close(start)
		wins := 0
		for i := 0; i < n; i++ {
			switch err := <-results; {
			case err == nil:
				wins++
			case errors.Is(err, auth.ErrChallengeRefused):
			default:
				t.Fatalf("completions=%d reset=%v wrongMixed=%v: unexpected error %v", n, useReset, mixWrong, err)
			}
		}
		if wins != 1 {
			t.Fatalf("completions=%d reset=%v wrongMixed=%v: %d succeeded, want exactly 1", n, useReset, mixWrong, wins)
		}
		if _, err := svc.CompleteChallenge(ctx, username, purpose, token, fmt.Sprintf("later attempt passphrase %d", seq), "prop"); !errors.Is(err, auth.ErrChallengeRefused) {
			t.Fatalf("completions=%d reset=%v wrongMixed=%v: a later attempt succeeded: %v", n, useReset, mixWrong, err)
		}
		var state string
		if err := db.QueryRowContext(ctx, "SELECT state FROM athena_users WHERE username = ?", username).Scan(&state); err != nil || state != auth.UserStateActive {
			t.Fatalf("user state = %q, %v; want active", state, err)
		}
		var consumed sql.NullTime
		if err := db.QueryRowContext(ctx, "SELECT consumed_at FROM athena_challenges WHERE user_id = (SELECT id FROM athena_users WHERE username = ?) AND purpose = ? ORDER BY created_at DESC LIMIT 1",
			username, purpose).Scan(&consumed); err != nil || !consumed.Valid {
			t.Fatalf("latest %s Challenge consumed_at valid = %v, %v", purpose, consumed.Valid, err)
		}
	})
}
