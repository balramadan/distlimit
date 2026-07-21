package redis

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// setupTestRedis membuat koneksi Redis dan memeriksa ketersediaan server.
// Jika Redis tidak dapat dijangkau, test akan di-skip secara otomatis.
func setupTestRedis(t *testing.T) (*goredis.Client, func()) {
	t.Helper()

	rdb := goredis.NewClient(&goredis.Options{
		Addr:     "localhost:6379",
		Password: "", // No password default
		DB:       15, // Gunakan DB 15 khusus untuk testing
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("⚠️ Skipped Redis integration test: Redis server not running on localhost:6379 (%v)", err)
	}

	// Clean DB sebelum test berjalan
	rdb.FlushDB(context.Background())

	cleanup := func() {
		rdb.FlushDB(context.Background())
		_ = rdb.Close()
	}

	return rdb, cleanup
}

// TestDriver_Redis_Sequential menguji eksekusi Lua Script secara berurutan
func TestDriver_Redis_Sequential(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	d := New(rdb, WithPrefix("test:distlimit:"))
	defer d.Close(context.Background())

	ctx := context.Background()
	key := "user_redis_1"
	limit := int64(3)
	window := 500 * time.Millisecond

	// 1. Request 1 s/d 3 harus diizinkan oleh Lua Script
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

	// 2. Request ke-4 harus ditolak (Exceeded)
	res, err := d.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("unexpected error on request 4: %v", err)
	}
	if res.Allowed {
		t.Errorf("request 4 should be rejected by Redis Lua script")
	}
	if res.Remaining != 0 {
		t.Errorf("expected remaining 0, got %d", res.Remaining)
	}

	// 3. Tunggu hingga window waktu habis/di-reset
	time.Sleep(window + 100*time.Millisecond)

	// 4. Request ke-5 diizinkan kembali setelah reset window
	res, err = d.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("unexpected error after window reset: %v", err)
	}
	if !res.Allowed {
		t.Errorf("request after window reset should be allowed")
	}
}

// TestDriver_Redis_Concurrent menguji keandalan sifat Atomic Lua Script pada kondisi serbuan paralel
func TestDriver_Redis_Concurrent(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	d := New(rdb)
	defer d.Close(context.Background())

	ctx := context.Background()
	key := "concurrent_redis_user"
	limit := int64(20)
	window := 2 * time.Second
	totalGoroutines := 100

	var allowedCount int64
	var blockedCount int64

	var wg sync.WaitGroup
	wg.Add(totalGoroutines)

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

	// Bukti Sifat Atomic Lua Script Redis: Tepat 20 allowed & 80 blocked
	if allowedCount != limit {
		t.Errorf("atomic Lua script error: expected exactly %d allowed requests, got %d", limit, allowedCount)
	}
	expectedBlocked := int64(totalGoroutines) - limit
	if blockedCount != expectedBlocked {
		t.Errorf("atomic Lua script error: expected %d blocked requests, got %d", expectedBlocked, blockedCount)
	}
}

// BenchmarkDriver_Redis_Allow mengukur latensi round-trip network I/O + Lua execution ke Redis
func BenchmarkDriver_Redis_Allow(b *testing.B) {
	rdb, cleanup := setupTestRedis(&testing.T{})
	if rdb == nil {
		b.Skip("Redis server not running")
	}
	defer cleanup()

	d := New(rdb)
	defer d.Close(context.Background())

	ctx := context.Background()
	key := "bench_redis_user"
	limit := int64(1_000_000_000)
	window := 1 * time.Minute

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = d.Allow(ctx, key, limit, window)
	}
}
