-- Scenario replay (internal/shared/sim/replay.go): the requests each
-- simulation received, in order, and which simulation a replay came from.
CREATE TABLE simulation_requests (
  simulation_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  offset_ms INTEGER NOT NULL,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  headers_json TEXT NOT NULL,
  body BLOB,
  status INTEGER,
  PRIMARY KEY (simulation_id, seq)
);

ALTER TABLE simulations ADD COLUMN replayed_from TEXT NOT NULL DEFAULT '';
