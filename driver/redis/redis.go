// Package redis implements a distributed rate limiting driver using Redis as the backend.
// It relies on Redis sorted sets and Lua scripting to implement a precise, sliding-window
// rate limiter across distributed services.
package redis

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/redis/go-redis/v9"
)

// Driver implements the distlimit.Driver interface. It communicates with a Redis backend
// using a UniversalClient (supporting standalone, cluster, or sentinel Redis deployments).
// All rate limit operations are executed atomically in Redis via a sliding-window Lua script.
//
// Driver is safe for concurrent use by multiple goroutines.
type Driver struct {
	client redis.UniversalClient
	prefix string
}

// Option configures functional parameters for the redis Driver.
type Option func(*Driver)

// WithPrefix sets a custom key prefix for all rate limit keys stored in Redis.
// This is useful for namespacing and avoiding key collisions.
// The default prefix is "distlimit:".
func WithPrefix(prefix string) Option {
	return func(d *Driver) {
		d.prefix = prefix
	}
}

// New creates and initializes a new Redis-backed rate limiting Driver.
func New(client redis.UniversalClient, opts ...Option) *Driver {
	d := &Driver{
		client: client,
		prefix: "distlimit:",
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Allow evaluates the rate limit for a key using a sliding window algorithm.
// It executes a Redis Lua script to clean up expired timestamps, check the request limit against
// the current window volume, add the current request timestamp if allowed, and return the result.
func (d *Driver) Allow(ctx context.Context, key string, limit int64, window time.Duration) (distlimit.Result, error) {
	fullKey := d.prefix + key
	now := time.Now()
	nowNs := now.UnixNano()
	windowNs := window.Nanoseconds()

	memberID := fmt.Sprintf("%d-%d", nowNs, now.Nanosecond())

	ttlSec := int64(math.Ceil(window.Seconds())) * 2
	if ttlSec < 1 {
		ttlSec = 1
	}

	keys := []string{fullKey}
	args := []interface{}{
		strconv.FormatInt(nowNs, 10),
		strconv.FormatInt(windowNs, 10),
		strconv.FormatInt(limit, 10),
		memberID,
		strconv.FormatInt(ttlSec, 10),
	}

	rawRes, err := slidingScript.Run(ctx, d.client, keys, args...).Result()
	if err != nil {
		return distlimit.Result{}, fmt.Errorf("distlimit/redis: %w", err)
	}

	resArray, ok := rawRes.([]interface{})
	if !ok || len(resArray) < 3 {
		return distlimit.Result{}, fmt.Errorf("distlimit/redis: Invalid lua response format")
	}

	allowed := resArray[0].(int64) == 1
	remaining := resArray[1].(int64)
	resetInNs := resArray[2].(int64)

	return distlimit.Result{
		Allowed:   allowed,
		Limit:     limit,
		Remaining: remaining,
		ResetIn:   time.Duration(resetInNs),
	}, nil
}

// Close implements the distlimit.Driver interface.
// For Redis, the client lifecycle is typically managed outside of this driver,
// so this method is a no-op that returns nil.
func (d *Driver) Close(ctx context.Context) error {
	return nil
} 
