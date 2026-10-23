-- Request correlation IDs (internal/shared/correlation), carried from the
-- HTTP request that caused them.
ALTER TABLE timeline_events ADD COLUMN request_id TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN request_id TEXT NOT NULL DEFAULT '';
ALTER TABLE webhook_deliveries ADD COLUMN request_id TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_timeline_request ON timeline_events (request_id);
