package distlimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/driver/memory"
	"github.com/balramadan/distlimit/metrics"
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

type mockObserver struct {
	events []metrics.Event
}

func (m *mockObserver) Observe(ctx context.Context, event metrics.Event) {
	m.events = append(m.events, event)
}

func TestLimiter_WithMetricObserver(t *testing.T) {
	memDriver := memory.New(1 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	obs := &mockObserver{}
	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithLimit(5),
		distlimit.WithWindow(1*time.Minute),
		distlimit.WithMetricObserver(obs),
	)
	if err != nil {
		t.Fatalf("failed to create limiter with observer: %v", err)
	}

	ctx := context.Background()
	_, err = limiter.AllowKey(ctx, "user123")
	if err != nil {
		t.Fatalf("AllowKey error: %v", err)
	}

	if len(obs.events) != 1 {
		t.Fatalf("expected 1 event recorded, got %d", len(obs.events))
	}

	evt := obs.events[0]
	if evt.Key != "user123" {
		t.Errorf("expected event key 'user123', got '%s'", evt.Key)
	}
	if !evt.Allowed {
		t.Errorf("expected event allowed=true")
	}
	if evt.Driver != "memory" {
		t.Errorf("expected event driver 'memory', got '%s'", evt.Driver)
	}
}

func BenchmarkLimiter_NoObserver(b *testing.B) {
	memDriver := memory.New(1 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	limiter, _ := distlimit.New(memDriver)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = limiter.AllowKey(ctx, "bench_key")
	}
}

func TestLimiter_UpdatePolicy(t *testing.T) {
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
	res, _ := limiter.AllowKey(ctx, "dynamic_key")
	if !res.Allowed {
		t.Fatalf("expected initial request allowed")
	}

	res, _ = limiter.AllowKey(ctx, "dynamic_key")
	if res.Allowed {
		t.Fatalf("expected 2nd request blocked under limit 1")
	}

	// Runtime update policy to limit 5
	limiter.UpdatePolicy(5, 1*time.Minute)

	res, _ = limiter.AllowKey(ctx, "dynamic_key")
	if !res.Allowed {
		t.Fatalf("expected request allowed after UpdatePolicy(5)")
	}
}

type customResolver struct{}

func (c *customResolver) ResolvePolicy(ctx context.Context, key string) (distlimit.Policy, bool) {
	if key == "vip_user" {
		return distlimit.Policy{Limit: 1000, Window: 1 * time.Minute}, true
	}
	return distlimit.Policy{}, false
}

func TestLimiter_PolicyResolver(t *testing.T) {
	memDriver := memory.New(1 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithLimit(1),
		distlimit.WithWindow(1*time.Minute),
		distlimit.WithPolicyResolver(&customResolver{}),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Normal user (limit 1)
	res, _ := limiter.AllowKey(ctx, "normal_user")
	if !res.Allowed {
		t.Fatalf("expected normal_user 1st request allowed")
	}
	res, _ = limiter.AllowKey(ctx, "normal_user")
	if res.Allowed {
		t.Fatalf("expected normal_user 2nd request blocked")
	}

	// VIP user (limit 1000)
	for i := 0; i < 10; i++ {
		res, _ = limiter.AllowKey(ctx, "vip_user")
		if !res.Allowed {
			t.Fatalf("expected vip_user request %d allowed", i+1)
		}
	}
}

func TestLimiter_ConcurrentUpdatePolicy(t *testing.T) {
	memDriver := memory.New(1 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	limiter, _ := distlimit.New(memDriver, distlimit.WithLimit(100))
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		for i := 1; i <= 100; i++ {
			limiter.UpdatePolicy(int64(i*10), 1*time.Minute)
			time.Sleep(1 * time.Millisecond)
		}
		close(done)
	}()

	for i := 0; i < 10; i++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					_, _ = limiter.AllowKey(ctx, "concurrent_key")
				}
			}
		}()
	}

	<-done
}
