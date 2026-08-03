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

CREATE TABLE zenvia_messages (
  id TEXT PRIMARY KEY,
  external_id TEXT,
  channel TEXT NOT NULL,
  direction TEXT NOT NULL DEFAULT 'OUT',
  sender TEXT NOT NULL,
  recipient TEXT NOT NULL,
  contents_json TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ACCEPTED',
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_zenvia_messages_channel ON zenvia_messages (channel);

CREATE TABLE zenvia_subscriptions (
  id TEXT PRIMARY KEY,
  event_type TEXT NOT NULL,
  webhook_url TEXT NOT NULL,
  webhook_headers_json TEXT,
  criteria_channel TEXT NOT NULL,
  criteria_direction TEXT,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_zenvia_subscriptions_event_channel ON zenvia_subscriptions (event_type, criteria_channel, status);

CREATE TABLE integraicp_credentials (
  id TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL,
  subject_key TEXT,
  subject_type TEXT,
  code_challenge TEXT NOT NULL,
  callback_uri TEXT NOT NULL,
  certificate_json TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE openapi_resources (
  spec_name TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (spec_name, resource_type, resource_id)
);

CREATE INDEX idx_openapi_resources_list ON openapi_resources (spec_name, resource_type);
