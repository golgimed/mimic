-- BRy Medical Signature / KMS pré-autorização tokens. Each token is consumed
-- by the HUB Signer's medical signing call and rejected once expired or
-- fully spent, mirroring how BRy's real KMS pré-autorização tracks
-- remaining uses server-side.
CREATE TABLE bry_medical_preauth_tokens (
  token TEXT PRIMARY KEY,
  uuid_chave TEXT NOT NULL,
  uses_remaining INTEGER NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_bry_medical_preauth_tokens_expires_at ON bry_medical_preauth_tokens (expires_at);
