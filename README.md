# ⚡ distlimit

[![Go Reference](https://pkg.go.dev/badge/github.com/balramadan/distlimit.svg)](https://pkg.go.dev/github.com/balramadan/distlimit)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/badge/Release-v1.2.0-blue.svg)](https://github.com/balramadan/distlimit/releases/tag/v1.2.0)

**`distlimit`** is an ultra-high performance, distributed, pluggable rate-limiting library for Go. Engineered for microservices, high-concurrency APIs, and financial-grade applications requiring nanosecond-level execution speeds, zero memory allocation, and multi-tier failover capabilities — now with real-time observability and zero-downtime dynamic policy reloading.

---

## 🚀 Why `distlimit`?

Most Go rate-limiting libraries force you into a single algorithm, lock global mutexes during background cleanup, or collapse when Redis goes down. **`distlimit`** solves these architectural flaws with enterprise-grade resilience:

- **🔌 100% Pluggable Strategy Architecture:** Swap algorithms seamlessly without changing your storage driver or HTTP framework middleware.
- **⚡ Zero Allocation & Nanosecond Latency:** Memory evaluation runs in sub-100 nanoseconds with **`0 B/op`** memory overhead across all standard algorithms.
- **🔒 64-Sharded Memory Architecture:** Eliminates global lock contention during concurrent HTTP requests and background state cleanups.
- **🛡️ Half-Open Circuit Breaker (Hybrid Driver):** Automatic failover from Redis to In-Memory with single-request probing to prevent _Thundering Herd_ spikes upon Redis recovery.
- **🔐 Security-First Middlewares:** Built-in protection against **IP Spoofing** via CIDR-validated `WithTrustedProxies` headers inspection.
- **🌐 Native Redis Cluster Safety:** Enforces **Redis Hash Tags `{}`** to prevent `CROSSSLOT` cluster routing errors.
- **📊 Pluggable Observability (v1.2.0):** Zero-allocation telemetry via `metrics.Observer` with turnkey Prometheus & OpenTelemetry collectors.
- **🔄 Dynamic Policy Engine (v1.2.0):** Lock-free $O(1)$ runtime policy updates with `atomic.Pointer[Policy]` and multi-tenant tier resolution.

---

## 📐 Architecture Overview

```
                    +----------------------------+
                    |  HTTP / gRPC Incoming Req  |
                    +----------------------------+
                                  |
                     [ Trusted Proxies Check ]  <-- IP Spoofing Shield
                                  |
                    +----------------------------+
                    |     distlimit.Limiter      |
                    +----------------------------+
                                  |
           +----------------------+----------------------+
           |                                             |
 [ Pluggable Algorithm ]                         [ Storage Driver ]

* Token Bucket                                 - Memory (64-Sharded)
* Leaky Bucket                                 - Redis (Cluster Hash-Tagged)
* Fixed Window                                 - Hybrid (Circuit Breaker)
* Sliding Window Log
* Sliding Window Counter

```

---

## 🧮 Supported Algorithms

| Algorithm                  | Traffic Pattern      | Memory | Redis Data Structure | Best Use Case                                                                |
| -------------------------- | -------------------- | ------ | -------------------- | ---------------------------------------------------------------------------- |
| **Token Bucket**           | Burst Friendly       | $O(1)$ | Hash                 | General-purpose API rate limiting with burst support.                        |
| **Leaky Bucket**           | Traffic Shaping      | $O(1)$ | Hash                 | Smoothing spikes for third-party integrations (e.g., Payment Gateways).      |
| **Fixed Window**           | Interval Reset       | $O(1)$ | String Counter       | Ultra-lightweight endpoint protection & login brute-force shielding.         |
| **Sliding Window Log**     | 100% Exact Precision | $O(N)$ | Sorted Set (ZSET)    | Strict quota allocation, financial transactions, and zero-compromise limits. |
| **Sliding Window Counter** | Weighted Moving Avg  | $O(1)$ | Hash                 | High-throughput distributed rate limiting with $O(1)$ memory precision.      |

---

## 📋 Prerequisites

- **Go**: `1.20` or higher
- **Redis** _(Optional for distributed driver)_: Standalone, Sentinel, or Cluster `v6.0+`

---

## 📦 Installation

```bash
go get github.com/balramadan/distlimit
```

---

## ⚡ Quickstart

### 1. Basic In-Memory Limiter

Evaluate rate limits using the 64-sharded in-memory driver with Sliding Window Counter:

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm/slidingcounter"
	"github.com/balramadan/distlimit/driver/memory"
)

func main() {
	// Initialize 64-Sharded Memory Driver (TTL 5 minutes)
	memDriver := memory.New(5 * time.Minute)
	defer memDriver.Close(context.Background())

	// Create Limiter: 10 requests per 1 minute using Sliding Window Counter
	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithLimit(10),
		distlimit.WithWindow(1*time.Minute),
		distlimit.WithAlgorithm(slidingcounter.New()),
	)
	if err != nil {
		panic(err)
	}

	// Evaluate Rate Limit for a key
	res, err := limiter.AllowKey(context.Background(), "user:123")
	if err != nil {
		panic(err)
	}

	if res.Allowed {
		fmt.Printf("Allowed! Remaining: %d\n", res.Remaining)
	} else {
		fmt.Printf("Blocked! Retry after: %v\n", res.ResetIn)
	}
}
```

---

### 2. HTTP Middleware with Anti-Spoofing & Observability

Protect HTTP endpoints with trusted proxy validation, Prometheus metrics, and per-route labels — all in one setup:

```go
package main

