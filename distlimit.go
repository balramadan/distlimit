// Package distlimit provides a high-performance, resilient, and pluggable rate limiting library for Go applications.
// It supports pluggable algorithm strategies (Token Bucket, Leaky Bucket, Sliding Window Counter, Sliding Window Log, Fixed Window),
// multiple storage drivers (In-Memory, Redis, Hybrid Dual-Tier), and turn-key middleware adapters for popular frameworks.
package distlimit

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/balramadan/distlimit/algorithm"
	"github.com/balramadan/distlimit/algorithm/slidingcounter"
	"github.com/balramadan/distlimit/metrics"
)

// Result is a type alias for algorithm.Result for backward compatibility with v1.0.0.
type Result = algorithm.Result

// State is a type alias for algorithm.State for backward compatibility with v1.0.0.
type State = algorithm.State

// Sentinels errors returned by distlimit.
var (
	// ErrInvalidLimit is returned when the configured rate limit is less than or equal to 0.
	ErrInvalidLimit = errors.New("distlimit: limit must be greater than 0")

	// ErrInvalidWindow is returned when the configured rate limit window is less than or equal to 0.
	ErrInvalidWindow = errors.New("distlimit: window must be greater than 0")

	// ErrNilDriver is returned when a nil Driver is supplied during Limiter initialization.
	ErrNilDriver = errors.New("distlimit: storage driver cannot be nil")

	// ErrNilAlgorithm is returned when a nil Algorithm is supplied to WithAlgorithm.
	ErrNilAlgorithm = errors.New("distlimit: algorithm strategy cannot be nil")
)

// KeyFunc defines a function signature for dynamically extracting a rate limit key from a Context.
type KeyFunc func(ctx context.Context) string

// Driver defines the interface that all rate limiting storage backends (Memory, Redis, Hybrid) must implement.
type Driver interface {
	// Allow evaluates rate limit rules for the specified key using the given algorithm strategy.
	Allow(ctx context.Context, key string, limit int64, window time.Duration, alg algorithm.Algorithm) (algorithm.Result, error)

	// Reset clears the rate limit state entry for the specified key in the storage driver.
	Reset(ctx context.Context, key string) error

	// Close gracefully releases any storage connections, background routines, or resources held by the driver.
	Close(ctx context.Context) error
}

// Config holds configuration parameters used during Limiter construction.
type Config struct {
	limit          int64
	window         time.Duration
	algorithm      algorithm.Algorithm
	keyFunc        KeyFunc
	observer       metrics.Observer
	policyResolver PolicyResolver
}

// Option configures functional parameters for Limiter initialization.
type Option func(*Config)

// WithLimit sets the maximum number of requests allowed within the configured time window.
func WithLimit(limit int64) Option {
	return func(c *Config) {
		c.limit = limit
	}
}

// WithWindow sets the duration of the rate limiting time window.
func WithWindow(window time.Duration) Option {
	return func(c *Config) {
		c.window = window
	}
}

// WithAlgorithm sets the pluggable rate limiting algorithm strategy (e.g., tokenbucket, leakybucket, slidingcounter).
func WithAlgorithm(alg algorithm.Algorithm) Option {
	return func(c *Config) {
		c.algorithm = alg
	}
}

// WithKeyFunc configures a custom key extraction function for extracting rate limit keys from context.
func WithKeyFunc(fn KeyFunc) Option {
	return func(c *Config) {
		if fn != nil {
			c.keyFunc = fn
		}
	}
}

// WithMetricObserver sets a custom telemetry/metrics observer for recording evaluation events.
func WithMetricObserver(obs metrics.Observer) Option {
	return func(c *Config) {
		if obs != nil {
			c.observer = obs
		}
	}
}

// WithPolicyResolver sets a dynamic policy resolver for evaluating tier-based rate limits per key.
func WithPolicyResolver(resolver PolicyResolver) Option {
	return func(c *Config) {
		if resolver != nil {
			c.policyResolver = resolver
		}
	}
}

