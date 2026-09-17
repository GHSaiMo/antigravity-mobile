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

	// 2. Query parameter (?auth_token=... or ?token=...)
	// Restricted strictly to:
	// - WebSocket upgrade requests (browsers cannot set headers on WebSocket connections)
	// - File/media raw download endpoints (e.g. <img> or file downloads where headers cannot be set)
	isWS := strings.Contains(strings.ToLower(r.Header.Get("Upgrade")), "websocket") ||
		r.URL.Path == "/connect-websocket" ||
		strings.HasPrefix(r.URL.Path, "/gateway/cascade/stream")
	isRawFile := strings.HasPrefix(r.URL.Path, "/api/v1/files/raw")
	if isWS || isRawFile {
		if token := r.URL.Query().Get("auth_token"); token != "" {
			return strings.TrimSpace(token)
		}
		if token := r.URL.Query().Get("token"); token != "" {
			return strings.TrimSpace(token)
		}
	}

	if c, err := r.Cookie(DeviceCookieName); err == nil {
		if tok := strings.TrimSpace(c.Value); tok != "" {
			return tok
		}
	}

	return ""
}

const DeviceCookieName = "agy_dt"

// IsWhitelistedPath checks if a request path should bypass authentication.
func IsWhitelistedPath(path string) bool {
	// Root and standard static web files
	if path == "/" || path == "/index.html" || path == "/manifest.json" ||
		path == "/sw.js" || path == "/style.css" || path == "/app.js" ||
		path == "/mermaid.min.js" || path == "/favicon.ico" {
		return true
	}

	// Web directory and static assets
	if strings.HasPrefix(path, "/web/") || strings.HasPrefix(path, "/icons/") {
		return true
	}

	// Auth and device management endpoints (handled by AuthHandler with its own permission checks)
	if path == "/api/v1/auth/pair" || path == "/api/v1/auth/session" || path == "/api/v1/auth/ws-ticket" ||
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

// AuthDisabledRequested reports whether AUTH_DISABLED is set in the environment.
func AuthDisabledRequested() bool {
	v := os.Getenv("AUTH_DISABLED")
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

// SecurityHeadersMiddleware injects defensive HTTP security response headers.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// S4: 'unsafe-inline' removed; replace with explicit SHA-256 hashes of known static scripts.
		// Regenerate hashes with: openssl dgst -sha256 -binary <file> | base64
		// app.js, mermaid.min.js, sw.js hashes must be updated whenever those files change.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'sha256-Xk1+itJeFRvwjzt1EHd9xXCB+HwHPF80YyAScWAl3Q8=' 'sha256-YbM1pG3wWnzhyYN49g5fPnen+2CKEFaZfopkkwSpNtY=' 'sha256-XKqsbto5R82BOuH6aUtFveBZbz4d08CZ8KPyGnkeoaY='; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}
