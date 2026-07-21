// Package memory implements a fast, thread-safe, in-memory rate limiting driver.
// It uses a lock-free token bucket implementation with atomic Compare-And-Swap (CAS) operations
// and a background janitor to prune expired rate limit keys.
package memory

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/balramadan/distlimit"
)

// bucket represents a single rate limit token bucket for a specific key.
type bucket struct {
	tokens       int64
	lastRefillNs int64
}

// Driver implements the distlimit.Driver interface. It manages rate limits in-memory using
// a sync.Map containing atomic token buckets. A background janitor cleans up inactive buckets
// periodically to prevent memory leaks.
//
// Driver is safe for concurrent use by multiple goroutines.
type Driver struct {
	buckets sync.Map
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// Option configures functional parameters for the memory Driver.
type Option func(*Driver)

// New creates and starts a new in-memory rate limiting Driver.
// The cleanupInterval parameter specifies how often the background janitor runs to scan and delete
// stale buckets that haven't been refilled or accessed recently. If cleanupInterval is <= 0,
// it defaults to 1 minute.
func New(cleanupInterval time.Duration, opts ...Option) *Driver {
	if cleanupInterval <= 0 {
		cleanupInterval = 1 * time.Minute
	}

	d := &Driver{
		stopCh: make(chan struct{}),
	}

	for _, opt := range opts {
		opt(d)
	}

	d.wg.Add(1)
	go d.startJanitor(cleanupInterval)

	return d
}

// Allow evaluates the rate limit for a key.
// It retrieves the token bucket corresponding to the key, refines/refills the tokens if the window
// has passed since the last refill, and decrements a token lock-freely using atomic Compare-And-Swap.
func (d *Driver) Allow(ctx context.Context, key string, limit int64, window time.Duration) (distlimit.Result, error) {
	now := time.Now()
	nowNs := now.UnixNano()
	windowNs := window.Nanoseconds()

	// 1. Retrieve or initialize the bucket for the key
	val, loaded := d.buckets.Load(key)
	var b *bucket
	if !loaded {
		newBucket := &bucket{
			tokens:       limit,
			lastRefillNs: nowNs,
		}
		actual, _ := d.buckets.LoadOrStore(key, newBucket)
		b = actual.(*bucket)
	} else {
		b = val.(*bucket)
	}

	// 2. Refill logic: Calculate token additions based on elapsed time
	lastNs := atomic.LoadInt64(&b.lastRefillNs)
	elapsedNs := nowNs - lastNs

	if elapsedNs >= windowNs {
		// Attempt to update the lastRefillNs atomically
		if atomic.CompareAndSwapInt64(&b.lastRefillNs, lastNs, nowNs) {
			// Window passed, refill tokens back to the maximum limit
			atomic.StoreInt64(&b.tokens, limit)
		}
	}

	// 3. Lock-free decrement using CAS loop
	for {
		currTokens := atomic.LoadInt64(&b.tokens)
		if currTokens <= 0 {
			// Limit exceeded: Return 0 remaining and calculate time until reset
			lastRefill := atomic.LoadInt64(&b.lastRefillNs)
			resetInNs := windowNs - (nowNs - lastRefill)
			if resetInNs < 0 {
				resetInNs = 0
			}

			return distlimit.Result{
				Allowed:   false,
				Limit:     limit,
				Remaining: 0,
				ResetIn:   time.Duration(resetInNs),
			}, nil
		}

		// Try decrementing the token count by 1
		if atomic.CompareAndSwapInt64(&b.tokens, currTokens, currTokens-1) {
			lastRefill := atomic.LoadInt64(&b.lastRefillNs)
			resetInNs := windowNs - (nowNs - lastRefill)
			if resetInNs < 0 {
				resetInNs = 0
			}

			return distlimit.Result{
				Allowed:   true,
				Limit:     limit,
				Remaining: currTokens - 1,
				ResetIn:   time.Duration(resetInNs),
			}, nil
		}
	}
}

// Close stops the background janitor goroutine and cleans up resources.
func (d *Driver) Close(ctx context.Context) error {
	select {
	case <-d.stopCh:
		// Already closed
		return nil
	default:
		close(d.stopCh)
		d.wg.Wait() // Wait for the janitor goroutine to exit gracefully
	}
	return nil
}

// startJanitor runs in a background goroutine, periodically scanning the sync.Map
// to delete buckets that have been inactive for more than 2x the cleanup interval.
func (d *Driver) startJanitor(interval time.Duration) {
	defer d.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			nowNs := time.Now().UnixNano()
			// Scan all buckets to clean up stale ones
			d.buckets.Range(func(k, v interface{}) bool {
				b := v.(*bucket)
				lastNs := atomic.LoadInt64(&b.lastRefillNs)
				// Delete from map if there has been no activity for more than 2x cleanup interval
				if nowNs-lastNs > (interval.Nanoseconds() * 2) {
					d.buckets.Delete(k)
				}
				return true
			})
		case <-d.stopCh:
			return
		}
	}
}
