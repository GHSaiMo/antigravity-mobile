package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/proxy"
)

func TestFCMClientSend_Success(t *testing.T) {
	var receivedMsg FCMPayload
	var receivedAuth string
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&receivedMsg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"multicast_id":100,"success":1,"failure":0,"canonical_ids":0,"results":[{"message_id":"0:123"}]}`))
	}))
	defer server.Close()

	cfg := config.NotificationConfig{
		Enabled:        true,
		FCMEnabled:     true,
		FCMServerKey:   "test_server_key_123",
		FCMDeviceToken: "device_token_abc_xyz",
		FCMEndpoint:    server.URL,
		IconURL:        "https://example.com/icon.png",
	}

	client := NewFCMClient(cfg)
	err := client.Send(context.Background(), BarkPayload{
		Title:    "⚠️ Antigravity 需要审批",
		Body:     "【项目】申请执行命令: make run",
		URL:      "antigravity://cascade/cas_123?action=review",
		Level:    "timeSensitive",
		Sound:    "alarm",
		Category: "antigravity_action",
	})

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if receivedAuth != "key=test_server_key_123" {
		t.Errorf("expected Authorization key=test_server_key_123, got %q", receivedAuth)
	}
	if receivedContentType != "application/json; charset=utf-8" {
		t.Errorf("expected Content-Type application/json; charset=utf-8, got %q", receivedContentType)
	}
	if receivedMsg.To != "device_token_abc_xyz" {
		t.Errorf("expected To device_token_abc_xyz, got %q", receivedMsg.To)
	}
	if receivedMsg.Priority != "high" {
		t.Errorf("expected priority high for timeSensitive, got %q", receivedMsg.Priority)
	}
	if receivedMsg.Notification == nil || receivedMsg.Notification.Title != "⚠️ Antigravity 需要审批" {
		t.Errorf("unexpected notification payload: %+v", receivedMsg.Notification)
	}
	if receivedMsg.Data["deeplink"] != "antigravity://cascade/cas_123?action=review" {
		t.Errorf("expected deeplink in data, got %q", receivedMsg.Data["deeplink"])
	}
	if receivedMsg.Data["category"] != "antigravity_action" {
		t.Errorf("expected category antigravity_action, got %q", receivedMsg.Data["category"])
	}
}

func TestFCMClientSend_Retry(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := atomic.AddInt32(&attempts, 1)
		if att == 1 {
			http.Error(w, `{"error":"Unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":1}`))
	}))
	defer server.Close()

	cfg := config.NotificationConfig{
		Enabled:        true,
		FCMEnabled:     true,
		FCMServerKey:   "key",
		FCMDeviceToken: "token",
		FCMEndpoint:    server.URL,
	}

	client := NewFCMClient(cfg)
	err := client.Send(context.Background(), BarkPayload{
		Title: "Test",
		Body:  "Retry",
	})

	if err != nil {
		t.Fatalf("expected successful send on retry, got: %v", err)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestFCMClientSend_MissingToken(t *testing.T) {
	cfg := config.NotificationConfig{
		Enabled:      true,
		FCMEnabled:   true,
		FCMServerKey: "key",
	}
	client := NewFCMClient(cfg)
	err := client.Send(context.Background(), BarkPayload{Title: "Test"})
	if err == nil {
		t.Errorf("expected error when device token is empty, got nil")
	}
}

func TestNotifier_MultiSender(t *testing.T) {
	var barkReceived int32
	barkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&barkReceived, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":200}`))
	}))
	defer barkServer.Close()

	var fcmReceived int32
	fcmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fcmReceived, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":1}`))
	}))
	defer fcmServer.Close()

	cfg := config.NotificationConfig{
		Enabled:        true,
		BarkEndpoint:   barkServer.URL,
		FCMEnabled:     true,
		FCMServerKey:   "fcm_key",
		FCMDeviceToken: "fcm_token",
		FCMEndpoint:    fcmServer.URL,
	}

	n := NewNotifier(cfg)
	if len(n.Senders()) != 2 {
		t.Fatalf("expected 2 senders, got %d", len(n.Senders()))
	}

	pi := &proxy.PendingInteraction{
		Type:      "permission",
		Action:    "run_command",
		Target:    "ls -la",
		StepIndex: 1,
	}

	err := n.NotifyAction("cas_multi", "Multi Test", pi)
	if err != nil {
		t.Fatalf("NotifyAction failed: %v", err)
	}

	// Give goroutines a brief moment to complete
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&barkReceived) != 1 {
		t.Errorf("expected Bark to receive 1 message, got %d", barkReceived)
	}
	if atomic.LoadInt32(&fcmReceived) != 1 {
		t.Errorf("expected FCM to receive 1 message, got %d", fcmReceived)
	}
}

func TestNotifier_UpdateFCMDeviceToken(t *testing.T) {
	var fcmCalls int32
	var lastReceivedToken string
	fcmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fcmCalls, 1)
		var p FCMPayload
		_ = json.NewDecoder(r.Body).Decode(&p)
		lastReceivedToken = p.To
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":1}`))
	}))
	defer fcmServer.Close()

	cfg := config.NotificationConfig{
		Enabled:      true,
		FCMServerKey: "fcm_key",
		FCMEndpoint:  fcmServer.URL,
		// No FCMDeviceToken initially
	}

	n := NewNotifier(cfg)
	// Dynamically register Android FCM token
	n.UpdateFCMDeviceToken("dynamic_token_999")

	if !n.IsEnabled() {
		t.Fatalf("expected Notifier to be enabled after registering token")
	}

	err := n.NotifyCompleted("cas_dynamic", "Dynamic Test", 3)
	if err != nil {
		t.Fatalf("NotifyCompleted failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&fcmCalls) != 1 {
		t.Errorf("expected 1 FCM call, got %d", fcmCalls)
	}
	if lastReceivedToken != "dynamic_token_999" {
		t.Errorf("expected token dynamic_token_999, got %q", lastReceivedToken)
	}
}
