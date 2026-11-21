package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestImplicitTrustRequestOK(t *testing.T) {
	t.Setenv("DDNS_HOST", "mgy.public.example.com")
	cases := []struct {
		method, host, origin, secSite, secMode string
		want                                   bool
	}{
		{"GET", "127.0.0.1:58900", "", "", "", true},
		{"GET", "localhost:58900", "", "", "", true},
		{"GET", "192.168.1.5:58900", "", "", "", true},
		{"GET", "", "", "", "", false},                                         // empty Host rejected (HIGH-01)
		{"GET", "mgy.public.example.com:58900", "", "", "", false},             // DDNS_HOST never gets implicit trust (CRIT-01)
		{"GET", "evil.example.com:58900", "", "", "", false},                   // DNS rebinding
		{"POST", "127.0.0.1:58900", "", "", "", true},                          // native client, no Origin
		{"POST", "127.0.0.1:58900", "http://localhost:58900", "", "", true},    // local origin
		{"POST", "127.0.0.1:58900", "https://evil.example.com", "", "", false}, // CSRF
		{"POST", "127.0.0.1:58900", "null", "", "", false},
		{"GET", "127.0.0.1:58900", "https://evil.example.com", "", "", false},                // CSWSH / cross-origin GET blocked (HIGH-01)
		{"GET", "127.0.0.1:58900", "", "cross-site", "websocket", false},                     // cross-site WS blocked
		{"GET", "127.0.0.1:58900", "", "cross-site", "cors", false},                          // cross-site fetch blocked
		{"GET", "127.0.0.1:58900", "", "cross-site", "navigate", true},                       // top-level link navigation allowed
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "/x", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.secSite != "" {
			r.Header.Set("Sec-Fetch-Site", c.secSite)
		}
		if c.secMode != "" {
			r.Header.Set("Sec-Fetch-Mode", c.secMode)
		}
		if got := ImplicitTrustRequestOK(r); got != c.want {
			t.Errorf("%s host=%s origin=%q secSite=%q secMode=%q: got %v want %v", c.method, c.host, c.origin, c.secSite, c.secMode, got, c.want)
		}
	}
}

func TestMiddlewareRejectsRebindingAndCSRFOnLoopback(t *testing.T) {
	store, err := NewAuthStore(filepath.Join(t.TempDir(), "a.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := AuthMiddlewareWithPolicy(store, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), AuthPolicy{ListenLoopback: true})
	do := func(method, host, origin string) int {
		r := httptest.NewRequest(method, "/gateway/status", nil)
		r.RemoteAddr = "127.0.0.1:4000"
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if c := do("GET", "127.0.0.1:58900", ""); c != 200 {
		t.Errorf("plain loopback: %d", c)
	}
	if c := do("GET", "evil.example.com:58900", ""); c != 401 {
		t.Errorf("rebinding host: %d", c)
	}
	if c := do("POST", "127.0.0.1:58900", "https://evil.example.com"); c != 401 {
		t.Errorf("cross-site POST: %d", c)
	}
}

func TestCookieWriteCSRFOK(t *testing.T) {
	cases := []struct {
		name         string
		method       string
		host         string
		origin       string
		referer      string
		secFetchSite string
		want         bool
	}{
		{"safe GET always allowed", "GET", "gw.example.com", "https://evil.example.com", "", "cross-site", true},
		{"same-origin Origin allowed", "POST", "gw.example.com:58900", "https://gw.example.com:58900", "", "same-origin", true},
		{"same-origin Sec-Fetch-Site without Origin allowed", "POST", "gw.example.com", "", "", "same-origin", true},
		{"cross-site Sec-Fetch-Site rejected", "POST", "gw.example.com", "https://gw.example.com", "", "cross-site", false},
		{"same-site subdomain Sec-Fetch-Site rejected", "POST", "gw.example.com", "https://sub.example.com", "", "same-site", false},
		{"cross-origin Origin rejected", "POST", "gw.example.com", "https://evil.example.com", "", "", false},
		{"null Origin rejected", "POST", "gw.example.com", "null", "", "", false},
		{"port mismatch Origin rejected", "POST", "127.0.0.1:58900", "http://127.0.0.1:8080", "", "", false},
		{"same-origin Referer fallback allowed", "DELETE", "127.0.0.1:58900", "", "http://127.0.0.1:58900/settings", "", true},
		{"cross-origin Referer fallback rejected", "DELETE", "127.0.0.1:58900", "", "https://evil.example.com/page", "", false},
		{"non-browser caller without Origin Referer or Sec-Fetch-Site allowed", "POST", "gw.example.com", "", "", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/api/v1/cockpit/switch", nil)
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				r.Header.Set("Referer", tc.referer)
			}
			if tc.secFetchSite != "" {
				r.Header.Set("Sec-Fetch-Site", tc.secFetchSite)
			}
			if got := CookieWriteCSRFOK(r); got != tc.want {
				t.Errorf("CookieWriteCSRFOK() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCookieWSOriginOK(t *testing.T) {
	cases := []struct {
		name         string
		host         string
		origin       string
		secFetchSite string
		want         bool
	}{
		{"same-origin tunnel WS allowed", "victim89.jiuge.space", "https://victim89.jiuge.space", "same-origin", true},
		{"sibling subdomain same-site CSWSH rejected", "victim89.jiuge.space", "https://evil1234.jiuge.space", "same-site", false},
		{"sibling subdomain without Sec-Fetch-Site rejected", "victim89.jiuge.space", "https://evil1234.jiuge.space", "", false},
		{"cross-site Origin rejected", "127.0.0.1:58900", "https://evil.example.com", "cross-site", false},
		{"missing Origin on cookie WS rejected", "victim89.jiuge.space", "", "", false},
		{"null Origin rejected", "victim89.jiuge.space", "null", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/gateway/terminal/stream", nil)
			r.Host = tc.host
			r.Header.Set("Upgrade", "websocket")
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.secFetchSite != "" {
				r.Header.Set("Sec-Fetch-Site", tc.secFetchSite)
			}
			if got := CookieWSOriginOK(r); got != tc.want {
				t.Errorf("CookieWSOriginOK() = %v, want %v", got, tc.want)
			}
		})
	}
}
