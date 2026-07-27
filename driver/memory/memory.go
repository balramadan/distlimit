// Package memory implements a fast, thread-safe, sharded in-memory rate limiting driver.
// It supports pluggable algorithm strategies with an automatic background janitor
// to prune expired rate limit keys per shard and prevent memory leaks.
package memory

import (
	"context"
	"hash/fnv"
	"sync"
	"time"

	"github.com/balramadan/distlimit/algorithm"
)

const shardCount = 64

type entry struct {
	state      algorithm.State
	lastAccess time.Time
}

type memoryShard struct {
	mu    sync.RWMutex
	store map[string]*entry
}

// Driver implements the distlimit.Driver interface using a 64-sharded lock architecture
// to minimize lock contention and prevent latency spikes during background cleanup.
//
// Driver is thread-safe and safe for concurrent use by multiple goroutines.
type Driver struct {
	shards      [shardCount]*memoryShard
	ttl         time.Duration
	cleanTicker *time.Ticker
	done        chan struct{}
	closeOnce   sync.Once
}

// New creates and starts a new sharded in-memory rate limiting Driver.
// The ttl parameter specifies how long inactive keys remain in memory before
// being pruned by the background janitor. Defaults to 5 minutes if ttl <= 0.
func New(ttl time.Duration) *Driver {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	d := &Driver{
		ttl:         ttl,
		cleanTicker: time.NewTicker(ttl),
		done:        make(chan struct{}),
	}

	for i := 0; i < shardCount; i++ {
		d.shards[i] = &memoryShard{
			store: make(map[string]*entry),
		}
	}

	go d.startCleanup()
	return d
}

// Name returns the unique identifier string of the memory storage driver.
func (d *Driver) Name() string {
	return "memory"
}

// getShard determines which memory shard manages the given key using FNV-1a hashing.
func (d *Driver) getShard(key string) *memoryShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	idx := h.Sum32() % shardCount
	return d.shards[idx]
}

// Allow evaluates the rate limit for a key on its isolated shard without acquiring a global memory lock.
// It respects context cancellation and returns an error if ctx is cancelled.
func (d *Driver) Allow(ctx context.Context, key string, limit int64, window time.Duration, alg algorithm.Algorithm) (algorithm.Result, error) {
	select {
	case <-ctx.Done():
		return algorithm.Result{}, ctx.Err()
	default:
	}

	shard := d.getShard(key)
	now := time.Now()

	shard.mu.Lock()
	e, exists := shard.store[key]
	if !exists {
		e = &entry{
			state:      algorithm.State{},
			lastAccess: now,
		}
		shard.store[key] = e
	} else {
		e.lastAccess = now
	}

	res := alg.EvaluateMemory(now, &e.state, limit, window)
	shard.mu.Unlock()

	return res, nil
}

// startCleanup runs a background ticker loop to periodically evict expired keys across shards.
func (d *Driver) startCleanup() {
	for {
		select {
		case <-d.cleanTicker.C:
			d.cleanup()
		case <-d.done:
			return
		}
	}
}

// cleanup evicts stale entries from each memory shard individually to prevent global lock contention.
func (d *Driver) cleanup() {
	now := time.Now()
	for i := 0; i < shardCount; i++ {
		shard := d.shards[i]
		shard.mu.Lock()
		for k, e := range shard.store {
			if now.Sub(e.lastAccess) > d.ttl {
				delete(shard.store, k)
			}
		}
		shard.mu.Unlock()
	}
}

// Reset clears the rate limit state entry for the specified key from its memory shard.
func (d *Driver) Reset(ctx context.Context, key string) error {
	shard := d.getShard(key)
	shard.mu.Lock()
	delete(shard.store, key)
	shard.mu.Unlock()
	return nil
}

// Close gracefully stops the background cleanup worker and releases associated resources.
func (d *Driver) Close(ctx context.Context) error {
	d.closeOnce.Do(func() {
		d.cleanTicker.Stop()
		close(d.done)
	})
	return nil
}
