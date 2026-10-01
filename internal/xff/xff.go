package xff

import (
	"net"
	"slices"
	"strings"
)

// ClientIP extracts the real client IP address from remoteAddr, X-Forwarded-For, and X-Real-IP
// using the rightmost non-trusted proxy traversal algorithm.
func ClientIP(remoteAddr, xff, xri string, trusted []string) string {
	remoteIP, _ := cleanIP(remoteAddr)
	if remoteIP == "" {
		remoteIP = remoteAddr
	}

	// If no trusted proxies configured or remoteAddr is not a trusted proxy,
	// never trust forwarded headers.
	if len(trusted) == 0 || !IsTrustedProxy(remoteIP, trusted) {
		return remoteIP
	}

	// Traverse X-Forwarded-For from right to left
	if xff != "" {
		parts := strings.Split(xff, ",")
		for _, v := range slices.Backward(parts) {
			ipStr, parsedIP := cleanIP(v)
			if ipStr == "" || parsedIP == nil {
				continue // ignore empty or malformed entries
			}

			if !isTrustedParsedIP(parsedIP, ipStr, trusted) {
				return ipStr
			}
		}
	}

	// Fallback to X-Real-IP if X-Forwarded-For produced no untrusted client IP
	if xri != "" {
		ipStr, parsedIP := cleanIP(xri)
		if parsedIP != nil && !isTrustedParsedIP(parsedIP, ipStr, trusted) {
			return ipStr
		}
	}

	return remoteIP
}

// cleanIP trims whitespace, strips any port (and IPv6 brackets), and returns the parsed IP.
func cleanIP(s string) (string, net.IP) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = strings.Trim(host, "[]")
	} else {
		s = strings.Trim(s, "[]")
	}
	ip := net.ParseIP(s)
	return s, ip
}

// IsTrustedProxy checks if an IP string matches any trusted IP or CIDR block.
func IsTrustedProxy(ipStr string, trusted []string) bool {
	ipStr, ip := cleanIP(ipStr)
	if ip == nil || len(trusted) == 0 {
		return false
	}
	return isTrustedParsedIP(ip, ipStr, trusted)
}

func isTrustedParsedIP(ip net.IP, ipStr string, trusted []string) bool {
	for _, t := range trusted {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if t == ipStr {
			return true
		}
		if _, ipNet, err := net.ParseCIDR(t); err == nil {
			if ipNet.Contains(ip) {
				return true
			}
		}
	}
	return false
}

