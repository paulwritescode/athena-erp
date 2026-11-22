package auth_test

import (
	"context"
	"testing"
	"time"

	"athena-erp/internal/auth"
)

// TestPurgeExpiredDeletesSpentChallengesAndDeadSessions seeds expired and
// live Challenges and sessions, purges, and checks only the dead rows go
// with one minimal Access Log row.
func TestPurgeExpiredDeletesSpentChallengesAndDeadSessions(t *testing.T) {
	ctx := context.Background()
	db := sharedDatabase(t)
	service := &auth.Service{DB: db}
	now := time.Now().UTC()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	exec(`INSERT INTO athena_users (id, username, display_name, state) VALUES
		('a1111111-1111-1111-1111-111111111111', 'purge-ad', 'Purge', 'active')`)
	exec(`INSERT INTO athena_challenges (id, user_id, purpose, digest, generation, expires_at) VALUES
		('c1111111-1111-1111-1111-111111111111', 'a1111111-1111-1111-1111-111111111111', 'first_login', 'deadbeef01', 0, ?),
		('c2222222-2222-2222-2222-222222222222', 'a1111111-1111-1111-1111-111111111111', 'first_login', 'deadbeef02', 0, ?)`,
		now.Add(-time.Hour), now.Add(time.Hour))
	exec(`UPDATE athena_challenges SET consumed_at = ? WHERE id = 'c2222222-2222-2222-2222-222222222222'`, now.Add(-time.Minute))
	exec(`INSERT INTO athena_challenges (id, user_id, purpose, digest, generation, expires_at) VALUES
		('c3333333-3333-3333-3333-333333333333', 'a1111111-1111-1111-1111-111111111111', 'first_login', 'deadbeef03', 0, ?)`,
		now.Add(time.Hour))
	exec(`INSERT INTO athena_sessions (id, token_hash, user_id, created_at, last_seen_at, expires_at) VALUES
		('s1111111-1111-1111-1111-111111111111', 'aa', 'a1111111-1111-1111-1111-111111111111', ?, ?, ?),
		('s2222222-2222-2222-2222-222222222222', 'bb', 'a1111111-1111-1111-1111-111111111111', ?, ?, ?),
		('s3333333-3333-3333-3333-333333333333', 'cc', 'a1111111-1111-1111-1111-111111111111', ?, ?, ?)`,
		now.Add(-2*time.Hour), now.Add(-2*time.Hour), now.Add(-time.Hour),
		now.Add(-2*time.Hour), now.Add(-2*time.Hour), now.Add(24*time.Hour),
		now, now, now.Add(24*time.Hour))

	challenges, sessions, err := service.PurgeExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if challenges != 2 || sessions != 2 {
		t.Fatalf("purged %d challenges and %d sessions, want 2 and 2", challenges, sessions)
	}
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_challenges`).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("challenges left = %d, %v, want 1", remaining, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_sessions`).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("sessions left = %d, %v, want 1", remaining, err)
	}
	rows, err := service.ListAccessLog(ctx, 10, 0, auth.EventExpiredPurged)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Outcome != auth.OutcomeOK {
		t.Fatalf("purge log rows = %+v, want one ok row", rows)
	}
}
