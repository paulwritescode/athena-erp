package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// Retention classes name the Z4 schedule (ADR-0033). The ledger,
// invoice, stock and audit classes keep rows 7 years and are never
// auto-deleted in beta; guest rows anonymise 90 days after the event;
// system rows purge on their table period.
const (
	ClassLedger  = "ledger"
	ClassInvoice = "invoice"
	ClassStock   = "stock"
	ClassAudit   = "audit"
	ClassGuest   = "guest"
)

// System retention keys name the purgeable system tables. They ride
// the disposal register's record_type column so holds and outcomes
// cover system rows the same way as Records.
const (
	SystemAccessLog = "access_log"
	SystemOutbox    = "outbox"
	SystemEvents    = "events"
	SystemJobs      = "jobs"
)

// Disposal actions name what applying a candidate does.
const (
	ActionAnonymise = "anonymise"
	ActionDelete    = "delete"
	ActionNone      = "none"
)

// Disposal outcomes name the recorded result of one candidate.
const (
	OutcomeDeleted    = "deleted"
	OutcomeAnonymised = "anonymised"
	OutcomeSkipped    = "skipped"
	OutcomeException  = "exception"
)

// Retention triggers name the timestamp column a Record Type's clock
// starts from.
const (
	TriggerCreated = "created"
	TriggerUpdated = "updated"
)

// retentionPeriods maps every disposable class and system key to its
// Z4 period. A year is 365 days; the 7-year set is absent on purpose,
// so no period lookup can ever schedule it.
var retentionPeriods = map[string]time.Duration{
	ClassGuest:      90 * 24 * time.Hour,
	SystemAccessLog: 365 * 24 * time.Hour,
	SystemOutbox:    30 * 24 * time.Hour,
	SystemEvents:    30 * 24 * time.Hour,
	SystemJobs:      90 * 24 * time.Hour,
}

// neverAutoDelete holds the 7-year retention classes. Apply refuses
// them even if a caller lists them as candidates.
var neverAutoDelete = map[string]bool{
	ClassLedger:  true,
	ClassInvoice: true,
	ClassStock:   true,
	ClassAudit:   true,
}

// candidateCap bounds one preview listing per source, so a large
// backlog disposes in batches instead of one giant transaction.
const candidateCap = 500

// Candidate is one row disposal may act on: a user Record by Type key
// and id, or a system row by retention key and id.
type Candidate struct {
	RecordType string
	RecordID   string
	Class      string
	Action     string
	Reason     string
}

// Outcome is the recorded result of applying one candidate.
type Outcome struct {
	Candidate Candidate
	Result    string
	Detail    string
}

// RecordProfile carries one Record Type's lifetime block to the
// disposal preview. The server builds it from stored definitions;
// disposal never reads definitions itself.
type RecordProfile struct {
	Key     string
	Class   string
	Trigger string
	Action  string
	Hold    bool
}

// LinksTo reports whether any other Record links to a Record. The
// server supplies it; disposal refuses linked candidates as
// dependent evidence.
type LinksTo func(ctx context.Context, recordType, recordID string) (bool, error)

