// Package webhooks delivers fault-injectable webhook events and logs every
// delivery attempt (including dropped ones) for the admin dashboard.
package webhooks

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/golgimed/mimic/internal/registry"
)

type DeliverInput struct {
	Provider     string
	ResourceType string
	ResourceID   string
	URL          string
	Payload      any
	Headers      map[string]string
	Timeout      time.Duration // defaults to 5s when zero
}

// LogDropped records a fault-injected dropped webhook without attempting delivery.
func LogDropped(db *sql.DB, input DeliverInput) error {
	payloadJSON, err := json.Marshal(input.Payload)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO webhook_deliveries (provider, resource_type, resource_id, url, payload_json, status, response_code)
		 VALUES (?, ?, ?, ?, ?, 'dropped', NULL)`,
		input.Provider, input.ResourceType, input.ResourceID, input.URL, string(payloadJSON),
	)
	return err
}

// ListDeliveries returns delivery history for a resource, oldest first, for
// the admin dashboard's item detail view.
func ListDeliveries(db *sql.DB, provider, resourceID string) ([]registry.WebhookDeliveryView, error) {
	rows, err := db.Query(
		`SELECT url, payload_json, status, response_code, created_at
		 FROM webhook_deliveries WHERE provider = ? AND resource_id = ? ORDER BY created_at`,
		provider, resourceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []registry.WebhookDeliveryView{}
	for rows.Next() {
		var (
			url, payloadJSON, status, createdAt string
			responseCode                        sql.NullInt64
		)
		if err := rows.Scan(&url, &payloadJSON, &status, &responseCode, &createdAt); err != nil {
			return nil, err
		}
		var payload any
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return nil, err
		}
		view := registry.WebhookDeliveryView{
			URL: url, Payload: payload, Status: status, CreatedAt: createdAt,
		}
		if responseCode.Valid {
			view.ResponseCode = &responseCode.Int64
		}
		out = append(out, view)
	}
	return out, rows.Err()
}

// Deliver POSTs the JSON payload to the target URL with a timeout, then
// records the outcome ("delivered"/"failed" + response code) to
// webhook_deliveries. Network/timeout errors are recorded as "failed" but
// not returned as errors, matching the original best-effort delivery
// semantics.
func Deliver(ctx context.Context, db *sql.DB, input DeliverInput) error {
	timeout := input.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	payloadJSON, err := json.Marshal(input.Payload)
	if err != nil {
		return err
	}

	status := "delivered"
	var responseCode sql.NullInt64

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, input.URL, bytes.NewReader(payloadJSON))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		for k, v := range input.Headers {
			req.Header.Set(k, v)
		}
		res, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			status = "failed"
		} else {
			responseCode = sql.NullInt64{Int64: int64(res.StatusCode), Valid: true}
			if res.StatusCode < 200 || res.StatusCode >= 300 {
				status = "failed"
			}
			res.Body.Close()
		}
	} else {
		status = "failed"
	}

	if _, err := db.Exec(
		`INSERT INTO webhook_deliveries (provider, resource_type, resource_id, url, payload_json, status, response_code)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		input.Provider, input.ResourceType, input.ResourceID, input.URL, string(payloadJSON), status, responseCode,
	); err != nil {
		return fmt.Errorf("record webhook delivery: %w", err)
	}
	return nil
}
