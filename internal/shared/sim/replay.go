package sim

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/golgimed/mimic/internal/shared/clock"
)

// MaxRecordedBody caps how much of a request body is kept for replay.
const MaxRecordedBody = 1 << 20

// notReplayed are request headers a replay must not copy: per-request
// identity (a replay gets its own simulation and request IDs) and
// transport details.
var notReplayed = map[string]bool{
	HeaderID:          true,
	"X-Request-Id":    true,
	"Content-Length":  true,
	"Connection":      true,
	"Accept-Encoding": true,
}

// RecordedRequest is one request a simulation received.
type RecordedRequest struct {
	Seq      int         `json:"seq"`
	OffsetMS int64       `json:"offsetMs"` // simulated time since the simulation started
	Method   string      `json:"method"`
	Path     string      `json:"path"` // including the query string
	Header   http.Header `json:"-"`
	Body     []byte      `json:"-"`
	Status   int         `json:"status"` // response status; 0 if unknown
}

// RecordRequest appends a request to s's replay log and returns its seq.
// Only the minimum needed to reproduce it is kept: method, path, the
// headers a provider can read (minus per-request identity), the body (up to
// MaxRecordedBody) and its simulated-time offset.
func (st *Store) RecordRequest(ctx context.Context, s *Simulation, r *http.Request, body []byte) (int, error) {
	header := http.Header{}
	for k, v := range r.Header {
		if !notReplayed[http.CanonicalHeaderKey(k)] {
			header[k] = v
		}
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return 0, err
	}
	var offset int64
	if created, err := time.Parse(time.RFC3339Nano, s.CreatedAt); err == nil {
		offset = max(0, clock.Now().Sub(created).Milliseconds())
	}
	var seq int
	err = st.db.QueryRowContext(context.WithoutCancel(ctx),
		`INSERT INTO simulation_requests (simulation_id, seq, offset_ms, method, path, headers_json, body)
		 VALUES (?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM simulation_requests WHERE simulation_id = ?), ?, ?, ?, ?, ?)
		 RETURNING seq`,
		s.ID, s.ID, offset, r.Method, r.URL.RequestURI(), string(headerJSON), body,
	).Scan(&seq)
	return seq, err
}

// RecordStatus stores the response status of a recorded request.
func (st *Store) RecordStatus(ctx context.Context, simulationID string, seq, status int) error {
	_, err := st.db.ExecContext(context.WithoutCancel(ctx),
		`UPDATE simulation_requests SET status = ? WHERE simulation_id = ? AND seq = ?`, status, simulationID, seq)
	return err
}

// Requests returns a simulation's recorded requests in order.
func (st *Store) Requests(ctx context.Context, simulationID string) ([]RecordedRequest, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT seq, offset_ms, method, path, headers_json, body, status FROM simulation_requests
		 WHERE simulation_id = ? ORDER BY seq`, simulationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []RecordedRequest{}
	for rows.Next() {
		var (
			rr         RecordedRequest
			headerJSON string
			status     sql.NullInt64
		)
		if err := rows.Scan(&rr.Seq, &rr.OffsetMS, &rr.Method, &rr.Path, &headerJSON, &rr.Body, &status); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(headerJSON), &rr.Header); err != nil {
			return nil, err
		}
		rr.Status = int(status.Int64)
		out = append(out, rr)
	}
	return out, rows.Err()
}
