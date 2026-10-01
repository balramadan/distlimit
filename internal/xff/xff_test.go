package xff

import (
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name     string
		remote   string
		xff      string
		xri      string
		trusted  []string
		expected string
	}{
		{
			name:     "Direct connection without trusted proxy",
			remote:   "203.0.113.10:12345",
			xff:      "198.51.100.1",
			trusted:  nil,
			expected: "203.0.113.10",
		},
		{
			name:     "Direct connection not in trusted list",
			remote:   "203.0.113.10",
			xff:      "198.51.100.1",
			trusted:  []string{"10.0.0.0/8"},
			expected: "203.0.113.10",
		},
		{
			name:     "Spoofed leftmost IP (Core Security Test)",
			remote:   "10.0.0.1",
			xff:      "9.9.9.9, 203.0.113.50",
			trusted:  []string{"10.0.0.0/8"},
			expected: "203.0.113.50",
		},
		{
			name:     "Multi-hop trusted proxy chain",
			remote:   "10.0.0.1:54321",
			xff:      "9.9.9.9, 203.0.113.50, 10.0.0.2, 10.0.0.3",
			trusted:  []string{"10.0.0.0/8"},
			expected: "203.0.113.50",
		},
		{
			name:     "IP with port in XFF",
			remote:   "10.0.0.1",
			xff:      "203.0.113.50:8080",
			trusted:  []string{"10.0.0.0/8"},
			expected: "203.0.113.50",
		},
		{
			name:     "IPv6 with brackets and port",
			remote:   "10.0.0.1",
			xff:      "[2001:db8::1]:9000",
			trusted:  []string{"10.0.0.0/8"},
			expected: "2001:db8::1",
		},
		{
			name:     "Malformed and garbage entries in XFF skipped",
			remote:   "10.0.0.1",
			xff:      "203.0.113.50, unknown, <script>, 10.0.0.2",
			trusted:  []string{"10.0.0.0/8"},
			expected: "203.0.113.50",
		},
		{
			name:     "Fallback to X-Real-IP when XFF is empty",
			remote:   "10.0.0.1",
			xff:      "",
			xri:      "198.51.100.20",
			trusted:  []string{"10.0.0.0/8"},
			expected: "198.51.100.20",
		},
		{
			name:     "X-Real-IP with port",
			remote:   "10.0.0.1",
			xff:      "",
			xri:      "198.51.100.20:443",
			trusted:  []string{"10.0.0.0/8"},
			expected: "198.51.100.20",
		},
		{
			name:     "X-Real-IP is also a trusted proxy -> fallback to remote",
			remote:   "10.0.0.1",
			xff:      "",
			xri:      "10.0.0.5",
			trusted:  []string{"10.0.0.0/8"},
			expected: "10.0.0.1",
		},
		{
			name:     "All XFF entries are trusted proxies -> fallback to remote",
			remote:   "10.0.0.1",
			xff:      "10.0.0.2, 10.0.0.3",
			trusted:  []string{"10.0.0.0/8"},
			expected: "10.0.0.1",
		},
		{
			name:     "Exact match in trusted proxy list",
			remote:   "192.168.1.100",
			xff:      "203.0.113.25",
			trusted:  []string{"192.168.1.100"},
			expected: "203.0.113.25",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClientIP(tc.remote, tc.xff, tc.xri, tc.trusted)
			if got != tc.expected {
				t.Errorf("ClientIP(%q, %q, %q, %v) = %q; want %q",
					tc.remote, tc.xff, tc.xri, tc.trusted, got, tc.expected)
			}
		})
	}
}

func TestIsTrustedProxy(t *testing.T) {
	trusted := []string{"10.0.0.0/8", "192.168.1.50", "2001:db8::/32"}

	tests := []struct {
		ip       string
		expected bool
	}{
		{"10.1.2.3", true},
		{"10.1.2.3:8080", true},
		{"192.168.1.50", true},
		{"192.168.1.51", false},
		{"2001:db8::1", true},
		{"[2001:db8::1]:443", true},
		{"8.8.8.8", false},
		{"invalid-ip", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.ip, func(t *testing.T) {
			got := IsTrustedProxy(tc.ip, trusted)
			if got != tc.expected {
				t.Errorf("IsTrustedProxy(%q) = %v; want %v", tc.ip, got, tc.expected)
			}
		})
	}
}
