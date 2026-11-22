package posting

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"athena-erp/internal/store"

	"github.com/google/uuid"
)

// CommandPost is the operations-table command of every post. One command
// keeps the (actor, command, operation id) key to one namespace; the event
// (submit, cancel) lives in the plan and the posting-group key instead.
const CommandPost = "post"

// Posting-log outcomes. Every attempt ends in exactly one row: successes in
// the posting transaction, failures through a separate connection after the
// rollback, so failed attempts stay visible (R6.6).
const (
	// OutcomePosted marks a plan written to the ledgers.
	OutcomePosted = "posted"
	// OutcomeReplayed marks an operation id or posting group that returned
	// an earlier result without writing again.
	OutcomeReplayed = "replayed"
	// OutcomeConflict marks an operation id reused with another payload.
	OutcomeConflict = "conflict"
	// OutcomeRejected marks a plan refused by validation or undecided
	// policy, with the code naming which.
	OutcomeRejected = "rejected"
	// OutcomeError marks a write the database refused.
	OutcomeError = "error"
)

// Posting-log codes, one per outcome family.
const (
	CodeOK                   = "ok"
	CodeDuplicateGroup       = "duplicate_group"
	CodeConflict             = "conflict"
	CodeInvalid              = "invalid"
	CodeBookPolicyOpen       = "book_policy_open"
	CodeCorrectionWindowOpen = "correction_window_open"
	CodeInternal             = "internal"
)

// codeFor maps a validation failure to its log code and outcome.
func codeFor(err error) (outcome, code string) {
	switch {
	case errors.Is(err, ErrBookPolicyOpen):
		return OutcomeRejected, CodeBookPolicyOpen
	case errors.Is(err, ErrCorrectionWindowOpen):
		return OutcomeRejected, CodeCorrectionWindowOpen
	default:
		return OutcomeRejected, CodeInvalid
	}
}

// storedOperation is one athena_operations row.
type storedOperation struct {
	Actor       string
	PayloadHash string
	Result      Result
}

// lookupOperation reads an operation by its key. Found is false with no
// error when no row exists.
func lookupOperation(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, actor, command, operationID string) (found bool, stored storedOperation, err error) {
	var payloadHash, resultJSON string
	err = q.QueryRowContext(ctx, `SELECT payload_hash, result_json FROM athena_operations WHERE actor = ? AND command = ? AND operation_id = ?`,
		actor, command, operationID).Scan(&payloadHash, &resultJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return false, storedOperation{}, nil
	}
	if err != nil {
		return false, storedOperation{}, err
	}
	var result Result
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return false, storedOperation{}, fmt.Errorf("decode stored result: %w", err)
	}
	return true, storedOperation{Actor: actor, PayloadHash: payloadHash, Result: result}, nil
}

// isDuplicate reports a unique-key collision. A nil Dialect cannot classify
// and is never a duplicate: callers then return the raw error instead of
// taking the replay path, and never panic (review follow-up 5).
func (e *Engine) isDuplicate(err error) bool {
	return e.Dialect != nil && e.Dialect.Classify(err) == store.ErrDuplicateKey
}

// insertGroup writes one athena_posting_groups row: one voucher revision,
// event and book posts once (R6.4, R6.10). The row id is the group id the
// ledger rows carry.
func insertGroup(ctx context.Context, tx *sql.Tx, groupID string, plan Plan, operationID string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO athena_posting_groups (id, voucher_type, voucher_id, voucher_revision, event, book, operation_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		groupID, plan.VoucherType, plan.VoucherID, plan.Revision, plan.Event, plan.Book, operationID)
	return err
}

// insertOperation writes one athena_operations row with the result the post
// will commit, so replays return it byte for byte.
func insertOperation(ctx context.Context, tx *sql.Tx, actor string, operationID, payloadHash string, result Result) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO athena_operations (id, workspace, actor, command, operation_id, payload_hash, result_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id.String(), nil, actor, CommandPost, operationID, payloadHash, string(raw))
	return err
}

