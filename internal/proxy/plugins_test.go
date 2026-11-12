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
