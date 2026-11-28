package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"antigravity-mobile/internal/netutil"
)

func TestResolveListenSpec(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		port       int
		enableIPv6 bool
		wantNet    string
		wantAddr   string
		wantIPv6   bool
	}{
		{
			name:       "default empty host ipv4 only",
			host:       "",
			port:       58900,
			enableIPv6: false,
			wantNet:    "tcp4",
			wantAddr:   ":58900",
			wantIPv6:   false,
		},
		{
			name:       "empty host with ipv6 dual-stack enabled",
			host:       "",
			port:       58900,
			enableIPv6: true,
			wantNet:    "tcp",
			wantAddr:   ":58900",
			wantIPv6:   true,
		},
		{
			name:       "explicit ipv4 loopback ignores ipv6 flag",
			host:       "127.0.0.1",
			port:       58900,
			enableIPv6: true,
			wantNet:    "tcp4",
			wantAddr:   "127.0.0.1:58900",
			wantIPv6:   false,
		},
		{
			name:       "explicit 0.0.0.0 remains ipv4",
			host:       "0.0.0.0",
			port:       58900,
			enableIPv6: true,
			wantNet:    "tcp4",
			wantAddr:   "0.0.0.0:58900",
			wantIPv6:   false,
		},
		{
			name:       "explicit :: enables dual-stack even without flag",
			host:       "::",
			port:       58900,
			enableIPv6: false,
			wantNet:    "tcp",
			wantAddr:   "[::]:58900",
			wantIPv6:   true,
		},
		{
			name:       "bracketed [::] normalized cleanly",
			host:       "[::]",
			port:       58943,
			enableIPv6: false,
			wantNet:    "tcp",
			wantAddr:   "[::]:58943",
			wantIPv6:   true,
		},
		{
			name:       "explicit ipv6 loopback",
			host:       "::1",
			port:       58900,
			enableIPv6: false,
			wantNet:    "tcp",
			wantAddr:   "[::1]:58900",
			wantIPv6:   true,
		},
		{
			name:       "explicit bracketed tailscale ipv6",
			host:       "[fd7a:115c:a1e0::1]",
			port:       58900,
			enableIPv6: false,
			wantNet:    "tcp",
			wantAddr:   "[fd7a:115c:a1e0::1]:58900",
			wantIPv6:   true,
		},
		{
			name:       "explicit public ipv6 literal",
			host:       "2408:8207::1",
			port:       58943,
			enableIPv6: false,
			wantNet:    "tcp",
			wantAddr:   "[2408:8207::1]:58943",
			wantIPv6:   true,
		},
		{
			name:       "hostname localhost with ipv6 enabled",
			host:       "localhost",
			port:       58900,
			enableIPv6: true,
			wantNet:    "tcp",
			wantAddr:   "localhost:58900",
			wantIPv6:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotNet, gotAddr, gotIPv6 := resolveListenSpec(tc.host, tc.port, tc.enableIPv6)
			if gotNet != tc.wantNet || gotAddr != tc.wantAddr || gotIPv6 != tc.wantIPv6 {
				t.Fatalf("resolveListenSpec(%q, %d, %v) = (%q, %q, %v), want (%q, %q, %v)",
					tc.host, tc.port, tc.enableIPv6, gotNet, gotAddr, gotIPv6, tc.wantNet, tc.wantAddr, tc.wantIPv6)
			}
		})
	}
}

func TestLocalControlHost(t *testing.T) {
	t.Setenv("MULTIGRAVITY_HOST", "")
	if got := localControlHost(); got != "127.0.0.1" {
		t.Fatalf("default localControlHost() = %q, want 127.0.0.1", got)
	}
	t.Setenv("MULTIGRAVITY_HOST", "::1")
	if got := localControlHost(); got != "[::1]" {
		t.Fatalf("localControlHost() for ::1 = %q, want [::1]", got)
	}
	t.Setenv("MULTIGRAVITY_HOST", "[::1]")
	if got := localControlHost(); got != "[::1]" {
		t.Fatalf("localControlHost() for [::1] = %q, want [::1]", got)
	}
	t.Setenv("MULTIGRAVITY_HOST", "::")
	if got := localControlHost(); got != "127.0.0.1" {
		t.Fatalf("localControlHost() for :: = %q, want 127.0.0.1", got)
	}
}

