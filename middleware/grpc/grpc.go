// Package grpc provides gRPC server interceptor middleware for rate limiting.
// It supports unary and streaming RPC interceptors with IP spoofing protection and context metadata key extraction.
package grpc

import (
	"context"
	"net"
	"strings"

	"github.com/balramadan/distlimit"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// KeyExtractor is a function type for extracting a rate limit key from a gRPC context.Context.
type KeyExtractor func(ctx context.Context) string

type config struct {
	trustedProxies []string
	keyExtractor   KeyExtractor
}

// Option configures functional parameters for the gRPC rate limit interceptors.
type Option func(*config)

// WithTrustedProxies configures a list of trusted proxy IPs or CIDR networks for safely parsing x-forwarded-for metadata.
func WithTrustedProxies(proxies []string) Option {
	return func(cfg *config) {
		cfg.trustedProxies = proxies
	}
}

// WithKeyExtractor configures a custom key extraction strategy from gRPC context.
func WithKeyExtractor(fn KeyExtractor) Option {
	return func(cfg *config) {
		cfg.keyExtractor = fn
	}
}

// ExtractPeerIP securely extracts the client's peer network address from the gRPC context & incoming metadata.
// Header metadata (x-forwarded-for, x-real-ip) is parsed only if the peer IP originates from a trusted proxy.
func ExtractPeerIP(ctx context.Context, trustedProxies []string) string {
	var remoteIP string

	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		host, _, err := net.SplitHostPort(p.Addr.String())
		if err != nil {
			remoteIP = p.Addr.String()
		} else {
			remoteIP = host
		}
	} else {
		return "unknown_peer"
	}

	if len(trustedProxies) == 0 {
		return remoteIP
	}

	if isTrustedProxy(remoteIP, trustedProxies) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if xff := md.Get("x-forwarded-for"); len(xff) > 0 && xff[0] != "" {
				parts := strings.Split(xff[0], ",")
				return strings.TrimSpace(parts[0])
			}
			if xri := md.Get("x-real-ip"); len(xri) > 0 && xri[0] != "" {
				return strings.TrimSpace(xri[0])
			}
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

// UnaryServerInterceptor returns a grpc.UnaryServerInterceptor that enforces rate limiting on incoming unary gRPC calls.
// If the rate limit is exceeded, it returns a status error with code codes.ResourceExhausted.
// If an internal rate limiter error occurs, it follows a fail-open policy and allows the request to proceed.
func UnaryServerInterceptor(limiter *distlimit.Limiter, opts ...Option) grpc.UnaryServerInterceptor {
	cfg := &config{
		trustedProxies: []string{},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		var key string
		if cfg.keyExtractor != nil {
			key = cfg.keyExtractor(ctx)
		} else {
			key = ExtractPeerIP(ctx, cfg.trustedProxies)
		}

		res, err := limiter.AllowKey(ctx, key)
		if err != nil {
			// Fail-Open Policy
			return handler(ctx, req)
		}

		if !res.Allowed {
			resetSec := int64(res.ResetIn.Seconds())
			if resetSec < 1 && res.ResetIn > 0 {
				resetSec = 1
			}

			return nil, status.Errorf(
				codes.ResourceExhausted,
				"rate limit exceeded for method %s: try again in %d seconds",
				info.FullMethod,
				resetSec,
			)
		}

		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a grpc.StreamServerInterceptor that enforces rate limiting on incoming streaming gRPC calls.
// If the rate limit is exceeded, it rejects stream establishment with a codes.ResourceExhausted status error.
// If an internal rate limiter error occurs, it follows a fail-open policy and allows the stream to proceed.
func StreamServerInterceptor(limiter *distlimit.Limiter, opts ...Option) grpc.StreamServerInterceptor {
	cfg := &config{
		trustedProxies: []string{},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()

		var key string
		if cfg.keyExtractor != nil {
			key = cfg.keyExtractor(ctx)
		} else {
			key = ExtractPeerIP(ctx, cfg.trustedProxies)
		}

		res, err := limiter.AllowKey(ctx, key)
		if err != nil {
			// Fail-Open Policy
			return handler(srv, ss)
		}

		if !res.Allowed {
			resetSec := int64(res.ResetIn.Seconds())
			if resetSec < 1 && res.ResetIn > 0 {
				resetSec = 1
			}

			return status.Errorf(
				codes.ResourceExhausted,
				"rate limit exceeded for stream %s: try again in %d seconds",
				info.FullMethod,
				resetSec,
			)
		}

		return handler(srv, ss)
	}
}
