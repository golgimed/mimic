// Package sim models a simulation: the deterministic context a sequence of
// requests — and everything they schedule — runs in. A simulation has an
// ID and a seed; every random decision Mimic makes inside it (fault
// probability rolls, latency jitter, generated fake data) draws from a
// stream derived from that seed, so the same requests under the same seed
// and configuration behave the same way.
package sim

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"

	"github.com/golgimed/mimic/internal/shared/clock"
)

// Headers: clients select a simulation with HeaderID; responses report the
// active one with HeaderID and HeaderSeed.
const (
	HeaderID   = "X-Mimic-Simulation"
	HeaderSeed = "X-Mimic-Seed"
	// ResponseHeaderID names the active simulation on every response.
	ResponseHeaderID = "X-Mimic-Simulation-ID"
)

// maxSeed keeps seeds exactly representable as JSON numbers in JavaScript.
const maxSeed = 1<<53 - 1

// ErrNotFound is returned for an unknown simulation ID.
var ErrNotFound = errors.New("simulation not found")

// Simulation is one simulation. Explicit simulations are created via
// POST /admin/simulations and selected per request with the
// X-Mimic-Simulation header; a request without one runs in a fresh
// implicit simulation of its own.
type Simulation struct {
	ID        string `json:"id"`
	Seed      int64  `json:"seed"`
	Scenario  string `json:"scenario,omitempty"`
	Profile   string `json:"profile,omitempty"`
	Implicit  bool   `json:"implicit"`
	CreatedAt string `json:"createdAt"`

	mu    sync.Mutex
	draws map[string]uint64
}

// Rand returns the next random stream for key. Streams are derived from
// (seed, key, n) where n counts earlier draws for the same key in this
// simulation, so repeated decisions differ from each other while the whole
// sequence stays reproducible for a given seed and request order.
func (s *Simulation) Rand(key string) *rand.Rand {
	s.mu.Lock()
	if s.draws == nil {
		s.draws = map[string]uint64{}
	}
	n := s.draws[key]
	s.draws[key] = n + 1
	s.mu.Unlock()

	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return rand.New(rand.NewPCG(uint64(s.Seed), h.Sum64()^(n*0x9E3779B97F4A7C15))) //NOSONAR -- deterministic simulation randomness, not security-sensitive
}

type ctxKey struct{}

// WithSimulation returns ctx carrying s.
func WithSimulation(ctx context.Context, s *Simulation) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// FromContext returns the simulation in ctx, or nil.
func FromContext(ctx context.Context) *Simulation {
	s, _ := ctx.Value(ctxKey{}).(*Simulation)
	return s
}

// Rand returns the next random stream for key in ctx's simulation. Outside
// any simulation it is randomly seeded (non-reproducible) — every
// simulated request runs inside one, so that only affects code exercised
// in isolation.
func Rand(ctx context.Context, key string) *rand.Rand {
	if s := FromContext(ctx); s != nil {
		return s.Rand(key)
	}
	return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) //NOSONAR -- simulation randomness, not security-sensitive
}

// Faker returns a gofakeit faker drawing from Rand(ctx, key).
func Faker(ctx context.Context, key string) *gofakeit.Faker {
	return gofakeit.New(Rand(ctx, key).Uint64())
}

// NewSeed returns a random seed in [1, 2^53-1].
func NewSeed() int64 {
	return rand.Int64N(maxSeed) + 1 //NOSONAR -- simulation seed, not security-sensitive
}

// ValidSeed reports whether seed is usable (1..2^53-1).
func ValidSeed(seed int64) bool { return seed >= 1 && seed <= maxSeed }

// Store persists simulations. Explicit simulations stay cached in memory
// for the process lifetime so their draw counters carry across requests
// and scheduled jobs.
type Store struct {
	db    *sql.DB
	mu    sync.Mutex
	cache map[string]*Simulation
}

var stores sync.Map // *sql.DB -> *Store

// StoreFor returns the Store for db (one per database, created on first
// use), so the HTTP layer and the scheduler share draw counters.
func StoreFor(db *sql.DB) *Store {
	s, _ := stores.LoadOrStore(db, &Store{db: db, cache: map[string]*Simulation{}})
	return s.(*Store)
}

// Params are a new simulation's inputs. Seed 0 means "generate one";
// Scenario "" means providers' default behavior (the caller validates the
// name); Profile "" means the MIMIC_PROFILE default.
type Params struct {
	Seed     int64
	Scenario string
	Profile  string
	Implicit bool
}

// Create persists a new simulation.
func (st *Store) Create(ctx context.Context, p Params) (*Simulation, error) {
	seed, scenario, implicit := p.Seed, p.Scenario, p.Implicit
	profile := p.Profile
	if profile == "" {
		profile = defaultProfile
	}
	if err := ValidateProfile(profile); err != nil {
		return nil, err
	}
	if seed == 0 {
		seed = NewSeed()
	}
	if !ValidSeed(seed) {
		return nil, fmt.Errorf("seed must be between 1 and %d", int64(maxSeed))
	}
	s := &Simulation{
		ID:        "sim_" + uuid.NewString(),
		Seed:      seed,
		Scenario:  scenario,
		Profile:   profile,
		Implicit:  implicit,
		CreatedAt: clock.Now().Format(time.RFC3339Nano),
	}
	if _, err := st.db.ExecContext(context.WithoutCancel(ctx),
		`INSERT INTO simulations (id, seed, scenario, profile, implicit, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		s.ID, s.Seed, s.Scenario, s.Profile, s.Implicit, s.CreatedAt,
	); err != nil {
		return nil, err
	}
	if !implicit {
		st.mu.Lock()
		st.cache[s.ID] = s
		st.mu.Unlock()
	}
	return s, nil
}

// Get returns a simulation by ID (ErrNotFound if unknown).
func (st *Store) Get(ctx context.Context, id string) (*Simulation, error) {
	st.mu.Lock()
	cached := st.cache[id]
	st.mu.Unlock()
	if cached != nil {
		return cached, nil
	}

	var s Simulation
	err := st.db.QueryRowContext(ctx,
		`SELECT id, seed, scenario, profile, implicit, created_at FROM simulations WHERE id = ?`, id,
	).Scan(&s.ID, &s.Seed, &s.Scenario, &s.Profile, &s.Implicit, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !s.Implicit {
		st.mu.Lock()
		if c := st.cache[id]; c != nil {
			st.mu.Unlock()
			return c, nil
		}
		st.cache[id] = &s
		st.mu.Unlock()
	}
	return &s, nil
}

// List returns the most recent simulations, newest first.
func (st *Store) List(ctx context.Context, includeImplicit bool, limit int) ([]*Simulation, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `SELECT id, seed, scenario, profile, implicit, created_at FROM simulations`
	if !includeImplicit {
		query += ` WHERE implicit = 0`
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	rows, err := st.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []*Simulation{}
	for rows.Next() {
		var s Simulation
		if err := rows.Scan(&s.ID, &s.Seed, &s.Scenario, &s.Profile, &s.Implicit, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// Reset drops the in-memory cache (after a flush wiped the table).
func (st *Store) Reset() {
	st.mu.Lock()
	st.cache = map[string]*Simulation{}
	st.mu.Unlock()
}
