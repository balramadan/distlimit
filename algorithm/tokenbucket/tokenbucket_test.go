package tokenbucket_test

import (
	"testing"
	"time"

	"github.com/balramadan/distlimit/algorithm"
	"github.com/balramadan/distlimit/algorithm/tokenbucket"
)

func TestTokenBucket_BurstAndRefill(t *testing.T) {
	strat := tokenbucket.New()
	state := &algorithm.State{}
	start := time.Now()

	limit := int64(3)
	window := 3 * time.Second // Refill rate = 1 token / detik

	// 1. Uji Burst Capacity (Request 1, 2, 3 harus allowed)
	for i := 1; i <= 3; i++ {
		res := strat.EvaluateMemory(start, state, limit, window)
		if !res.Allowed {
			t.Fatalf("expected request %d to be allowed during burst", i)
		}
	}

	// 2. Request 4 pada waktu yang sama harus diblokir (0 token tersisa)
	res := strat.EvaluateMemory(start, state, limit, window)
	if res.Allowed {
		t.Fatalf("expected request 4 to be blocked due to capacity exhaustion")
	}

	// 3. Simulasikan waktu berlalu 1 detik (1 token terisi kembali)
	after1Sec := start.Add(1 * time.Second)
	res = strat.EvaluateMemory(after1Sec, state, limit, window)
	if !res.Allowed {
		t.Fatalf("expected request after 1s refill to be allowed")
	}

	// 4. Request berikutnya langsung terblokir lagi
	res = strat.EvaluateMemory(after1Sec, state, limit, window)
	if res.Allowed {
		t.Fatalf("expected request to be blocked again")
	}
}

func BenchmarkTokenBucket_EvaluateMemory(b *testing.B) {
	strat := tokenbucket.New()
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
