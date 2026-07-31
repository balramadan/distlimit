package otel_test

import (
	"context"
	"testing"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/driver/memory"
	"github.com/balramadan/distlimit/metrics"
	distotel "github.com/balramadan/distlimit/metrics/otel"
	"go.opentelemetry.io/otel/sdk/metric"
)

func TestOTelObserver_Observe(t *testing.T) {
	mp := metric.NewMeterProvider()
	obs, err := distotel.NewObserver(
		distotel.WithMeterProvider(mp),
		distotel.WithMeterName("test_distlimit_otel"),
	)
	if err != nil {
		t.Fatalf("failed to create otel observer: %v", err)
	}

	memDriver := memory.New(1 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithMetricObserver(obs),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	_, err = limiter.AllowKey(ctx, "user_otel_1")
	if err != nil {
		t.Fatalf("AllowKey failed: %v", err)
	}

	obs.RecordHybridFallback(ctx, "redis_down")
}

func TestOTelObserver_ImplementsObserver(t *testing.T) {
	var _ metrics.Observer = (*distotel.Observer)(nil)
}