// Limiter is the central rate limiting coordinator. It combines a storage Driver, rate rules,
// and an algorithm strategy to evaluate incoming requests.
//
// Limiter is thread-safe and designed for concurrent use by multiple goroutines.
type Limiter struct {
	driver         Driver
	policy         atomic.Pointer[Policy]
	algorithm      algorithm.Algorithm
	keyFunc        KeyFunc
	observer       metrics.Observer
	policyResolver PolicyResolver
}

// New creates and initializes a new Limiter instance with the specified storage driver and optional options.
// If WithAlgorithm is omitted, it defaults to the Sliding Window Counter algorithm strategy for backward compatibility.
// Returns an error if driver is nil, or if limit or window are invalid (<= 0).
func New(driver Driver, opts ...Option) (*Limiter, error) {
	if driver == nil {
		return nil, ErrNilDriver
	}

	cfg := &Config{
		limit:  100,             // Default limit: 100 requests
		window: 1 * time.Minute, // Default window: 1 minute
		keyFunc: func(ctx context.Context) string {
			return "global"
		},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	// Validation
	if cfg.limit <= 0 {
		return nil, ErrInvalidLimit
	}
	if cfg.window <= 0 {
		return nil, ErrInvalidWindow
	}

	// Fallback to Sliding Counter if no algorithm was explicitly provided
	if cfg.algorithm == nil {
		cfg.algorithm = slidingcounter.New()
	}

	l := &Limiter{
		driver:         driver,
		algorithm:      cfg.algorithm,
		keyFunc:        cfg.keyFunc,
		observer:       cfg.observer,
		policyResolver: cfg.policyResolver,
	}

	l.policy.Store(&Policy{
		Limit:  cfg.limit,
		Window: cfg.window,
	})

	return l, nil
}

// UpdatePolicy updates the default rate limit policy atomically at runtime without lock contention.
func (l *Limiter) UpdatePolicy(limit int64, window time.Duration) {
	if limit > 0 && window > 0 {
		l.policy.Store(&Policy{
			Limit:  limit,
			Window: window,
		})
	}
}

// Allow evaluates the rate limit key extracted via the configured KeyFunc against the active limits.
func (l *Limiter) Allow(ctx context.Context) (Result, error) {
	key := l.keyFunc(ctx)
	if key == "" {
		key = "global"
	}
	return l.AllowKey(ctx, key)
}

// AllowKey evaluates the rate limit for an explicitly specified key string against the active limits.
func (l *Limiter) AllowKey(ctx context.Context, key string) (Result, error) {
	if key == "" {
		key = "global"
	}

	activePolicy := *l.policy.Load()
	if l.policyResolver != nil {
		if dynamicPolicy, ok := l.policyResolver.ResolvePolicy(ctx, key); ok && dynamicPolicy.Limit > 0 && dynamicPolicy.Window > 0 {
			activePolicy = dynamicPolicy
		}
	}

	var start time.Time
	if l.observer != nil {
		start = time.Now()
	}

	res, err := l.driver.Allow(ctx, key, activePolicy.Limit, activePolicy.Window, l.algorithm)

	if l.observer != nil {
		l.observer.Observe(ctx, metrics.Event{
			Key:       key,
			Route:     metrics.RouteFromContext(ctx),
			Allowed:   res.Allowed,
			Limit:     res.Limit,
			Remaining: res.Remaining,
			ResetIn:   res.ResetIn,
			Driver:    l.driverName(),
			Algorithm: l.algorithm.Name(),
			Duration:  time.Since(start),
		})
	}

	return res, err
}

func (l *Limiter) driverName() string {
	type nameable interface {
		Name() string
	}
	if n, ok := l.driver.(nameable); ok {
		return n.Name()
	}
	return "unknown"
}

// ResetKey clears the rate limit state for the specified key in the underlying storage driver.
func (l *Limiter) ResetKey(ctx context.Context, key string) error {
	if key == "" {
		key = "global"
	}
	return l.driver.Reset(ctx, key)
}

// Close gracefully closes the underlying storage driver and releases associated resources.
func (l *Limiter) Close(ctx context.Context) error {
	return l.driver.Close(ctx)
}
