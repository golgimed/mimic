// Package timeline records what happened during a simulation — requests,
// responses, state changes, jobs, webhooks, faults — as one ordered event
// log, readable via GET /admin/timeline and the dashboard.
//
// Recording is context-scoped: whoever owns the database (the core mux for
// requests, the scheduler for jobs) puts a Recorder into the context, and
// code further down just calls Record(ctx, ...). Without a Recorder in the
// context Record is a no-op, so packages stay usable in isolation.
package timeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/golgimed/mimic/internal/shared/correlation"
	"github.com/golgimed/mimic/internal/shared/sim"
)

// Type is a timeline event type.
type Type string

const (
	RequestReceived  Type = "RequestReceived"
	ResponseReturned Type = "ResponseReturned"
	ResourceCreated  Type = "ResourceCreated"
	StateChanged     Type = "StateChanged"
	JobScheduled     Type = "JobScheduled"
	JobStarted       Type = "JobStarted"
	JobCompleted     Type = "JobCompleted"
	JobFailed        Type = "JobFailed"
	WebhookDelivered Type = "WebhookDelivered"
	WebhookFailed    Type = "WebhookFailed"
	WebhookDropped   Type = "WebhookDropped"
	FaultApplied     Type = "FaultApplied"
)

// Event is one timeline entry. Data holds type-specific details (HTTP
// method/path/status, from/to states, job kind, webhook URL, fault kind).
type Event struct {
	ID           int64          `json:"id"`
	Time         string         `json:"ts"`
	Type         Type           `json:"type"`
	Provider     string         `json:"provider,omitempty"`
	ResourceType string         `json:"resourceType,omitempty"`
	ResourceID   string         `json:"resourceId,omitempty"`
	RequestID    string         `json:"requestId,omitempty"`
	SimulationID string         `json:"simulationId,omitempty"`
	JobID        int64          `json:"jobId,omitempty"`
	Data         map[string]any `json:"data,omitempty"`
}

// Recorder writes events to the timeline_events table.
type Recorder struct{ db *sql.DB }

func NewRecorder(db *sql.DB) *Recorder { return &Recorder{db: db} }

type recorderKey struct{}

// WithRecorder returns ctx carrying rec.
func WithRecorder(ctx context.Context, rec *Recorder) context.Context {
	return context.WithValue(ctx, recorderKey{}, rec)
}

// Record appends ev to the timeline of the Recorder in ctx, if any.
// Best-effort: a failed write is logged, never returned — observability
// must not change simulated behavior.
func Record(ctx context.Context, ev Event) {
	rec, _ := ctx.Value(recorderKey{}).(*Recorder)
	if rec == nil {
		return
	}
	if ev.RequestID == "" {
		ev.RequestID = correlation.RequestID(ctx)
	}
	if ev.SimulationID == "" {
		if s := sim.FromContext(ctx); s != nil {
			ev.SimulationID = s.ID
		}
	}
	if ev.Time == "" {
		ev.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	var data any
	if len(ev.Data) > 0 {
		raw, err := json.Marshal(ev.Data)
		if err != nil {
			slog.Warn("timeline: marshal event data", "type", ev.Type, "error", err)
			return
		}
		data = string(raw)
	}
	var jobID any
	if ev.JobID != 0 {
		jobID = ev.JobID
	}
	if _, err := rec.db.ExecContext(context.WithoutCancel(ctx),
		`INSERT INTO timeline_events (ts, type, provider, resource_type, resource_id, request_id, simulation_id, job_id, data_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.Time, string(ev.Type), ev.Provider, ev.ResourceType, ev.ResourceID, ev.RequestID, ev.SimulationID, jobID, data,
	); err != nil {
		slog.Warn("timeline: record event", "type", ev.Type, "error", err)
	}
}

// Filter narrows List. Zero fields don't filter.
type Filter struct {
	RequestID    string
	SimulationID string
	Provider     string
	ResourceID   string
	Limit        int // default 500
}

// List returns matching events, oldest first.
func List(ctx context.Context, db *sql.DB, f Filter) ([]Event, error) {
	var where []string
	var args []any
	if f.RequestID != "" {
		where = append(where, "request_id = ?")
		args = append(args, f.RequestID)
	}
	if f.SimulationID != "" {
		where = append(where, "simulation_id = ?")
		args = append(args, f.SimulationID)
	}
	if f.Provider != "" {
		where = append(where, "provider = ?")
		args = append(args, f.Provider)
	}
	if f.ResourceID != "" {
		where = append(where, "resource_id = ?")
		args = append(args, f.ResourceID)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 500
	}
	query := `SELECT id, ts, type, provider, resource_type, resource_id, request_id, simulation_id, job_id, data_json FROM timeline_events`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	// Newest N, returned oldest first.
	query = `SELECT * FROM (` + query + ` ORDER BY id DESC LIMIT ?) ORDER BY id`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []Event{}
	for rows.Next() {
		var (
			ev    Event
			typ   string
			jobID sql.NullInt64
			data  sql.NullString
		)
		if err := rows.Scan(&ev.ID, &ev.Time, &typ, &ev.Provider, &ev.ResourceType, &ev.ResourceID, &ev.RequestID, &ev.SimulationID, &jobID, &data); err != nil {
			return nil, err
		}
		ev.Type = Type(typ)
		ev.JobID = jobID.Int64
		if data.Valid {
			if err := json.Unmarshal([]byte(data.String), &ev.Data); err != nil {
				return nil, err
			}
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}
