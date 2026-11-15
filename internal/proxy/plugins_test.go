package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleGetPluginsResolvesSidecarEnabledState(t *testing.T) {
	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetAllPlugins"):
			_, _ = w.Write([]byte(`{
				"plugins": [
					{
						"name": "swarm",
						"displayName": "Swarm",
						"path": "file:///builtin/plugins/swarm",
						"description": "Unlock the power of agent swarms.",
						"isGlobal": true,
						"isBuiltin": true,
						"sidecars": [{"sidecarId": "swarm/swarm_board"}]
					},
					{
						"name": "stopped-plugin",
						"displayName": "Stopped Plugin",
						"path": "file:///plugins/stopped",
						"sidecars": [{"sidecarId": "stopped-plugin/worker"}]
					},
					{
						"name": "disabled-plugin",
						"disabled": true
					}
				]
			}`))
		case strings.HasSuffix(r.URL.Path, "/GetSidecars"):
			_, _ = w.Write([]byte(`{
				"sidecars": [
					{
						"sidecarId": "swarm/swarm_board",
						"status": "SIDECAR_STATUS_RUNNING",
						"pluginPath": "file:///builtin/plugins/swarm",
						"userConfig": {"enabled": true}
					},
					{
						"sidecarId": "stopped-plugin/worker",
						"status": "SIDECAR_STATUS_STOPPED",
						"pluginPath": "file:///plugins/stopped",
						"userConfig": {"enabled": false}
					}
				]
			}`))
		case strings.HasSuffix(r.URL.Path, "/ListPluginAuthProviders"):
			_, _ = w.Write([]byte(`{"providers":[]}`))
		default:
			http.NotFound(w, r)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/gateway/plugins", nil)
	w := httptest.NewRecorder()
	p.HandleGetPlugins(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp PluginListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if len(resp.Plugins) != 3 {
		t.Fatalf("expected 3 plugins, got %d", len(resp.Plugins))
	}

	byID := map[string]PluginItem{}
	for _, pl := range resp.Plugins {
		byID[pl.ID] = pl
	}
	if !byID["swarm"].Enabled || byID["swarm"].Name != "Swarm" || byID["swarm"].Kind != "builtin" {
		t.Errorf("expected swarm enabled=true name=Swarm kind=builtin, got %+v", byID["swarm"])
	}
	if byID["stopped-plugin"].Enabled {
		t.Errorf("expected stopped-plugin enabled=false, got %+v", byID["stopped-plugin"])
	}
	if byID["disabled-plugin"].Enabled {
		t.Errorf("expected disabled-plugin enabled=false, got %+v", byID["disabled-plugin"])
	}
}

// TestHandleGetPluginsNormalisesAuthProviders guards the provider list contract that
// the mobile clients rely on: every provider id must be non-empty and unique, because
// Android uses it as a LazyColumn key (duplicates throw) and iOS uses it as ForEach id.
// Upstream (Antigravity 2.28.1+) sends {plugin, provider, connected}; older builds sent
// {id, name, signedIn}. The previous fixture returned an empty array, so neither shape
// was covered and empty ids shipped to clients.
func TestHandleGetPluginsNormalisesAuthProviders(t *testing.T) {
	cases := []struct {
		name         string
		upstream     string
		wantCount    int
		wantNames    map[string]string
		wantSignedIn map[string]bool
	}{
		{
			name: "antigravity 2.28.1 shape plugin/provider/connected",
			upstream: `{"providers":[
				{"plugin":"gdocs","provider":"google","type":"oauth2","connected":true},
				{"plugin":"gdrive","provider":"google","type":"oauth2"}
			]}`,
			wantCount:    2,
			wantNames:    map[string]string{"gdocs:google": "gdocs", "gdrive:google": "gdrive"},
			wantSignedIn: map[string]bool{"gdocs:google": true, "gdrive:google": false},
		},
		{
			name: "duplicate upstream entries still get unique ids",
			upstream: `{"providers":[
				{"plugin":"gdocs","provider":"google","connected":true},
				{"plugin":"gdocs","provider":"google","connected":true}
			]}`,
			wantCount: 2,
		},
		{
			name:      "fully empty provider objects still get non-empty unique ids",
			upstream:  `{"providers":[{},{},{}]}`,
			wantCount: 3,
		},
		{
			name:      "legacy id/name/signedIn shape keeps working",
			upstream:  `{"providers":[{"id":"legacy","name":"Legacy","signedIn":true}]}`,
			wantCount: 1,
			wantNames: map[string]string{"legacy:Legacy": "legacy"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/GetAllPlugins"):
					_, _ = w.Write([]byte(`{"plugins":[]}`))
				case strings.HasSuffix(r.URL.Path, "/GetSidecars"):
					_, _ = w.Write([]byte(`{"sidecars":[]}`))
				case strings.HasSuffix(r.URL.Path, "/ListPluginAuthProviders"):
					_, _ = w.Write([]byte(tc.upstream))
				default:
					http.NotFound(w, r)
				}
			})

			req := httptest.NewRequest(http.MethodGet, "/gateway/plugins", nil)
			w := httptest.NewRecorder()
			p.HandleGetPlugins(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
			}

			var resp PluginListResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response failed: %v", err)
			}
			if len(resp.Providers) != tc.wantCount {
				t.Fatalf("expected %d providers, got %d: %+v", tc.wantCount, len(resp.Providers), resp.Providers)
			}

			seen := map[string]bool{}
			for _, pv := range resp.Providers {
				if strings.TrimSpace(pv.ID) == "" {
					t.Fatalf("provider id must not be empty: %+v", resp.Providers)
				}
				if seen[pv.ID] {
					t.Fatalf("duplicate provider id %q would crash the Android list: %+v", pv.ID, resp.Providers)
				}
				seen[pv.ID] = true
			}

			for id, wantName := range tc.wantNames {
				found := false
				for _, pv := range resp.Providers {
					if pv.ID != id {
						continue
					}
					found = true
					if pv.Name != wantName {
						t.Errorf("provider %q name = %q, want %q", id, pv.Name, wantName)
					}
				}
				if !found {
					t.Errorf("expected provider id %q, got %+v", id, resp.Providers)
				}
			}

			for id, wantSignedIn := range tc.wantSignedIn {
				for _, pv := range resp.Providers {
					if pv.ID == id && pv.SignedIn != wantSignedIn {
						t.Errorf("provider %q signedIn = %v, want %v", id, pv.SignedIn, wantSignedIn)
					}
				}
			}
		})
	}
}
