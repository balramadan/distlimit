// Package hybrid implements a two-tier resilience rate limiting driver.
// It combines a primary distributed driver (e.g., Redis) with a local fallback driver
// (e.g., In-Memory) to provide failover capabilities when the primary driver is unavailable.
package hybrid

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/balramadan/distlimit"
)

// Driver implements the distlimit.Driver interface, orchestrating a dual-tier rate limiting strategy.
// It coordinates a primary driver and a fallback driver, utilizing a fail-open circuit breaker pattern.
// If the primary driver experiences failures, the driver shifts requests to the fallback driver for
// a configured cool-off period.
//
// Driver is safe for concurrent use by multiple goroutines.
type Driver struct {
	primary         distlimit.Driver
	fallback        distlimit.Driver
	onError         func(err error)
	coolOffDuration time.Duration

	isPrimaryDown int32 // 0 = Healthy, 1 = Down
	lastFailedNs  int64 // Timestamp in nanoseconds when the primary driver last failed
	fallbackCount int64 // Total number of times fallback execution has been triggered
}

// Option configures functional parameters for the hybrid Driver.
type Option func(*Driver)

// WithOnError registers a callback function that is invoked whenever the primary driver encounters an error.
// This is typically used for external logging, alerting, or telemetry/metrics collection.
func WithOnError(fn func(err error)) Option {
	return func(d *Driver) {
		d.onError = fn
	}
}

// WithCoolOffDuration sets the duration that the Driver stays in Fallback mode
// before attempting to reconnect/retry the primary driver.
// The default duration is 5 seconds.
func WithCoolOffDuration(duration time.Duration) Option {
	return func(d *Driver) {
		d.coolOffDuration = duration
	}
}

// New creates and initializes a new hybrid Driver instance with a primary driver,
// a fallback driver, and optional configurations.
func New(primary distlimit.Driver, fallback distlimit.Driver, opts ...Option) *Driver {
	d := &Driver{
		primary:         primary,
		fallback:        fallback,
		coolOffDuration: 5 * time.Second, // Default cool-off duration of 5 seconds
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Allow evaluates the rate limit key against the configured rules.
// It first checks if the primary driver is in a cool-off state. If so, it immediately executes
// the fallback driver to avoid hanging on a failing database.
// If the primary driver is healthy, it attempts to evaluate using it. Any error from the primary
// driver triggers the fallback mechanism, registers the failure timestamp, and transitions the
// driver state to unhealthy.
func (d *Driver) Allow(ctx context.Context, key string, limit int64, window time.Duration) (distlimit.Result, error) {
	nowNs := time.Now().UnixNano()

	// 1. Check if the primary driver is currently in the "Cool-Off" state
	if atomic.LoadInt32(&d.isPrimaryDown) == 1 {
		lastFail := atomic.LoadInt64(&d.lastFailedNs)
		if nowNs-lastFail < d.coolOffDuration.Nanoseconds() {
			// Cool-off active: Route immediately to the fallback driver (In-Memory)
			return d.executeFallback(ctx, key, limit, window, nil)
		}
		// Cool-off expired: Reset health status and retry primary driver
		atomic.StoreInt32(&d.isPrimaryDown, 0)
	}

	// 2. Attempt rate limiting evaluation on the primary driver
	res, err := d.primary.Allow(ctx, key, limit, window)
	if err == nil {
		return res, nil
	}

	// 3. Primary driver failed: Transition to unhealthy and record timestamps
	atomic.StoreInt32(&d.isPrimaryDown, 1)
	atomic.StoreInt64(&d.lastFailedNs, nowNs)

	// Execute custom error handler if registered
	if d.onError != nil {
		d.onError(err)
	}

	// 4. Gracefully fall back to the In-Memory driver
	return d.executeFallback(ctx, key, limit, window, err)
}

// executeFallback executes the fallback driver and increments the fallback counter.
func (d *Driver) executeFallback(ctx context.Context, key string, limit int64, window time.Duration, primaryErr error) (distlimit.Result, error) {
	atomic.AddInt64(&d.fallbackCount, 1)

	res, err := d.fallback.Allow(ctx, key, limit, window)
	if err != nil {
		// Fallback driver also failed, propagate error to the caller
		return distlimit.Result{}, err
	}

	return res, nil
}

// FallbackCount returns the total number of times the fallback driver has been triggered.
func (d *Driver) FallbackCount() int64 {
	return atomic.LoadInt64(&d.fallbackCount)
}

// IsPrimaryHealthy returns true if the primary driver is currently healthy and active.
func (d *Driver) IsPrimaryHealthy() bool {
	return atomic.LoadInt32(&d.isPrimaryDown) == 0
}

// Close gracefully terminates both the primary and fallback drivers.
func (d *Driver) Close(ctx context.Context) error {
	_ = d.primary.Close(ctx)
	_ = d.fallback.Close(ctx)
	return nil
}
