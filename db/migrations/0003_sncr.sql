-- SNCR simulator state. Matches the real SNCR API (ANVISA Manual da API,
-- 1ª edição, 2026-06 + Instruções de apoio ao processo de integração v1.0,
-- 2026-07 — see internal/providers/sncr/README.md), not an assumption.

-- One-time session tokens issued by GET /sncr/api/v1/auth/login, exchanged
-- exactly once by GET /sncr/api/v1/auth/token for the real access token.
-- conselho/uf/documento are the simulator-only identity the login call was
-- given (see README) — carried here so the token exchange can mint an
-- access token bound to that identity, same as the real flow binds it to
-- whichever Gov.br professional actually logged in.
CREATE TABLE sncr_auth_sessions (
  session_token TEXT PRIMARY KEY,
  access_token TEXT NOT NULL,
  conselho TEXT NOT NULL,
  uf TEXT NOT NULL,
  documento TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Issued access tokens (Bearer), mapped back to the simulated authenticated
-- professional so the numeracoes endpoints can enforce "authenticated
-- professional must match the requested conselho/uf/documento".
CREATE TABLE sncr_access_tokens (
  access_token TEXT PRIMARY KEY,
  conselho TEXT NOT NULL,
  uf TEXT NOT NULL,
  documento TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Tracks the running numbering sequence and remaining daily/monthly balance
-- per (kind, receita_or_tipo, conselho, uf, documento, period_key) so
-- repeated calls hand out non-overlapping numbers/ranges and
-- low-balance/limit-reached responses are simulated deterministically.
--
-- kind='notificacao': one row per (receita, prescriber, day) — the 50/day
-- limit is per receita type, so NRA and NRB balances don't share a row.
--
-- kind='especial_retencao': two different row shapes share this table.
--   - receita_or_tipo='RCE' or 'RET': tracks that type's own number
--     sequence (next_sequence) so RCE and RET each get their own series.
--   - receita_or_tipo='COMBINED': tracks the monthly 3-requests/3000-numbers
--     limit, which the manual states applies jointly across RCE+RET for a
--     given prescriber — next_sequence is unused on this row.
CREATE TABLE sncr_numbering_sequences (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL, -- 'notificacao' | 'especial_retencao'
  receita_or_tipo TEXT NOT NULL, -- NRA/NRB/NRB2/NRR/NRT, or RCE/RET/COMBINED
  conselho TEXT NOT NULL,
  uf TEXT NOT NULL,
  documento TEXT NOT NULL,
  next_sequence INTEGER NOT NULL DEFAULT 1,
  period_key TEXT NOT NULL, -- date (notificacao, daily) or year-month (especial_retencao, monthly)
  issued_in_period INTEGER NOT NULL DEFAULT 0,
  requests_in_period INTEGER NOT NULL DEFAULT 0, -- especial_retencao COMBINED row: 3 requests/month limit
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (kind, receita_or_tipo, conselho, uf, documento, period_key)
);
