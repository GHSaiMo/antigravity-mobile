package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// AuthPolicy controls loopback-trust and AUTH_DISABLED. Zero value preserves
// historical local-dev behavior (loopback may administer; AUTH_DISABLED honored
// only when the process is actually listening on loopback with no tunnel).
type AuthPolicy struct {
	TunnelEnabled  bool
	ListenLoopback bool
}

type contextKey string

const (
	// DeviceContextKey is the context key for the authenticated PairedDevice.
	DeviceContextKey contextKey = "auth_device"
)

// ExtractToken retrieves the bearer token from the Authorization header,
// or from query parameters for WebSocket upgrade and media download routes where custom headers cannot be set.
func ExtractToken(r *http.Request) string {
	// 1. Authorization: Bearer <token>
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	// 2. Cookie (HttpOnly session cookie - C-1)
	if c, err := r.Cookie(DeviceCookieName); err == nil {
		if tok := strings.TrimSpace(c.Value); tok != "" {
			return tok
		}
	}

	// 3. [DEPRECATED / L-1] Query parameter (?auth_token=... or ?token=...)
	// Restricted strictly to:
	// - WebSocket upgrade requests (browsers cannot set headers on WebSocket connections)
	// - File/media raw download endpoints (e.g. <img> or file downloads where headers cannot be set)
	// Clients are strongly urged to migrate to short-lived WS tickets or HttpOnly Cookie.
	// Can be completely disabled via MULTIGRAVITY_DISABLE_QUERY_TOKEN=1 (M-4).
	if IsQueryTokenDisabled() {
		return ""
	}

	isWS := strings.Contains(strings.ToLower(r.Header.Get("Upgrade")), "websocket") ||
		r.URL.Path == "/connect-websocket" ||
		strings.HasPrefix(r.URL.Path, "/gateway/cascade/stream")
	isRawFile := strings.HasPrefix(r.URL.Path, "/api/v1/files/raw")
	if isWS || isRawFile {
		token := strings.TrimSpace(r.URL.Query().Get("auth_token"))
		if token == "" {
			token = strings.TrimSpace(r.URL.Query().Get("token"))
		}
		if token != "" {
			log.Printf("[DEPRECATED] client %s passed token in query param for %s; migrate to Authorization header, cookie or /api/v1/auth/ws-ticket", ExtractClientIP(r), r.URL.Path)
			return token
		}
	}

	return ""
}

// IsQueryTokenDisabled reports whether query string token authentication is disabled via environment variable (M-4).
func IsQueryTokenDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MULTIGRAVITY_DISABLE_QUERY_TOKEN")))
	if v == "" {
		v = strings.ToLower(strings.TrimSpace(os.Getenv("DISABLE_QUERY_TOKEN")))
	}
	return v == "1" || v == "true" || v == "yes"
}

const DeviceCookieName = "agy_dt"

// IsWhitelistedPath checks if a request path should bypass authentication.
func IsWhitelistedPath(path string) bool {
	// Root and standard static web files (Mobile & Desktop)
	if path == "/" || path == "/index.html" || path == "/manifest.json" ||
		path == "/sw.js" || path == "/style.css" || path == "/app.js" ||
		path == "/mermaid.min.js" || path == "/favicon.ico" ||
		path == "/zh-CN.js" || path == "/view-switcher.js" || path == "/view-switcher.css" ||
		path == "/main.js" || path == "/jetbox.css" || path == "/compiled_tailwind.css" ||
		path == "/prism_bundle.js" || path == "/diff_worker.js" || path == "/icon.png" ||
		strings.HasPrefix(path, "/symbols-icons/") ||
		strings.HasPrefix(path, "/c/") || path == "/history" {
		return true
	}

	// Web directory and static assets
	if strings.HasPrefix(path, "/web/") || strings.HasPrefix(path, "/icons/") {
		return true
	}

	// Auth and device management endpoints (handled by AuthHandler with its own permission checks)
	if path == "/api/v1/auth/pair" || path == "/api/v1/auth/unpair" || path == "/api/v1/auth/session" || path == "/api/v1/auth/ws-ticket" ||
		path == "/api/v1/devices" || path == "/api/v1/devices/" || strings.HasPrefix(path, "/api/v1/devices/") {
		return true
	}

	// Public health probe only. /gateway/status, cascade touch/invalidate require a device token.
	if path == "/healthz" || path == "/readyz" {
		return true
	}

	return false
}

// DeviceFromContext retrieves the authenticated PairedDevice from request context.
func DeviceFromContext(ctx context.Context) (*PairedDevice, bool) {
	dev, ok := ctx.Value(DeviceContextKey).(*PairedDevice)
	return dev, ok
}

