package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Security settings belong to one Workspace. Rate values are bucket capacities
// and the number of tokens replenished per named period.
const (
	IdleMinutes      = "security.session.idle_minutes"
	BuildIdleMinutes = "security.session.build_idle_minutes"
	AbsoluteHours    = "security.session.absolute_hours"
	SignInIP         = "security.rate.ip_per_minute"
	SignInUsername   = "security.rate.username_per_15_minutes"
	SessionWrites    = "security.rate.writes_per_minute"
	SessionReports   = "security.rate.reports_per_minute"
	SessionExports   = "security.rate.exports_per_minute"
)

type bounds struct{ initial, min, max int }

var securityBounds = map[string]bounds{
	IdleMinutes: {30, 5, 480}, BuildIdleMinutes: {15, 5, 480}, AbsoluteHours: {12, 1, 168},
	SignInIP: {10, 1, 10000}, SignInUsername: {5, 1, 10000}, SessionWrites: {60, 1, 10000},
	SessionReports: {10, 1, 10000}, SessionExports: {10, 1, 10000},
}

type SecuritySettings map[string]int

func validateSetting(key string, value int) error {
	b, ok := securityBounds[key]
	if !ok {
		return fmt.Errorf("%w: unknown security setting %q", ErrInvalid, key)
	}
	if value < b.min || value > b.max {
		return fmt.Errorf("%w: %s must be between %d and %d", ErrInvalid, key, b.min, b.max)
	}
	return nil
}

func readSecurity(ctx context.Context, db execQuerier) (SecuritySettings, error) {
	settings := SecuritySettings{}
	for key, b := range securityBounds {
		settings[key] = b.initial
	}
	rows, err := db.QueryContext(ctx, "SELECT setting_key, setting_value FROM athena_security_settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var value int
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if err := validateSetting(key, value); err != nil {
			return nil, err
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

func (s *Service) Security(ctx context.Context) (SecuritySettings, error) {
	return readSecurity(ctx, s.DB)
}

func (s *Service) SetSecurity(ctx context.Context, key string, value int) error {
	if err := validateSetting(key, value); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_security_settings (setting_key,setting_value) VALUES (?,?)
	ON DUPLICATE KEY UPDATE setting_value=VALUES(setting_value)`, key, value)
	return err
}

// Build access includes Desk administration permissions and their write-only counterparts.
func (p *Principal) BuildAccess() bool {
	for _, perm := range []string{PermissionBuild, PermissionFilesRead, PermissionFilesWrite, PermissionRulesRead, PermissionRulesWrite,
		PermissionLedgerRead, PermissionLedgerWrite, PermissionAccessUsers, PermissionAccessRoles, PermissionAccessAssign,
		PermissionAccessOwnership, PermissionAccessRecover, PermissionAccessSessions} {
		if p.HasSystem(perm) {
			return true
		}
	}
	return false
}

type SessionOptions struct {
	RememberDevice bool
	Device         string
	PreviousToken  string
}

type SessionEnded struct{ Reason string }

func (e *SessionEnded) Error() string { return ErrUnauthenticated.Error() }
func (e *SessionEnded) Unwrap() error { return ErrUnauthenticated }

const StepUpWindow = 15 * time.Minute

func (s *Service) NeedsStepUp(p *Principal) bool {
	return p != nil && p.BuildAccess() && !s.now().Before(p.StepUpAt.Add(StepUpWindow))
}

// Dated assignments change automatically at UTC midnight, even without an
// administrator editing a Role. End the session at the first such transition.
func permissionBoundary(p *Principal, now time.Time) *time.Time {
	var next *time.Time
	for _, a := range p.Assignments {
		for i, d := range []*time.Time{a.ActiveFrom, a.ExpiresAt} {
			if d == nil {
				continue
			}
			at := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
			if i == 1 {
				at = at.AddDate(0, 0, 1)
			}
			if at.After(now) && (next == nil || at.Before(*next)) {
				next = &at
			}
		}
	}
	return next
}

func (s *Service) startSession(ctx context.Context, tx *sql.Tx, userID string, now time.Time, opt SessionOptions) (LoginResult, error) {
	p, err := s.readPrincipal(ctx, tx, userID)
	if err != nil {
		return LoginResult{}, err
	}
	settings, err := readSecurity(ctx, tx)
	if err != nil {
		return LoginResult{}, err
	}
	idle, hours := settings[IdleMinutes], settings[AbsoluteHours]
	if p.BuildAccess() {
		idle = settings[BuildIdleMinutes]
	} else if opt.RememberDevice {
		hours = 168
		p.Remembered = true
	}
	p.SessionID, p.StepUpAt = newID(), now
	p.RateID = p.SessionID
	p.IdleDuration = time.Duration(idle) * time.Minute
	p.IdleExpiresAt = now.Add(p.IdleDuration)
	p.ExpiresAt = now.Add(time.Duration(hours) * time.Hour)
	token, err := newSessionToken()
	if err != nil {
		return LoginResult{}, err
	}
	if opt.PreviousToken != "" {
		if _, err := tx.ExecContext(ctx, "UPDATE athena_sessions SET revoked_at=? WHERE token_hash=? AND revoked_at IS NULL", now, hashToken(opt.PreviousToken)); err != nil {
			return LoginResult{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO athena_sessions
	(id, token_hash, user_id, created_at, last_seen_at, expires_at, idle_minutes, step_up_at, device, permissions_until, remembered, rate_id)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, p.SessionID, hashToken(token), userID, now, now, p.ExpiresAt, idle, now, opt.Device, permissionBoundary(p, now), p.Remembered, p.RateID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("start session: %w", err)
	}
	return LoginResult{Token: token, Principal: p, ExpiresAt: p.ExpiresAt}, nil
}

// StepUp atomically replaces both identifiers. A stale request cannot rotate a
// revoked session, and the original absolute deadline survives the rotation.
// A wrong password counts toward the account lockout, like a failed sign-in.
func (s *Service) StepUp(ctx context.Context, p *Principal, password, requestID string) (LoginResult, error) {
	if p == nil {
		return LoginResult{}, ErrUnauthenticated
	}
	if err := s.verifyActorPassword(ctx, p, password, "step up", requestID); err != nil {
		return LoginResult{}, err
	}
	now := s.now()
	token, err := newSessionToken()
	if err != nil {
		return LoginResult{}, err
	}
	id := newID()
	res, err := s.DB.ExecContext(ctx, `UPDATE athena_sessions SET id=?, token_hash=?, step_up_at=?, last_seen_at=?
	WHERE id=? AND user_id=? AND revoked_at IS NULL AND expires_at>? AND TIMESTAMPADD(MINUTE,idle_minutes,last_seen_at)>?
	AND (permissions_until IS NULL OR permissions_until>?)
	AND EXISTS (SELECT 1 FROM athena_users u WHERE u.id=? AND u.disabled=0)`, id, hashToken(token), now, now, p.SessionID, p.UserID, now, now, now, p.UserID)
	if err != nil {
		return LoginResult{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return LoginResult{}, err
	}
	if n != 1 {
		return LoginResult{}, &SessionEnded{"revoked"}
	}
	s.LogAccess(ctx, AccessEvent{Event: EventStepUp, Outcome: OutcomeOK, UserID: p.UserID, Username: p.Username, RequestID: requestID})
	q := *p
	q.SessionID, q.StepUpAt, q.IdleExpiresAt = id, now, now.Add(p.IdleDuration)
	return LoginResult{Token: token, Principal: &q, ExpiresAt: p.ExpiresAt}, nil
}
