package nethttp

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/balramadan/distlimit"
)

type KeyExtractor func(r *http.Request) string

type config struct {
	keyExtractor KeyExtractor
	onExceeded   http.HandlerFunc
}

type Option func(*config)

func WithOnExceeded(handler http.HandlerFunc) Option {
	return func(cfg *config) {
		cfg.onExceeded = handler
	}
}

func DefaultIPExtractor(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func New(limiter *distlimit.Limiter, opts ...Option) func(http.Handler) http.Handler {
	cfg := &config{
		keyExtractor: DefaultIPExtractor,
		onExceeded: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"Too Many Requests","message":"Rate limit exceeded. Please try again later."}`))
		},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := cfg.keyExtractor(r)
			res, err := limiter.AllowKey(r.Context(), key)

			if err != nil {
				// Fail-Open Policy: Jika terjadi internal error pada rate limiter, izinkan request lewat agar app tidak crash
				next.ServeHTTP(w, r)
				return
			}

			resetSec := int64(res.ResetIn.Seconds())
			if resetSec < 1 && res.ResetIn > 0 {
				resetSec = 1
			}

			// 1. Injeksi Standard IETF RateLimit Headers
			w.Header().Set("RateLimit-Limit", strconv.FormatInt(res.Limit, 10))
			w.Header().Set("RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
			w.Header().Set("RateLimit-Reset", strconv.FormatInt(resetSec, 10))

			// 2. Injeksi Legacy X-RateLimit Headers (untuk kompatibilitas client lama)
			w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(res.Limit, 10))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetSec, 10))

			// 3. Jika Limit Terlampaui -> Berikan HTTP 429 + Retry-After Header
			if !res.Allowed {
				w.Header().Set("Retry-After", strconv.FormatInt(resetSec, 10))
				cfg.onExceeded(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
