// Package slidingcounter implements the Sliding Window Counter rate limiting algorithm.
//
// It calculates an estimated request volume using a weighted moving average of the current
// and previous window counters, providing high precision with strictly O(1) memory footprint.
package slidingcounter

import (
	"math"
	"time"

	"github.com/balramadan/distlimit/algorithm"
)

// Strategy implements the algorithm.Algorithm interface for Sliding Window Counter.
type Strategy struct{}

// New creates a new instance of the Sliding Window Counter algorithm strategy.
func New() *Strategy {
	return &Strategy{}
}

// Name returns the unique identifier of the algorithm strategy.
func (s *Strategy) Name() string {
	return "sliding_counter"
}

// EvaluateMemory evaluates the sliding window counter rate limit state synchronously in RAM.
func (s *Strategy) EvaluateMemory(now time.Time, state *algorithm.State, limit int64, window time.Duration) algorithm.Result {
	if window <= 0 {
		window = 1 * time.Second
	}

	currentWindowBoundary := now.Truncate(window)

	// Inisialisasi atau rotasi window
	if state.LastUpdate.IsZero() {
		state.LastUpdate = currentWindowBoundary
		state.Count = 0
		state.PrevCount = 0
	} else {
		timeDiff := now.Sub(state.LastUpdate)
		if timeDiff >= 2*window {
			// Lebih dari 2 window terlewati, reset total
			state.LastUpdate = currentWindowBoundary
			state.Count = 0
			state.PrevCount = 0
		} else if timeDiff >= window {
			// 1 window terlewati, rotasi count sekarang menjadi prevCount
			state.PrevCount = state.Count
			state.Count = 0
			state.LastUpdate = currentWindowBoundary
		}
	}

	// Hitung bobot pergeseran window saat ini (0.0 s.d. 1.0)
	elapsed := now.Sub(state.LastUpdate).Seconds()
	windowSec := window.Seconds()
	weight := (windowSec - elapsed) / windowSec
	if weight < 0 {
		weight = 0
	}

	// Estimasi total request = (PrevCount * bobot sisa) + Count saat ini
	estimatedPrevCount := float64(state.PrevCount) * weight
	estimatedCount := int64(math.Floor(estimatedPrevCount)) + state.Count

	var allowed bool
	var remaining int64

	if estimatedCount+1 <= limit {
		allowed = true
		state.Count++
		estimatedCount++
		remaining = limit - estimatedCount
	} else {
		allowed = false
		remaining = limit - estimatedCount
		if remaining < 0 {
			remaining = 0
		}
	}

	// Sisa waktu hingga window saat ini berakhir
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

// RedisScript returns the high-performance atomic Lua script for Redis execution.
func (s *Strategy) RedisScript() string {
	return `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])

local current_window = math.floor(now_ms / window_ms)

local data = redis.call('HMGET', key, 'window', 'count', 'prev_count')
local win = tonumber(data[1]) or 0
local count = tonumber(data[2]) or 0
local prev_count = tonumber(data[3]) or 0

if win == 0 then
    win = current_window
    count = 0
    prev_count = 0
elseif current_window > win then
    if current_window == win + 1 then
        prev_count = count
    else
        prev_count = 0
    end
    count = 0
    win = current_window
end

local elapsed_ms = now_ms - (win * window_ms)
local weight = (window_ms - elapsed_ms) / window_ms
if weight < 0 then weight = 0 end

local est_count = math.floor(prev_count * weight) + count

local allowed = 0
local remaining = 0

if est_count + 1 <= limit then
    allowed = 1
    count = count + 1
    est_count = est_count + 1
    remaining = limit - est_count
else
    allowed = 0
    remaining = math.max(0, limit - est_count)
end

redis.call('HMSET', key, 'window', win, 'count', count, 'prev_count', prev_count)
redis.call('PEXPIRE', key, math.ceil(window_ms * 2))

local reset_ms = ((win + 1) * window_ms) - now_ms
if reset_ms < 0 then reset_ms = 0 end

return { allowed, remaining, reset_ms }
`
}
