// Package audit writes and verifies the Audit Log: an append-only log of
// every change to a Record (spec 12). Each row carries the SHA-256 of its own
// content and of the previous row (a hash chain), so an edited, inserted or
// removed row is detectable. The chain head is stored in a separate table.
//
// Writers append inside the transaction that makes the change. The head row
// is locked first, so appends are serial and a rolled-back change leaves no
// row and no gap.
package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"athena-erp/internal/store"

	"github.com/google/uuid"
)

// Log is the name of the data-change chain.
const Log = "audit"

// Change is one field that changed. Old and New are nil for an empty value.
// Rows imported from the pre-audit history carry only the field key.
type Change struct {
	Field string  `json:"field"`
	Old   *string `json:"old,omitempty"`
	New   *string `json:"new,omitempty"`
}

// Entry is what a writer supplies for one change to one Record.
type Entry struct {
	RecordType string
	RecordID   string
	Revision   int64
	// Action is created, updated, deleted, submitted, cancelled or
	// name_override.
	Action  string
	Changes []Change
	Actor   string
	// Source is api, cli, rule, import or job (spec 12.1).
	Source    string
	RequestID string
}

// Row is a stored Audit Log row.
type Row struct {
	Seq       int64
	ID        string
	Entry     Entry
	At        time.Time
	PrevHash  string
	Hash      string
	RawChange string
}

// Tx is the part of *sql.Tx the writer needs.
type Tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Append adds one row to the chain inside tx and returns it. It takes the
// head row lock, so concurrent writers wait for each other until commit.
func Append(ctx context.Context, tx Tx, d store.Dialect, e Entry) (Row, error) {
	return AppendAt(ctx, tx, d, e, time.Now())
}