// BearerToken extracts the raw token from Authorization: Bearer only (never query).
func BearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// ConstantTimeTokenEquals compares two tokens in constant time.
func ConstantTimeTokenEquals(got, want string) bool {
	if want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// AuthDisabledRequested reports whether MULTIGRAVITY_AUTH_DISABLED (or AUTH_DISABLED) is set in the environment.
func AuthDisabledRequested() bool {
	v := os.Getenv("MULTIGRAVITY_AUTH_DISABLED")
	if v == "" {
		v = os.Getenv("AUTH_DISABLED")
	}
	return v == "true" || v == "1"
}

// AuthMiddleware creates an HTTP middleware that verifies device authentication.
func AuthMiddleware(store *AuthStore, next http.Handler) http.Handler {
	return AuthMiddlewareWithPolicy(store, next, AuthPolicy{})
}

// AuthMiddlewareWithPolicy is AuthMiddleware with explicit loopback/tunnel policy.
func AuthMiddlewareWithPolicy(store *AuthStore, next http.Handler, policy AuthPolicy) http.Handler {
	// AUTH_DISABLED only bypasses auth for genuine loopback listeners with no tunnel.
	// RemoteAddr==127.0.0.1 is not sufficient: FRP/SSH -L make internet clients look local.
	authDisabled := AuthDisabledRequested()
	effectiveDisabled := authDisabled && !policy.TunnelEnabled && policy.ListenLoopback
	if authDisabled && !effectiveDisabled {
		log.Println("⚠️  AUTH_DISABLED ignored: tunnel is enabled or gateway is not loopback-only")
	}
	if effectiveDisabled {
		log.Println("⚠️⚠️⚠️  WARNING: AUTH_DISABLED is set — authentication is bypassed for LOOPBACK requests only ⚠️⚠️⚠️")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if effectiveDisabled && IsLoopbackAddr(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path

		// Whitelisted paths bypass authentication
		if IsWhitelistedPath(path) {
			next.ServeHTTP(w, r)
			return
		}

		var device *PairedDevice
		var ok bool

		// S9: Support short-lived one-time ticket for WebSocket and raw file requests
		// to avoid putting long-lived device tokens into URLs/query strings.
		isWS := strings.Contains(strings.ToLower(r.Header.Get("Upgrade")), "websocket") ||
			r.URL.Path == "/connect-websocket" ||
			strings.HasPrefix(r.URL.Path, "/gateway/cascade/stream")
		isRawFile := strings.HasPrefix(r.URL.Path, "/api/v1/files/raw")

		if (isWS || isRawFile) && r.URL.Query().Get("ticket") != "" {
			ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
			device, ok = store.ValidateWSTicket(ticket)
		}

		if !ok || device == nil {
			token := ExtractToken(r)
			if token == "" {
				log.Printf("[AUDIT:AUTH_FAILURE] reason=missing_token ip=%s path=%s", ExtractClientIP(r), r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "unauthorized: missing authentication token",
				})
				return
			}

			if adminTok := GetAdminToken(); adminTok != "" && ConstantTimeTokenEquals(token, adminTok) {
				device = &PairedDevice{
					DeviceID:   "admin-local",
					DeviceName: "Local Administrator",
					Platform:   "macos",
					CreatedAt:  time.Now(),
					LastSeenAt: time.Now(),
					LastSeenIP: ExtractClientIP(r),
				}
				ok = true
			} else {
				device, ok = store.ValidateToken(token)
			}
		}

		if !ok || device == nil {
			log.Printf("[AUDIT:AUTH_FAILURE] reason=invalid_or_revoked_token ip=%s path=%s", ExtractClientIP(r), r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "unauthorized: invalid or revoked token",
			})
			return
		}

		// P5: non-blocking channel send to background worker — no goroutine spawn per request
		store.EnqueueLastSeen(device.DeviceID, ExtractClientIP(r))

		// Attach authenticated device to request context
		ctx := context.WithValue(r.Context(), DeviceContextKey, device)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type staticHeaderItem struct {
	key string
	val string
}

var defaultSecurityHeaders = []staticHeaderItem{
	{"X-Content-Type-Options", "nosniff"},
	{"X-Frame-Options", "DENY"},
	{"Referrer-Policy", "strict-origin-when-cross-origin"},
	{"Permissions-Policy", "camera=(), microphone=(), geolocation=()"},
	// S4: 'unsafe-inline' removed; explicit SHA-256 hashes of known static scripts.
	{"Content-Security-Policy", "default-src 'self'; script-src 'self' 'sha256-Xk1+itJeFRvwjzt1EHd9xXCB+HwHPF80YyAScWAl3Q8=' 'sha256-YbM1pG3wWnzhyYN49g5fPnen+2CKEFaZfopkkwSpNtY=' 'sha256-XKqsbto5R82BOuH6aUtFveBZbz4d08CZ8KPyGnkeoaY='; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'"},
}

const defaultHSTSValue = "max-age=31536000; includeSubDomains"

// SecurityHeadersMiddleware injects defensive HTTP security response headers.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		for i := 0; i < len(defaultSecurityHeaders); i++ {
			h.Set(defaultSecurityHeaders[i].key, defaultSecurityHeaders[i].val)
		}
		isSSL := r.TLS != nil ||
			strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") ||
			os.Getenv("MULTIGRAVITY_SSL") == "1" ||
			strings.EqualFold(os.Getenv("MULTIGRAVITY_SSL"), "true")
		if isSSL {
			h.Set("Strict-Transport-Security", defaultHSTSValue)
		}
		next.ServeHTTP(w, r)
	})
}

// MaxBytesMiddleware wraps incoming request bodies with http.MaxBytesReader to mitigate OOM / body flooding attacks (L-4).
func MaxBytesMiddleware(maxBytes int64, next http.Handler) http.Handler {
	if maxBytes <= 0 {
		maxBytes = 64 * 1024 * 1024 // 64MB default safety limit
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

