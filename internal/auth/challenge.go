package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Challenge purposes. A first-login Challenge invites a user with no
// password; a reset Challenge lets an existing user choose a new one; an
// activation code is a short credential for a sensitive step such as an
// ownership transfer (ADR-0017).
const (
	ChallengePurposeFirstLogin = "first_login"
	ChallengePurposeReset      = "reset"
	ChallengePurposeActivation = "activation"
)

// Challenge lifetimes. A reset Challenge is short-lived because the user is
// waiting for it; an invite lasts a week so an administrator can hand it over;
// an activation code lasts an hour.
const (
	InviteLifetime     = 7 * 24 * time.Hour
	ResetLifetime      = 24 * time.Hour
	ActivationLifetime = time.Hour
	// MaxChallengeAttempts bounds wrong presentations of one Challenge. Five
	// matches the sign-in lockout, so short activation codes cannot be
	// guessed at scale.
	MaxChallengeAttempts = 5
)

// challengeLifetime returns the lifetime for a purpose.
func challengeLifetime(purpose string) (time.Duration, error) {
	switch purpose {
	case ChallengePurposeFirstLogin:
		return InviteLifetime, nil
	case ChallengePurposeReset:
		return ResetLifetime, nil
	case ChallengePurposeActivation:
		return ActivationLifetime, nil
	}
	return 0, fmt.Errorf("%w: unknown Challenge purpose %q", ErrInvalid, purpose)
}

// Challenge is one stored single-use credential. The raw value is shown once
// when issued and never stored; the row holds only its digest. Digest is
// loaded for comparison only and must never reach a log, an error or a span.
type Challenge struct {
	ID         string
	UserID     string
	Purpose    string
	Generation int
	ExpiresAt  time.Time
	Attempts   int
	Digest     string
}

// execQuerier is the database surface the auth helpers need inside or
// outside a transaction. Both *sql.DB and *sql.Tx satisfy it.
type execQuerier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// ChallengeDigest returns the stored form of a raw Challenge value.
func ChallengeDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ActivationAlphabet encodes short activation codes. It skips 0, O, 1, I and
// L so a code read aloud is unambiguous.
const ActivationAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

func newToken(purpose string) (string, error) {
	if purpose == ChallengePurposeActivation {
		raw := make([]byte, 8)
		if _, err := rand.Read(raw); err != nil {
			return "", fmt.Errorf("generate activation code: %w", err)
		}
		code := make([]byte, len(raw))
		for i, b := range raw {
			code[i] = ActivationAlphabet[int(b)%len(ActivationAlphabet)]
		}
		return string(code), nil
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate Challenge: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// IssueChallenge stores a Challenge for the user at the given generation and
// returns the row with the raw value, which the caller shows once. An older
// Challenge for the same user and purpose stays stored; it is superseded
// because its generation no longer matches the user's.
func IssueChallenge(ctx context.Context, db execQuerier, userID, purpose string, generation int, now time.Time) (Challenge, string, error) {
	lifetime, err := challengeLifetime(purpose)
	if err != nil {
		return Challenge{}, "", err
	}
	token, err := newToken(purpose)
	if err != nil {
		return Challenge{}, "", err
	}
	c := Challenge{ID: newID(), UserID: userID, Purpose: purpose, Generation: generation, ExpiresAt: now.Add(lifetime)}
	if _, err := db.ExecContext(ctx, "INSERT INTO athena_challenges (id, user_id, purpose, digest, generation, expires_at) VALUES (?,?,?,?,?,?)",
		c.ID, c.UserID, c.Purpose, ChallengeDigest(token), c.Generation, c.ExpiresAt); err != nil {
		return Challenge{}, "", fmt.Errorf("issue Challenge: %w", err)
	}
	return c, token, nil
}

// latestChallenge loads the newest Challenge for the user and purpose,
// whatever its state, so the caller can tell "no Challenge" from "expired",
// "consumed" and "superseded".
func latestChallenge(ctx context.Context, db execQuerier, userID, purpose string) (Challenge, bool, error) {
	var c Challenge
	var consumed sql.NullTime
	err := db.QueryRowContext(ctx, `SELECT id, user_id, purpose, generation, expires_at, attempts, digest, consumed_at FROM athena_challenges
		WHERE user_id = ? AND purpose = ? ORDER BY created_at DESC, id DESC LIMIT 1`, userID, purpose).
		Scan(&c.ID, &c.UserID, &c.Purpose, &c.Generation, &c.ExpiresAt, &c.Attempts, &c.Digest, &consumed)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Challenge{}, false, nil
	case err != nil:
		return Challenge{}, false, fmt.Errorf("read Challenge: %w", err)
	case consumed.Valid:
		return Challenge{}, false, errChallengeConsumed
	}
	return c, true, nil
}

var errChallengeConsumed = errors.New("consumed")

// challengeByID loads one Challenge row, locking it when forUpdate is set so
// a completion rechecks the row it is about to consume.
func challengeByID(ctx context.Context, db execQuerier, id string, forUpdate bool) (Challenge, error) {
	var c Challenge
	var consumed sql.NullTime
	query := `SELECT id, user_id, purpose, generation, expires_at, attempts, digest, consumed_at FROM athena_challenges WHERE id = ?`
	if forUpdate {
		query += " FOR UPDATE"
	}
	err := db.QueryRowContext(ctx, query, id).Scan(&c.ID, &c.UserID, &c.Purpose, &c.Generation, &c.ExpiresAt, &c.Attempts, &c.Digest, &consumed)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Challenge{}, fmt.Errorf("%w: Challenge", ErrNotFound)
	case err != nil:
		return Challenge{}, fmt.Errorf("read Challenge: %w", err)
	case consumed.Valid:
		return Challenge{}, errChallengeConsumed
	}
	return c, nil
}

// countAttempt records one wrong presentation of the Challenge.
func countAttempt(ctx context.Context, db execQuerier, id string) error {
	if _, err := db.ExecContext(ctx, "UPDATE athena_challenges SET attempts = attempts + 1 WHERE id = ?", id); err != nil {
		return fmt.Errorf("count Challenge attempt: %w", err)
	}
	return nil
}

// consumeChallenge marks the Challenge used. It reports false when another
// completion already consumed it, so two concurrent completions yield exactly
// one success.
func consumeChallenge(ctx context.Context, db execQuerier, id string, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx, "UPDATE athena_challenges SET consumed_at = ? WHERE id = ? AND consumed_at IS NULL", now, id)
	if err != nil {
		return false, fmt.Errorf("consume Challenge: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("consume Challenge: %w", err)
	}
	return n == 1, nil
}

// validateChallenge compares the presented value with the stored Challenge
// and validates it against the user's generation and the time. It returns a
// fixed reason phrase for the Access Log, or "" when the Challenge is good.
// wrongValue reports a mismatch, which the caller counts as an attempt; every
// other refusal leaves the counter alone. The caller maps every reason to one
// public error.
func validateChallenge(c Challenge, token string, generation int, now time.Time) (reason string, wrongValue bool) {
	if c.Attempts >= MaxChallengeAttempts {
		return "too many attempts", false
	}
	if subtle.ConstantTimeCompare([]byte(ChallengeDigest(token)), []byte(c.Digest)) != 1 {
		return "wrong value", true
	}
	switch {
	case now.After(c.ExpiresAt):
		return "expired", false
	case c.Generation != generation:
		return "superseded", false
	}
	return "", false
}
