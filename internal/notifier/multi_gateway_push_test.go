package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"antigravity-mobile/internal/config"
)

func TestAPNsMultiGatewayLabelAndGatewayID(t *testing.T) {
	store, err := NewPushDeviceStore("")
	if err != nil {
		t.Fatal(err)
	}
	tokSingle := strings.Repeat("a", 64)
	tokMulti := strings.Repeat("b", 64)
	if err := store.UpsertAlertToken("dev-single", "ios", tokSingle, "production", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAlertToken("dev-multi", "ios", tokMulti, "production", nil, nil); err != nil {
		t.Fatal(err)
	}
	showTrue := true
	if err := store.UpdateGatewayLabel("dev-multi", &showTrue, "Mac mini"); err != nil {
		t.Fatal(err)
	}

	var (
		mu   sync.Mutex
		reqs = make(map[string]APNsRelayAlertRequest)
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body APNsRelayAlertRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode alert req: %v", err)
		}
		mu.Lock()
		reqs[body.DeviceToken] = body
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"status":200}`))
	}))
	defer srv.Close()

	client := NewAPNsRelayClient(srv.URL, store)
	n := NewNotifier(config.NotificationConfig{Enabled: true})
	n.SetGatewayIdentity("gw_1234567890abcdef", "DefaultHost")
	n.AttachAPNsClient(client, true)

	if err := n.NotifyCompleted("casc-999", "Build App", 4); err != nil {
		t.Fatalf("NotifyCompleted failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 APNs dispatches, got %d", len(reqs))
	}
	rSingle := reqs[tokSingle]
	if rSingle.GatewayID != "gw_1234567890abcdef" {
		t.Errorf("expected gateway_id on single-host device, got %q", rSingle.GatewayID)
	}
	if rSingle.Subtitle != "" {
		t.Errorf("expected empty subtitle when show_gateway_label=false, got %q", rSingle.Subtitle)
	}
	if rSingle.ThreadID != "multigravity" {
		t.Errorf("expected default thread_id=multigravity, got %q", rSingle.ThreadID)
	}
	if !strings.Contains(rSingle.URL, "gw=gw_1234567890abcdef") {
		t.Errorf("expected URL to carry gw query param, got %q", rSingle.URL)
	}

	rMulti := reqs[tokMulti]
	if rMulti.Subtitle != "Mac mini" {
		t.Errorf("expected custom subtitle 'Mac mini', got %q", rMulti.Subtitle)
	}
	if rMulti.ThreadID != "multigravity-gw_1234567890abcdef" {
		t.Errorf("expected per-gateway thread_id, got %q", rMulti.ThreadID)
	}
}

func TestFCMMultiDeviceFanOutAndGatewayLabels(t *testing.T) {
	var (
		mu   sync.Mutex
		sent = make(map[string]FCMPayload)
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body FCMPayload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode fcm req: %v", err)
		}
		mu.Lock()
		sent[body.To] = body
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":1}`))
	}))
	defer srv.Close()

	store, err := NewPushDeviceStore("")
	if err != nil {
		t.Fatal(err)
	}
	n := NewNotifier(config.NotificationConfig{
		Enabled:     true,
		FCMEnabled:  true,
		FCMEndpoint: srv.URL,
	})
	n.SetGatewayIdentity("gw_abcdef0123456789", "Office-PC")
	n.AttachPushStore(store)

	showFalse := false
	showTrue := true
	if err := n.UpsertFCMDevice("and-1", "fcm_tok_1", "zh", nil, &showFalse, ""); err != nil {
		t.Fatal(err)
	}
	if err := n.UpsertFCMDevice("and-2", "fcm_tok_2", "en", nil, &showTrue, "Workstation"); err != nil {
		t.Fatal(err)
	}

	if err := n.sendToAll(context.Background(), PushPayload{
		Title:     "Done",
		Body:      "Finished",
		URL:       "antigravity://cascade/c-123",
		Category:  "antigravity_complete",
		CascadeID: "c-123",
	}); err != nil {
		t.Fatalf("sendToAll failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 2 {
		t.Fatalf("expected fan-out to 2 Android devices, got %d", len(sent))
	}
	p1 := sent["fcm_tok_1"]
	if p1.Data["gatewayId"] != "gw_abcdef0123456789" {
		t.Errorf("expected gatewayId on device 1, got %q", p1.Data["gatewayId"])
	}
	if p1.Data["gatewayLabel"] != "" {
		t.Errorf("expected no gatewayLabel when show=false, got %q", p1.Data["gatewayLabel"])
	}
	p2 := sent["fcm_tok_2"]
	if p2.Data["gatewayLabel"] != "Workstation" {
		t.Errorf("expected gatewayLabel=Workstation on device 2, got %q", p2.Data["gatewayLabel"])
	}
	if !strings.Contains(p2.Data["deeplink"], "gw=gw_abcdef0123456789") {
		t.Errorf("expected deeplink to contain gw param, got %q", p2.Data["deeplink"])
	}
}
