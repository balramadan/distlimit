package slidingcounter_test

import (
	"testing"
	"time"

	"github.com/balramadan/distlimit/algorithm"
	"github.com/balramadan/distlimit/algorithm/slidingcounter"
)

func TestSlidingCounter_WeightedMovingAverage(t *testing.T) {
	strat := slidingcounter.New()
	state := &algorithm.State{}
	start := time.Now().Truncate(1 * time.Minute)

	limit := int64(10)
	window := 1 * time.Minute

	// 1. Isikan 10 request pada window 1 (Window penuh)
	for i := 1; i <= 10; i++ {
		res := strat.EvaluateMemory(start, state, limit, window)
		if !res.Allowed {
			t.Fatalf("expected request %d in window 1 to be allowed", i)
		}
	}

	// Request ke-11 terblokir
	res := strat.EvaluateMemory(start, state, limit, window)
	if res.Allowed {
		t.Fatalf("expected request 11 to be blocked")
	}

	// 2. Maju ke window 2, detik ke-30 (t_elapsed = 30s -> Bobot window 1 tinggal 50%)
	t30s := start.Add(1*time.Minute + 30*time.Second)

	// Estimasi request di detik ke-30: (10 * 0.5) + 0 + 1 = 6 request -> Harus diizinkan
	res = strat.EvaluateMemory(t30s, state, limit, window)
	if !res.Allowed {
		t.Fatalf("expected request at t=1m30s to be allowed due to 50%% window 1 decay")
	}

	// Tambahkan 4 request lagi (Total est = 6 + 4 = 10 -> Penuh)
	for i := 0; i < 4; i++ {
		_ = strat.EvaluateMemory(t30s, state, limit, window)
	}

	// Request ke-6 di detik 30 terblokir
	res = strat.EvaluateMemory(t30s, state, limit, window)
	if res.Allowed {
		t.Fatalf("expected excess request at t=1m30s to be blocked")
	}
}

func BenchmarkSlidingCounter_EvaluateMemory(b *testing.B) {
	strat := slidingcounter.New()
	state := &algorithm.State{}
	now := time.Now()
	limit := int64(1000000)
	window := 1 * time.Minute

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		now = now.Add(1 * time.Microsecond)
		_ = strat.EvaluateMemory(now, state, limit, window)
	}
}
