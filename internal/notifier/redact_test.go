package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/proxy"
)

func TestPushRedaction_APNs_FCM_And_LiveActivity(t *testing.T) {
	secretTitle := "Project-Aurora-Secret-Repo"
	secretCmd := "psql postgres://prod-db.internal/billing -c 'DROP TABLE audit'"
	secretFile := "/Users/alice/corp-secret/payroll.env"
	secretQuestion := "Should I rotate the Stripe prod key?"
	secretFocus := "Decrypting customer vault keys"
	secretGatewayLabel := "Corp-Mac-Studio-Prod"
	secretEmail := "ceo@acme-secret.example"

	var (
		mu             sync.Mutex
		apnsAlerts     []APNsRelayAlertRequest
		apnsLA         []APNsLiveActivityRequest
		fcmPayloads    []FCMPayload
	)

	apnsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/push/alert":
			var req APNsRelayAlertRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				mu.Lock()
				apnsAlerts = append(apnsAlerts, req)
				mu.Unlock()
			}
		case "/v1/push/live-activity":
			var req APNsLiveActivityRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				mu.Lock()
				apnsLA = append(apnsLA, req)
				mu.Unlock()
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"status":200}`))
	}))
	defer apnsSrv.Close()

	fcmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req FCMPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			mu.Lock()
			fcmPayloads = append(fcmPayloads, req)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":1}`))
	}))
	defer fcmSrv.Close()

	store, err := NewPushDeviceStore("")
	if err != nil {
		t.Fatalf("NewPushDeviceStore: %v", err)
	}
	enTrue := true
	hexTok := strings.Repeat("ab", 32)
	hexStartTok := strings.Repeat("cd", 32)
	if err := store.UpsertAlertToken("dev-ios-zh", "ios", hexTok, "production", &enTrue, &enTrue); err != nil {
		t.Fatalf("UpsertAlertToken: %v", err)
	}
	_ = store.UpdateLanguage("dev-ios-zh", "zh")
	_ = store.UpdateGatewayLabel("dev-ios-zh", &enTrue, secretGatewayLabel)
	if err := store.UpsertLiveActivityStartToken("dev-ios-zh", "ios", hexStartTok, "production", &enTrue); err != nil {
		t.Fatalf("UpsertLiveActivityStartToken: %v", err)
	}

	hexTokEn := strings.Repeat("ef", 32)
	if err := store.UpsertAlertToken("dev-ios-en", "ios", hexTokEn, "production", &enTrue, &enTrue); err != nil {
		t.Fatalf("UpsertAlertToken en: %v", err)
	}
	_ = store.UpdateLanguage("dev-ios-en", "en")
	_ = store.UpdateGatewayLabel("dev-ios-en", &enTrue, secretGatewayLabel)

	if err := store.UpsertFCMToken("dev-and-ja", "fcm-tok-ja", "ja", &enTrue); err != nil {
		t.Fatalf("UpsertFCMToken: %v", err)
	}
	_ = store.UpdateGatewayLabel("dev-and-ja", &enTrue, secretGatewayLabel)

	cfg := config.NotificationConfig{
		Enabled:                 true,
		APNSEnabled:             true,
		APNSRelayURL:            apnsSrv.URL,
		LiveActivityPushEnabled: true,
		PushRedact:              true,
		FCMEnabled:              true,
		FCMEndpoint:             fcmSrv.URL,
		FCMServerKey:            "test-key",
	}
	n := NewNotifier(cfg)
	n.SetGatewayIdentity("gw_1234567890abcdef", secretGatewayLabel)
	n.AttachPushStore(store)
	n.AttachAPNsClient(NewAPNsRelayClient(apnsSrv.URL, store), true)

	// 1. High-risk command approval
	if err := n.NotifyAction("cid-1", secretTitle, &proxy.PendingInteraction{
		Type:      "run_command",
		StepIndex: 4,
		Target:    secretCmd,
		ApprovalPreview: &proxy.ApprovalPreviewDetails{
			RiskLevel: "high",
		},
	}); err != nil {
		t.Fatalf("NotifyAction high-risk: %v", err)
	}

	// 2. File permission approval
	if err := n.NotifyAction("cid-2", secretTitle, &proxy.PendingInteraction{
		Type:      "file_permission",
		StepIndex: 5,
		Target:    secretFile,
	}); err != nil {
		t.Fatalf("NotifyAction file: %v", err)
	}

	// 3. Question
	if err := n.NotifyAction("cid-3", secretTitle, &proxy.PendingInteraction{
		Type:      "ask_question",
		StepIndex: 6,
		Title:     secretQuestion,
	}); err != nil {
		t.Fatalf("NotifyAction question: %v", err)
	}

	// 4. Proceed / Completed / Failed / Stall / Quota
	_ = n.NotifyProceed("cid-4", secretTitle, 7)
	_ = n.NotifyCompleted("cid-5", secretTitle, 12)
	_ = n.NotifyFailed("cid-6", secretTitle, 3)
	_ = n.NotifyStallAlert("cid-7", secretTitle, 9, 4)
	_ = n.NotifyQuotaAlertWithLevel("critical", "q-1", "🚨 Gemini 5h 额度仅剩 3%", "账号 "+secretEmail)

	// 5. Live Activity Push-to-Start with ActivePlan ratio 3/5 and sensitive fields
	details := &proxy.TrajectoryDetails{
		CascadeID:   "cid-la",
		Title:       secretTitle,
		Status:      "CASCADE_RUN_STATUS_RUNNING",
		TotalSteps:  8,
		ActiveFocus: secretFocus,
		ActivePlan: &proxy.StepPlanItem{
			DoneCount:  3,
			TotalCount: 5,
			Items:      []proxy.StepPlanItemEntry{{Title: secretFocus, Status: "in_progress"}},
		},
		RunningTasks: []proxy.RunningTaskItem{
			{ID: "t1", ToolSummary: secretFocus, CommandLine: secretCmd},
		},
	}
	n.LiveActivity().SyncFromDetails(context.Background(), details)

	// Verify zero leakage across ALL APNs alerts, FCM payloads, and Live Activity payloads
	secrets := []string{secretTitle, secretCmd, secretFile, secretQuestion, secretFocus, secretGatewayLabel, secretEmail, "billing", "payroll", "Stripe"}

	mu.Lock()
	defer mu.Unlock()

	if len(apnsAlerts) == 0 || len(fcmPayloads) == 0 || len(apnsLA) == 0 {
		t.Fatalf("expected pushes across all channels: apns=%d fcm=%d la=%d", len(apnsAlerts), len(fcmPayloads), len(apnsLA))
	}

	for i, a := range apnsAlerts {
		raw, _ := json.Marshal(a)
		s := string(raw)
		for _, sec := range secrets {
			if strings.Contains(s, sec) {
				t.Errorf("APNs alert[%d] leaked secret %q: %s", i, sec, s)
			}
		}
		if a.Subtitle != "" {
			t.Errorf("APNs alert[%d] expected empty redacted subtitle, got %q", i, a.Subtitle)
		}
	}

	// Ensure high-risk category & step count survived redaction
	foundHighRisk := false
	foundCompletedSteps := false
	for _, a := range apnsAlerts {
		if a.Category == "antigravity_action_high" {
			foundHighRisk = true
		}
		if a.Category == "antigravity_complete" && strings.Contains(a.Body, "12") {
			foundCompletedSteps = true
		}
	}
	if !foundHighRisk {
		t.Errorf("expected high-risk category preserved under redaction")
	}
	if !foundCompletedSteps {
		t.Errorf("expected numeric step count (12) preserved in redacted completion alert")
	}

	for i, f := range fcmPayloads {
		raw, _ := json.Marshal(f)
		s := string(raw)
		for _, sec := range secrets {
			if strings.Contains(s, sec) {
				t.Errorf("FCM payload[%d] leaked secret %q: %s", i, sec, s)
			}
		}
		if f.Data["gatewayLabel"] != "" {
			t.Errorf("FCM payload[%d] leaked gatewayLabel %q", i, f.Data["gatewayLabel"])
		}
	}

	// Verify Live Activity retained pure numeric progress "3/5" and stripped all text/commands
	la := apnsLA[0]
	laRaw, _ := json.Marshal(la)
	laStr := string(laRaw)
	for _, sec := range secrets {
		if strings.Contains(laStr, sec) {
			t.Errorf("Live Activity payload leaked secret %q: %s", sec, laStr)
		}
	}
	if la.ContentState.LatestAction != "3/5" {
		t.Errorf("expected redacted Live Activity LatestAction to be pure ratio '3/5', got %q", la.ContentState.LatestAction)
	}
	if len(la.ContentState.RunningTasks) != 0 || la.ContentState.ActiveTaskTitle != "" || la.ContentState.ActiveTaskCommand != "" {
		t.Errorf("expected redacted Live Activity to strip RunningTasks/ActiveTask, got %+v", la.ContentState)
	}
	if la.Attributes == nil || la.Attributes.GatewayLabel != "" {
		t.Errorf("expected redacted Live Activity Attributes with empty GatewayLabel, got %+v", la.Attributes)
	}
}

func TestConfigPushRedactEnvAliases(t *testing.T) {
	for _, envKey := range []string{"MGY_PUSH_REDACT", "PUSH_REDACT", "APNS_REDACT"} {
		t.Run(envKey, func(t *testing.T) {
			t.Setenv("MGY_PUSH_REDACT", "")
			t.Setenv("PUSH_REDACT", "")
			t.Setenv("APNS_REDACT", "")
			t.Setenv(envKey, "1")
			cfg := config.GetNotificationConfig()
			if !cfg.PushRedact {
				t.Fatalf("expected PushRedact=true when %s=1", envKey)
			}
		})
	}
	_ = time.Now()
}