// replayGroupResult answers a posting-group collision: another operation
// already posted this voucher revision, event and book, so this attempt
// posts nothing and returns the first result (R6.10). The winner's operation
// row is read through a fresh connection: it committed with the group row,
// which a repeatable-read transaction snapshot might not see yet. When the
// winner carries this same operation id, the payloads must match or the
// attempt conflicts (R6.8); any other operation id replays unconditionally.
func (e *Engine) replayGroupResult(ctx context.Context, tx *sql.Tx, plan Plan, operationID string, opts PostOptions, hash string) (Result, error) {
	var winnerID string
	err := e.DB.QueryRowContext(ctx, `SELECT operation_id FROM athena_posting_groups WHERE voucher_type = ? AND voucher_id = ? AND voucher_revision = ? AND event = ? AND book = ?`,
		plan.VoucherType, plan.VoucherID, plan.Revision, plan.Event, plan.Book).Scan(&winnerID)
	if err != nil {
		e.logFailure(ctx, plan, operationID, opts, OutcomeError, CodeInternal, hash, err)
		return Result{}, err
	}
	var winnerActor, winnerHash, winnerResult string
	err = e.DB.QueryRowContext(ctx, `SELECT actor, payload_hash, result_json FROM athena_operations WHERE command = ? AND operation_id = ? LIMIT 1`,
		CommandPost, winnerID).Scan(&winnerActor, &winnerHash, &winnerResult)
	if err != nil {
		e.logFailure(ctx, plan, operationID, opts, OutcomeError, CodeInternal, hash, err)
		return Result{}, err
	}
	if winnerID == operationID && winnerActor == opts.Actor && winnerHash != hash {
		e.logFailure(ctx, plan, operationID, opts, OutcomeConflict, CodeConflict, hash, ErrConflict)
		return Result{}, fmt.Errorf("%w: operation %q already posted another payload", ErrConflict, operationID)
	}
	var result Result
	if err := json.Unmarshal([]byte(winnerResult), &result); err != nil {
		e.logFailure(ctx, plan, operationID, opts, OutcomeError, CodeInternal, hash, err)
		return Result{}, fmt.Errorf("decode stored result: %w", err)
	}
	result.Replayed = true
	if err := insertLogRow(ctx, tx, plan, operationID, opts, OutcomeReplayed, CodeDuplicateGroup, "", hash); err != nil {
		return Result{}, err
	}
	return result, nil
}

// replayOperationResult answers an operations-table collision: this
// (actor, command, operation id) committed concurrently, so the attempt
// replays its result when the payload matches and conflicts otherwise.
func (e *Engine) replayOperationResult(ctx context.Context, tx *sql.Tx, plan Plan, operationID string, opts PostOptions, hash string) (Result, error) {
	found, stored, err := lookupOperation(ctx, e.DB, opts.Actor, CommandPost, operationID)
	if err != nil || !found {
		if err == nil {
			err = fmt.Errorf("operation %q collided but left no row", operationID)
		}
		e.logFailure(ctx, plan, operationID, opts, OutcomeError, CodeInternal, hash, err)
		return Result{}, err
	}
	if stored.PayloadHash != hash {
		e.logFailure(ctx, plan, operationID, opts, OutcomeConflict, CodeConflict, hash, ErrConflict)
		return Result{}, fmt.Errorf("%w: operation %q already posted another payload", ErrConflict, operationID)
	}
	stored.Result.Replayed = true
	if err := insertLogRow(ctx, tx, plan, operationID, opts, OutcomeReplayed, CodeOK, "", hash); err != nil {
		return Result{}, err
	}
	return stored.Result, nil
}

// insertLogRow writes one athena_posting_log row inside the given
// transaction.
func insertLogRow(ctx context.Context, tx *sql.Tx, plan Plan, operationID string, opts PostOptions, outcome, code, message, hash string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO athena_posting_log (id, voucher_type, voucher_id, voucher_revision, event, book, operation_id, outcome, code, message, plan_hash, actor, request_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), plan.VoucherType, plan.VoucherID, plan.Revision, plan.Event, plan.Book, truncate(operationID, 140), outcome, code, truncate(message, 600), hash, opts.Actor, nullText(opts.RequestID))
	return err
}

// logFailure records a failed attempt through a separate connection, so the
// row survives the posting transaction's rollback. Logging is best effort:
// its own error never masks the failure it records.
func (e *Engine) logFailure(ctx context.Context, plan Plan, operationID string, opts PostOptions, outcome, code, hash string, failure error) {
	id, err := uuid.NewV7()
	if err != nil {
		return
	}
	_, _ = e.DB.ExecContext(ctx, `INSERT INTO athena_posting_log (id, voucher_type, voucher_id, voucher_revision, event, book, operation_id, outcome, code, message, plan_hash, actor, request_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), plan.VoucherType, plan.VoucherID, plan.Revision, plan.Event, plan.Book, truncate(operationID, 140), outcome, code, truncate(failure.Error(), 600), hash, opts.Actor, nullText(opts.RequestID))
}

func truncate(message string, limit int) string {
	runes := []rune(message)
	if len(runes) <= limit {
		return message
	}
	return string(runes[:limit])
}
