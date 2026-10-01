# Changelog

All notable changes to the **`distlimit`** project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [v1.2.1] - 2026-10-01

### Security
- **IP Spoofing Vulnerability (CWE-345):** Fixed a security vulnerability where taking the leftmost element (`parts[0]`) of the `X-Forwarded-For` header allowed malicious clients to spoof their client IP and bypass rate limits. Replaced with an RFC-compliant **Rightmost Non-Trusted Proxy Traversal** algorithm.
- **Port Stripping & IPv6 Normalization:** Automatically strips ports (`host:port`) and brackets (`[2001:db8::1]:port`) across `X-Forwarded-For`, `X-Real-IP`, and `RemoteAddr` before evaluation.
- **Malformed String Sanitization:** Non-IP entries or malicious injection payloads inside `X-Forwarded-For` and `X-Real-IP` are safely skipped during traversal.

### Changed
- **Consolidated IP Extraction (`internal/xff`):** Centralized duplicate `ExtractClientIP` and `isTrustedProxy` logic from all 6 middleware packages (`nethttp`, `gin`, `fiber`, `echo`, `echov5`, `grpc`) into a single internal package.

---

## [v1.2.0] - 2026-07-27

### Added
- **Observability & Telemetry Engine (`metrics/`):**
  - Pluggable `metrics.Observer` interface and `metrics.Event` struct with $0\text{ B/op}$ overhead when disabled.
  - Native Prometheus integration (`metrics/prometheus`) tracking requests total, duration histogram, and hybrid fallbacks.
  - Native OpenTelemetry integration (`metrics/otel`) with MeterProvider metric instrumentation.
- **Lock-Free Dynamic Policy Engine:**
  - Dynamic limit updates via `limiter.UpdatePolicy(limit, window)` powered by lock-free `atomic.Pointer[Policy]`.
  - Multi-tenant tier resolution support through `PolicyResolver` interface and `WithPolicyResolver()`.
- **Middleware Route Labeling:**
  - Added `WithRouteLabeling(true)` across all 6 middleware adapters to record route patterns (e.g., `/api/v1/users/:id` or gRPC `FullMethod`) into metric labels.
- **Examples:** Added runnable sample applications in `examples/prometheus-metrics` and `examples/dynamic-policy`.

---

## [v1.1.1] - 2026-07-23

### Fixed
- **`nethttp` Header Formatting:** Replaced `http.StatusText()` with `strconv.FormatInt()` so rate limit headers output numerical values (`X-RateLimit-Limit: 100`).
- **Hybrid Driver Race Condition:** Converted `isDown` circuit breaker state transition to use atomic `CompareAndSwap` to prevent duplicate failure callbacks during concurrent outages.
- **`WithKeyFunc(nil)` Guard:** Added safety check to prevent nil pointer dereferences when an explicit `nil` key function is passed.

### Added
- **Manual Reset API:** Introduced `Limiter.ResetKey()` and `Driver.Reset()` across Memory, Redis, and Hybrid drivers.
- **IETF RFC 6585 Headers:** Added standardized `RateLimit-Limit`, `RateLimit-Remaining`, and `RateLimit-Reset` headers alongside legacy `X-RateLimit-*` headers across all HTTP adapters.
- **Empty Key Fallback:** Added automatic conversion of empty key strings to `"global"`.

---

## [v1.1.0] - 2026-07-23

### Added
- **5 Pluggable Rate-Limiting Algorithms:**
  - Token Bucket (burst friendly)
  - Leaky Bucket (traffic shaping)
  - Fixed Window (lightweight interval counters)
  - Sliding Window Log (100% exact precision)
  - Sliding Window Counter (weighted moving average approximation)
- **3 Pluggable Storage Drivers:**
  - In-Memory driver with 64-shard architecture to eliminate global lock contention.
  - Redis driver with Redis Cluster Hash Tags `{}` to prevent cross-slot routing errors.
  - Hybrid driver with half-open circuit breaker and single-request probe recovery.
- **6 Framework Middleware Adapters:**
  - Standard Go `net/http`
  - Gin (`gin-gonic/gin`)
  - Fiber v3 (`gofiber/fiber/v3`)
  - Echo v4 (`labstack/echo/v4`)
  - Echo v5 (`labstack/echo/v5`)
  - gRPC Unary & Streaming interceptors (`google.golang.org/grpc`)
- **Initial Trusted Proxy Filtering:** Added `WithTrustedProxies` option with CIDR subnet matching.

---

## [v1.0.1] - 2026-07-23

### Documentation
- Updated repository license links to reference local repository files.

---

## [v1.0.0] - 2026-07-23

### Added
- Initial public release of `distlimit` distributed rate-limiting library for Go.

[v1.2.1]: https://github.com/balramadan/distlimit/compare/v1.2.0...v1.2.1
[v1.2.0]: https://github.com/balramadan/distlimit/compare/v1.1.1...v1.2.0
[v1.1.1]: https://github.com/balramadan/distlimit/compare/v1.1.0...v1.1.1
[v1.1.0]: https://github.com/balramadan/distlimit/compare/v1.0.1...v1.1.0
[v1.0.1]: https://github.com/balramadan/distlimit/compare/v1.0.0...v1.0.1
[v1.0.0]: https://github.com/balramadan/distlimit/releases/tag/v1.0.0
