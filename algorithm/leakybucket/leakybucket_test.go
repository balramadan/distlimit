package leakybucket_test

import (
	"testing"
	"time"

	"github.com/balramadan/distlimit/algorithm"
	"github.com/balramadan/distlimit/algorithm/leakybucket"
)

func TestLeakyBucket_LeakAndOverflow(t *testing.T) {
	strat := leakybucket.New()
	state := &algorithm.State{}
	start := time.Now()

	limit := int64(3)
	window := 3 * time.Second // Leak rate = 1 request / detik

	// 1. Isi ember hingga penuh (Request 1, 2, 3)
	for i := 1; i <= 3; i++ {
		res := strat.EvaluateMemory(start, state, limit, window)
		if !res.Allowed {
			t.Fatalf("expected request %d to fill bucket and be allowed", i)
		}
	}

	// 2. Request 4 pada t=0s harus langsung meluap / ditolak (Bucket full = 3/3)
	res := strat.EvaluateMemory(start, state, limit, window)
	if res.Allowed {
		t.Fatalf("expected request 4 to be rejected due to overflow")
	}

	// 3. Simulasikan waktu berlalu 1 detik (1 request "bocor/keluar", tersisa 2/3)
	after1Sec := start.Add(1 * time.Second)
	res = strat.EvaluateMemory(after1Sec, state, limit, window)
	if !res.Allowed {
		t.Fatalf("expected request after 1s leak to be allowed")
	}

	// 4. Request berikutnya di detik yang sama langsung meluap lagi
	res = strat.EvaluateMemory(after1Sec, state, limit, window)
	if res.Allowed {
		t.Fatalf("expected immediate request to overflow bucket")
	}
}

func BenchmarkLeakyBucket_EvaluateMemory(b *testing.B) {
	strat := leakybucket.New()
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
