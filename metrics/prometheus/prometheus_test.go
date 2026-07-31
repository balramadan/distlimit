package prometheus_test

import (
	"context"
	"testing"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/driver/memory"
	"github.com/balramadan/distlimit/metrics"
	distprom "github.com/balramadan/distlimit/metrics/prometheus"
	"github.com/prometheus/client_golang/prometheus"
)

func TestPrometheusObserver_Observe(t *testing.T) {
	reg := prometheus.NewRegistry()
	obs := distprom.NewObserver(
		distprom.WithNamespace("test_app"),
		distprom.WithSubsystem("ratelimit"),
		distprom.WithRegisterer(reg),
	)

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
	_, err = limiter.AllowKey(ctx, "user_prom_1")
	if err != nil {
		t.Fatalf("AllowKey failed: %v", err)
	}

	obs.RecordHybridFallback("redis_connection_error")

	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather prometheus metrics: %v", err)
	}

	if len(metricFamilies) == 0 {
		t.Fatalf("expected prometheus metrics to be registered and gathered")
	}

	var foundRequestsTotal bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "test_app_ratelimit_requests_total" {
			foundRequestsTotal = true
			if len(mf.GetMetric()) == 0 {
				t.Errorf("expected metric values for requests_total")
			}
		}
	}

	if !foundRequestsTotal {
		t.Errorf("expected metric 'test_app_ratelimit_requests_total' in gathered metrics")
	}
}

func TestPrometheusObserver_ImplementsObserver(t *testing.T) {
	var _ metrics.Observer = (*distprom.Observer)(nil)
}
