package tunnel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"antigravity-mobile/internal/config"
)

func TestRegisterOrFetchTunnelWithOptions_ForwardCompatPayloadAndCacheGuard(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MULTIGRAVITY_DATA_DIR", tmpDir)

	// Pre-seed cache with an unsupported future mode; registration must ignore it and re-fetch.
	cachePath := filepath.Join(tmpDir, "cf_tunnel.json")
	staleRelayCache := `{"success":true,"mode":"relay_v1","url":"https://deadbeef.ap.jiuge.space","token":"mgyr1.fake.sig"}`
	if err := os.WriteFile(cachePath, []byte(staleRelayCache), 0600); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	var received struct {
		MachineID     string   `json:"machine_id"`
		Platform      string   `json:"platform"`
		ClientVersion string   `json:"client_version"`
		Capabilities  []string `json:"capabilities"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tunnel/register" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":        true,
			"reused":         false,
			"mode":           "cloudflare",
			"region":         "ap",
			"relay_endpoint": "",
			"zone_domain":    "jiuge.space",
			"tunnel_id":      "tun-123",
			"tunnel_name":    "mgy-abcd1234",
			"subdomain":      "abcd1234.jiuge.space",
			"url":            "https://abcd1234.jiuge.space",
			"token":          "cf-token-xyz",
		})
	}))
	defer srv.Close()

	res, err := RegisterOrFetchTunnelWithOptions(context.Background(), srv.URL, "", RegisterOptions{
		ClientVersion: "1.2.5",
		Capabilities:  []string{ModeCloudflare},
	})
	if err != nil {
		t.Fatalf("RegisterOrFetchTunnelWithOptions failed: %v", err)
	}

	if received.ClientVersion != "1.2.5" {
		t.Errorf("expected client_version=1.2.5, got %q", received.ClientVersion)
	}
	if len(received.Capabilities) != 1 || received.Capabilities[0] != ModeCloudflare {
		t.Errorf("expected capabilities=[cloudflare], got %v", received.Capabilities)
	}
	if res.EffectiveMode() != ModeCloudflare {
		t.Errorf("expected mode=cloudflare, got %q", res.EffectiveMode())
	}
	if res.Region != "ap" || res.ZoneDomain != "jiuge.space" {
		t.Errorf("unexpected metadata: region=%q zone_domain=%q", res.Region, res.ZoneDomain)
	}

	// Second call should reuse cache without hitting the mock server
	srv.Close()
	cached, err := RegisterOrFetchTunnelWithOptions(context.Background(), srv.URL, "", RegisterOptions{
		ClientVersion: "1.2.5",
		Capabilities:  []string{ModeCloudflare},
	})
	if err != nil {
		t.Fatalf("expected cache hit after server close, got err: %v", err)
	}
	if !cached.Reused || cached.URL != "https://abcd1234.jiuge.space" {
		t.Errorf("unexpected cached result: %+v", cached)
	}
}

func TestNewProvider_LazyCloudflaredProvisioning(t *testing.T) {
	orig := ensureCloudflaredBinaryFn
	defer func() { ensureCloudflaredBinaryFn = orig }()

	var calls int
	ensureCloudflaredBinaryFn = func(ctx context.Context) (string, error) {
		calls++
		return "/fake/bin/cloudflared", nil
	}

	// 1. Non-Cloudflare mode (relay_v1) constructs PoolProvider and MUST NOT trigger cloudflared download
	relayProv, err := NewProvider(context.Background(), &CFTunnelResult{
		Mode:          ModeRelayV1,
		RelayEndpoint: "wss://relay.jiuge.space/_mgy_tunnel",
		Subdomain:     "r-abcd1234.jiuge.space",
		URL:           "https://r-abcd1234.jiuge.space",
		Token:         "mgyr1.payload.sig",
	}, &config.CloudflareConfig{Enabled: true})
	if err != nil {
		t.Fatalf("expected relay_v1 provider to succeed, got err: %v", err)
	}
	if relayProv.Mode() != ModeRelayV1 || relayProv.PublicURL() != "https://r-abcd1234.jiuge.space" {
		t.Fatalf("unexpected relay provider state: mode=%q url=%q", relayProv.Mode(), relayProv.PublicURL())
	}
	if calls != 0 {
		t.Fatalf("expected ensureCloudflaredBinaryFn NOT to be called for relay_v1, got %d call(s)", calls)
	}

	// 2. Cloudflare mode (including empty legacy mode) provisions cloudflared
	prov, err := NewProvider(context.Background(), &CFTunnelResult{
		Subdomain: "abcd1234.jiuge.space",
		URL:       "https://abcd1234.jiuge.space",
		Token:     "cf-token",
	}, &config.CloudflareConfig{Enabled: true})
	if err != nil {
		t.Fatalf("NewProvider cloudflare failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call to ensureCloudflaredBinaryFn, got %d", calls)
	}
	if prov.Mode() != ModeCloudflare || prov.PublicURL() != "https://abcd1234.jiuge.space" {
		t.Errorf("unexpected provider state: mode=%q url=%q", prov.Mode(), prov.PublicURL())
	}
}
