// Package echov5 provides rate limiting middleware for the Echo v5 web framework.
// It features IP spoofing protection with trusted proxy CIDR filtering and custom key extraction.
package echov5

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/metrics"
	"github.com/labstack/echo/v5"
)

// Config holds configuration parameters for the Echo v5 rate limit middleware.
type Config struct {
	trustedProxies     []string
	keyFunc            func(c *echo.Context) string
	enableRouteLabeling bool
}

// Option configures functional parameters for the Echo v5 rate limit middleware.
type Option func(*Config)

// WithTrustedProxies configures a list of trusted proxy IPs or CIDR networks for safely parsing X-Forwarded-For headers.
func WithTrustedProxies(proxies []string) Option {
	return func(cfg *Config) {
		cfg.trustedProxies = proxies
	}
}

// WithKeyFunc configures a custom key extraction function based on the Echo v5 *echo.Context.
func WithKeyFunc(fn func(c *echo.Context) string) Option {
	return func(cfg *Config) {
		cfg.keyFunc = fn
	}
}

// WithRouteLabeling enables passing the Echo v5 route path pattern as the route label in telemetry events.
func WithRouteLabeling(enable bool) Option {
	return func(cfg *Config) {
		cfg.enableRouteLabeling = enable
	}
}

// ExtractClientIP securely extracts the client's real IP address from an Echo v5 *echo.Context.
// Header values (X-Forwarded-For, X-Real-IP) are parsed only if RemoteAddr originates from a trusted proxy.
func ExtractClientIP(c *echo.Context, trustedProxies []string) string {
	remoteIP, _, err := net.SplitHostPort(c.Request().RemoteAddr)
	if err != nil {
		remoteIP = c.Request().RemoteAddr
	}

	if len(trustedProxies) == 0 {
		return remoteIP
	}

	if isTrustedProxy(remoteIP, trustedProxies) {
		if xff := c.Request().Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
		if xri := c.Request().Header.Get("X-Real-IP"); xri != "" {
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

// New returns an echo.MiddlewareFunc compatible with Echo v5, configured with the specified distlimit.Limiter and options.
func New(limiter *distlimit.Limiter, opts ...Option) echo.MiddlewareFunc {
	cfg := &Config{
		trustedProxies: []string{},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := c.Request().Context()
			if cfg.enableRouteLabeling {
				path := c.Path()
				if path == "" {
					path = c.Request().URL.Path
				}
				ctx = metrics.ContextWithRoute(ctx, path)
				c.SetRequest(c.Request().WithContext(ctx))
			}

			var key string
			if cfg.keyFunc != nil {
				key = cfg.keyFunc(c)
			} else {
				key = ExtractClientIP(c, cfg.trustedProxies)
			}

			res, err := limiter.AllowKey(ctx, key)
			if err != nil {
				// Fail-Open Strategy
				return next(c)
			}

			resetSec := int64(res.ResetIn.Seconds())
			if resetSec < 1 && res.ResetIn > 0 {
				resetSec = 1
			}

			c.Response().Header().Set("RateLimit-Limit", strconv.FormatInt(res.Limit, 10))
			c.Response().Header().Set("RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
			c.Response().Header().Set("RateLimit-Reset", strconv.FormatInt(resetSec, 10))

			c.Response().Header().Set("X-RateLimit-Limit", strconv.FormatInt(res.Limit, 10))
			c.Response().Header().Set("X-RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
			c.Response().Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetSec, 10))

			if !res.Allowed {
				c.Response().Header().Set("Retry-After", strconv.FormatInt(resetSec, 10))
				return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "Too Many Requests"})
			}

			return next(c)
		}
	}
}
