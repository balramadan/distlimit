// Package fiber provides rate limiting middleware for the Fiber v3 web framework.
// It features IP spoofing protection with trusted proxy CIDR filtering and custom key extraction.
package fiber

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/balramadan/distlimit"
	"github.com/gofiber/fiber/v3"
)

// Config holds configuration parameters for the Fiber rate limit middleware.
type Config struct {
	trustedProxies []string
	keyFunc        func(c fiber.Ctx) string
}

// Option configures functional parameters for the Fiber rate limit middleware.
type Option func(*Config)

// WithTrustedProxies configures a list of trusted proxy IPs or CIDR networks for safely parsing X-Forwarded-For headers.
func WithTrustedProxies(proxies []string) Option {
	return func(cfg *Config) {
		cfg.trustedProxies = proxies
	}
}

// WithKeyFunc configures a custom key extraction function based on fiber.Ctx.
func WithKeyFunc(fn func(c fiber.Ctx) string) Option {
	return func(cfg *Config) {
		cfg.keyFunc = fn
	}
}

// ExtractClientIP securely extracts the client's real IP address from a Fiber v3 fiber.Ctx.
// Header values (X-Forwarded-For, X-Real-IP) are parsed only if c.IP() originates from a trusted proxy.
func ExtractClientIP(c fiber.Ctx, trustedProxies []string) string {
	remoteIP := c.IP()

	if len(trustedProxies) == 0 {
		return remoteIP
	}

	if isTrustedProxy(remoteIP, trustedProxies) {
		if xff := c.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
		if xri := c.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	return remoteIP
}

func isTrustedProxy(ipStr string, trustedProxies []string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, cidr := range trustedProxies {
		if cidr == ipStr {
			return true
		}
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil && ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// New returns a fiber.Handler middleware for Fiber v3 configured with the specified distlimit.Limiter and options.
func New(limiter *distlimit.Limiter, opts ...Option) fiber.Handler {
	cfg := &Config{
		trustedProxies: []string{},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(c fiber.Ctx) error {
		var key string
		if cfg.keyFunc != nil {
			key = cfg.keyFunc(c)
		} else {
			key = ExtractClientIP(c, cfg.trustedProxies)
		}

		res, err := limiter.AllowKey(c.Context(), key)
		if err != nil {
			// Fail-Open Strategy
			return c.Next()
		}

		resetSec := int64(res.ResetIn.Seconds())
		if resetSec < 1 && res.ResetIn > 0 {
			resetSec = 1
		}

		c.Set("RateLimit-Limit", strconv.FormatInt(res.Limit, 10))
		c.Set("RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
		c.Set("RateLimit-Reset", strconv.FormatInt(resetSec, 10))

		c.Set("X-RateLimit-Limit", strconv.FormatInt(res.Limit, 10))
		c.Set("X-RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
		c.Set("X-RateLimit-Reset", strconv.FormatInt(resetSec, 10))

		if !res.Allowed {
			c.Set("Retry-After", strconv.FormatInt(resetSec, 10))
			return c.Status(http.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Too Many Requests",
			})
		}

		return c.Next()
	}
}
