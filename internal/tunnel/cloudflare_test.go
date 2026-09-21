package tunnel

import (
	"strings"
	"testing"
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
	for _, u := range urls {
		if !strings.HasPrefix(u, "https://") {
			t.Errorf("expected HTTPS download URL, got %s", u)
		}
	}
}
