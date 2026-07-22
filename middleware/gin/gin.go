// Package gin provides rate limiting middleware for the Gin web framework.
// It features IP spoofing protection with trusted proxy CIDR filtering and custom key extraction.
package gin

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/balramadan/distlimit"
	"github.com/gin-gonic/gin"
)

// Config holds configuration parameters for the Gin rate limit middleware.
type Config struct {
	trustedProxies []string
	keyFunc        func(c *gin.Context) string
}

// Option configures functional parameters for the Gin rate limit middleware.
type Option func(*Config)

// WithTrustedProxies configures a list of trusted proxy IPs or CIDR networks for safely parsing X-Forwarded-For headers.
func WithTrustedProxies(proxies []string) Option {
	return func(cfg *Config) {
		cfg.trustedProxies = proxies
	}
}

// WithKeyFunc configures a custom key extraction function based on gin.Context.
func WithKeyFunc(fn func(c *gin.Context) string) Option {
	return func(cfg *Config) {
		cfg.keyFunc = fn
	}
}

// ExtractClientIP securely extracts the client's real IP address from a Gin request context.
// Header values (X-Forwarded-For, X-Real-IP) are parsed only if RemoteAddr originates from a trusted proxy.
func ExtractClientIP(c *gin.Context, trustedProxies []string) string {
	remoteIP, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		remoteIP = c.Request.RemoteAddr
	}

	if len(trustedProxies) == 0 {
		return remoteIP
	}

	if isTrustedProxy(remoteIP, trustedProxies) {
		if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
		if xri := c.GetHeader("X-Real-IP"); xri != "" {
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

// New returns a gin.HandlerFunc middleware configured with the specified distlimit.Limiter and options.
func New(limiter *distlimit.Limiter, opts ...Option) gin.HandlerFunc {
	cfg := &Config{
		trustedProxies: []string{},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(c *gin.Context) {
		var key string
		if cfg.keyFunc != nil {
			key = cfg.keyFunc(c)
		} else {
			key = ExtractClientIP(c, cfg.trustedProxies)
		}

		res, err := limiter.AllowKey(c.Request.Context(), key)
		if err != nil {
			// Fail-Open Policy
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", strconv.FormatInt(res.Limit, 10))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))

		if !res.Allowed {
			c.Header("Retry-After", strconv.FormatInt(int64(res.ResetIn.Seconds()), 10))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Too Many Requests"})
			return
		}

		c.Next()
	}
}
