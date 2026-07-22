package slidinglog_test

import (
	"testing"
	"time"

	"github.com/balramadan/distlimit/algorithm"
	"github.com/balramadan/distlimit/algorithm/slidinglog"
)

func TestSlidingLog_ExactPrecision(t *testing.T) {
	strat := slidinglog.New()
	state := &algorithm.State{}
	start := time.Now()

	limit := int64(3)
	window := 1 * time.Second

	// 1. Isikan 3 request pada t=0 (Allowed)
	for i := 1; i <= 3; i++ {
		res := strat.EvaluateMemory(start, state, limit, window)
		if !res.Allowed {
			t.Fatalf("expected request %d to be allowed", i)
		}
	}

	// 2. Request 4 pada t=500ms harus diblokir
	t500ms := start.Add(500 * time.Millisecond)
	res := strat.EvaluateMemory(t500ms, state, limit, window)
	if res.Allowed {
		t.Fatalf("expected request at t=500ms to be blocked")
	}

	// 3. Request pada t=1100ms (1.1s) -> timestamp pertama t=0s sudah terlewati/dihapus, diizinkan kembali
	t1100ms := start.Add(1100 * time.Millisecond)
	res = strat.EvaluateMemory(t1100ms, state, limit, window)
	if !res.Allowed {
		t.Fatalf("expected request at t=1.1s to be allowed after log sliding")
	}
}

func BenchmarkSlidingLog_EvaluateMemory(b *testing.B) {
	strat := slidinglog.New()
	state := &algorithm.State{}
	now := time.Now()
	limit := int64(1000)
	window := 1 * time.Minute

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		now = now.Add(1 * time.Millisecond)
		_ = strat.EvaluateMemory(now, state, limit, window)
	}
}
