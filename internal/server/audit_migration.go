package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"athena-erp/internal/audit"
	"athena-erp/internal/store"
)

// Migration 8 (ADR-0012) creates the Audit Log, its chain head and the number
// counters, copies every row of the old Record history into the chain, and
// drops the old table.

// legacyHistoryTable8 is the Record history table as migration 5 leaves it.
const legacyHistoryTable8 = "athena_record_history"

func createAuditAndNumbering(ctx context.Context, x store.Execer) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS athena_audit_log (
			log_name VARCHAR(40) NOT NULL,
			seq BIGINT NOT NULL,
			id CHAR(36) NOT NULL,
			record_type_name VARCHAR(140) NOT NULL,
			record_id CHAR(36) NOT NULL,
			revision BIGINT NOT NULL,
			action VARCHAR(30) NOT NULL,
			changes JSON NOT NULL,
			actor VARCHAR(255) NOT NULL,
			source VARCHAR(20) NOT NULL,
			request_id CHAR(36) NULL,
			created_at TIMESTAMP(3) NOT NULL,
			prev_hash CHAR(64) NOT NULL,
			hash CHAR(64) NOT NULL,
			PRIMARY KEY (log_name, seq),
			UNIQUE KEY uq_audit_log_id (id),
			KEY idx_audit_log_record (record_type_name, record_id, seq),
			KEY idx_audit_log_request (request_id),
			KEY idx_audit_log_actor (actor, created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS athena_audit_heads (
			log_name VARCHAR(40) NOT NULL PRIMARY KEY,
			head_seq BIGINT NOT NULL,
			head_hash VARCHAR(64) NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	} {
		if _, err := x.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if _, err := x.ExecContext(ctx, dialect.Upsert("athena_audit_heads", []string{"log_name", "head_seq", "head_hash"}, "log_name", nil), audit.Log, 0, ""); err != nil {
		return err
	}
	if _, err := x.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS athena_number_counters (
		counter_key VARCHAR(255) NOT NULL PRIMARY KEY,
		record_type_name VARCHAR(140) NOT NULL,
		last_number BIGINT NOT NULL,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}
	exists, err := dialect.TableExists(ctx, x, legacyHistoryTable8)
	if err != nil || !exists {
		return err
	}
	if err := importHistory(ctx, x); err != nil {
		return err
	}
	_, err = x.ExecContext(ctx, dialect.DropTable(legacyHistoryTable8))
	return err
}

type beginner interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

// importHistory appends the old history to the chain in time order, in one
// transaction. Old rows recorded only the keys of the changed fields, so the
// imported changes have no old or new values. A repeat run after the copy
// committed (but before the old table was dropped) finds the import rows
// and does nothing.
func importHistory(ctx context.Context, x store.Execer) error {
	b, ok := x.(beginner)
	if !ok {
		return errors.New("the migration connection cannot start a transaction")
	}
	var done bool
	if err := x.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM athena_audit_log WHERE `source` = 'import')").Scan(&done); err != nil {
		return err
	}
	if done {
		return nil
	}
	tx, err := b.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, "SELECT record_type_name, record_id, revision, action, changed_fields, actor, created_at FROM "+dialect.QuoteIdent(legacyHistoryTable8)+" ORDER BY created_at, id")
	if err != nil {
		return err
	}
	var entries []audit.Entry
	var times []time.Time
	for rows.Next() {
		var e audit.Entry
		var keys []byte
		var at time.Time
		if err := rows.Scan(&e.RecordType, &e.RecordID, &e.Revision, &e.Action, &keys, &e.Actor, &at); err != nil {
			rows.Close()
			return err
		}
		var fields []string
		if err := json.Unmarshal(keys, &fields); err != nil {
			rows.Close()
			return fmt.Errorf("history row of %s %s: %w", e.RecordType, e.RecordID, err)
		}
		for _, field := range fields {
			e.Changes = append(e.Changes, audit.Change{Field: field})
		}
		e.Source = "import"
		entries, times = append(entries, e), append(times, at)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for i, e := range entries {
		// AppendAt keeps each change's original time.
		if _, err := audit.AppendAt(ctx, tx, dialect, e, times[i]); err != nil {
			return err
		}
	}
	return tx.Commit()
}
