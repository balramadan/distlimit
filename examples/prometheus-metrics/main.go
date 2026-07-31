package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm/tokenbucket"
	"github.com/balramadan/distlimit/driver/memory"
	distprom "github.com/balramadan/distlimit/metrics/prometheus"
	distlimitnethttp "github.com/balramadan/distlimit/middleware/nethttp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// 1. Inisialisasi Prometheus Metrics Observer
	promObserver := distprom.NewObserver(
		distprom.WithNamespace("demo"),
		distprom.WithSubsystem("api"),
	)

	// 2. Inisialisasi Memory Driver (TTL 5 Menit)
	memDriver := memory.New(5 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	// 3. Inisialisasi Limiter dengan Metric Observer
	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithLimit(10),
		distlimit.WithWindow(1*time.Minute),
		distlimit.WithAlgorithm(tokenbucket.New()),
		distlimit.WithMetricObserver(promObserver),
	)
	if err != nil {
		tFatalf("Failed to initialize limiter: %v", err)
	}

	mux := http.NewServeMux()

	// Handler API Utama
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","message":"Hello from Prometheus Instrumented API!"}`))
	})

	// Pasang Middleware dengan Route Labeling Enabled
	rateLimitedAPI := distlimitnethttp.New(
		limiter,
		distlimitnethttp.WithRouteLabeling(true),
	)(apiHandler)

	mux.Handle("/api/hello", rateLimitedAPI)

	// Expose Endpoint Metrik Prometheus
	mux.Handle("/metrics", promhttp.Handler())

	log.Println("⚡ Prometheus Instrumented Server running on http://localhost:8080")
	log.Println("📊 Metrics endpoint available at http://localhost:8080/metrics")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func tFatalf(format string, args ...any) {
	log.Fatalf(format, args...)
}
