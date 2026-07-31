// Package redis implements a distributed rate limiting driver using Redis as the backend.
// It relies on atomic Redis Lua scripts provided by pluggable algorithm strategies
// to enforce rate limits consistently across distributed microservices.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/balramadan/distlimit/algorithm"
	"github.com/redis/go-redis/v9"
)

// Driver implements the distlimit.Driver interface using Redis as the storage backend.
// All rate limit operations execute atomically in Redis via algorithm-specific Lua scripts.
//
// Driver is thread-safe and safe for concurrent use by multiple goroutines.
type Driver struct {
	client redis.UniversalClient
	prefix string
}

// Option configures functional parameters for the Redis Driver.
type Option func(*Driver)

// WithPrefix sets a custom key prefix for all rate limit keys stored in Redis.
// This is useful for namespacing and avoiding key collisions across environments or services.
// The default prefix is "distlimit".
func WithPrefix(prefix string) Option {
	return func(d *Driver) {
		d.prefix = prefix
	}
}

// New creates and initializes a new Redis-backed rate limiting Driver.
func New(client redis.UniversalClient, opts ...Option) *Driver {
	d := &Driver{
		client: client,
		prefix: "distlimit",
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Name returns the unique identifier string of the Redis storage driver.
func (d *Driver) Name() string {
	return "redis"
}

// Allow evaluates the rate limit for a key using the Lua script provided by the algorithm strategy.
// It sends evaluation parameters (limit, window, current timestamp) to Redis and parses the response.
func (d *Driver) Allow(ctx context.Context, key string, limit int64, window time.Duration, alg algorithm.Algorithm) (algorithm.Result, error) {
	redisKey := fmt.Sprintf("%s:{%s}", d.prefix, key)

	nowMs := time.Now().UnixMilli()
	windowMs := window.Milliseconds()
	if windowMs < 1 {
		windowMs = 1
	}

	// Retrieve the algorithm-specific atomic Lua script
	script := alg.RedisScript()

	// Execute the Lua script in Redis atomically
	evalRes, err := d.client.Eval(ctx, script, []string{redisKey}, limit, windowMs, nowMs).Result()
	if err != nil {
		return algorithm.Result{}, fmt.Errorf("distlimit redis: %w", err)
	}

	// Parse the array response returned by the Lua script
	vals, ok := evalRes.([]interface{})
	if !ok || len(vals) < 3 {
		return algorithm.Result{}, fmt.Errorf("distlimit redis: invalid script response format")
	}

	allowedVal, ok1 := toInt64(vals[0])
	remainingVal, ok2 := toInt64(vals[1])
	resetMsVal, ok3 := toInt64(vals[2])

	if !ok1 || !ok2 || !ok3 {
		return algorithm.Result{}, fmt.Errorf("distlimit redis: failed to parse script response values")
	}

	return algorithm.Result{
		Allowed:   allowedVal == 1,
		Limit:     limit,
		Remaining: remainingVal,
		ResetIn:   time.Duration(resetMsVal) * time.Millisecond,
	}, nil
}

// toInt64 safely converts dynamic numerical types returned by Redis response drivers to int64.
func toInt64(val interface{}) (int64, bool) {
	switch v := val.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

// Reset deletes the rate limit key (and its sequence key if applicable) from Redis.
func (d *Driver) Reset(ctx context.Context, key string) error {
	redisKey := fmt.Sprintf("%s:{%s}", d.prefix, key)
	seqKey := fmt.Sprintf("%s:seq", redisKey)
	return d.client.Del(ctx, redisKey, seqKey).Err()
}

// Close closes the underlying Redis client connection pool.
func (d *Driver) Close(ctx context.Context) error {
	return d.client.Close()
}
