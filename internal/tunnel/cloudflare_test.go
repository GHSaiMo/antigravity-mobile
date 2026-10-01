package tunnel

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/config"
)

func TestGetStableMachineID(t *testing.T) {
	id1 := GetStableMachineID()
	if id1 == "" {
		t.Fatalf("expected non-empty machine id")
	}
	id2 := GetStableMachineID()
	if id1 != id2 {
		t.Fatalf("expected stable machine id across calls, got %s vs %s", id1, id2)
	}
	if len(id1) < 16 {
		t.Fatalf("machine id too short: %s", id1)
	}
}

func TestCloudflareTunnel_PublicURL(t *testing.T) {
	res := &CFTunnelResult{
		Success:   true,
		Subdomain: "abc12345.mgy.example.com",
		URL:       "https://abc12345.mgy.example.com",
		Token:     "test-token",
	}
	tun := NewCloudflareTunnel(res)
	if tun.PublicURL() != "https://abc12345.mgy.example.com" {
		t.Errorf("unexpected public URL: %s", tun.PublicURL())
	}
	if tun.Subdomain() != "abc12345.mgy.example.com" {
		t.Errorf("unexpected subdomain: %s", tun.Subdomain())
	}
}

func TestGetCloudflaredDownloadURLs(t *testing.T) {
	urls := getCloudflaredDownloadURLs()
	if len(urls) == 0 {
		t.Fatalf("expected download urls for current platform")
	}

	var hasFast, hasProxy, hasOfficial bool
	for _, u := range urls {
		if !strings.HasPrefix(u, "https://") {
			t.Errorf("expected HTTPS download URL, got %s", u)
		}
		if strings.Contains(u, "ghfast.top") {
			hasFast = true
		}
		if strings.Contains(u, "ghproxy.net") {
			hasProxy = true
		}
		if strings.HasPrefix(u, "https://github.com/") {
			hasOfficial = true
		}
	}

	if !hasFast {
		t.Errorf("expected ghfast.top in download URLs")
	}
	if !hasProxy {
		t.Errorf("expected ghproxy.net in download URLs")
	}
	if !hasOfficial {
		t.Errorf("expected official github.com URL as fallback")
	}
}

func TestCreateDownloadHTTPClient(t *testing.T) {
	// 1. Mirror client: must force direct (Proxy == nil)
	mirrorClient := createDownloadHTTPClient("https://ghfast.top/https://github.com/foo/bar")
	if mirrorClient.Timeout < 4*time.Minute {
		t.Errorf("expected at least 4m timeout, got %v", mirrorClient.Timeout)
	}
	tr, ok := mirrorClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport")
	}
	if tr.Proxy != nil {
		t.Errorf("expected mirror transport Proxy to be nil (forced direct)")
	}

	// 2. Official GitHub client
	officialClient := createDownloadHTTPClient("https://github.com/foo/bar")
	if officialClient.Timeout < 4*time.Minute {
		t.Errorf("expected at least 4m timeout, got %v", officialClient.Timeout)
	}
}

func TestDetectLocalProxy_Env(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9999")
	proxyURL := detectLocalProxy()
	if proxyURL == nil {
		t.Fatalf("expected detected proxy URL")
	}
	if proxyURL.Host != "127.0.0.1:9999" {
		t.Errorf("expected host 127.0.0.1:9999, got %s", proxyURL.Host)
	}
}

func TestBuildTunnelArgs(t *testing.T) {
	// 1. Full config with DNS resolvers
	cfg := &config.CloudflareConfig{
		EdgeIPVersion: "4",
		Protocol:      "http2",
		Region:        "us",
		DNSResolvers:  "223.5.5.5,119.29.29.29:53",
	}
	args := buildTunnelArgs(cfg)
	expected := []string{
		"tunnel",
		"--edge-ip-version", "4",
		"--region", "us",
		"--protocol", "http2",
		"--dns-resolver-addrs", "223.5.5.5:53",
		"--dns-resolver-addrs", "119.29.29.29:53",
		"run",
	}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i := range expected {
		if args[i] != expected[i] {
			t.Errorf("arg[%d]: expected %q, got %q", i, expected[i], args[i])
		}
	}

	// 2. DNS override disabled
	cfgDisabled := &config.CloudflareConfig{
		DNSResolvers: "system",
	}
	argsDisabled := buildTunnelArgs(cfgDisabled)
	for _, a := range argsDisabled {
		if a == "--dns-resolver-addrs" {
			t.Errorf("expected no --dns-resolver-addrs when disabled with 'system'")
		}
	}
}

func TestIsBenignCloudflareLog(t *testing.T) {
	benignCases := []string{
		"context canceled",
		"2026-10-01T02:29:55Z ERR failed to serve incoming request error=\"already connected to this server, trying another address\"",
		"2026-10-01T02:29:55Z WRN Unable to establish connection. error=\"already connected to this server, trying another address\" connIndex=3 event=0 ip=198.41.200.113",
		"2026-10-01T02:29:57Z WRN Connection terminated error=\"already connected to this server, trying another address\" connIndex=3",
		"2026-10-01T01:59:59Z ERR Failed to initialize DNS local resolver error=\"lookup region1.v2.argotunnel.com: i/o timeout\"",
		"2026-10-01T02:05:04Z ERR Failed to refresh DNS local resolver error=\"lookup region1.v2.argotunnel.com: i/o timeout\"",
	}

	for _, line := range benignCases {
		if !isBenignCloudflareLog(line) {
			t.Errorf("expected benign log to be filtered: %q", line)
		}
	}

	criticalCases := []string{
		"ERR Fatal error: failed to authenticate with token",
		"ERR Cannot dial edge: connection refused",
		"ERR Registration failed",
	}

	for _, line := range criticalCases {
		if isBenignCloudflareLog(line) {
			t.Errorf("expected critical error NOT to be filtered: %q", line)
		}
	}
}
