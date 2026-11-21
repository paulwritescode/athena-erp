package auth

import (
	"context"
	"database/sql"
	"time"

	"athena-erp/internal/notifications"
)

// ActorID returns the account behind a change. Operator actions have no account.
func ActorID(ctx context.Context) string {
	if p := PrincipalFrom(ctx); p != nil {
		return p.UserID
	}
	return ""
}

func notifyAccess(ctx context.Context, tx *sql.Tx, userID string) error {
	return notifications.Write(ctx, tx, userID, notifications.Event{
		Kind: notifications.AccessChanged, SubjectType: "user", SubjectID: userID, ActorID: ActorID(ctx),
	})
}

// countFailedLogin serializes competing failures before deciding whether this
// attempt locks the account. A root account can receive an alert but never locks.
func (s *Service) countFailedLogin(ctx context.Context, user User, now time.Time, requestID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var attempts int
	var locked sql.NullTime
	if err := tx.QueryRowContext(ctx, "SELECT failed_attempts,locked_until FROM athena_users WHERE id=? FOR UPDATE", user.ID).Scan(&attempts, &locked); err != nil {
		return err
	}
	if locked.Valid && locked.Time.After(now) && !IsRoot(user.Username) {
		if err := writeAccess(ctx, tx, now, AccessEvent{Event: EventLoginFailed, Outcome: OutcomeDenied, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: "locked"}); err != nil {
			return err
		}
		return tx.Commit()
	}
	attempts++
	event := EventLoginFailed
	n := notifications.Event{Kind: notifications.SignInFailed, SubjectType: "user", SubjectID: user.ID, Params: notifications.Params{Attempts: attempts}}
	stored := attempts
	var until any
	if attempts >= MaxFailedAttempts && !IsRoot(user.Username) {
		stored, until, event, n.Kind = 0, now.Add(LockDuration), EventLockout, notifications.AccountLocked
	}
	if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET failed_attempts=?,locked_until=? WHERE id=?", stored, until, user.ID); err != nil {
		return err
	}
	if attempts >= 3 {
		if err := notifications.Write(ctx, tx, user.ID, n); err != nil {
			return err
		}
	}
	if err := writeAccess(ctx, tx, now, AccessEvent{Event: event, Outcome: OutcomeDenied, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: "wrong password"}); err != nil {
		return err
	}
	return tx.Commit()
}