// recordKeyPattern matches a Record Type key, so resolved table names
// cannot inject SQL. It mirrors the server's recordTypeKeyPattern.
var recordKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,50}$`)

// resolveTable maps a candidate's Record Type to its table and whether
// the id column is an integer (the access log) rather than text.
func resolveTable(recordType string) (table string, idIsInt bool, err error) {
	switch recordType {
	case SystemAccessLog:
		return "athena_access_log", true, nil
	case SystemOutbox:
		return "athena_outbox", false, nil
	case SystemEvents:
		return "athena_events", false, nil
	case SystemJobs:
		return "athena_jobs", false, nil
	}
	if !recordKeyPattern.MatchString(recordType) {
		return "", false, fmt.Errorf("jobs: disposal cannot resolve Record Type %q", recordType)
	}
	return "p_workspace__" + recordType, false, nil
}

// SystemCandidates lists past-retention system rows: access log rows
// older than a year, dispatched or dead outbox rows older than 30
// days, events older than 30 days, and finished jobs older than 90
// days. Pending outbox rows and queued or leased jobs never list:
// they are live work, not history.
func SystemCandidates(ctx context.Context, q Querier, now time.Time) ([]Candidate, error) {
	ctx, span := otel.Tracer("athena/jobs").Start(ctx, "athena.jobs.disposal_preview_system")
	defer span.End()
	outcome := "ok"
	defer func() { span.SetAttributes(attribute.String("athena.outcome", outcome)) }()
	candidates, err := systemCandidates(ctx, q, now)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		return nil, err
	}
	span.SetAttributes(attribute.Int("athena.candidates", len(candidates)))
	return candidates, nil
}

func systemCandidates(ctx context.Context, q Querier, now time.Time) ([]Candidate, error) {
	var out []Candidate
	queries := []struct {
		key    string
		class  string
		action string
		sql    string
		since  time.Duration
		intID  bool
	}{
		{SystemAccessLog, SystemAccessLog, ActionDelete, `SELECT id FROM athena_access_log WHERE at < ? ORDER BY id LIMIT 500`, retentionPeriods[SystemAccessLog], true},
		{SystemOutbox, SystemOutbox, ActionDelete, `SELECT id FROM athena_outbox WHERE status IN ('dispatched', 'dead') AND created_at < ? ORDER BY created_at LIMIT 500`, retentionPeriods[SystemOutbox], false},
		{SystemEvents, SystemEvents, ActionDelete, `SELECT id FROM athena_events WHERE created_at < ? ORDER BY created_at LIMIT 500`, retentionPeriods[SystemEvents], false},
		{SystemJobs, SystemJobs, ActionDelete, `SELECT id FROM athena_jobs WHERE status IN ('succeeded', 'dead') AND created_at < ? ORDER BY created_at LIMIT 500`, retentionPeriods[SystemJobs], false},
	}
	for _, query := range queries {
		cutoff := now.Add(-query.since).UTC()
		rows, err := q.QueryContext(ctx, query.sql, cutoff)
		if err != nil {
			return nil, fmt.Errorf("jobs: preview %s candidates: %w", query.key, err)
		}
		for rows.Next() {
			var id string
			if query.intID {
				var num int64
				if err := rows.Scan(&num); err != nil {
					rows.Close()
					return nil, fmt.Errorf("jobs: preview %s candidates: %w", query.key, err)
				}
				id = fmt.Sprintf("%d", num)
			} else if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, fmt.Errorf("jobs: preview %s candidates: %w", query.key, err)
			}
			out = append(out, Candidate{RecordType: query.key, RecordID: id, Class: query.class, Action: query.action, Reason: query.key + " past retention"})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("jobs: preview %s candidates: %w", query.key, err)
		}
	}
	return out, nil
}

// RecordCandidates lists past-retention user Records for types whose
// lifetime block names a disposable class. Held types and the 7-year
// classes never list; an unknown class is a caller bug and fails
// closed.
func RecordCandidates(ctx context.Context, q Querier, now time.Time, types []RecordProfile) ([]Candidate, error) {
	ctx, span := otel.Tracer("athena/jobs").Start(ctx, "athena.jobs.disposal_preview_records")
	defer span.End()
	outcome := "ok"
	defer func() { span.SetAttributes(attribute.String("athena.outcome", outcome)) }()
	candidates, err := recordCandidates(ctx, q, now, types)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		return nil, err
	}
	span.SetAttributes(attribute.Int("athena.candidates", len(candidates)))
	return candidates, nil
}

func recordCandidates(ctx context.Context, q Querier, now time.Time, types []RecordProfile) ([]Candidate, error) {
	var out []Candidate
	for _, profile := range types {
		if profile.Hold || neverAutoDelete[profile.Class] {
			continue
		}
		period, ok := retentionPeriods[profile.Class]
		if !ok {
			return nil, fmt.Errorf("jobs: Record Type %q names unknown retention class %q", profile.Key, profile.Class)
		}
		column := "created_at"
		if profile.Trigger == TriggerUpdated {
			column = "updated_at"
		} else if profile.Trigger != "" && profile.Trigger != TriggerCreated {
			return nil, fmt.Errorf("jobs: Record Type %q names unknown retention trigger %q", profile.Key, profile.Trigger)
		}
		table, _, err := resolveTable(profile.Key)
		if err != nil {
			return nil, err
		}
		action := profile.Action
		if action == "" {
			action = ActionAnonymise
		}
		rows, err := q.QueryContext(ctx, `SELECT id FROM `+table+` WHERE `+column+` < ? ORDER BY `+column+` LIMIT 500`, now.Add(-period).UTC())
		if err != nil {
			return nil, fmt.Errorf("jobs: preview %s candidates: %w", profile.Key, err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, fmt.Errorf("jobs: preview %s candidates: %w", profile.Key, err)
			}
			out = append(out, Candidate{RecordType: profile.Key, RecordID: id, Class: profile.Class, Action: action, Reason: profile.Key + " past retention"})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("jobs: preview %s candidates: %w", profile.Key, err)
		}
	}
	return out, nil
}

// Apply disposes one preview batch in the caller's transaction and
// records every outcome in the disposal register. Held rows and rows
// linked by other Records skip; the 7-year set refuses even when
// listed; guest rows record an exception because anonymisation needs
// per-field markers beta does not have yet; system rows delete. The
// span carries counts and the outcome only.
func Apply(ctx context.Context, tx *sql.Tx, candidates []Candidate, links LinksTo, actor string) ([]Outcome, error) {
	ctx, span := otel.Tracer("athena/jobs").Start(ctx, "athena.jobs.disposal_apply")
	defer span.End()
	outcome := "ok"
	defer func() { span.SetAttributes(attribute.String("athena.outcome", outcome)) }()
	outcomes, err := apply(ctx, tx, candidates, links, actor)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		return nil, err
	}
	counts := map[string]int{}
	for _, o := range outcomes {
		counts[o.Result]++
	}
	span.SetAttributes(attribute.Int("athena.candidates", len(outcomes)),
		attribute.Int("athena.deleted", counts[OutcomeDeleted]),
		attribute.Int("athena.skipped", counts[OutcomeSkipped]),
		attribute.Int("athena.exceptions", counts[OutcomeException]))
	return outcomes, nil
}

func apply(ctx context.Context, tx *sql.Tx, candidates []Candidate, links LinksTo, actor string) ([]Outcome, error) {
	outcomes := make([]Outcome, 0, len(candidates))
	for _, candidate := range candidates {
		result, detail, err := applyOne(ctx, tx, candidate, links)
		if err != nil {
			return nil, err
		}
		outcome := Outcome{Candidate: candidate, Result: result, Detail: detail}
		if err := recordOutcome(ctx, tx, outcome, actor); err != nil {
			return nil, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

func applyOne(ctx context.Context, tx *sql.Tx, candidate Candidate, links LinksTo) (string, string, error) {
	held, err := isHeld(ctx, tx, candidate.RecordType, candidate.RecordID)
	if err != nil {
		return "", "", err
	}
	if held {
		return OutcomeSkipped, "held", nil
	}
	if neverAutoDelete[candidate.Class] {
		return OutcomeSkipped, "the 7-year set is never auto-deleted in beta", nil
	}
	if _, ok := retentionPeriods[candidate.Class]; !ok {
		return OutcomeException, fmt.Sprintf("unknown retention class %q", candidate.Class), nil
	}
	if !isSystemKey(candidate.RecordType) {
		linked, err := links(ctx, candidate.RecordType, candidate.RecordID)
		if err != nil {
			return "", "", fmt.Errorf("jobs: check links to %s %s: %w", candidate.RecordType, candidate.RecordID, err)
		}
		if linked {
			return OutcomeSkipped, "linked by other records", nil
		}
	}
	switch candidate.Action {
	case ActionDelete:
		if candidate.Class == ClassGuest {
			return OutcomeException, "guest rows anonymise, never delete", nil
		}
		deleted, err := deleteRow(ctx, tx, candidate)
		if err != nil {
			return "", "", err
		}
		if !deleted {
			return OutcomeException, "row already gone", nil
		}
		return OutcomeDeleted, candidate.Reason, nil
	case ActionAnonymise:
		return OutcomeException, "guest anonymisation needs field markers beta does not have yet", nil
	default:
		return OutcomeException, fmt.Sprintf("unknown disposal action %q", candidate.Action), nil
	}
}

// deleteRow removes one candidate row and reports whether it existed.
func deleteRow(ctx context.Context, tx *sql.Tx, candidate Candidate) (bool, error) {
	table, _, err := resolveTable(candidate.RecordType)
	if err != nil {
		return false, err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE id = ?`, candidate.RecordID)
	if err != nil {
		return false, fmt.Errorf("jobs: delete %s %s: %w", candidate.RecordType, candidate.RecordID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("jobs: delete %s %s: %w", candidate.RecordType, candidate.RecordID, err)
	}
	return n > 0, nil
}

