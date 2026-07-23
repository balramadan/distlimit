package hybrid_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/balramadan/distlimit/algorithm"
	"github.com/balramadan/distlimit/driver/hybrid"
	"github.com/balramadan/distlimit/driver/memory"
)

type mockAlgorithm struct{}

func (m *mockAlgorithm) Name() string { return "mock" }
func (m *mockAlgorithm) EvaluateMemory(now time.Time, state *algorithm.State, limit int64, window time.Duration) algorithm.Result {
	return algorithm.Result{Allowed: true, Limit: limit, Remaining: limit - 1, ResetIn: window}
}
func (m *mockAlgorithm) RedisScript() string { return "" }

// failingDriver mensimulasikan primary driver (Redis) yang sedang outage
type failingDriver struct{}

func (f *failingDriver) Allow(ctx context.Context, key string, limit int64, window time.Duration, alg algorithm.Algorithm) (algorithm.Result, error) {
	return algorithm.Result{}, errors.New("redis connection refused")
}
func (f *failingDriver) Reset(ctx context.Context, key string) error { return nil }
func (f *failingDriver) Close(ctx context.Context) error           { return nil }

// workingDriver mensimulasikan primary driver yang berjalan normal
type workingDriver struct{}

func (w *workingDriver) Allow(ctx context.Context, key string, limit int64, window time.Duration, alg algorithm.Algorithm) (algorithm.Result, error) {
	return algorithm.Result{Allowed: true, Limit: limit, Remaining: limit - 1, ResetIn: window}, nil
}
func (w *workingDriver) Reset(ctx context.Context, key string) error { return nil }
func (w *workingDriver) Close(ctx context.Context) error           { return nil }

func TestHybridDriver_NormalFlow(t *testing.T) {
	primary := &workingDriver{}
	fallback := memory.New(1 * time.Minute)
	defer func() { _ = fallback.Close(context.Background()) }()

	hDriver := hybrid.New(primary, fallback)
	alg := &mockAlgorithm{}
	ctx := context.Background()

	res, err := hDriver.Allow(ctx, "user1", 10, 1*time.Minute, alg)
	if err != nil || !res.Allowed {
		t.Fatalf("expected allowed=true from primary, got err=%v", err)
	}

	if hDriver.IsPrimaryDown() {
		t.Fatalf("expected primary to be healthy (IsPrimaryDown=false)")
	}
}

func TestHybridDriver_FailoverToFallback(t *testing.T) {
	primary := &failingDriver{}
	fallback := memory.New(1 * time.Minute)
	defer func() { _ = fallback.Close(context.Background()) }()

	var errorLogged bool
	hDriver := hybrid.New(
		primary,
		fallback,
		hybrid.WithCoolOffDuration(50*time.Millisecond),
		hybrid.WithOnError(func(err error) {
			errorLogged = true
		}),
	)

	alg := &mockAlgorithm{}
	ctx := context.Background()

	// Call 1: Primary gagal, beralih ke Fallback
	res, err := hDriver.Allow(ctx, "user1", 10, 1*time.Minute, alg)
	if err != nil || !res.Allowed {
		t.Fatalf("expected failover to fallback allowed=true, got err=%v", err)
	}

	if !hDriver.IsPrimaryDown() {
		t.Fatalf("expected primary to be marked as down")
	}

	if !errorLogged {
		t.Fatalf("expected onError callback to be triggered")
	}

	if hDriver.FallbackCount() != 1 {
		t.Fatalf("expected FallbackCount = 1, got %d", hDriver.FallbackCount())
	}
}

func TestHybridDriver_CoolOffRecovery(t *testing.T) {
	primary := &failingDriver{}
	fallback := memory.New(1 * time.Minute)
	defer func() { _ = fallback.Close(context.Background()) }()

	coolOff := 50 * time.Millisecond
	hDriver := hybrid.New(primary, fallback, hybrid.WithCoolOffDuration(coolOff))
	alg := &mockAlgorithm{}
	ctx := context.Background()

	// 1. Pemicu awal failure
	_, _ = hDriver.Allow(ctx, "user1", 10, 1*time.Minute, alg)
	if !hDriver.IsPrimaryDown() {
		t.Fatalf("expected primary to be down")
	}

	// 2. Selama masa cool-off, request langsung ke fallback tanpa re-evaluating primary
	_, _ = hDriver.Allow(ctx, "user1", 10, 1*time.Minute, alg)
	if hDriver.FallbackCount() != 2 {
		t.Fatalf("expected FallbackCount = 2, got %d", hDriver.FallbackCount())
	}

	// 3. Tunggu hingga cool-off selesai
	time.Sleep(coolOff + 10*time.Millisecond)

	// Request berikutnya akan mencoba primary lagi
	_, _ = hDriver.Allow(ctx, "user1", 10, 1*time.Minute, alg)
	if hDriver.FallbackCount() != 3 {
		t.Fatalf("expected FallbackCount = 3, got %d", hDriver.FallbackCount())
	}
}
