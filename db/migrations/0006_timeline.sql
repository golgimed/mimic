-- Simulation timeline: one row per notable lifecycle event (request,
-- response, state change, job, webhook, fault), see internal/shared/timeline.
CREATE TABLE timeline_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts TEXT NOT NULL,
  type TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT '',
  resource_type TEXT NOT NULL DEFAULT '',
  resource_id TEXT NOT NULL DEFAULT '',
  job_id INTEGER,
  data_json TEXT
);

CREATE INDEX idx_timeline_resource ON timeline_events (provider, resource_id);
