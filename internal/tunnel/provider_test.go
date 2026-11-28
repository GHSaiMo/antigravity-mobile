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
	"antigravity-mobile/internal/relay"
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

func TestUpdateCachedRelayPool_PersistsActiveNodeAndGuardsCloudflareCache(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MULTIGRAVITY_DATA_DIR", tmpDir)

	nodeA := relay.NodeConfig{
		ID:        "la2-direct",
		Region:    "us",
		Priority:  18,
		Endpoint:  "wss://ctl.la2.jiuge.space/_mgy_tunnel",
		HealthURL: "https://ctl.la2.jiuge.space/healthz",
		Subdomain: "r-abcd1234.la2.jiuge.space",
		URL:       "https://r-abcd1234.la2.jiuge.space",
		Token:     "mgyr1.la2.sig",
	}
	nodeB := relay.NodeConfig{
		ID:        "hk1-direct",
		Region:    "ap",
		Priority:  6,
		Endpoint:  "wss://ctl.hk1.jiuge.space/_mgy_tunnel",
		HealthURL: "https://ctl.hk1.jiuge.space/healthz",
		Subdomain: "r-abcd1234.hk1.jiuge.space",
		URL:       "https://r-abcd1234.hk1.jiuge.space",
		Token:     "mgyr1.hk1.sig",
	}
	nodeC := relay.NodeConfig{
		ID:        "tw2-direct",
		Region:    "ap",
		Priority:  4,
		Endpoint:  "wss://ctl.tw2.multigravity.app/_mgy_tunnel",
		HealthURL: "https://ctl.tw2.multigravity.app/healthz",
		Subdomain: "r-abcd1234.tw2.multigravity.app",
		URL:       "https://r-abcd1234.tw2.multigravity.app",
		Token:     "mgyr1.tw2.sig",
	}

	// 1. Existing Cloudflare cache must NOT be overwritten when base is nil (e.g. DualTrack safety).
	cachePath := filepath.Join(tmpDir, "cf_tunnel.json")
	cfSeed := `{"success":true,"mode":"cloudflare","subdomain":"abcd1234.jiuge.space","url":"https://abcd1234.jiuge.space","token":"cf-tok"}`
	if err := os.WriteFile(cachePath, []byte(cfSeed), 0600); err != nil {
		t.Fatalf("seed cf cache: %v", err)
	}
	if err := UpdateCachedRelayPool(&nodeB, []relay.NodeConfig{nodeA, nodeB}, nil); err != nil {
		t.Fatalf("UpdateCachedRelayPool returned unexpected err: %v", err)
	}
	loaded := LoadCachedTunnelResult()
	if loaded == nil || loaded.EffectiveMode() != ModeCloudflare || loaded.URL != "https://abcd1234.jiuge.space" {
		t.Fatalf("expected cloudflare cache to remain untouched, got %+v", loaded)
	}

	// 2. Explicit relay_v1 base + active fastest node promotes nodeB to top-level URL/Subdomain/Token.
	base := &CFTunnelResult{
		Success:       true,
		Mode:          ModeRelayV1,
		Region:        "us",
		RelayEndpoint: nodeA.Endpoint,
		Subdomain:     nodeA.Subdomain,
		URL:           nodeA.URL,
		Token:         nodeA.Token,
		TunnelID:      "relay-abcd1234",
		TunnelName:    "mgy-abcd1234",
		RelayNodes:    []relay.NodeConfig{nodeA, nodeB},
	}
	if err := UpdateCachedRelayPool(&nodeB, base.RelayNodes, base); err != nil {
		t.Fatalf("UpdateCachedRelayPool(base): %v", err)
	}
	loaded = LoadCachedTunnelResult()
	if loaded == nil || loaded.EffectiveMode() != ModeRelayV1 {
		t.Fatalf("expected relay_v1 cached result, got %+v", loaded)
	}
	if loaded.URL != nodeB.URL || loaded.Subdomain != nodeB.Subdomain || loaded.Token != nodeB.Token || loaded.RelayEndpoint != nodeB.Endpoint {
		t.Fatalf("expected active nodeB promoted into cache, got url=%q sub=%q ep=%q", loaded.URL, loaded.Subdomain, loaded.RelayEndpoint)
	}
	if len(loaded.RelayNodes) != 2 {
		t.Fatalf("expected 2 relay_nodes preserved, got %d", len(loaded.RelayNodes))
	}

	// 3. Periodic pool refresh (active == nil) updates RelayNodes while keeping current active nodeB.
	if err := UpdateCachedRelayPool(nil, []relay.NodeConfig{nodeA, nodeB, nodeC}, nil); err != nil {
		t.Fatalf("UpdateCachedRelayPool(refresh): %v", err)
	}
	loaded = LoadCachedTunnelResult()
	if loaded == nil || loaded.URL != nodeB.URL || len(loaded.RelayNodes) != 3 {
		t.Fatalf("expected active nodeB preserved with 3 refreshed nodes, got url=%q count=%d", loaded.URL, len(loaded.RelayNodes))
	}

	// 4. Subsequent failover/OnConnected promotes nodeC while retaining all 3 RelayNodes.
	if err := UpdateCachedRelayPool(&nodeC, nil, nil); err != nil {
		t.Fatalf("UpdateCachedRelayPool(failover): %v", err)
	}
	loaded = LoadCachedTunnelResult()
	if loaded == nil || loaded.URL != nodeC.URL || loaded.Subdomain != nodeC.Subdomain || len(loaded.RelayNodes) != 3 {
		t.Fatalf("expected active nodeC promoted with 3 nodes retained, got url=%q sub=%q count=%d", loaded.URL, loaded.Subdomain, len(loaded.RelayNodes))
	}
}
