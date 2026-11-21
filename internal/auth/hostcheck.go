package auth

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// acceptableHostname reports whether a Host/Origin hostname may be served with address-based trust.
// Only IP literals, localhost, and mDNS (.local) pass; public/DDNS domains and arbitrary DNS names
// (e.g. an attacker domain rebound to 127.0.0.1 or a reverse-proxied public domain) never qualify
// for unauthenticated implicit trust (CRIT-01).
func acceptableHostname(h string) bool {
	h = strings.ToLower(strings.Trim(strings.TrimSpace(h), "[]"))
	if h == "" {
		return false
	}
	if net.ParseIP(h) != nil || h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") {
		return true
	}
	return false
}

// ImplicitTrustRequestOK gates loopback/LAN trust on request-level evidence that the request was
// not issued by a hostile web page: a non-empty Host must be an IP/localhost/mDNS name (anti DNS
// rebinding), non-navigation cross-site Fetch requests are rejected, and any Origin header (even on
// GET / WebSocket / SSE) must be acceptable too (anti CSRF & CSWSH, HIGH-01).
// Native clients and curl send no Origin/Sec-Fetch-Site and are unaffected.
func ImplicitTrustRequestOK(r *http.Request) bool {
	if r == nil {
		return false
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if !acceptableHostname(host) {
		return false
	}
	if sfs := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); sfs == "cross-site" {
		sfm := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Mode")))
		if sfm != "navigate" {
			return false
		}
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	if isConfiguredAllowedOrigin(origin) {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false // includes "null"
	}
	return acceptableHostname(u.Hostname())
}

func isConfiguredAllowedOrigin(origin string) bool {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u.Hostname() == "" {
		return false
	}
	for _, env := range []string{"MULTIGRAVITY_ALLOWED_ORIGINS", "ALLOWED_ORIGINS"} {
		for _, o := range strings.Split(os.Getenv(env), ",") {
			if au, err := url.Parse(strings.TrimSpace(o)); err == nil && au.Hostname() != "" {
				if strings.EqualFold(au.Scheme, u.Scheme) && strings.EqualFold(au.Host, u.Host) {
					return true
				}
			}
		}
	}
	return false
}

func sameOriginHost(u *url.URL, reqHost string) bool {
	if u == nil || u.Hostname() == "" {
		return false
	}
	reqHost = strings.TrimSpace(reqHost)
	if reqHost == "" {
		return false
	}
	reqHostname := reqHost
	reqPort := ""
	if h, p, err := net.SplitHostPort(reqHost); err == nil {
		reqHostname = h
		reqPort = p
	}
	reqHostname = strings.Trim(reqHostname, "[]")
	if !strings.EqualFold(u.Hostname(), reqHostname) {
		return false
	}
	uPort := u.Port()
	if uPort == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			uPort = "443"
		case "http":
			uPort = "80"
		}
	}
	if reqPort == "" {
		return uPort == "" || uPort == "80" || uPort == "443"
	}
	return uPort == reqPort
}

// CookieWriteCSRFOK enforces same-origin protection for state-changing requests
// authenticated via session cookies (SEC-08). Native mobile clients using
// Authorization: Bearer tokens or non-browser callers with no Fetch/Origin
// metadata are unaffected.
func CookieWriteCSRFOK(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" && isConfiguredAllowedOrigin(origin) {
		return true
	}

	if sfs := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); sfs != "" {
		if sfs != "same-origin" && sfs != "none" {
			return false
		}
	}

	if origin != "" {
		if origin == "null" {
			return false
		}
		u, err := url.Parse(origin)
		if err != nil || u.Hostname() == "" {
			return false
		}
		return sameOriginHost(u, r.Host)
	}

	if ref := strings.TrimSpace(r.Header.Get("Referer")); ref != "" {
		if isConfiguredAllowedOrigin(ref) {
			return true
		}
		u, err := url.Parse(ref)
		if err != nil || u.Hostname() == "" {
			return false
		}
		return sameOriginHost(u, r.Host)
	}

	return true
}

// CookieWSOriginOK enforces strict same-origin verification on WebSocket upgrade requests
// authenticated via browser session cookies (with no Bearer token and no one-time ?ticket=).
// This blocks Cross-Site WebSocket Hijacking (CSWSH), including from sibling subdomains sharing
// the same parent eTLD+1 such as *.jiuge.space where SameSite=Lax cookies are sent (HIGH-01).
func CookieWSOriginOK(r *http.Request) bool {
	if r == nil {
		return false
	}
	if sfs := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); sfs != "" {
		if sfs != "same-origin" && sfs != "none" {
			return false
		}
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" || origin == "null" {
		return false
	}
	if isConfiguredAllowedOrigin(origin) {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	return sameOriginHost(u, r.Host)
}
