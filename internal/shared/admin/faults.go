// Package admin implements the simulator's control-plane: fault injection
// configuration/consumption and the cross-provider dashboard aggregation
// endpoints.
package admin

import (
	"database/sql"
	"sync"

	"github.com/golgimed/mimic/internal/shared/behavior"
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
}

type Store struct {
	db           *sql.DB
	rateLimiters sync.Map
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

const faultColumns = `id, provider, route_pattern, fault_kind, fault_value, remaining_uses, created_at, probability, delay_distribution`

func scanFault(row interface {
	Scan(dest ...any) error
}) (*FaultConfig, error) {
	var f FaultConfig
	if err := row.Scan(&f.ID, &f.Provider, &f.RoutePattern, &f.FaultKind, &f.FaultValue, &f.RemainingUses, &f.CreatedAt, &f.Probability, &f.DelayDistribution); err != nil {
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
}

func (s *Store) CreateFault(input CreateFaultInput) (*FaultConfig, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(
		`INSERT INTO fault_config (id, provider, route_pattern, fault_kind, fault_value, remaining_uses, probability, delay_distribution)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Provider, input.RoutePattern, string(input.FaultKind), input.FaultValue, input.Times, input.Probability, input.DelayDistribution,
	)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(
		`SELECT `+faultColumns+`
		 FROM fault_config WHERE id = ?`, id,
	)
	return scanFault(row)
}

func (s *Store) ListFaults() ([]FaultConfig, error) {
	rows, err := s.db.Query(
		`SELECT ` + faultColumns + `
		 FROM fault_config ORDER BY created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
func (s *Store) ConsumeMatchingFault(provider, routePattern string) (*FaultConfig, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRow(
		`SELECT `+faultColumns+`
		 FROM fault_config
		 WHERE provider = ? AND (route_pattern = ? OR route_pattern IS NULL)
		 ORDER BY route_pattern IS NULL ASC, created_at ASC
		 LIMIT 1`,
		provider, routePattern,
	)
	f, err := scanFault(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if !behavior.ShouldApply(f.Probability) {
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