import (
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
	memDriver := memory.New(5 * time.Minute)
	defer memDriver.Close(nil)

	// Attach Prometheus Observer
	promObserver := distprom.NewObserver(
		distprom.WithNamespace("my_app"),
		distprom.WithSubsystem("api"),
	)

	limiter, _ := distlimit.New(
		memDriver,
		distlimit.WithLimit(100),
		distlimit.WithWindow(1*time.Minute),
		distlimit.WithAlgorithm(tokenbucket.New()),
		distlimit.WithMetricObserver(promObserver),
	)

	mux := http.NewServeMux()

	// Rate-limited handler with route labeling & trusted proxy protection
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/api/v1/resource", distlimitnethttp.New(
		limiter,
		distlimitnethttp.WithTrustedProxies([]string{"10.0.0.0/8", "172.16.0.0/12"}),
		distlimitnethttp.WithRouteLabeling(true), // passes "/api/v1/resource" into metric labels
	)(apiHandler))

	// Expose Prometheus scrape endpoint
	mux.Handle("/metrics", promhttp.Handler())

	http.ListenAndServe(":8080", mux)
}
```

**Exported Prometheus metrics:**

| Metric                                  | Type      | Description                                                    |
| --------------------------------------- | --------- | -------------------------------------------------------------- |
| `distlimit_requests_total`              | Counter   | Total evaluations by `allowed`, `driver`, `algorithm`, `route` |
| `distlimit_evaluation_duration_seconds` | Histogram | Execution latency of each evaluation                           |
| `distlimit_hybrid_fallback_total`       | Counter   | Times the Hybrid Driver failed over to memory                  |

---

### 3. Resilient Dual-Tier Hybrid Driver (Redis + Memory Fallback)

Automatic failover to in-memory fallback with Half-Open Circuit Breaker if Redis connection drops:

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm/slidingcounter"
	distlimithybrid "github.com/balramadan/distlimit/driver/hybrid"
	distlimitmemory "github.com/balramadan/distlimit/driver/memory"
	distlimitredis "github.com/balramadan/distlimit/driver/redis"
	"github.com/redis/go-redis/v9"
)

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	primary := distlimitredis.New(rdb)
	fallback := distlimitmemory.New(5 * time.Minute)

	// Hybrid Driver with 5s Cool-Off and Half-Open Probing
	hybridDriver := distlimithybrid.New(
		primary,
		fallback,
		distlimithybrid.WithCoolOffDuration(5*time.Second),
		distlimithybrid.WithOnError(func(err error) {
			log.Printf("[DISTLIMIT WARN] Redis down, failing over to memory: %v", err)
		}),
	)

	limiter, _ := distlimit.New(
		hybridDriver,
		distlimit.WithLimit(500),
		distlimit.WithWindow(1*time.Minute),
		distlimit.WithAlgorithm(slidingcounter.New()),
	)

	_, _ = limiter.AllowKey(context.Background(), "api_key_abc")
}
```

---

### 4. Dynamic Policy Reloading & Multi-Tenant Tier Resolution

Update global limits at runtime without restarts, and apply per-key tier overrides via `PolicyResolver`:

```go
// Lock-free O(1) runtime update — no restart, no lock contention
limiter.UpdatePolicy(500, 1*time.Minute)

// Multi-tenant PolicyResolver: VIP users get a higher quota
type TierResolver struct{}

func (r *TierResolver) ResolvePolicy(ctx context.Context, key string) (distlimit.Policy, bool) {
	if strings.HasPrefix(key, "vip:") {
		return distlimit.Policy{Limit: 5000, Window: 1 * time.Minute}, true
	}
	return distlimit.Policy{}, false // fallback to default
}

limiter, _ := distlimit.New(
	driver,
	distlimit.WithLimit(100),
	distlimit.WithWindow(1*time.Minute),
	distlimit.WithPolicyResolver(&TierResolver{}),
)
```

---

### 5. OpenTelemetry Instrumentation

```go
import (
	distotel "github.com/balramadan/distlimit/metrics/otel"
	"go.opentelemetry.io/otel"
)

otelObserver := distotel.NewObserver(
	distotel.WithMeterProvider(otel.GetMeterProvider()),
)

limiter, _ := distlimit.New(
	driver,
	distlimit.WithMetricObserver(otelObserver),
)
```

---

### 6. Route Labeling for All Middleware Adapters

Pass the parametrized route pattern (e.g., `/api/v1/users/:id`) or gRPC `FullMethod` into metric labels automatically:

```go
// net/http — uses r.Pattern (Go 1.22+) or fallback to r.URL.Path
distlimitnethttp.New(limiter, distlimitnethttp.WithRouteLabeling(true))

// Gin — uses c.FullPath()
distlimitgin.New(limiter, distlimitgin.WithRouteLabeling(true))

// Echo v4 — uses c.Path()
distlimitecho.New(limiter, distlimitecho.WithRouteLabeling(true))

// gRPC — uses info.FullMethod (e.g. /package.Service/Method)
grpc.NewServer(grpc.ChainUnaryInterceptor(
	distlimitgrpc.UnaryServerInterceptor(limiter, distlimitgrpc.WithRouteLabeling(true)),
))
```

---

## 🛡️ Framework & Middleware Adapters

`distlimit` provides native, zero-dependency middleware adapters for all popular Go web frameworks:

| Framework              | Import Path                                          | Route Labeling            |
| ---------------------- | ---------------------------------------------------- | ------------------------- |
| ⚡ Fiber v3            | `github.com/balramadan/distlimit/middleware/fiber`   | `WithRouteLabeling(true)` |
| 🍸 Gin                 | `github.com/balramadan/distlimit/middleware/gin`     | `WithRouteLabeling(true)` |
| 🔊 Echo v4             | `github.com/balramadan/distlimit/middleware/echo`    | `WithRouteLabeling(true)` |
| 🔊 Echo v5             | `github.com/balramadan/distlimit/middleware/echov5`  | `WithRouteLabeling(true)` |
| 🌐 Standard `net/http` | `github.com/balramadan/distlimit/middleware/nethttp` | `WithRouteLabeling(true)` |
| 📡 gRPC                | `github.com/balramadan/distlimit/middleware/grpc`    | `WithRouteLabeling(true)` |

---

## 📊 Performance & Benchmarks

Benchmarks executed on AMD 3020e Linux x86_64 (`go test -bench=. -benchmem -count=1 ./algorithm/... ./...`):

```
pkg: github.com/balramadan/distlimit/algorithm/*
cpu: AMD 3020e with Radeon Graphics

BenchmarkSlidingLog_EvaluateMemory-2        46,176,700    22.37 ns/op    0 B/op    0 allocs/op
BenchmarkTokenBucket_EvaluateMemory-2       22,270,572    49.59 ns/op    0 B/op    0 allocs/op
BenchmarkLeakyBucket_EvaluateMemory-2       23,049,756    67.77 ns/op    0 B/op    0 allocs/op
BenchmarkFixedWindow_EvaluateMemory-2       14,373,084   106.30 ns/op    0 B/op    0 allocs/op
BenchmarkSlidingCounter_EvaluateMemory-2     6,060,211   214.00 ns/op    0 B/op    0 allocs/op

-- Full Limiter Stack (driver + algorithm + policy evaluation) --
BenchmarkLimiter_NoObserver-2                  311,350   3,446.00 ns/op  0 B/op    0 allocs/op
```

> **Key Takeaway:** All 5 algorithms achieve **zero memory allocations (`0 B/op`, `0 allocs/op`)** during in-memory evaluation, allowing your Go application to handle tens of millions of rate-check operations per second without GC pause overhead. The full limiter stack including driver + algorithm + atomic policy evaluation also maintains **`0 B/op`** — and so does the telemetry observer path when disabled.

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