func TestDualStackListenerAcceptsIPv4AndIPv6(t *testing.T) {
	listenNet, listenAddr, ipv6Active := resolveListenSpec("", 0, true)
	if !ipv6Active || listenNet != "tcp" {
		t.Fatalf("unexpected listen spec: net=%q addr=%q ipv6=%v", listenNet, listenAddr, ipv6Active)
	}
	rawLn, err := net.Listen(listenNet, listenAddr)
	if err != nil {
		t.Fatalf("net.Listen(%q, %q): %v", listenNet, listenAddr, err)
	}
	defer rawLn.Close()

	srv := &http.Server{Handler: setupTestRouter(t)}
	go srv.Serve(netutil.NewAdaptiveListener(rawLn))
	defer srv.Close()

	_, portStr, err := net.SplitHostPort(rawLn.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", rawLn.Addr().String(), err)
	}

	client := &http.Client{Timeout: 3 * time.Second}

	// IPv4 loopback must always work on dual-stack listener.
	resp4, err := client.Get("http://127.0.0.1:" + portStr + "/healthz")
	if err != nil {
		t.Fatalf("IPv4 loopback request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp4.Body)
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("IPv4 /healthz status = %d, want 200", resp4.StatusCode)
	}

	// Verify IPv6 loopback whenever the host kernel has IPv6 enabled.
	probeV6, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Logf("IPv6 loopback unavailable in this test environment (%v); skipping IPv6 dial assertion", err)
		return
	}
	_ = probeV6.Close()

	resp6, err := client.Get("http://[::1]:" + portStr + "/healthz")
	if err != nil {
		t.Fatalf("IPv6 loopback request to dual-stack listener failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp6.Body)
	resp6.Body.Close()
	if resp6.StatusCode != http.StatusOK {
		t.Fatalf("IPv6 /healthz status = %d, want 200", resp6.StatusCode)
	}
}

func writeSelfSignedCert(t *testing.T, certPath, keyPath string, serial int64) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

func TestReloadingTLSConfigHotReloadAndHSTS(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "fullchain.pem")
	keyPath := filepath.Join(dir, "privkey.pem")
	writeSelfSignedCert(t, certPath, keyPath, 1001)

	tlsCfg, err := newReloadingTLSConfig(certPath, keyPath, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("newReloadingTLSConfig: %v", err)
	}

	rawLn, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer rawLn.Close()

	srv := &http.Server{Handler: setupTestRouter(t)}
	tlsLn := tls.NewListener(netutil.NewAdaptiveListener(rawLn), tlsCfg)
	go srv.Serve(tlsLn)
	defer srv.Close()

	_, portStr, err := net.SplitHostPort(rawLn.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	portNum, _ := strconv.Atoi(portStr)
	if portNum <= 0 {
		t.Fatalf("unexpected bound port: %q", portStr)
	}

	fetchSerial := func() (int64, string) {
		client := &http.Client{
			Timeout: 3 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				DisableKeepAlives: true,
			},
		}
		resp, err := client.Get("https://127.0.0.1:" + portStr + "/healthz")
		if err != nil {
			t.Fatalf("HTTPS /healthz failed: %v", err)
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("HTTPS /healthz status = %d, want 200", resp.StatusCode)
		}
		if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
			t.Fatalf("expected peer TLS certificate")
		}
		return resp.TLS.PeerCertificates[0].SerialNumber.Int64(), resp.Header.Get("Strict-Transport-Security")
	}

	serial1, hsts1 := fetchSerial()
	if serial1 != 1001 {
		t.Fatalf("initial cert serial = %d, want 1001", serial1)
	}
	if hsts1 == "" {
		t.Fatalf("expected HSTS header on native TLS listener")
	}

	// Replace certificate with a new serial and future mtime to simulate ACME renewal.
	time.Sleep(25 * time.Millisecond)
	writeSelfSignedCert(t, certPath, keyPath, 2002)
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(certPath, future, future)
	_ = os.Chtimes(keyPath, future, future)

	serial2, _ := fetchSerial()
	if serial2 != 2002 {
		t.Fatalf("hot-reloaded cert serial = %d, want 2002", serial2)
	}

	// Corrupt cert mid-write: reloader must retain previous valid certificate (2002) instead of failing handshakes.
	time.Sleep(25 * time.Millisecond)
	_ = os.WriteFile(certPath, []byte("not-a-valid-pem"), 0600)
	future2 := future.Add(2 * time.Second)
	_ = os.Chtimes(certPath, future2, future2)

	serial3, _ := fetchSerial()
	if serial3 != 2002 {
		t.Fatalf("expected fallback to previous cert serial 2002 on malformed file, got %d", serial3)
	}
}

func TestWrapForceE2EEHandler_IPv6Classification(t *testing.T) {
	router := setupTestRouter(t)
	forced := wrapForceE2EEHandler(router, true)

	// Loopback, ULA, and Tailscale IPv6 direct peers bypass ForceE2EE.
	for _, addr := range []string{
		"[::1]:54321",
		"[fd00::10]:54321",
		"[fd7a:115c:a1e0::1]:54321",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
		req.RemoteAddr = addr
		rr := httptest.NewRecorder()
		forced.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 for private/loopback IPv6 peer %s, got %d", addr, rr.Code)
		}
	}

	// Public global unicast IPv6 peer must be gated with 426 Upgrade Required under ForceE2EE.
	reqPub := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	reqPub.RemoteAddr = "[2408:8207::1]:54321"
	rrPub := httptest.NewRecorder()
	forced.ServeHTTP(rrPub, reqPub)
	if rrPub.Code != http.StatusUpgradeRequired {
		t.Fatalf("expected 426 for public IPv6 peer under ForceE2EE, got %d body=%s", rrPub.Code, rrPub.Body.String())
	}
}
