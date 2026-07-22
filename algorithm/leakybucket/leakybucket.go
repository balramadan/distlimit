// Package leakybucket implements the Leaky Bucket rate limiting algorithm for traffic shaping.
//
// Unlike Token Bucket which permits sudden request bursts, Leaky Bucket enforces a smooth,
// constant outflow rate. Requests that exceed the bucket capacity are immediately rejected.
package leakybucket

import (
	"math"
	"time"

	"github.com/balramadan/distlimit/algorithm"
)

// Strategy implements the algorithm.Algorithm interface for Leaky Bucket.
type Strategy struct{}

// New creates a new instance of the Leaky Bucket algorithm strategy.
func New() *Strategy {
	return &Strategy{}
}

// Name returns the unique identifier of the algorithm strategy.
func (s *Strategy) Name() string {
	return "leaky_bucket"
}

// EvaluateMemory evaluates the leaky bucket rate limit state synchronously in RAM.
// state.Tokens digunakan untuk merepresentasikan level "air" (water level) saat ini.
func (s *Strategy) EvaluateMemory(now time.Time, state *algorithm.State, limit int64, window time.Duration) algorithm.Result {
	capacity := float64(limit)
	leakRate := capacity / window.Seconds() // Jumlah request yang "bocor/keluar" per detik

	if state.LastUpdate.IsZero() {
		state.Tokens = 0.0 // Ember awalnya kosong
		state.LastUpdate = now
	} else {
		// Hitung seberapa banyak "air" yang sudah bocor/keluar sejak request terakhir
		elapsed := now.Sub(state.LastUpdate).Seconds()
		if elapsed > 0 {
			state.Tokens = math.Max(0.0, state.Tokens-(elapsed*leakRate))
			state.LastUpdate = now
		}
	}

	var allowed bool
	// Cek apakah menambah 1 request baru akan membuat ember meluap (overflow)
	if state.Tokens+1.0 <= capacity {
		allowed = true
		state.Tokens += 1.0 // Tambahkan 1 unit air ke dalam ember
	} else {
		allowed = false
	}

	remaining := int64(math.Max(0.0, capacity-state.Tokens))

	// Hitung estimasi waktu pemulihan (ResetIn)
	var resetIn time.Duration
	if !allowed {
		// Jika terblokir: durasi hingga ada ruang cukup untuk 1 request baru
		excess := (state.Tokens + 1.0) - capacity
		resetInSec := excess / leakRate
		resetIn = time.Duration(resetInSec * float64(time.Second))
	} else {
		// Jika diizinkan: durasi hingga ember benar-benar kosong kembali
		resetInSec := state.Tokens / leakRate
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

// RedisScript returns the atomic Lua script for Redis execution.
func (s *Strategy) RedisScript() string {
	return `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])

local leak_rate = limit / window_ms

local data = redis.call('HMGET', key, 'water', 'last_updated')
local water = tonumber(data[1])
local last_updated = tonumber(data[2])

if not water or not last_updated then
    water = 0.0
    last_updated = now_ms
else
    local delta_ms = math.max(0, now_ms - last_updated)
    water = math.max(0.0, water - (delta_ms * leak_rate))
    last_updated = now_ms
end

local allowed = 0
if water + 1.0 <= limit then
    allowed = 1
    water = water + 1.0
end

local remaining = math.max(0, math.floor(limit - water))

local reset_ms = 0
if allowed == 0 then
    local excess = (water + 1.0) - limit
    reset_ms = math.ceil(excess / leak_rate)
else
    reset_ms = math.ceil(water / leak_rate)
end

redis.call('HMSET', key, 'water', water, 'last_updated', last_updated)
redis.call('PEXPIRE', key, math.ceil(window_ms * 2))

return { allowed, remaining, reset_ms }
`
}
