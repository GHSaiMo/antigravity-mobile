package auth

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// acceptableHostname reports whether a Host/Origin hostname may be served with address-based trust.
// IP literals, localhost, mDNS (.local) and explicitly configured domains pass; any other
// DNS name (e.g. an attacker domain rebound to 127.0.0.1) does not.
func acceptableHostname(h string) bool {
	h = strings.ToLower(strings.Trim(strings.TrimSpace(h), "[]"))
	if h == "" {
		return false
	}
	if net.ParseIP(h) != nil || h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") {
		return true
	}
	for _, env := range []string{"DDNS_HOST", "MULTIGRAVITY_HOST", "GATEWAY_HOST"} {
		if t := strings.ToLower(strings.TrimSpace(os.Getenv(env))); t != "" && (h == t || strings.HasSuffix(h, "."+t)) {
			return true
		}
	}
	for _, env := range []string{"MULTIGRAVITY_ALLOWED_ORIGINS", "ALLOWED_ORIGINS"} {
		for _, o := range strings.Split(os.Getenv(env), ",") {
			if u, err := url.Parse(strings.TrimSpace(o)); err == nil && u.Hostname() != "" && strings.EqualFold(u.Hostname(), h) {
				return true
			}
		}
	}
	return false
}

// ImplicitTrustRequestOK gates loopback/LAN trust on request-level evidence that the request was
// not issued by a hostile web page: the Host must be an IP/localhost/configured name (anti DNS
// rebinding) and, for state-changing methods, any Origin must be acceptable too (anti CSRF).
// Native clients and curl send no Origin and are unaffected.
func ImplicitTrustRequestOK(r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host != "" && !acceptableHostname(host) {
		return false
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false // includes "null"
	}
	return acceptableHostname(u.Hostname())
}
