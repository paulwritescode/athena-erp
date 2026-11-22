package server

import (
	"context"
	"database/sql"
	"log"
	"time"

	"athena-erp/internal/auth"
	"athena-erp/internal/events"
	"athena-erp/internal/jobs"
)

// PurgeExpiredKind is the job kind that purges spent Challenges and dead
// sessions on a schedule (Requirement 8.19).
const PurgeExpiredKind = "purge_expired"

// WorkerOptions tunes the background workers Serve starts. The zero value
// runs jobs and the purge with production intervals; tests shrink them.
// The CLI opens no workers: one-shot commands must not claim work behind
// the server's back.
type WorkerOptions struct {
	// JobHandlers runs one job kind each. Empty kinds sit queued until a
	// deploy registers them.
	JobHandlers map[string]jobs.Handler
	// EventHandlers runs one event consumer each. The event dispatcher
	// only ticks when at least one handler is registered, so events are
	// never marked dispatched with nobody to receive them.
	EventHandlers map[string]events.Handler
	// PollInterval ticks the drain; zero means five seconds.
	PollInterval time.Duration
	// PurgeInterval enqueues the purge job when none is pending; zero
	// means one hour.
	PurgeInterval time.Duration
}

// startWorkers recovers interrupted jobs and occurrences, then drains due
// jobs and (once consumers exist) due events until ctx ends. It returns
// after the first recovery; the loop runs in the background.
func startWorkers(ctx context.Context, db *sql.DB, opts WorkerOptions) {
	poll := opts.PollInterval
	if poll <= 0 {
		poll = 5 * time.Second
	}
	purgeEvery := opts.PurgeInterval
	if purgeEvery <= 0 {
		purgeEvery = time.Hour
	}
	runner := &jobs.Runner{DB: db, Errors: dialect, Handlers: opts.JobHandlers, Owner: "server"}
	dispatcher := &events.Dispatcher{DB: db, Errors: dialect, Handlers: opts.EventHandlers, Owner: "server"}
	if released, dead, err := runner.Recover(ctx); err != nil {
		log.Printf("workers: recover jobs: %v", err)
	} else if released+dead > 0 {
		log.Printf("workers: recovered %d jobs to queued, %d to dead", released, dead)
	}
	if done, dead, refreshed, err := jobs.RecoverOccurrences(ctx, db, "server", time.Minute); err != nil {
		log.Printf("workers: recover occurrences: %v", err)
	} else if done+dead+refreshed > 0 {
		log.Printf("workers: recovered %d occurrences done, %d dead, %d refreshed", done, dead, refreshed)
	}
	go func() {
		ticker := time.NewTicker(poll)
		defer ticker.Stop()
		lastPurge := time.Time{}
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				drainJobs(ctx, runner)
				if len(opts.EventHandlers) > 0 {
					drainEvents(ctx, dispatcher)
				}
				if now.Sub(lastPurge) >= purgeEvery {
					lastPurge = now
					ensurePurge(ctx, db)
				}
			}
		}
	}()
}

// maxDrainPerTick caps one tick's work so a deep backlog cannot starve
// the next tick's purge check.
const maxDrainPerTick = 100

func drainJobs(ctx context.Context, runner *jobs.Runner) {
	for i := 0; i < maxDrainPerTick; i++ {
		ran, err := runner.RunNext(ctx)
		if err != nil {
			log.Printf("workers: job: %v", err)
		}
		if !ran {
			return
		}
	}
}

func drainEvents(ctx context.Context, dispatcher *events.Dispatcher) {
	for i := 0; i < maxDrainPerTick; i++ {
		ran, err := dispatcher.DispatchNext(ctx)
		if err != nil {
			log.Printf("workers: event: %v", err)
		}
		if !ran {
			return
		}
	}
}

// ensurePurge enqueues the purge job unless one is already queued or
// leased. The purge is idempotent, so a racing enqueue is harmless.
func ensurePurge(ctx context.Context, db *sql.DB) {
	enqueued, err := jobs.EnqueueIfAbsent(ctx, db, jobs.EnqueueOptions{Kind: PurgeExpiredKind})
	if err != nil {
		log.Printf("workers: enqueue purge: %v", err)
	} else if enqueued {
		log.Print("workers: enqueued the expired purge")
	}
}

// defaultJobHandlers runs the kernel's own jobs: the scheduled purge of
// spent Challenges and dead sessions.
func defaultJobHandlers(db *sql.DB) map[string]jobs.Handler {
	return map[string]jobs.Handler{
		PurgeExpiredKind: func(ctx context.Context, _ *sql.Tx, _ jobs.Job) (map[string]any, error) {
			challenges, sessions, err := (&auth.Service{DB: db}).PurgeExpired(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]any{"challenges": challenges, "sessions": sessions}, nil
		},
	}
}
