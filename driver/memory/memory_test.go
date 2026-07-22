package memory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/balramadan/distlimit/algorithm"
	"github.com/balramadan/distlimit/driver/memory"
)

// mockAlgorithm adalah algoritma sederhana untuk kebutuhan pengujian driver
type mockAlgorithm struct{}

func (m *mockAlgorithm) Name() string { return "mock" }

func (m *mockAlgorithm) EvaluateMemory(now time.Time, state *algorithm.State, limit int64, window time.Duration) algorithm.Result {
	state.Count++
	allowed := state.Count <= limit
	remaining := limit - state.Count
	if remaining < 0 {
		remaining = 0
	}
	return algorithm.Result{
		Allowed:   allowed,
		Limit:     limit,
		Remaining: remaining,
		ResetIn:   window,
	}
}

func (m *mockAlgorithm) RedisScript() string { return "" }

func TestMemoryDriver_Allow(t *testing.T) {
	d := memory.New(1 * time.Minute)
	defer func() { _ = d.Close(context.Background()) }()

	alg := &mockAlgorithm{}
	ctx := context.Background()

	// Request 1: Allowed
	res, err := d.Allow(ctx, "user1", 2, 10*time.Second, alg)
	if err != nil || !res.Allowed || res.Remaining != 1 {
		t.Fatalf("expected allowed=true, remaining=1; got allowed=%v, remaining=%d, err=%v", res.Allowed, res.Remaining, err)
	}

	// Request 2: Allowed
	res, err = d.Allow(ctx, "user1", 2, 10*time.Second, alg)
	if err != nil || !res.Allowed || res.Remaining != 0 {
		t.Fatalf("expected allowed=true, remaining=0; got allowed=%v, remaining=%d, err=%v", res.Allowed, res.Remaining, err)
	}

	// Request 3: Blocked (Limit exceeded)
	res, err = d.Allow(ctx, "user1", 2, 10*time.Second, alg)
	if err != nil || res.Allowed {
		t.Fatalf("expected allowed=false; got allowed=%v, err=%v", res.Allowed, err)
	}
}

func TestMemoryDriver_ConcurrentAccess(t *testing.T) {
	d := memory.New(1 * time.Minute)
	defer func() { _ = d.Close(context.Background()) }()

	alg := &mockAlgorithm{}
	ctx := context.Background()

	var wg sync.WaitGroup
	concurrentReqs := 100

	for i := 0; i < concurrentReqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = d.Allow(ctx, "concurrent_key", 1000, 10*time.Second, alg)
		}()
	}

	wg.Wait()
}
