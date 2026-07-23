// Package hybrid implements a dual-tier resilient rate limiting driver with a Half-Open Circuit Breaker.
// It orchestrates a primary distributed storage driver (e.g., Redis) and a local fallback storage driver
// (e.g., In-Memory) to provide high availability during primary infrastructure outages.
package hybrid

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm"
)

// Driver implements the distlimit.Driver interface, coordinating a primary driver and a fallback driver.
// It uses an atomic Half-Open Circuit Breaker pattern to prevent thundering herd problems on Redis recovery.
//
// Driver is thread-safe and safe for concurrent use by multiple goroutines.
type Driver struct {
	primary       distlimit.Driver
	fallback      distlimit.Driver
	coolOff       time.Duration
	onError       func(err error)
	isDown        atomic.Bool
	lastFailUnix  atomic.Int64
	probingActive atomic.Bool
	fallbackCount atomic.Uint64
}

// Option configures functional parameters for the hybrid Driver.
type Option func(*Driver)

// WithCoolOffDuration sets the cool-off recovery duration before attempting to probe the primary driver again.
// The default duration is 10 seconds.
func WithCoolOffDuration(d time.Duration) Option {
	return func(h *Driver) {
		h.coolOff = d
	}
}

// WithOnError registers an error callback function invoked whenever the primary driver encounters an error.
func WithOnError(fn func(err error)) Option {
	return func(h *Driver) {
		h.onError = fn
	}
}

// New creates and initializes a new hybrid Driver with a primary driver, fallback driver, and optional configurations.
func New(primary, fallback distlimit.Driver, opts ...Option) *Driver {
	h := &Driver{
		primary:  primary,
		fallback: fallback,
		coolOff:  10 * time.Second,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// Allow evaluates the rate limit key against the primary driver if healthy.
// If the primary driver is down, it uses a Half-Open Circuit Breaker to allow a single probing request
// to test primary health after the cool-off duration, routing all other concurrent traffic to fallback.
func (h *Driver) Allow(ctx context.Context, key string, limit int64, window time.Duration, alg algorithm.Algorithm) (algorithm.Result, error) {
	isDown := h.isDown.Load()

	if isDown {
		lastFail := time.Unix(0, h.lastFailUnix.Load())

		// Check if cool-off period has expired
		if time.Since(lastFail) >= h.coolOff {
			// HALF-OPEN CIRCUIT BREAKER: Atomic CAS ensures only 1 probing request attempts the primary driver
			if h.probingActive.CompareAndSwap(false, true) {
				res, err := h.primary.Allow(ctx, key, limit, window, alg)
				if err == nil {
					// Probing succeeded: Reset status to closed (healthy)
					h.isDown.Store(false)
					h.probingActive.Store(false)
					return res, nil
				}

				// Probing failed: Reset cool-off timer and record error
				h.lastFailUnix.Store(time.Now().UnixNano())
				h.probingActive.Store(false)
				if h.onError != nil {
					h.onError(err)
				}
			}
		}

		// Fallback execution for concurrent traffic during cool-off or failed probing
		h.fallbackCount.Add(1)
		return h.fallback.Allow(ctx, key, limit, window, alg)
	}

	// Normal Closed state: Evaluate using primary driver
	res, err := h.primary.Allow(ctx, key, limit, window, alg)
	if err != nil {
		// Primary failed: Transition circuit breaker to Open state atomically
		if h.isDown.CompareAndSwap(false, true) {
			h.lastFailUnix.Store(time.Now().UnixNano())
			if h.onError != nil {
				h.onError(err)
			}
		}
		h.fallbackCount.Add(1)

		return h.fallback.Allow(ctx, key, limit, window, alg)
	}

	return res, nil
}

// IsPrimaryDown returns true if the primary driver is currently marked as down.
func (h *Driver) IsPrimaryDown() bool {
	return h.isDown.Load()
}

// FallbackCount returns the total number of times execution was delegated to the fallback driver.
func (h *Driver) FallbackCount() uint64 {
	return h.fallbackCount.Load()
}

// Reset clears the rate limit state entry for the specified key from both fallback and primary drivers.
func (h *Driver) Reset(ctx context.Context, key string) error {
	_ = h.fallback.Reset(ctx, key)
	if !h.isDown.Load() {
		return h.primary.Reset(ctx, key)
	}
	return nil
}

// Close gracefully closes both the primary and fallback storage drivers.
func (h *Driver) Close(ctx context.Context) error {
	_ = h.primary.Close(ctx)
	return h.fallback.Close(ctx)
}
