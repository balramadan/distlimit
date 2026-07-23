// Package echo provides rate limiting middleware for the Echo v4 web framework.
// It features IP spoofing protection with trusted proxy CIDR filtering and custom key extraction.
package echo

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/balramadan/distlimit"
	"github.com/labstack/echo/v4"
)

type Config struct {
	trustedProxies []string
	keyFunc        func(c echo.Context) string
}

type Option func(*Config)

// WithTrustedProxies menentukan daftar IP/CIDR proxy yang dipercayai untuk membaca X-Forwarded-For.
func WithTrustedProxies(proxies []string) Option {
	return func(cfg *Config) {
		cfg.trustedProxies = proxies
	}
}

// WithKeyFunc kustomisasi fungsi penentuan key rate limit berdasarkan Echo Context.
func WithKeyFunc(fn func(c echo.Context) string) Option {
	return func(cfg *Config) {
		cfg.keyFunc = fn
	}
}

// ExtractClientIP mengekstrak IP client secara aman dari Echo Context.
func ExtractClientIP(c echo.Context, trustedProxies []string) string {
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

// New mengembalikan Echo v4 Middleware Handler.
func New(limiter *distlimit.Limiter, opts ...Option) echo.MiddlewareFunc {
	cfg := &Config{
		trustedProxies: []string{},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			var key string
			if cfg.keyFunc != nil {
				key = cfg.keyFunc(c)
			} else {
				key = ExtractClientIP(c, cfg.trustedProxies)
			}

			res, err := limiter.AllowKey(c.Request().Context(), key)
			if err != nil {
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
