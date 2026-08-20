-- BRy SCAD collection state. The original request is retained as JSON so the
-- simulator can round-trip documented fields without imposing an incomplete
-- local schema on a broad provider contract.
CREATE TABLE bry_scad_collections (
  chave TEXT PRIMARY KEY,
  payload_json TEXT NOT NULL,
  situacao TEXT NOT NULL DEFAULT 'PENDENTE',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_bry_scad_collections_created_at ON bry_scad_collections (created_at);
