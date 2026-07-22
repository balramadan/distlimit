// Package nethttp provides rate limiting middleware for Go's standard net/http package.
// It features IP spoofing protection with trusted proxy CIDR filtering and custom key extraction.
package nethttp

import (
	"net"
	"net/http"
	"strings"

	"github.com/balramadan/distlimit"
)

// Config holds configuration parameters for the net/http rate limit middleware.
type Config struct {
	trustedProxies []string
	keyFunc        distlimit.KeyFunc
}

// Option configures functional parameters for the net/http rate limit middleware.
type Option func(*Config)

// WithTrustedProxies configures a list of trusted proxy IPs or CIDR networks for safely parsing X-Forwarded-For headers.
func WithTrustedProxies(proxies []string) Option {
	return func(c *Config) {
		c.trustedProxies = proxies
	}
}

// WithKeyFunc configures a custom key extraction function from the request context.
func WithKeyFunc(fn distlimit.KeyFunc) Option {
	return func(c *Config) {
		c.keyFunc = fn
	}
}

// ExtractClientIP securely extracts the client's real IP address from an http.Request.
// Header values (X-Forwarded-For, X-Real-IP) are parsed only if RemoteAddr originates from a trusted proxy.
func ExtractClientIP(r *http.Request, trustedProxies []string) string {
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr
	}

	if len(trustedProxies) == 0 {
		return remoteIP
	}

	if isTrustedProxy(remoteIP, trustedProxies) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
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

// New returns a standard net/http middleware adapter function `func(http.Handler) http.Handler` configured with the specified distlimit.Limiter and options.
func New(limiter *distlimit.Limiter, opts ...Option) func(http.Handler) http.Handler {
	cfg := &Config{
		trustedProxies: []string{},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var key string
			if cfg.keyFunc != nil {
				key = cfg.keyFunc(r.Context())
			} else {
				key = ExtractClientIP(r, cfg.trustedProxies)
			}

			res, err := limiter.AllowKey(r.Context(), key)
			if err != nil {
				// Fail-Open Strategy
				next.ServeHTTP(w, r)
				return
			}

			// Set Rate Limit Headers
			w.Header().Set("X-RateLimit-Limit", strings.TrimSpace(http.StatusText(int(res.Limit))))
			w.Header().Set("X-RateLimit-Remaining", strings.TrimSpace(http.StatusText(int(res.Remaining))))

			if !res.Allowed {
				w.Header().Set("Retry-After", strings.TrimSpace(http.StatusText(int(res.ResetIn.Seconds()))))
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"Too Many Requests"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
