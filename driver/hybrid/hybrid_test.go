package hybrid

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/driver/memory"
)

// mockDriver adalah driver buatan untuk menyimulasikan Primary Driver (Redis) yang bisa dibuat error secara dinamis.
type mockDriver struct {
	shouldFail bool
	err        error
}

func (m *mockDriver) Allow(ctx context.Context, key string, limit int64, window time.Duration) (distlimit.Result, error) {
	if m.shouldFail {
		return distlimit.Result{}, m.err
	}
	return distlimit.Result{
		Allowed:   true,
		Limit:     limit,
		Remaining: limit - 1,
		ResetIn:   window,
	}, nil
}

func (m *mockDriver) Close(ctx context.Context) error {
	return nil
}

// TestHybridDriver_NormalOperation menguji saat Primary Driver (Redis) sehat
func TestHybridDriver_NormalOperation(t *testing.T) {
	primaryMock := &mockDriver{shouldFail: false}
	fallbackMem := memory.New(1 * time.Minute)
	defer fallbackMem.Close(context.Background())

	h := New(primaryMock, fallbackMem)
	defer h.Close(context.Background())

	ctx := context.Background()
	res, err := h.Allow(ctx, "user_1", 10, 1*time.Minute)

	if err != nil {
		t.Fatalf("unexpected error during normal operation: %v", err)
	}
	if !res.Allowed {
		t.Errorf("request should be allowed by primary driver")
	}
	if h.FallbackCount() != 0 {
		t.Errorf("expected 0 fallback count during normal operation, got %d", h.FallbackCount())
	}
	if !h.IsPrimaryHealthy() {
		t.Errorf("primary should be healthy during normal operation")
	}
}

// TestHybridDriver_PrimaryFails_GracefulFallback menguji simulasi saat Primary Driver MATI
func TestHybridDriver_PrimaryFails_GracefulFallback(t *testing.T) {
	expectedErr := errors.New("redis connection refused / timeout")
	primaryMock := &mockDriver{shouldFail: true, err: expectedErr}
	fallbackMem := memory.New(1 * time.Minute)
	defer fallbackMem.Close(context.Background())

	var capturedError error
	onErrorCallback := func(err error) {
		capturedError = err
	}

	h := New(
		primaryMock,
		fallbackMem,
		WithOnError(onErrorCallback),
		WithCoolOffDuration(200*time.Millisecond),
	)
	defer h.Close(context.Background())

	ctx := context.Background()

	// 1. Panggilan Pertama: Primary Error -> Harus Otomatis Fallback ke Memory TANPA Melempar Error!
	res, err := h.Allow(ctx, "user_fallback", 5, 1*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error! Hybrid driver should handle fallback gracefully, got: %v", err)
	}

	// Memastikan request tetap diizinkan oleh Fallback Driver (In-Memory)
	if !res.Allowed {
		t.Errorf("request should be allowed by fallback memory driver")
	}

	// Memastikan OnError callback berhasil menangkap error Redis
	if capturedError == nil || capturedError.Error() != expectedErr.Error() {
		t.Errorf("expected captured error '%v', got '%v'", expectedErr, capturedError)
	}

	// Memastikan status Primary berubah menjadi unhealthy & FallbackCount bertambah
	if h.IsPrimaryHealthy() {
		t.Errorf("primary should be marked as unhealthy after error")
	}
	if h.FallbackCount() != 1 {
		t.Errorf("expected fallback count to be 1, got %d", h.FallbackCount())
	}
}

// TestHybridDriver_CoolOff_And_Recovery menguji periode Cool-Off dan pemulihan otomatis saat Primary Redis HIDUP KEMBALI
func TestHybridDriver_CoolOff_And_Recovery(t *testing.T) {
	primaryMock := &mockDriver{shouldFail: true, err: errors.New("redis temporary down")}
	fallbackMem := memory.New(1 * time.Minute)
	defer fallbackMem.Close(context.Background())

	coolOffDuration := 150 * time.Millisecond
	h := New(primaryMock, fallbackMem, WithCoolOffDuration(coolOffDuration))
	defer h.Close(context.Background())

	ctx := context.Background()

	// 1. Pemicu Pertama: Primary Mati -> Masuk Mode Fallback & Cool-Off
	_, _ = h.Allow(ctx, "user_cooloff", 10, 1*time.Minute)
	if h.IsPrimaryHealthy() {
		t.Errorf("expected primary to be marked as unhealthy")
	}

	// 2. Simulasi Redis Pulih Kembali
	primaryMock.shouldFail = false

	// 3. Panggilan dalam masa Cool-Off (< 150ms): Harus LANGSUNG ke Fallback tanpa mencoba Redis lagi
	_, _ = h.Allow(ctx, "user_cooloff", 10, 1*time.Minute)
	if h.FallbackCount() != 2 {
		t.Errorf("expected fallback count to be 2 during cool-off period, got %d", h.FallbackCount())
	}

	// 4. Tunggu hingga masa Cool-Off Selesai
	time.Sleep(coolOffDuration + 20*time.Millisecond)

	// 5. Panggilan setelah Cool-Off: Harus mencoba Primary Redis lagi dan SUKSES (Primary Recovery!)
	res, err := h.Allow(ctx, "user_cooloff", 10, 1*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error after recovery: %v", err)
	}
	if !res.Allowed {
		t.Errorf("request after recovery should be allowed")
	}

	// Memastikan status Primary pulih kembali menjadi sehat!
	if !h.IsPrimaryHealthy() {
		t.Errorf("primary should be marked as healthy again after successful request")
	}
}
