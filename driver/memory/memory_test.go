package memory

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDriver_Allow_Sequential(t *testing.T) {
	d := New(100 * time.Millisecond)
	defer d.Close(context.Background())

	ctx := context.Background()
	key := "user_test_1"
	limit := int64(3)
	window := 200 * time.Millisecond

	// 1. Request 1s/d 3 harus diizinkan (Allowed = true)
	for i := int64(1); i <= limit; i++ {
		res, err := d.Allow(ctx, key, limit, window)
		if err != nil {
			t.Fatalf("unexpected error on request %d: %v", i, err)
		}
		if !res.Allowed {
			t.Errorf("request %d should be allowed", i)
		}
		expectedRemaining := limit - i
		if res.Remaining != expectedRemaining {
			t.Errorf("request %d: expected remaining %d, got %d", i, expectedRemaining, res.Remaining)
		}
	}

	// 2. Request ke-4 harus ditolak (Allowed = false, Remaining = 0)
	res, err := d.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("unexpected error on request 4: %v", err)
	}
	if res.Allowed {
		t.Errorf("request 4 should be rejected (rate limit exceeded)")
	}
	if res.Remaining != 0 {
		t.Errorf("expected remaining 0 when limit exceeded, got %d", res.Remaining)
	}

	// 3. Tunggu hingga window waktu habis/di-reset
	time.Sleep(window + 10*time.Millisecond)

	// 4. Request ke-5 harus diizinkan kembali setelah reset window
	res, err = d.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("unexpected error after window reset: %v", err)
	}
	if !res.Allowed {
		t.Errorf("request after window reset should be allowed")
	}
	if res.Remaining != limit-1 {
		t.Errorf("expected remaining %d after reset, got %d", limit-1, res.Remaining)
	}
}

func TestDriver_Concurrent_Race_Accuracy(t *testing.T) {
	d := New(1 * time.Second)
	defer d.Close(context.Background())

	ctx := context.Background()
	key := "concurrent_user"
	limit := int64(50)
	window := 1 * time.Second
	totalGoroutines := 200

	var allowedCount int64
	var blockedCount int64

	var wg sync.WaitGroup
	wg.Add(totalGoroutines)

	// Tembak 200 request bersamaan dari goroutine berbeda
	for i := 0; i < totalGoroutines; i++ {
		go func() {
			defer wg.Done()
			res, err := d.Allow(ctx, key, limit, window)
			if err != nil {
				t.Errorf("unexpected error in goroutine: %v", err)
				return
			}
			if res.Allowed {
				atomic.AddInt64(&allowedCount, 1)
			} else {
				atomic.AddInt64(&blockedCount, 1)
			}
		}()
	}

	wg.Wait()

	// Presisi Atomic Check: Harus Tepat 50 yang sukses dan 150 yang diblokir
	if allowedCount != limit {
		t.Errorf("atomic precision error: expected exactly %d allowed requests, got %d", limit, allowedCount)
	}
	expectedBlocked := int64(totalGoroutines) - limit
	if blockedCount != expectedBlocked {
		t.Errorf("atomic precision error: expected %d blocked requests, got %d", expectedBlocked, blockedCount)
	}
}

func BenchmarkDriver_Allow_SingleThread(b *testing.B) {
	d := New(1 * time.Minute)
	defer d.Close(context.Background())

	ctx := context.Background()
	key := "bench_single_user"
	limit := int64(1_000_000_000) // Limit besar agar tidak memicu rate limit saat benchmark
	window := 1 * time.Minute

	b.ResetTimer()
	b.ReportAllocs() // Melaporkan alokasi memori (B/op dan allocs/op)

	for i := 0; i < b.N; i++ {
		_, _ = d.Allow(ctx, key, limit, window)
	}
}

func BenchmarkDriver_Allow_Parallel(b *testing.B) {
	d := New(1 * time.Minute)
	defer d.Close(context.Background())

	ctx := context.Background()
	limit := int64(1_000_000_000)
	window := 1 * time.Minute

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = d.Allow(ctx, "bench_parallel_user", limit, window)
		}
	})
}
