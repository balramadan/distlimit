// Package fixedwindow implements the Fixed Window rate limiting algorithm.
//
// Time is partitioned into discrete fixed windows (e.g., every 1 minute).
// A counter is incremented for each incoming request and resets to zero at the boundary of a new window.
package fixedwindow

import (
	"time"

	"github.com/balramadan/distlimit/algorithm"
)

// Strategy implements the algorithm.Algorithm interface for Fixed Window.
type Strategy struct{}

// New creates a new instance of the Fixed Window algorithm strategy.
func New() *Strategy {
	return &Strategy{}
}

// Name returns the unique identifier of the algorithm strategy.
func (s *Strategy) Name() string {
	return "fixed_window"
}

// EvaluateMemory evaluates the fixed window rate limit state synchronously in RAM.
func (s *Strategy) EvaluateMemory(now time.Time, state *algorithm.State, limit int64, window time.Duration) algorithm.Result {
	if window <= 0 {
		window = 1 * time.Second
	}

	// Tentukan batas waktu awal interval window saat ini
	currentWindowBoundary := now.Truncate(window)

	// Jika berpindah ke window baru (atau request pertama), reset counter ke 0
	if state.LastUpdate.IsZero() || !state.LastUpdate.Equal(currentWindowBoundary) {
		state.Count = 0
		state.LastUpdate = currentWindowBoundary
	}

	state.Count++

	allowed := state.Count <= limit
	remaining := limit - state.Count
	if remaining < 0 {
		remaining = 0
	}

	// Hitung sisa waktu hingga window saat ini berakhir
	nextWindowBoundary := currentWindowBoundary.Add(window)
	resetIn := nextWindowBoundary.Sub(now)
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

// RedisScript returns the high-performance atomic Lua script utilizing Redis INCR.
func (s *Strategy) RedisScript() string {
	return `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])

local current_window = math.floor(now_ms / window_ms)

-- KEYS[1] berformat "distlimit:{user1}"
-- Maka window_key menjadi "distlimit:{user1}:292182"
-- Keduanya memiliki Hash Tag {user1}, sehingga 100% aman di Redis Cluster!
local window_key = key .. ":" .. current_window

local count = redis.call('INCR', window_key)
if count == 1 then
    redis.call('PEXPIRE', window_key, window_ms)
end

local allowed = 0
local remaining = 0

if count <= limit then
    allowed = 1
    remaining = limit - count
else
    allowed = 0
    remaining = 0
end

local reset_ms = ((current_window + 1) * window_ms) - now_ms
if reset_ms < 0 then
    reset_ms = 0
end

return { allowed, remaining, reset_ms }
`
}
