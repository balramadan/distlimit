package gin

import (
	"net/http"
	"strconv"

	"github.com/balramadan/distlimit"
	"github.com/gin-gonic/gin"
)

// KeyExtractor adalah fungsi untuk mengekstrak key dari gin.Context
type KeyExtractor func(c *gin.Context) string

type config struct {
	keyExtractor KeyExtractor
	onExceeded   gin.HandlerFunc
}

// Option adalah fungsi konfigurasi untuk Gin middleware.
type Option func(*config)

// WithKeyExtractor mengganti strategi ekstraksi key pada Gin.
func WithKeyExtractor(fn KeyExtractor) Option {
	return func(cfg *config) {
		cfg.keyExtractor = fn
	}
}

// WithOnExceeded mengganti handler kustom saat rate limit Gin terlampaui.
func WithOnExceeded(handler gin.HandlerFunc) Option {
	return func(cfg *config) {
		cfg.onExceeded = handler
	}
}

// DefaultIPExtractor menggunakan fungsi bawaan ClientIP() dari Gin
func DefaultIPExtractor(c *gin.Context) string {
	return c.ClientIP()
}

// New mengembalikan gin.HandlerFunc yang dapat langsung dipasang via router.Use()
func New(limiter *distlimit.Limiter, opts ...Option) gin.HandlerFunc {
	cfg := &config{
		keyExtractor: DefaultIPExtractor,
		onExceeded: func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "Too Many Requests",
				"message": "Rate limit exceeded. Please try again later.",
			})
		},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return func(c *gin.Context) {
		key := cfg.keyExtractor(c)
		res, err := limiter.AllowKey(c.Request.Context(), key)

		if err != nil {
			// Fail-Open Policy
			c.Next()
			return
		}

		resetSec := int64(res.ResetIn.Seconds())
		if resetSec < 1 && res.ResetIn > 0 {
			resetSec = 1
		}

		// 1. Injeksi Standard IETF RateLimit Headers
		c.Header("RateLimit-Limit", strconv.FormatInt(res.Limit, 10))
		c.Header("RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
		c.Header("RateLimit-Reset", strconv.FormatInt(resetSec, 10))

		// 2. Injeksi Legacy X-RateLimit Headers
		c.Header("X-RateLimit-Limit", strconv.FormatInt(res.Limit, 10))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetSec, 10))

		// 3. Jika Limit Terlampaui
		if !res.Allowed {
			c.Header("Retry-After", strconv.FormatInt(resetSec, 10))
			cfg.onExceeded(c)
			return
		}

		c.Next()
	}
}
