// Package algorithm defines core interfaces and shared data structures for rate limiting algorithm strategies.
package algorithm

import "time"

// Result represents the outcome of a rate limit evaluation.
type Result struct {
	// Allowed indicates whether the current request is permitted under the rate limit quota.
	Allowed bool
	// Limit is the maximum request capacity configured for the time window.
	Limit int64
	// Remaining is the remaining request quota available in the active window.
	Remaining int64
	// ResetIn specifies the duration until the rate limit quota resets or recovers.
	ResetIn time.Duration
}

// State stores transient state for a rate limit key evaluated in memory.
type State struct {
	Count      int64
	PrevCount  int64
	LastUpdate time.Time
	Tokens     float64
	Timestamps []int64
}

// Algorithm defines the interface for pluggable rate limiting algorithms.
// Algorithms provide both a local in-memory evaluation implementation and an atomic Redis Lua script.
type Algorithm interface {
	// Name returns the unique identifier string of the algorithm strategy.
	Name() string

	// EvaluateMemory evaluates the rate limit state in local RAM.
	EvaluateMemory(now time.Time, state *State, limit int64, window time.Duration) Result

	// RedisScript returns the atomic Lua script for Redis distribution.
	RedisScript() string
}