// AppendAt is Append with an explicit row time, used when importing older
// history. The time is kept to the millisecond.
func AppendAt(ctx context.Context, tx Tx, d store.Dialect, e Entry, at time.Time) (Row, error) {
	var seq int64
	var prev string
	head := "SELECT `head_seq`, `head_hash` FROM athena_audit_heads WHERE `log_name` = ? " + d.ForUpdate()
	if err := tx.QueryRowContext(ctx, head, Log).Scan(&seq, &prev); err != nil {
		return Row{}, fmt.Errorf("lock audit chain head: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Row{}, err
	}
	if e.Changes == nil {
		e.Changes = []Change{}
	}
	changes, err := json.Marshal(e.Changes)
	if err != nil {
		return Row{}, err
	}
	row := Row{Seq: seq + 1, ID: id.String(), Entry: e, At: at.UTC().Truncate(time.Millisecond), PrevHash: prev, RawChange: string(changes)}
	row.Hash = hashRow(row)
	if _, err := tx.ExecContext(ctx, "INSERT INTO athena_audit_log (`log_name`, `seq`, `id`, `record_type_name`, `record_id`, `revision`, `action`, `changes`, `actor`, `source`, `request_id`, `created_at`, `prev_hash`, `hash`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
		Log, row.Seq, row.ID, e.RecordType, e.RecordID, e.Revision, e.Action, row.RawChange, e.Actor, e.Source, nullable(e.RequestID), row.At, row.PrevHash, row.Hash); err != nil {
		return Row{}, fmt.Errorf("append audit row: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE athena_audit_heads SET `head_seq` = ?, `head_hash` = ? WHERE `log_name` = ?", row.Seq, row.Hash, Log); err != nil {
		return Row{}, fmt.Errorf("advance audit chain head: %w", err)
	}
	return row, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// hashRow is SHA-256 over the previous hash and every stored column except
// the hash itself. Fields are length-prefixed, so no two rows share an
// encoding. The changes are hashed in their canonical re-encoded form, so
// the result does not depend on how the database stores JSON text.
func hashRow(r Row) string {
	h := sha256.New()
	put := func(s string) {
		h.Write([]byte(strconv.Itoa(len(s))))
		h.Write([]byte{':'})
		h.Write([]byte(s))
	}
	put(r.PrevHash)
	put(strconv.FormatInt(r.Seq, 10))
	put(r.ID)
	put(r.Entry.RecordType)
	put(r.Entry.RecordID)
	put(strconv.FormatInt(r.Entry.Revision, 10))
	put(r.Entry.Action)
	put(canonicalChanges(r.RawChange))
	put(r.Entry.Actor)
	put(r.Entry.Source)
	put(r.Entry.RequestID)
	put(strconv.FormatInt(r.At.UnixMilli(), 10))
	return hex.EncodeToString(h.Sum(nil))
}

// canonicalChanges re-encodes stored change JSON through Change, so key order
// and spacing never matter. Text that is not a change list hashes as is, and
// then fails verification if it was altered.
func canonicalChanges(raw string) string {
	var changes []Change
	if err := json.Unmarshal([]byte(raw), &changes); err != nil {
		return raw
	}
	out, err := json.Marshal(changes)
	if err != nil {
		return raw
	}
	return string(out)
}

// Anchor is a chain position exported earlier (for example the daily digest
// from `athena audit verify`). Verification checks the chain still holds it.
type Anchor struct {
	Seq  int64
	Hash string
}

// Result reports one verification.
type Result struct {
	OK bool
	// Rows is how many rows were checked.
	Rows    int64
	HeadSeq int64
	// HeadHash is the hash of the last row; export it as the next anchor.
	HeadHash string
	// BrokenSeq is the first row that fails, or 0 when the break is at the
	// head (missing tail rows) or at an anchor.
	BrokenSeq int64
	Reason    string
}

// Verify walks the chain from the first row to the last. It fails on a
// missing, reordered, edited or inserted row, on a head that does not match
// the last row (deleted tail), and on an anchor the chain no longer holds.
func Verify(ctx context.Context, db *sql.DB, anchor *Anchor) (Result, error) {
	var res Result
	// One snapshot for the head and the rows, so a write that commits during
	// the walk is not mistaken for a removed tail.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return res, fmt.Errorf("begin audit verification: %w", err)
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, "SELECT `head_seq`, `head_hash` FROM athena_audit_heads WHERE `log_name` = ?", Log).Scan(&res.HeadSeq, &res.HeadHash); err != nil {
		return res, fmt.Errorf("read audit chain head: %w", err)
	}
	rows, err := tx.QueryContext(ctx, "SELECT `seq`, `id`, `record_type_name`, `record_id`, `revision`, `action`, `changes`, `actor`, `source`, `request_id`, `created_at`, `prev_hash`, `hash` FROM athena_audit_log WHERE `log_name` = ? ORDER BY `seq`", Log)
	if err != nil {
		return res, fmt.Errorf("read audit log: %w", err)
	}
	defer rows.Close()
	var prevSeq int64
	var prevHash, last string
	anchored := anchor == nil
	for rows.Next() {
		var r Row
		var changes []byte
		var request sql.NullString
		if err := rows.Scan(&r.Seq, &r.ID, &r.Entry.RecordType, &r.Entry.RecordID, &r.Entry.Revision, &r.Entry.Action, &changes, &r.Entry.Actor, &r.Entry.Source, &request, &r.At, &r.PrevHash, &r.Hash); err != nil {
			return res, fmt.Errorf("read audit row: %w", err)
		}
		r.Entry.RequestID, r.RawChange, r.At = request.String, string(changes), r.At.UTC()
		res.Rows++
		switch {
		case r.Seq != prevSeq+1:
			return fail(res, r.Seq, fmt.Sprintf("row %d follows row %d: a row is missing or reordered", r.Seq, prevSeq)), nil
		case r.PrevHash != prevHash:
			return fail(res, r.Seq, fmt.Sprintf("row %d does not link to the row before it", r.Seq)), nil
		case hashRow(r) != r.Hash:
			return fail(res, r.Seq, fmt.Sprintf("row %d was changed after it was written", r.Seq)), nil
		}
		if anchor != nil && r.Seq == anchor.Seq {
			if r.Hash != anchor.Hash {
				return fail(res, r.Seq, fmt.Sprintf("row %d no longer matches the anchor", r.Seq)), nil
			}
			anchored = true
		}
		prevSeq, prevHash, last = r.Seq, r.Hash, r.Hash
	}
	if err := rows.Err(); err != nil {
		return res, fmt.Errorf("read audit log: %w", err)
	}
	if prevSeq != res.HeadSeq || last != res.HeadHash {
		return fail(res, 0, fmt.Sprintf("the chain ends at row %d but its head says row %d: rows were removed from the end", prevSeq, res.HeadSeq)), nil
	}
	if !anchored {
		return fail(res, 0, fmt.Sprintf("the chain has no row %d to match the anchor", anchor.Seq)), nil
	}
	res.OK = true
	return res, nil
}

func fail(res Result, seq int64, reason string) Result {
	res.OK, res.BrokenSeq, res.Reason = false, seq, reason
	return res
}

// ParseAnchor reads "seq:hash".
func ParseAnchor(text string) (*Anchor, error) {
	var seq int64
	var hash string
	for i := 0; i < len(text); i++ {
		if text[i] == ':' {
			n, err := strconv.ParseInt(text[:i], 10, 64)
			if err != nil || n < 1 {
				break
			}
			seq, hash = n, text[i+1:]
			break
		}
	}
	if seq == 0 || len(hash) != 64 {
		return nil, errors.New("an anchor reads seq:hash, for example 42:<64 hex characters>")
	}
	return &Anchor{Seq: seq, Hash: hash}, nil
}

// Rows returns the Audit Log rows of one Record, newest first.
func Rows(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, recordType, recordID string, limit int) ([]Row, error) {
	rows, err := q.QueryContext(ctx, "SELECT `seq`, `id`, `revision`, `action`, `changes`, `actor`, `source`, `request_id`, `created_at`, `prev_hash`, `hash` FROM athena_audit_log WHERE `log_name` = ? AND `record_type_name` = ? AND `record_id` = ? ORDER BY `seq` DESC LIMIT ?", Log, recordType, recordID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		r := Row{Entry: Entry{RecordType: recordType, RecordID: recordID}}
		var changes []byte
		var request sql.NullString
		if err := rows.Scan(&r.Seq, &r.ID, &r.Entry.Revision, &r.Entry.Action, &changes, &r.Entry.Actor, &r.Entry.Source, &request, &r.At, &r.PrevHash, &r.Hash); err != nil {
			return nil, err
		}
		r.Entry.RequestID, r.RawChange, r.At = request.String, string(changes), r.At.UTC()
		if err := json.Unmarshal(changes, &r.Entry.Changes); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
