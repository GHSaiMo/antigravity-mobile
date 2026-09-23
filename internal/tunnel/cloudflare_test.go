package tunnel

import (
	"net/http"
	"strings"
	"testing"
	"time"
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
