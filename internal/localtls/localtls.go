package localtls

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
)

var tlsSessionCache = tls.NewLRUClientSessionCache(32)

// ClientConfig is used only when dialing the local Antigravity language_server
// (self-signed cert on 127.0.0.1). Pair with DialContext / DialTLSContext so
// non-loopback hosts cannot inherit InsecureSkipVerify.
func ClientConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		ClientSessionCache: tlsSessionCache,
	}
}

// DialContext restricts raw TCP dials exclusively to loopback addresses while
// letting http.Transport perform its own TLS + HTTP/2 ALPN ("h2") negotiation.
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if !IsLoopbackHost(host) {
		return nil, fmt.Errorf("refusing loopback-only connection to non-loopback address %s", addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, network, addr)
}

// DialTLSContext allows InsecureSkipVerify exclusively for loopback addresses
// (HTTP/1.1 TLS without h2 ALPN, used by WebSocket upgrade dialers).
func DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if !IsLoopbackHost(host) {
		return nil, fmt.Errorf("refusing insecure TLS to non-loopback address %s", addr)
	}
	d := &tls.Dialer{Config: ClientConfig()}
	return d.DialContext(ctx, network, addr)
}

// IsLoopbackHost reports whether host is localhost / 127.0.0.1 / ::1.
func IsLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// NewLoopbackTransport returns an http.Transport strictly configured for loopback-only
// communication with the local Antigravity language_server, with HTTP/2 multiplexing enabled.
// All TCP dials are gated through DialContext so InsecureSkipVerify cannot reach any remote host.
func NewLoopbackTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig:   ClientConfig(),
		DialContext:       DialContext,
		ForceAttemptHTTP2: true,
	}
}
