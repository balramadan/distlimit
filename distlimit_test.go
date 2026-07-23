package distlimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm"
	"github.com/balramadan/distlimit/driver/memory"
)

func TestLimiter_WithNilKeyFunc_NoPanic(t *testing.T) {
	memDriver := memory.New(1 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	limiter, err := distlimit.New(memDriver, distlimit.WithKeyFunc(nil))
	if err != nil {
		t.Fatalf("expected no error creating limiter, got: %v", err)
	}

	ctx := context.Background()
	res, err := limiter.Allow(ctx)
	if err != nil {
		t.Fatalf("expected no error on Allow with nil WithKeyFunc, got: %v", err)
	}
	if !res.Allowed {
		t.Fatalf("expected request to be allowed")
	}
}

func TestLimiter_EmptyKey_FallsBackToGlobal(t *testing.T) {
	memDriver := memory.New(1 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	limiter, err := distlimit.New(memDriver, distlimit.WithKeyFunc(func(ctx context.Context) string {
		return ""
	}))
	if err != nil {
		t.Fatalf("expected no error creating limiter, got: %v", err)
	}

	ctx := context.Background()
	res, err := limiter.Allow(ctx)
	if err != nil {
		t.Fatalf("expected no error on Allow with empty key, got: %v", err)
	}
	if !res.Allowed {
		t.Fatalf("expected request to be allowed")
	}
}

func TestLimiter_ResetKey(t *testing.T) {
	memDriver := memory.New(1 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithLimit(1),
		distlimit.WithWindow(1*time.Minute),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	// 1st request - allowed
	res, err := limiter.AllowKey(ctx, "user1")
	if err != nil || !res.Allowed {
		t.Fatalf("expected 1st request to be allowed, got err=%v, allowed=%v", err, res.Allowed)
	}

	// 2nd request - blocked
	res, err = limiter.AllowKey(ctx, "user1")
	if err != nil || res.Allowed {
		t.Fatalf("expected 2nd request to be blocked, got err=%v, allowed=%v", err, res.Allowed)
	}

	// Reset key
	if err := limiter.ResetKey(ctx, "user1"); err != nil {
		t.Fatalf("failed to reset key: %v", err)
	}

	// 3rd request after reset - allowed again
	res, err = limiter.AllowKey(ctx, "user1")
	if err != nil || !res.Allowed {
		t.Fatalf("expected request after ResetKey to be allowed, got err=%v, allowed=%v", err, res.Allowed)
	}
}

type dummyAlgorithm struct{}

func (d *dummyAlgorithm) Name() string { return "dummy" }
func (d *dummyAlgorithm) EvaluateMemory(now time.Time, state *algorithm.State, limit int64, window time.Duration) algorithm.Result {
	return algorithm.Result{Allowed: true, Limit: limit, Remaining: limit - 1, ResetIn: window}
}
func (d *dummyAlgorithm) RedisScript() string { return "" }
