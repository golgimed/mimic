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
