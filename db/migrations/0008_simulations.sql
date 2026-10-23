-- Simulations (internal/shared/sim): the seed every random decision in a
-- simulation derives from. Jobs and timeline events carry the simulation
-- they belong to, so asynchronous work reuses the same seed.
CREATE TABLE simulations (
  id TEXT PRIMARY KEY,
  seed INTEGER NOT NULL,
  implicit INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

ALTER TABLE jobs ADD COLUMN simulation_id TEXT NOT NULL DEFAULT '';
ALTER TABLE timeline_events ADD COLUMN simulation_id TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_timeline_simulation ON timeline_events (simulation_id);
