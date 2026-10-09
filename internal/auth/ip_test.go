package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		expectedIP string
	}{
		{
			name:       "Direct Public RemoteAddr without headers",
			remoteAddr: "117.136.12.34:54321",
			headers:    nil,
			expectedIP: "117.136.12.34",
		},
		{
			name:       "Direct IPv6 RemoteAddr without headers",
			remoteAddr: "[2409:8a00:1234::56]:49152",
			headers:    nil,
			expectedIP: "2409:8a00:1234::56",
		},
		{
			name:       "Loopback with CF-Connecting-IP",
			remoteAddr: "127.0.0.1:58900",
			headers: map[string]string{
				"CF-Connecting-IP": "203.0.113.195",
			},
			expectedIP: "203.0.113.195",
		},
		{
			name:       "Loopback with X-Real-IP",
			remoteAddr: "127.0.0.1:58900",
			headers: map[string]string{
				"X-Real-IP": "198.51.100.5",
			},
			expectedIP: "198.51.100.5",
		},
		{
			// The last hop is appended by our own proxy; earlier entries are client-supplied.
			name:       "Loopback with X-Forwarded-For multi-hop",
			remoteAddr: "127.0.0.1:58900",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.10, 198.51.100.1",
			},
			expectedIP: "198.51.100.1",
		},
		{
			name:       "Public RemoteAddr ignores spoofed X-Forwarded-For",
			remoteAddr: "117.136.12.34:54321",
			headers: map[string]string{
				"X-Forwarded-For": "1.2.3.4",
			},
			expectedIP: "117.136.12.34",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			req.RemoteAddr = tc.remoteAddr
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			ip := ExtractClientIP(req)
			if ip != tc.expectedIP {
				t.Errorf("expected %s, got %s", tc.expectedIP, ip)
			}
		})
	}
}

func TestRateLimitKeyIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"127.0.0.1:12345", "loopback"},
		{"[::1]:58900", "loopback"},
		{"192.168.1.50:8080", "192.168.1.50"},
		{"10.0.0.1", "10.0.0.1"},
		{"[2001:db8:abcd:0012:1:2:3:4]:1234", "2001:db8:abcd:12::/64"},
		{"2001:db8:abcd:0012:9:8:7:6", "2001:db8:abcd:12::/64"},
		{"invalid-host", "invalid-host"},
	}

	for _, tc := range tests {
		got := RateLimitKeyIP(tc.input)
		if got != tc.expected {
			t.Errorf("RateLimitKeyIP(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}


func TestExtractClientIP_ForwardedHeaderTrust(t *testing.T) {
	mk := func(remote string, h map[string]string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		for k, v := range h {
			r.Header.Set(k, v)
		}
		return r
	}
	// Non-loopback peers can never choose their IP.
	if got := ExtractClientIP(mk("192.168.1.9:1", map[string]string{"X-Forwarded-For": "1.2.3.4"})); got != "192.168.1.9" {
		t.Errorf("got %s", got)
	}
	// Cloudflare request: CF-Connecting-IP wins; client-supplied XFF/X-Real-IP are ignored.
	if got := ExtractClientIP(mk("127.0.0.1:1", map[string]string{"CF-Connecting-IP": "9.9.9.9", "CF-Ray": "x", "X-Real-IP": "6.6.6.6"})); got != "9.9.9.9" {
		t.Errorf("got %s", got)
	}
	if got := ExtractClientIP(mk("127.0.0.1:1", map[string]string{"CF-Ray": "x", "X-Forwarded-For": "6.6.6.6"})); got != "127.0.0.1" {
		t.Errorf("got %s", got)
	}
	// Local proxy: nearest hop (last XFF entry), not the client-supplied first one.
	if got := ExtractClientIP(mk("127.0.0.1:1", map[string]string{"X-Forwarded-For": "6.6.6.6, 8.8.8.8"})); got != "8.8.8.8" {
		t.Errorf("got %s", got)
	}
}
