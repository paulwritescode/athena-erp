package auth

import (
	"context"
	"fmt"
)

// PurgeExpired deletes spent single-use Challenges and dead sessions: a
// Challenge that expired or was consumed, and a session past its absolute
// expiry or its idle deadline. It returns the deleted counts and keeps one
// minimal Access Log row; the counts stay out of the log with the job
// result instead. The purge job runs it on a schedule.
func (s *Service) PurgeExpired(ctx context.Context) (challenges, sessions int64, err error) {
	now := s.now()
	challengesResult, err := s.DB.ExecContext(ctx, `DELETE FROM athena_challenges
		WHERE expires_at <= ? OR consumed_at IS NOT NULL`, now)
	if err != nil {
		return 0, 0, fmt.Errorf("auth: purge challenges: %w", err)
	}
	challenges, err = challengesResult.RowsAffected()
	if err != nil {
		return 0, 0, fmt.Errorf("auth: purge challenges: %w", err)
	}
	sessionsResult, err := s.DB.ExecContext(ctx, `DELETE FROM athena_sessions
		WHERE expires_at <= ? OR TIMESTAMPADD(MINUTE, idle_minutes, last_seen_at) <= ?`, now, now)
	if err != nil {
		return 0, 0, fmt.Errorf("auth: purge sessions: %w", err)
	}
	sessions, err = sessionsResult.RowsAffected()
	if err != nil {
		return 0, 0, fmt.Errorf("auth: purge sessions: %w", err)
	}
	s.LogAccess(ctx, AccessEvent{Event: EventExpiredPurged, Outcome: OutcomeOK, Detail: "scheduled"})
	return challenges, sessions, nil
}
