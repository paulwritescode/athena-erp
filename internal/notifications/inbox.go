// Package notifications stores inbox items alongside the event that caused them.
package notifications

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	v1 "athena-erp/gen/athena/v1"
	"github.com/google/uuid"
)

type Kind string

const (
	InviteAccepted Kind = "invite_accepted"
	AccessChanged  Kind = "access_changed"
	SignInFailed   Kind = "sign_in_failed"
	AccountLocked  Kind = "account_locked"
	SessionEnded   Kind = "session_ended"
	SaveStopped    Kind = "save_stopped"
	PeriodClosed   Kind = "period_closed"
)

// Params is deliberately closed to arbitrary data from Records or requests.
type Params struct {
	Attempts int `json:"attempts,omitempty"`
}
type Event struct {
	Kind                                 Kind
	SubjectType, SubjectID, SubjectLabel string
	ActorID                              string
	Params                               Params
}

func Write(ctx context.Context, tx *sql.Tx, recipient string, e Event) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	params, err := json.Marshal(e.Params)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO athena_notifications
 (id,recipient_user_id,kind,subject_type,subject_id,subject_label,actor_user_id,params,created_at)
 VALUES (?,?,?,?,?,?,NULLIF(?,''),?,UTC_TIMESTAMP(6))`, id.String(), recipient, e.Kind, e.SubjectType, e.SubjectID, e.SubjectLabel, e.ActorID, params)
	return err
}

func message(kind Kind, label string, p Params) string {
	switch kind {
	case InviteAccepted:
		return label + " accepted your invitation."
	case AccessChanged:
		return "Your access has changed."
	case SignInFailed:
		return fmt.Sprintf("There have been %d unsuccessful attempts to sign in to your account.", p.Attempts)
	case AccountLocked:
		return "Sign-in to your account has been temporarily locked after repeated unsuccessful attempts."
	case SessionEnded:
		return "An administrator ended your session."
	case SaveStopped:
		return "A check stopped your save. Review the form and try again."
	case PeriodClosed:
		return label + " is closed for changes."
	default:
		return "You have a new notification."
	}
}

var ErrInput = errors.New("invalid inbox request")

func List(ctx context.Context, db *sql.DB, recipient string, size int32, token string, unread bool) (*v1.ListNotificationsResponse, error) {
	if size == 0 {
		size = 30
	}
	if size < 1 || size > 100 {
		return nil, fmt.Errorf("%w: page size must be between 1 and 100", ErrInput)
	}
	query := `SELECT n.id,n.kind,n.subject_type,n.subject_id,n.subject_label,COALESCE(n.actor_user_id,''),COALESCE(u.display_name,''),n.params,n.created_at,n.read_at
 FROM athena_notifications n LEFT JOIN athena_users u ON u.id=n.actor_user_id WHERE n.recipient_user_id=?`
	args := []any{recipient}
	if unread {
		query += " AND n.read_at IS NULL"
	}
	if token != "" {
		raw, err := base64.RawURLEncoding.DecodeString(token)
		stamp, id, ok := strings.Cut(string(raw), "|")
		at, timeErr := time.Parse(time.RFC3339Nano, stamp)
		if err != nil || !ok || timeErr != nil || uuid.Validate(id) != nil {
			return nil, fmt.Errorf("%w: invalid page token", ErrInput)
		}
		query += " AND (n.created_at < ? OR (n.created_at = ? AND n.id < ?))"
		args = append(args, at.UTC(), at.UTC(), id)
	}
	rows, err := db.QueryContext(ctx, query+" ORDER BY n.created_at DESC,n.id DESC LIMIT ?", append(args, size+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &v1.ListNotificationsResponse{}
	for rows.Next() {
		if len(out.Notifications) == int(size) {
			last := out.Notifications[size-1]
			out.NextPageToken = base64.RawURLEncoding.EncodeToString([]byte(last.CreatedAt + "|" + last.Id))
			break
		}
		n := &v1.Notification{}
		var created time.Time
		var read sql.NullTime
		var params []byte
		if err := rows.Scan(&n.Id, &n.Kind, &n.SubjectType, &n.SubjectId, &n.SubjectLabel, &n.ActorUserId, &n.ActorLabel, &params, &created, &read); err != nil {
			return nil, err
		}
		var p Params
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		n.Text = message(Kind(n.Kind), n.SubjectLabel, p)
		n.CreatedAt = created.UTC().Format(time.RFC3339Nano)
		if read.Valid {
			n.ReadAt = read.Time.UTC().Format(time.RFC3339Nano)
		}
		out.Notifications = append(out.Notifications, n)
	}
	return out, rows.Err()
}

func UnreadCount(ctx context.Context, db *sql.DB, recipient string) (int64, error) {
	var count int64
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM athena_notifications WHERE recipient_user_id=? AND read_at IS NULL", recipient).Scan(&count)
	return count, err
}

const markSQL = "UPDATE athena_notifications SET read_at=UTC_TIMESTAMP(6) WHERE recipient_user_id=? AND read_at IS NULL"

func MarkRead(ctx context.Context, db *sql.DB, recipient string, ids []string) (int64, error) {
	if len(ids) < 1 || len(ids) > 100 {
		return 0, fmt.Errorf("%w: choose 1 to 100 notifications", ErrInput)
	}
	args := []any{recipient}
	for _, id := range ids {
		if uuid.Validate(id) != nil {
			return 0, fmt.Errorf("%w: invalid notification ID", ErrInput)
		}
		args = append(args, id)
	}
	result, err := db.ExecContext(ctx, markSQL+" AND id IN (?"+strings.Repeat(",?", len(ids)-1)+")", args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
func MarkAllRead(ctx context.Context, db *sql.DB, recipient string) (int64, error) {
	result, err := db.ExecContext(ctx, markSQL, recipient)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
