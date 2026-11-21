package auth

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Access Log events. The log records who signed in, who failed to, and what
// was denied. It holds identifiers and outcome codes only: never a password,
// a token or a Record value.
const (
	EventLogin           = "login"
	EventLoginFailed     = "login_failed"
	EventLockout         = "lockout"
	EventLogout          = "logout"
	EventDenied          = "denied"
	EventPasswordChanged = "password_changed"
	EventPasswordReset   = "password_reset"
	EventUserCreated     = "user_created"
	EventUserUpdated     = "user_updated"
	EventRoleSaved       = "role_saved"
	EventRoleDeleted     = "role_deleted"
	// Challenge events record invite and reset delivery and completion. The
	// detail names the purpose and the outcome only; the Challenge value
	// itself never reaches the log (ADR-0017).
	EventInviteCreated     = "invite_created"
	EventInviteResent      = "invite_resent"
	EventResetStarted      = "reset_started"
	EventChallengeIssued   = "challenge_issued"
	EventChallengeComplete = "challenge_completed"
	EventChallengeFailed   = "challenge_failed"
	// EventRateLimited records a rejected request. The row carries the
	// procedure and the limit category only, never the request values.
	EventRateLimited = "rate_limited"
	// EventStepUp records a password re-verification for a privileged
	// action (step-up, ownership transfer, reset start). The failure rows
	// never carry the password.
	EventStepUp       = "step_up"
	EventStepUpFailed = "step_up_failed"
)

// Outcomes of a logged event.
const (
	OutcomeOK     = "ok"
	OutcomeDenied = "denied"
)

// AccessEvent is one Access Log row to write.
type AccessEvent struct {
	Event      string
	Outcome    string
	UserID     string
	Username   string
	Procedure  string
	RecordType string
	Action     string
	RequestID  string
	// Detail is a short fixed phrase such as "locked", never user input.
	Detail string
}

// AccessRow is one stored Access Log row.
type AccessRow struct {
	ID int64
	At time.Time
	AccessEvent
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// LogAccess appends a row. A failure is reported to the process log without
// the event's content, because the caller is already answering a request and
// has nothing better to do with it.
func (s *Service) LogAccess(ctx context.Context, e AccessEvent) {
	err := writeAccess(context.WithoutCancel(ctx), s.DB, s.now(), e)
	if err != nil {
		log.Printf("access log: could not write a %s row", e.Event)
	}
}

func writeAccess(ctx context.Context, db execQuerier, at time.Time, e AccessEvent) error {
	_, err := db.ExecContext(ctx, "INSERT INTO athena_access_log (at, event, outcome, user_id, username, procedure_name, record_type, action, request_id, detail) VALUES (?,?,?,?,?,?,?,?,?,?)",
		at, e.Event, e.Outcome, nullable(e.UserID), nullable(e.Username), nullable(e.Procedure), nullable(e.RecordType), nullable(e.Action), nullable(e.RequestID), nullable(e.Detail))
	return err
}

// ListAccessLog returns rows newest first. beforeID, when positive, returns
// only rows with a smaller ID, for paging. event, when set, filters by event.
func (s *Service) ListAccessLog(ctx context.Context, limit int, beforeID int64, event string) ([]AccessRow, error) {
	query := "SELECT id, at, event, outcome, user_id, username, procedure_name, record_type, action, request_id, detail FROM athena_access_log WHERE 1=1"
	var args []any
	if beforeID > 0 {
		query += " AND id < ?"
		args = append(args, beforeID)
	}
	if event != "" {
		query += " AND event = ?"
		args = append(args, event)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read access log: %w", err)
	}
	defer rows.Close()
	var out []AccessRow
	for rows.Next() {
		var r AccessRow
		var user, name, procedure, recordType, action, request, detail sql.NullString
		if err := rows.Scan(&r.ID, &r.At, &r.Event, &r.Outcome, &user, &name, &procedure, &recordType, &action, &request, &detail); err != nil {
			return nil, fmt.Errorf("read access log: %w", err)
		}
		r.UserID, r.Username, r.Procedure, r.RecordType, r.Action, r.RequestID, r.Detail = user.String, name.String, procedure.String, recordType.String, action.String, request.String, detail.String
		out = append(out, r)
	}
	return out, rows.Err()
}
