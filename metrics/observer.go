// Package metrics provides observability interfaces and telemetry data structures for distlimit.
package metrics

import (
	"context"
	"time"
)

// Event represents a single rate limit evaluation telemetry snapshot.
type Event struct {
	// Key is the rate limit key evaluated.
	Key string
	// Route is the HTTP/RPC route pattern associated with the request (if route labeling is enabled).
	Route string
	// Allowed indicates whether the request was permitted.
	Allowed bool
	// Limit is the configured maximum request capacity.
	Limit int64
	// Remaining is the remaining request quota available.
	Remaining int64
	// ResetIn specifies the duration until quota resets.
	ResetIn time.Duration
	// Driver is the storage driver identifier (e.g., "memory", "redis", "hybrid").
	Driver string
	// Algorithm is the strategy identifier (e.g., "sliding_counter", "token_bucket").
	Algorithm string
	// Duration is the internal execution latency of the rate limit evaluation.
	Duration time.Duration
}

type routeContextKey struct{}

// ContextWithRoute returns a copy of parent context carrying the given route pattern string.
func ContextWithRoute(ctx context.Context, route string) context.Context {
	if route == "" {
		return ctx
	}
	return context.WithValue(ctx, routeContextKey{}, route)
}

// RouteFromContext extracts the route pattern string from context if present.
func RouteFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(routeContextKey{}).(string); ok {
		return v
	}
	return ""
}

// Observer defines the interface for collecting telemetry metrics and traces.
type Observer interface {
	// Observe receives rate limit evaluation events for recording metrics/telemetry.
	Observe(ctx context.Context, event Event)
}

// noopObserver is a default observer implementation that performs no operations.
type noopObserver struct{}

// NewNoopObserver creates an observer that discards all recorded events.
func NewNoopObserver() Observer {
	return &noopObserver{}
}

// Observe implements Observer by taking no action.
func (n *noopObserver) Observe(ctx context.Context, event Event) {}
