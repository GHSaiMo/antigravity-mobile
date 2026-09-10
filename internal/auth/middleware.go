package auth

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

type contextKey string

const (
	// DeviceContextKey is the context key for the authenticated PairedDevice.
	DeviceContextKey contextKey = "auth_device"
)

// ExtractToken retrieves the bearer token from the Authorization header or query parameter.
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
	if token := r.URL.Query().Get("auth_token"); token != "" {
		return strings.TrimSpace(token)
	}
	if token := r.URL.Query().Get("token"); token != "" {
		return strings.TrimSpace(token)
	}

	return ""
}

// IsWhitelistedPath checks if a request path should bypass authentication.
func IsWhitelistedPath(path string) bool {
	// Root and standard static web files
	if path == "/" || path == "/index.html" || path == "/manifest.json" ||
		path == "/sw.js" || path == "/style.css" || path == "/app.js" ||
		path == "/favicon.ico" {
		return true
	}

	// Web directory and static assets
	if strings.HasPrefix(path, "/web/") || strings.HasPrefix(path, "/icons/") {
		return true
	}

	// Auth and device management endpoints (handled by AuthHandler with its own permission checks)
	if strings.HasPrefix(path, "/api/v1/auth/") || strings.HasPrefix(path, "/api/v1/devices") {
		return true
	}

	// Gateway basic status probe (allows health checks and connectivity testing)
	if path == "/gateway/status" {
		return true
	}

	return false
}

// DeviceFromContext retrieves the authenticated PairedDevice from request context.
func DeviceFromContext(ctx context.Context) (*PairedDevice, bool) {
	dev, ok := ctx.Value(DeviceContextKey).(*PairedDevice)
	return dev, ok
}

// AuthMiddleware creates an HTTP middleware that verifies device authentication.
func AuthMiddleware(store *AuthStore, next http.Handler) http.Handler {
	// AUTH_DISABLED only bypasses auth for loopback (localhost) requests.
	// Remote requests always require authentication regardless of this flag.
	authDisabled := os.Getenv("AUTH_DISABLED") == "true" || os.Getenv("AUTH_DISABLED") == "1"
	if authDisabled {
		log.Println("⚠️⚠️⚠️  WARNING: AUTH_DISABLED is set — authentication is bypassed for LOOPBACK requests only ⚠️⚠️⚠️")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authDisabled && isLoopbackAddr(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path

		// Whitelisted paths bypass authentication
		if IsWhitelistedPath(path) {
			next.ServeHTTP(w, r)
			return
		}

		token := ExtractToken(r)
		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "unauthorized: missing authentication token",
			})
			return
		}

		device, ok := store.ValidateToken(token)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "unauthorized: invalid or revoked token",
			})
			return
		}

		// Asynchronously update device last seen metadata
		go store.UpdateLastSeen(device.DeviceID, r.RemoteAddr)

		// Attach authenticated device to request context
		ctx := context.WithValue(r.Context(), DeviceContextKey, device)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isLoopbackAddr checks whether a remote address (host:port) is from localhost.
func isLoopbackAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
