// Package slidinglog implements the Sliding Window Log rate limiting algorithm.
//
// It maintains a log of request timestamps within the active sliding time window.
// This provides 100% exact request counting precision.
package slidinglog

import (
	"time"

	"github.com/balramadan/distlimit/algorithm"
)

// Strategy implements the algorithm.Algorithm interface for Sliding Window Log.
type Strategy struct{}

// New creates a new instance of the Sliding Window Log algorithm strategy.
func New() *Strategy {
	return &Strategy{}
}

// Name returns the unique identifier string of the algorithm strategy.
func (s *Strategy) Name() string {
	return "sliding_log"
}

// EvaluateMemory evaluates the sliding log rate limit state in local RAM.
// It trims expired timestamps and re-allocates memory when slice capacity waste exceeds 50%.
func (s *Strategy) EvaluateMemory(now time.Time, state *algorithm.State, limit int64, window time.Duration) algorithm.Result {
	nowMs := now.UnixMilli()
	cutoffMs := nowMs - window.Milliseconds()

	// 1. Trim stale timestamps outside the sliding window
	validIndex := 0
	for _, ts := range state.Timestamps {
		if ts > cutoffMs {
			break
		}
		validIndex++
	}

	// Re-allocate underlying array if capacity waste exceeds 50% to prevent heap bloat
	if validIndex > 0 {
		newLen := len(state.Timestamps) - validIndex
		if cap(state.Timestamps) > 2*newLen && newLen > 0 {
			newBuf := make([]int64, newLen)
			copy(newBuf, state.Timestamps[validIndex:])
			state.Timestamps = newBuf
		} else {
			state.Timestamps = state.Timestamps[validIndex:]
		}
	}

	currentCount := int64(len(state.Timestamps))
	var allowed bool
	var remaining int64

	if currentCount < limit {
		allowed = true
		state.Timestamps = append(state.Timestamps, nowMs)
		remaining = limit - (currentCount + 1)
	} else {
		allowed = false
		remaining = 0
	}

	var resetIn time.Duration
	if len(state.Timestamps) > 0 {
		oldestMs := state.Timestamps[0]
		resetInMs := (oldestMs + window.Milliseconds()) - nowMs
		if resetInMs < 0 {
			resetInMs = 0
		}
		resetIn = time.Duration(resetInMs) * time.Millisecond
	} else {
		resetIn = window
	}

	return algorithm.Result{
		Allowed:   allowed,
		Limit:     limit,
		Remaining: remaining,
		ResetIn:   resetIn,
	}
}

// RedisScript returns the atomic Redis Lua script utilizing Redis Sorted Sets (ZSET)
// with atomic sequence incrementing to guarantee unique member elements under high concurrency.
func (s *Strategy) RedisScript() string {
	return `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])

-- 1. Hapus timestamp di luar window
local clear_before = now_ms - window_ms
redis.call('ZREMRANGEBYSCORE', key, 0, clear_before)

local count = redis.call('ZCARD', key)
local allowed = 0
local remaining = 0

if count < limit then
    allowed = 1
    
    -- Dapatkan sequence unik atomik untuk mencegah tabrakan member ZSET
    -- Format key: "distlimit:{user1}:seq" (Aman untuk Redis Cluster)
    local seq = redis.call('INCR', key .. ":seq")
    redis.call('PEXPIRE', key .. ":seq", math.ceil(window_ms * 2))
    
    local unique_member = now_ms .. ":" .. seq
    redis.call('ZADD', key, now_ms, unique_member)
    
    count = count + 1
    remaining = limit - count
else
    allowed = 0
    remaining = 0
end

redis.call('PEXPIRE', key, math.ceil(window_ms * 2))

local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
local reset_ms = window_ms
if oldest and #oldest >= 2 then
    local oldest_score = tonumber(oldest[2])
    reset_ms = math.max(0, (oldest_score + window_ms) - now_ms)
end

return { allowed, remaining, reset_ms }
`
}
