// Package tokenbucket implements the Token Bucket rate limiting algorithm.
//
// Token Bucket allows bursty traffic up to the bucket capacity while continuously refilling tokens at a constant rate.
package tokenbucket

import (
	"math"
	"time"

	"github.com/balramadan/distlimit/algorithm"
)

// Strategy implements the algorithm.Algorithm interface for Token Bucket.
type Strategy struct{}

// New creates a new instance of the Token Bucket algorithm strategy.
func New() *Strategy {
	return &Strategy{}
}

// Name returns the unique identifier of the algorithm strategy.
func (s *Strategy) Name() string {
	return "token_bucket"
}

// EvaluateMemory evaluates the token bucket rate limit state synchronously in RAM.
// Note: This method expects state modification to be guarded by a mutex from the caller.
func (s *Strategy) EvaluateMemory(now time.Time, state *algorithm.State, limit int64, window time.Duration) algorithm.Result {
	capacity := float64(limit)
	fillRate := capacity / window.Seconds()

	// Inisialisasi token pada request pertama kali
	if state.LastUpdate.IsZero() {
		state.Tokens = capacity
		state.LastUpdate = now
	} else {
		// Hitung penambahan token berdasarkan durasi waktu yang telah berlalu
		elapsed := now.Sub(state.LastUpdate).Seconds()
		if elapsed > 0 {
			state.Tokens = math.Min(capacity, state.Tokens+(elapsed*fillRate))
			state.LastUpdate = now
		}
	}

	var allowed bool
	if state.Tokens >= 1.0 {
		allowed = true
		state.Tokens -= 1.0
	}

	remaining := int64(state.Tokens)

	// Hitung estimasi waktu pemulihan (ResetIn)
	var resetIn time.Duration
	if !allowed {
		// Jika terblokir: durasi sampai minimal 1 token tersedia
		needed := 1.0 - state.Tokens
		resetInSec := needed / fillRate
		resetIn = time.Duration(resetInSec * float64(time.Second))
	} else {
		// Jika diizinkan: durasi sampai kapasitas bucket terisi penuh kembali
		missing := capacity - state.Tokens
		resetInSec := missing / fillRate
		resetIn = time.Duration(resetInSec * float64(time.Second))
	}

	if resetIn < 0 {
		resetIn = 0
	}

	return algorithm.Result{
		Allowed:   allowed,
		Limit:     limit,
		Remaining: remaining,
		ResetIn:   resetIn,
	}
}

// RedisScript returns the high-performance atomic Lua script for Redis distribution.
func (s *Strategy) RedisScript() string {
	return `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])

local fill_rate = limit / window_ms

local data = redis.call('HMGET', key, 'tokens', 'last_updated')
local tokens = tonumber(data[1])
local last_updated = tonumber(data[2])

if not tokens or not last_updated then
    tokens = limit
    last_updated = now_ms
else
    local delta_ms = math.max(0, now_ms - last_updated)
    tokens = math.min(limit, tokens + (delta_ms * fill_rate))
    last_updated = now_ms
end

local allowed = 0
if tokens >= 1.0 then
    allowed = 1
    tokens = tokens - 1.0
end

local reset_ms = 0
if allowed == 0 then
    local needed = 1.0 - tokens
    reset_ms = math.ceil(needed / fill_rate)
else
    local missing = limit - tokens
    reset_ms = math.ceil(missing / fill_rate)
end

redis.call('HMSET', key, 'tokens', tokens, 'last_updated', last_updated)
redis.call('PEXPIRE', key, math.ceil(window_ms * 2))

return { allowed, math.floor(tokens), reset_ms }
`
}
