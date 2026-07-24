# ⚡ distlimit

[![Go Reference](https://pkg.go.dev/badge/github.com/balramadan/distlimit.svg)](https://pkg.go.dev/github.com/balramadan/distlimit)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**`distlimit`** is an ultra-high performance, distributed, pluggable rate-limiting library for Go. Engineered for microservices, high-concurrency APIs, and financial-grade applications requiring nanosecond-level execution speeds, zero memory allocation, and multi-tier failover capabilities.

---

## 🚀 Why `distlimit`?

Most Go rate-limiting libraries force you into a single algorithm, lock global mutexes during background cleanup, or collapse when Redis goes down. **`distlimit`** solves these architectural flaws with enterprise-grade resilience:

- **🔌 100% Pluggable Strategy Architecture:** Swap algorithms seamlessly without changing your storage driver or HTTP framework middleware.
- **⚡ Zero Allocation & Nanosecond Latency:** Memory evaluation runs in sub-100 nanoseconds with **`0 B/op`** memory overhead across all standard algorithms.
- **🔒 64-Sharded Memory Architecture:** Eliminates global lock contention during concurrent HTTP requests and background state cleanups.
- **🛡️ Half-Open Circuit Breaker (Hybrid Driver):** Automatic failover from Redis to In-Memory with single-request probing to prevent _Thundering Herd_ spikes upon Redis recovery.
- **🔐 Security-First Middlewares:** Built-in protection against **IP Spoofing** via CIDR-validated `WithTrustedProxies` headers inspection.
- **🌐 Native Redis Cluster Safety:** Enforces **Redis Hash Tags `{}`** to prevent `CROSSSLOT` cluster routing errors.

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

### 1. Basic In-Memory Limiter with Sliding Window Counter

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

### 2. Secure Web Framework Middleware (Fiber / Gin / Echo / net/http)

Protect your HTTP endpoints with built-in Anti-IP Spoofing protection using `WithTrustedProxies`:

```go
package main

import (
	"context"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm/tokenbucket"
	"github.com/balramadan/distlimit/driver/memory"
	distlimitfiber "github.com/balramadan/distlimit/middleware/fiber"
	"github.com/gofiber/fiber/v3"
)

func main() {
	memDriver := memory.New(5 * time.Minute)
	defer memDriver.Close(context.Background())

	limiter, _ := distlimit.New(
		memDriver,
		distlimit.WithLimit(100),
		distlimit.WithWindow(1*time.Minute),
		distlimit.WithAlgorithm(tokenbucket.New()),
	)

	app := fiber.New()

	// Enable Middleware with Strict Trusted Proxies (Cloudflare / Nginx Subnet)
	app.Use(distlimitfiber.New(
		limiter,
		distlimitfiber.WithTrustedProxies([]string{"10.0.0.0/8", "172.16.0.0/12"}),
	))

	app.Get("/api/data", func(c fiber.Ctx) error {
		return c.SendString("Hello World!")
	})

	app.Listen(":3000")
}

```

---

### 3. Resilient Dual-Tier Hybrid Driver (Redis Primary + Memory Fallback)

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
			log.Printf("[DISTLIMIT WARN] Redis Primary down, failing over to memory: %v", err)
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

## 📊 Performance & Benchmarks

Benchmarks executed on AMD 3020e Linux x86_64 (`go test -bench=. -benchmem ./algorithm/...`):

```
pkg: github.com/balramadan/distlimit/algorithm/*
cpu: AMD 3020e with Radeon Graphics

BenchmarkSlidingLog_EvaluateMemory-2         60,631,044    20.36 ns/op    0 B/op    0 allocs/op
BenchmarkTokenBucket_EvaluateMemory-2        22,322,690    46.18 ns/op    0 B/op    0 allocs/op
BenchmarkLeakyBucket_EvaluateMemory-2        22,839,754    47.76 ns/op    0 B/op    0 allocs/op
BenchmarkFixedWindow_EvaluateMemory-2        14,618,232    77.27 ns/op    0 B/op    0 allocs/op
BenchmarkSlidingCounter_EvaluateMemory-2      8,917,860   129.10 ns/op    0 B/op    0 allocs/op

```

> **Key Takeaway:** All 5 algorithms achieve **zero memory allocations (`0 B/op`, `0 allocs/op`)** during in-memory evaluation, allowing your Go application to handle tens of millions of rate-check operations per second without GC pause overhead.

---

## 🛡️ Framework & Middleware Adapters

`distlimit` provides native, zero-dependency middleware adapters for all popular Go web frameworks:

- ⚡ [`middleware/fiber`](https://www.google.com/search?q=middleware/fiber) — Fiber v3
- 🍸 [`middleware/gin`](https://www.google.com/search?q=middleware/gin) — Gin Framework
- 🔊 [`middleware/echo`](https://www.google.com/search?q=middleware/echo) — Echo v4
- 🔊 [`middleware/echov5`](https://www.google.com/search?q=middleware/echov5) — Echo v5
- 🌐 [`middleware/nethttp`](https://www.google.com/search?q=middleware/nethttp) — Standard `net/http` & Chi
- 📡 [`middleware/grpc`](https://www.google.com/search?q=middleware/grpc) — gRPC Unary & Streaming Interceptors

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