// recordOutcome writes one disposal register row.
func recordOutcome(ctx context.Context, tx *sql.Tx, outcome Outcome, actor string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO athena_disposal_register (id, record_type, record_id, action, outcome, detail, actor) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id.String(), outcome.Candidate.RecordType, outcome.Candidate.RecordID, outcome.Candidate.Action, outcome.Result, trimDetail(outcome.Detail), actor); err != nil {
		return fmt.Errorf("jobs: record disposal of %s %s: %w", outcome.Candidate.RecordType, outcome.Candidate.RecordID, err)
	}
	return nil
}

// trimDetail caps a register detail at the column width.
func trimDetail(detail string) string {
	if len(detail) > 600 {
		return detail[:600]
	}
	return detail
}

// isSystemKey reports whether a candidate Record Type names a system
// table rather than a user Record Type.
func isSystemKey(recordType string) bool {
	switch recordType {
	case SystemAccessLog, SystemOutbox, SystemEvents, SystemJobs:
		return true
	}
	return false
}

// ReapplyRegister re-applies recorded deletions after a restore: every
// register row with outcome deleted deletes its row again when the row
// came back with the backup. It returns the reapplied count. Skips and
// exceptions record decisions, not effects, so they reapply to nothing.
func ReapplyRegister(ctx context.Context, tx *sql.Tx) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT record_type, record_id FROM athena_disposal_register WHERE outcome = ?`, OutcomeDeleted)
	if err != nil {
		return 0, fmt.Errorf("jobs: reapply disposal register: %w", err)
	}
	type doomed struct{ recordType, recordID string }
	var pending []doomed
	for rows.Next() {
		var d doomed
		if err := rows.Scan(&d.recordType, &d.recordID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("jobs: reapply disposal register: %w", err)
		}
		pending = append(pending, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("jobs: reapply disposal register: %w", err)
	}
	reapplied := 0
	for _, d := range pending {
		deleted, err := deleteRow(ctx, tx, Candidate{RecordType: d.recordType, RecordID: d.recordID})
		if err != nil {
			return 0, err
		}
		if deleted {
			reapplied++
		}
	}
	return reapplied, nil
}

// PlaceHold parks a hold so disposal skips one row until ReleaseHold.
func PlaceHold(ctx context.Context, q Querier, recordType, recordID, reason, placedBy string) error {
	if recordType == "" || recordID == "" {
		return fmt.Errorf("jobs: a hold needs a Record Type and id")
	}
	if len(reason) > 600 || len(placedBy) > 255 {
		return fmt.Errorf("jobs: hold reason or placer is too long")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO athena_holds (id, record_type, record_id, reason, placed_by) VALUES (?, ?, ?, ?, ?)`, id.String(), recordType, recordID, reason, placedBy); err != nil {
		return fmt.Errorf("jobs: hold %s %s: %w", recordType, recordID, err)
	}
	return nil
}

// ReleaseHold releases every active hold on one row and reports
// whether it released any.
func ReleaseHold(ctx context.Context, q Querier, recordType, recordID string) (bool, error) {
	res, err := q.ExecContext(ctx, `UPDATE athena_holds SET released_at = CURRENT_TIMESTAMP WHERE record_type = ? AND record_id = ? AND released_at IS NULL`, recordType, recordID)
	if err != nil {
		return false, fmt.Errorf("jobs: release hold on %s %s: %w", recordType, recordID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("jobs: release hold on %s %s: %w", recordType, recordID, err)
	}
	return n > 0, nil
}

// isHeld reports whether an active hold covers one row.
func isHeld(ctx context.Context, q Querier, recordType, recordID string) (bool, error) {
	var exists bool
	if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM athena_holds WHERE record_type = ? AND record_id = ? AND released_at IS NULL)`, recordType, recordID).Scan(&exists); err != nil {
		return false, fmt.Errorf("jobs: check hold on %s %s: %w", recordType, recordID, err)
	}
	return exists, nil
}
