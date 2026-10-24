// Package scheduler is a generic DB-backed job queue: jobs are inserted with
// a kind, a JSON payload, and a run_at time, and Tick (driven by a
// lane runners.SchedulerRunner on a fixed interval) dispatches due jobs to
// their registered handler.
package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/golgimed/mimic/internal/shared/clock"
	"github.com/golgimed/mimic/internal/shared/correlation"
	"github.com/golgimed/mimic/internal/shared/sim"
	"github.com/golgimed/mimic/internal/shared/timeline"
)

type JobHandler func(ctx context.Context, payload json.RawMessage) error

type Scheduler struct {
	db       *sql.DB
	rec      *timeline.Recorder
	mu       sync.RWMutex
	handlers map[string]JobHandler
}

func New(db *sql.DB) *Scheduler {
	return &Scheduler{db: db, rec: timeline.NewRecorder(db), handlers: make(map[string]JobHandler)}
}

func (s *Scheduler) RegisterJobHandler(kind string, handler JobHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[kind] = handler
}

// ScheduleJob enqueues a job of kind to run at runAt. ctx is the
// scheduling context (e.g. the HTTP request that caused it), used for the
// timeline.
func (s *Scheduler) ScheduleJob(ctx context.Context, kind string, payload any, runAt time.Time) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal job payload: %w", err)
	}
	runAtStr := runAt.UTC().Format(timeLayout)
	res, err := s.db.ExecContext(context.WithoutCancel(ctx),
		"INSERT INTO jobs (kind, payload_json, run_at, status, request_id, simulation_id) VALUES (?, ?, ?, 'pending', ?, ?)",
		kind, string(payloadJSON), runAtStr, correlation.RequestID(ctx), simulationID(ctx),
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	timeline.Record(timeline.WithRecorder(ctx, s.rec), timeline.Event{
		Type: timeline.JobScheduled, JobID: id, Data: map[string]any{"kind": kind, "runAt": runAtStr},
	})
	return nil
}

type jobRow struct {
	id          int64
	payloadJSON string
	requestID   string
	simID       string
}

func simulationID(ctx context.Context) string {
	if s := sim.FromContext(ctx); s != nil {
		return s.ID
	}
	return ""
}

type due struct {
	jobRow
	kind string
}

// Tick selects every due pending job and dispatches it to its registered
// handler, marking it processing -> done/failed. Matches the JobFn signature
// expected by lane's runners.SchedulerRunner.
func (s *Scheduler) Tick(ctx context.Context) error {
	jobs, err := s.fetchDueJobs(ctx)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		s.processJob(ctx, j)
	}
	return nil
}

// timeLayout is fixed-width so run_at compares correctly as a string
// (RFC3339Nano drops trailing zeros, which breaks lexical ordering).
const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

func (s *Scheduler) fetchDueJobs(ctx context.Context) ([]due, error) {
	now := clock.Now().Format(timeLayout)
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, kind, payload_json, request_id, simulation_id FROM jobs WHERE status = 'pending' AND run_at <= ? ORDER BY run_at",
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("query due jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []due
	for rows.Next() {
		var j due
		if err := rows.Scan(&j.id, &j.kind, &j.payloadJSON, &j.requestID, &j.simID); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// RunUntil runs, in run_at order, every pending job due at or before
// target, calling step(runAt) before each so the simulation clock reads
// the job's due time while it runs. Jobs scheduled by those jobs are
// picked up too if they fall due before target. It is the clock.Runner
// behind POST /admin/clock/advance.
func (s *Scheduler) RunUntil(ctx context.Context, target time.Time, step func(time.Time)) error {
	limit := target.UTC().Format(timeLayout)
	for range maxRunUntilJobs {
		var (
			j     due
			runAt string
		)
		err := s.db.QueryRowContext(ctx,
			"SELECT id, kind, payload_json, request_id, simulation_id, run_at FROM jobs WHERE status = 'pending' AND run_at <= ? ORDER BY run_at, id LIMIT 1",
			limit,
		).Scan(&j.id, &j.kind, &j.payloadJSON, &j.requestID, &j.simID, &runAt)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("query next due job: %w", err)
		}
		if t, err := time.Parse(timeLayout, runAt); err == nil {
			step(t)
		} else if t, err := time.Parse(time.RFC3339Nano, runAt); err == nil {
			step(t)
		}
		s.processJob(ctx, j)
	}
	return fmt.Errorf("more than %d jobs fell due while advancing the clock; possible scheduling loop", maxRunUntilJobs)
}

// maxRunUntilJobs bounds one RunUntil, guarding against a job that keeps
// rescheduling itself with no delay.
const maxRunUntilJobs = 10000

// processJob marks j processing, dispatches it to its registered handler (if
// any), and marks it done/failed. Errors are logged, not returned — one
// job's failure never stops Tick from processing the rest.
//
// The job runs with the correlation ID of the request that scheduled it,
// so everything it records (state changes, webhooks, follow-up jobs)
// stays traceable to that request.
func (s *Scheduler) processJob(ctx context.Context, j due) {
	ctx = correlation.WithRequestID(timeline.WithRecorder(ctx, s.rec), j.requestID)
	log := slog.With("job_id", j.id, "kind", j.kind, "request_id", j.requestID, "simulation_id", j.simID)
	// Restore the scheduling request's simulation so the job draws from
	// the same seed (and the same draw counters, for explicit ones).
	if j.simID != "" {
		if simulation, err := sim.StoreFor(s.db).Get(ctx, j.simID); err == nil {
			ctx = sim.WithSimulation(ctx, simulation)
		} else {
			log.Warn("scheduler: job's simulation not found; running unseeded", "error", err)
		}
	}
	jobEvent := func(typ timeline.Type, data map[string]any) {
		data["kind"] = j.kind
		timeline.Record(ctx, timeline.Event{Type: typ, JobID: j.id, Data: data})
	}

	if _, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'processing' WHERE id = ?", j.id); err != nil {
		log.Error("scheduler: failed to mark job processing", "error", err)
		return
	}

	s.mu.RLock()
	handler, ok := s.handlers[j.kind]
	s.mu.RUnlock()

	jobEvent(timeline.JobStarted, map[string]any{})
	var handlerErr error
	if ok {
		handlerErr = handler(ctx, json.RawMessage(j.payloadJSON))
	}

	if handlerErr != nil {
		jobEvent(timeline.JobFailed, map[string]any{"error": handlerErr.Error()})
		if _, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'failed' WHERE id = ?", j.id); err != nil {
			log.Error("scheduler: failed to mark job failed", "handler_error", handlerErr, "error", err)
		} else {
			log.Error("scheduler: job handler failed", "error", handlerErr)
		}
		return
	}
	jobEvent(timeline.JobCompleted, map[string]any{})
	if _, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'done' WHERE id = ?", j.id); err != nil {
		log.Error("scheduler: failed to mark job done", "error", err)
	}
}
