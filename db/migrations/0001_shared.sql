CREATE TABLE jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  run_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_jobs_status_run_at ON jobs (status, run_at);

CREATE TABLE webhook_deliveries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  provider TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  url TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  status TEXT NOT NULL,
  response_code INTEGER,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_webhook_deliveries_resource ON webhook_deliveries (provider, resource_type, resource_id);

CREATE TABLE fault_config (
  id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  route_pattern TEXT,
  fault_kind TEXT NOT NULL,
  fault_value TEXT,
  remaining_uses INTEGER,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
