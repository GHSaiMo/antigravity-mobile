package localtls

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// ClientConfig is used only when dialing the local Antigravity language_server
// (self-signed cert on 127.0.0.1). Pair with DialTLSContext so non-loopback
// hosts cannot inherit InsecureSkipVerify.
func ClientConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}
}

// DialTLSContext allows InsecureSkipVerify exclusively for loopback addresses.
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
// communication with the local Antigravity language_server. All TLS dials are gated
// through DialTLSContext to ensure InsecureSkipVerify cannot be applied to any remote host.
func NewLoopbackTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: ClientConfig(),
		DialTLSContext:  DialTLSContext,
	}
}
