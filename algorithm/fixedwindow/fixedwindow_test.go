package fixedwindow_test

import (
	"testing"
	"time"

	"github.com/balramadan/distlimit/algorithm"
	"github.com/balramadan/distlimit/algorithm/fixedwindow"
)

func TestFixedWindow_ResetOnNewWindow(t *testing.T) {
	strat := fixedwindow.New()
	state := &algorithm.State{}
	start := time.Now().Truncate(1 * time.Minute)

	limit := int64(2)
	window := 1 * time.Minute

	// 1. Request 1 & 2 pada window yang sama (Allowed)
	res1 := strat.EvaluateMemory(start, state, limit, window)
	res2 := strat.EvaluateMemory(start.Add(10*time.Second), state, limit, window)

	if !res1.Allowed || !res2.Allowed {
		t.Fatalf("expected requests within limit to be allowed")
	}

	// 2. Request 3 pada window yang sama (Blocked)
	res3 := strat.EvaluateMemory(start.Add(20*time.Second), state, limit, window)
	if res3.Allowed {
		t.Fatalf("expected request exceeding limit to be blocked")
	}

	// 3. Request 4 pada window baru (+1 Menit) harus otomatis ter-reset & diizinkan kembali
	nextWindowTime := start.Add(1*time.Minute + 1*time.Second)
	res4 := strat.EvaluateMemory(nextWindowTime, state, limit, window)
	if !res4.Allowed {
		t.Fatalf("expected request in new window interval to be allowed")
	}

	if res4.Remaining != 1 {
		t.Fatalf("expected remaining=1 in new window, got %d", res4.Remaining)
	}
}

func BenchmarkFixedWindow_EvaluateMemory(b *testing.B) {
	strat := fixedwindow.New()
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
