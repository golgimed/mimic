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
