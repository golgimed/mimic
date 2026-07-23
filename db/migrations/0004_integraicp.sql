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
