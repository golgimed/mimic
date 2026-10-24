// Package admin implements the simulator's control-plane: fault injection
// configuration/consumption and the cross-provider dashboard aggregation
// endpoints.
package admin

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/golgimed/mimic/internal/shared/behavior"
	"github.com/golgimed/mimic/internal/shared/sim"
	"github.com/google/uuid"
)

type FaultKind string

const (
	FaultDelayMS        FaultKind = "delay_ms"
	FaultHTTPStatus     FaultKind = "http_status"
	FaultTimeout        FaultKind = "timeout"
	FaultInvalidPayload FaultKind = "invalid_payload"
	FaultWebhookDropped FaultKind = "webhook_dropped"
	FaultWebhookInvalid FaultKind = "webhook_invalid"
	FaultRateLimited    FaultKind = "rate_limited"
)

type FaultConfig struct {
	ID                string    `json:"id"`
	Provider          string    `json:"provider"`
	RoutePattern      *string   `json:"routePattern"`
	FaultKind         FaultKind `json:"faultKind"`
	FaultValue        *string   `json:"faultValue"`
	RemainingUses     *int64    `json:"remainingUses"`
	CreatedAt         string    `json:"createdAt"`
	Probability       *float64  `json:"probability"`
	DelayDistribution *string   `json:"delayDistribution"`
	SimulationID      string    `json:"simulationId,omitempty"`
}

type Store struct {
	db           *sql.DB
	rateLimiters sync.Map
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Query strings are complete literals (each lists the columns scanFault
// reads, in order): nothing request-derived ever reaches the SQL text, and
// all variable input is passed as bind parameters.
const (
	selectFaultByID = `SELECT id, provider, route_pattern, fault_kind, fault_value, remaining_uses, created_at, probability, delay_distribution, simulation_id
		FROM fault_config WHERE id = ?`
	selectAllFaults = `SELECT id, provider, route_pattern, fault_kind, fault_value, remaining_uses, created_at, probability, delay_distribution, simulation_id
		FROM fault_config ORDER BY created_at`
	// Faults scoped to the request's simulation (scenario faults) win over
	// global ones; then exact routes over provider-wide ones.
	selectMatchingFault = `SELECT id, provider, route_pattern, fault_kind, fault_value, remaining_uses, created_at, probability, delay_distribution, simulation_id
		FROM fault_config
		WHERE provider = ? AND (route_pattern = ? OR route_pattern IS NULL)
		  AND (simulation_id = '' OR simulation_id = ?)
		ORDER BY simulation_id = '' ASC, route_pattern IS NULL ASC, created_at ASC
		LIMIT 1`
)

func scanFault(row interface {
	Scan(dest ...any) error
}) (*FaultConfig, error) {
	var f FaultConfig
	if err := row.Scan(&f.ID, &f.Provider, &f.RoutePattern, &f.FaultKind, &f.FaultValue, &f.RemainingUses, &f.CreatedAt, &f.Probability, &f.DelayDistribution, &f.SimulationID); err != nil {
		return nil, err
	}
	return &f, nil
}

type CreateFaultInput struct {
	Provider          string
	RoutePattern      *string
	FaultKind         FaultKind
	FaultValue        *string
	Times             *int64
	Probability       *float64
	DelayDistribution *string
	// SimulationID scopes the fault to one simulation (scenario faults).
	SimulationID string
}

func (s *Store) CreateFault(input CreateFaultInput) (*FaultConfig, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(
		`INSERT INTO fault_config (id, provider, route_pattern, fault_kind, fault_value, remaining_uses, probability, delay_distribution, simulation_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Provider, input.RoutePattern, string(input.FaultKind), input.FaultValue, input.Times, input.Probability, input.DelayDistribution, input.SimulationID,
	)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(selectFaultByID, id)
	return scanFault(row)
}

func (s *Store) ListFaults() ([]FaultConfig, error) {
	rows, err := s.db.Query(selectAllFaults)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []FaultConfig{}
	for rows.Next() {
		f, err := scanFault(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

func (s *Store) DeleteFault(id string) (bool, error) {
	res, err := s.db.Exec("DELETE FROM fault_config WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// DeleteFaultsByProvider removes every fault_config row for a provider. Used
// to reseed spec-declared (x-mimic-behavior) faults fresh on every boot.
func (s *Store) DeleteFaultsByProvider(provider string) error {
	_, err := s.db.Exec("DELETE FROM fault_config WHERE provider = ?", provider)
	return err
}

// ConsumeMatchingFault finds the best-matching active fault for a provider +
// route, consumes one use (deleting it once remaining_uses hits 0), and
// returns it. Exact route_pattern matches win over provider-wide (null
// pattern) faults. A fault with a probability that doesn't fire this time is
// treated as no match and its remaining_uses is left untouched.
//
// The probability roll draws from ctx's simulation seed (internal/shared/
// sim), so it is reproducible for a given seed and request sequence.
func (s *Store) ConsumeMatchingFault(ctx context.Context, provider, routePattern string) (*FaultConfig, error) {
	tx, err := s.db.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	simID := ""
	if s := sim.FromContext(ctx); s != nil {
		simID = s.ID
	}
	row := tx.QueryRow(selectMatchingFault, provider, routePattern, simID)
	f, err := scanFault(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if f.Probability != nil && !behavior.ShouldApply(sim.Rand(ctx, "fault:"+f.randKey()), f.Probability) {
		return nil, tx.Commit()
	}

	if f.RemainingUses != nil {
		if *f.RemainingUses <= 1 {
			if _, err := tx.Exec("DELETE FROM fault_config WHERE id = ?", f.ID); err != nil {
				return nil, err
			}
		} else {
			if _, err := tx.Exec("UPDATE fault_config SET remaining_uses = remaining_uses - 1 WHERE id = ?", f.ID); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return f, nil
}

// randKey names this fault's random stream by what it targets (not its
// random ID), so the same fault configuration under the same simulation
// seed rolls the same way across runs.
func (f *FaultConfig) randKey() string {
	route := "*"
	if f.RoutePattern != nil {
		route = *f.RoutePattern
	}
	return f.Provider + ":" + route + ":" + string(f.FaultKind)
}
