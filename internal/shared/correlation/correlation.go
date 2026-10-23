// Package correlation carries the request ID that ties a simulated HTTP
// request to everything it causes later — timeline events, scheduled jobs,
// state transitions, webhook deliveries and logs.
package correlation

import (
	"context"

	"github.com/google/uuid"
)

// Header is the response (and outgoing webhook) header exposing the ID.
const Header = "X-Mimic-Request-ID"

// ClientHeader is the conventional client-supplied request ID header;
// Mimic reuses its value when it is a sane token.
const ClientHeader = "X-Request-ID"

type key struct{}

// WithRequestID returns ctx carrying id.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, key{}, id)
}

// RequestID returns the request ID in ctx, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(key{}).(string)
	return id
}

// FromClient returns the client-supplied ID if it is usable (1-128
// visible ASCII characters), otherwise a fresh UUID.
func FromClient(value string) string {
	if len(value) == 0 || len(value) > 128 {
		return uuid.NewString()
	}
	for i := 0; i < len(value); i++ {
		if value[i] < 0x21 || value[i] > 0x7e {
			return uuid.NewString()
		}
	}
	return value
}
