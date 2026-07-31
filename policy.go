package distlimit

import (
	"context"
	"time"
)

// Policy defines the rate limit capacity rules (Limit and Window duration).
type Policy struct {
	// Limit is the maximum number of requests allowed within the Window duration.
	Limit int64
	// Window is the time duration of the rate limit window.
	Window time.Duration
}

// PolicyResolver defines an interface for dynamically resolving rate limit policies per key or user tier.
type PolicyResolver interface {
	// ResolvePolicy returns a dynamic Policy for the specified key.
	// If false is returned, the default Limiter policy will be used.
	ResolvePolicy(ctx context.Context, key string) (Policy, bool)
}
