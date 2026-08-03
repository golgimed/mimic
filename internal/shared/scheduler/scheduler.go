// Package scheduler is a generic DB-backed job queue: jobs are inserted with
// a kind, a JSON payload, and a run_at time, and Tick (driven by a
// lane runners.SchedulerRunner on a fixed interval) dispatches due jobs to
// their registered handler.
package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type JobHandler func(ctx context.Context, payload json.RawMessage) error

type Scheduler struct {
	db       *sql.DB
	mu       sync.RWMutex
	handlers map[string]JobHandler
}

func New(db *sql.DB) *Scheduler {
	return &Scheduler{db: db, handlers: make(map[string]JobHandler)}
}

func (s *Scheduler) RegisterJobHandler(kind string, handler JobHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[kind] = handler
}

func (s *Scheduler) ScheduleJob(kind string, payload any, runAt time.Time) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal job payload: %w", err)
	}
	_, err = s.db.Exec(
		"INSERT INTO jobs (kind, payload_json, run_at, status) VALUES (?, ?, ?, 'pending')",
		kind, string(payloadJSON), runAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

type jobRow struct {
	id          int64
	payloadJSON string
}

// Tick selects every due pending job and dispatches it to its registered
// handler, marking it processing -> done/failed. Matches the JobFn signature
// expected by lane's runners.SchedulerRunner.
func (s *Scheduler) Tick(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, kind, payload_json FROM jobs WHERE status = 'pending' AND run_at <= ? ORDER BY run_at",
		now,
	)
	if err != nil {
		return fmt.Errorf("query due jobs: %w", err)
	}

	type due struct {
		jobRow
		kind string
	}
	var jobs []due
	for rows.Next() {
		var j due
		if err := rows.Scan(&j.id, &j.kind, &j.payloadJSON); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	for _, j := range jobs {
		if _, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'processing' WHERE id = ?", j.id); err != nil {
			slog.Error("scheduler: failed to mark job processing", "job_id", j.id, "kind", j.kind, "error", err)
			continue
		}

		s.mu.RLock()
		handler, ok := s.handlers[j.kind]
		s.mu.RUnlock()

		var handlerErr error
		if ok {
			handlerErr = handler(ctx, json.RawMessage(j.payloadJSON))
		}

		if handlerErr != nil {
			if _, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'failed' WHERE id = ?", j.id); err != nil {
				slog.Error("scheduler: failed to mark job failed", "job_id", j.id, "kind", j.kind, "handler_error", handlerErr, "error", err)
			} else {
				slog.Error("scheduler: job handler failed", "job_id", j.id, "kind", j.kind, "error", handlerErr)
			}
			continue
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'done' WHERE id = ?", j.id); err != nil {
			slog.Error("scheduler: failed to mark job done", "job_id", j.id, "kind", j.kind, "error", err)
		}
	}

	return nil
}
