package cockpit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetQuotas(t *testing.T) {
	resp, err := GetQuotas()
	if err != nil {
		t.Skipf("Local Cockpit installation not available: %v, skipping local integration test", err)
		return
	}
	if resp.CurrentAccount == nil || len(resp.Accounts) == 0 {
		t.Skip("No active accounts found in local Cockpit, skipping local integration test")
		return
	}
	t.Logf("Current Account: %s (%s)", resp.CurrentAccount.Email, resp.CurrentAccount.Name)
	t.Logf("Total Accounts: %d", len(resp.Accounts))
	for i, a := range resp.Accounts {
		argonStr := "—"
		if a.Gemini4Argon != nil {
			argonStr = a.Gemini4Argon.ResetFriendly
		}
		g5hStr := "—"
		if a.Gemini5h != nil {
			g5hStr = fmt.Sprintf("%.1f%% (%s)", a.Gemini5h.RemainingPercent, a.Gemini5h.ResetFriendly)
		}
		c5hStr := "—"
		if a.Claude5h != nil {
			c5hStr = fmt.Sprintf("%.1f%% (%s)", a.Claude5h.RemainingPercent, a.Claude5h.ResetFriendly)
		}
		gWkStr := "—"
		if a.GeminiWeekly != nil {
			gWkStr = fmt.Sprintf("%.1f%% (%s)", a.GeminiWeekly.RemainingPercent, a.GeminiWeekly.ResetFriendly)
		}
		cWkStr := "—"
		if a.ClaudeWeekly != nil {
			cWkStr = fmt.Sprintf("%.1f%% (%s)", a.ClaudeWeekly.RemainingPercent, a.ClaudeWeekly.ResetFriendly)
		}
		t.Logf("[%d] %s [tier=%s] (current: %v) | Gemini 5h: %s | Claude 5h: %s | Argon: %v (%s)",
			i, a.Email, a.SubscriptionTier, a.IsCurrent, g5hStr, c5hStr, a.Gemini4Argon != nil, argonStr)
		t.Logf("    Weekly -> Gemini: %s | Claude: %s", gWkStr, cWkStr)
	}
}

func TestGetQuotas_Hermetic(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", tmpDir)
	InvalidateQuotaCache()

	// 1. Create accounts.json
	accountsJSON := `{
		"current_account_id": "acc-mock-1",
		"accounts": [
			{"id": "acc-mock-1", "email": "hermetic@example.com", "name": "Hermetic Tester"}
		]
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, "accounts.json"), []byte(accountsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Create mock cache
	cacheDir := filepath.Join(tmpDir, "cache", "quota_api_v1_desktop", "authorized")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	emailNorm := "hermetic@example.com"
	h := sha256.Sum256([]byte(emailNorm))
	hashHex := hex.EncodeToString(h[:])
	cacheFile := filepath.Join(cacheDir, hashHex+".json")

	mockCache := `{
		"updatedAt": 1720000000,
		"payload": {
			"quota_summary": {
				"groups": [
					{
						"buckets": [
							{"bucketId": "gemini-5h", "remainingFraction": 0.85, "resetTime": "2026-09-14T20:00:00Z"},
							{"bucketId": "3p-5h", "remainingFraction": 0.42, "resetTime": "2026-09-14T21:00:00Z"}
						]
					}
				]
			}
		}
	}`
	if err := os.WriteFile(cacheFile, []byte(mockCache), 0644); err != nil {
		t.Fatal(err)
	}

	resp, err := GetQuotas()
	if err != nil {
		t.Fatalf("GetQuotas failed in hermetic test: %v", err)
	}

	if resp.CurrentAccount == nil {
		t.Fatalf("expected CurrentAccount to be non-nil")
	}
	if resp.CurrentAccount.Email != "hermetic@example.com" {
		t.Errorf("expected email hermetic@example.com, got %s", resp.CurrentAccount.Email)
	}
	if len(resp.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(resp.Accounts))
	}
	acc := resp.Accounts[0]
	if acc.Gemini5h == nil || acc.Gemini5h.RemainingPercent != 85.0 {
		t.Errorf("expected Gemini5h 85%%, got %+v", acc.Gemini5h)
	}
	if acc.Claude5h == nil || acc.Claude5h.RemainingPercent != 42.0 {
		t.Errorf("expected Claude5h 42%%, got %+v", acc.Claude5h)
	}
}

func TestGetQuotas_LiveEmailOverridesCockpitCurrent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", tmpDir)
	InvalidateQuotaCache()

	accountsJSON := `{
		"current_account_id": "acc-cockpit",
		"accounts": [
			{"id": "acc-cockpit", "email": "cockpit@example.com", "name": "Cockpit"},
			{"id": "acc-live", "email": "live@example.com", "name": "Live"}
		]
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, "accounts.json"), []byte(accountsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	resp, err := GetQuotas("live@example.com")
	if err != nil {
		t.Fatalf("GetQuotas: %v", err)
	}
	if resp.CurrentAccount == nil {
		t.Fatal("expected CurrentAccount")
	}
	if resp.CurrentAccount.Email != "live@example.com" {
		t.Fatalf("current email = %s, want live@example.com (runtime identity must beat Cockpit current_account_id)", resp.CurrentAccount.Email)
	}
}

func TestFormatResetFriendly(t *testing.T) {
	now := time.Now()

	// 1. Empty string
	if got := formatResetFriendly(""); got != "未知" {
		t.Errorf("expected '未知', got '%s'", got)
	}

	// 2. Past time (already reset)
	past := now.Add(-5 * time.Minute).Format(time.RFC3339)
	if got := formatResetFriendly(past); got != "已就绪" {
		t.Errorf("expected '已就绪', got '%s'", got)
	}

	// 3. Less than 24h: 7h 3m
	under24h := now.Add(7*time.Hour + 3*time.Minute + 10*time.Second).Format(time.RFC3339)
	if got := formatResetFriendly(under24h); got != "7h 3m" {
		t.Errorf("expected '7h 3m', got '%s'", got)
	}

	// 4. More than 24h: 3d 11h 34m
	over24h := now.Add(3*24*time.Hour + 11*time.Hour + 34*time.Minute + 10*time.Second).Format(time.RFC3339)
	if got := formatResetFriendly(over24h); got != "3d 11h 34m" {
		t.Errorf("expected '3d 11h 34m', got '%s'", got)
	}

	// 5. Less than 1h: 34m
	under1h := now.Add(34*time.Minute + 10*time.Second).Format(time.RFC3339)
	if got := formatResetFriendly(under1h); got != "34m" {
		t.Errorf("expected '34m', got '%s'", got)
	}

	// 6. Less than 1m
	under1m := now.Add(20 * time.Second).Format(time.RFC3339)
	if got := formatResetFriendly(under1m); got != "<1m" {
		t.Errorf("expected '<1m', got '%s'", got)
	}
}

func TestEnsureCockpitAntigravityConfig_AlignsConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", tmpDir)

	configFile := filepath.Join(tmpDir, "config.json")
	initialConfig := map[string]any{
		"antigravity_launch_on_switch": true,
		"antigravity_app_path":        "",
		"other_field":                 "keep_me",
	}
	b, err := json.Marshal(initialConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, b, 0644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureCockpitAntigravityConfig(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	readBytes, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	var updated map[string]any
	if err := json.Unmarshal(readBytes, &updated); err != nil {
		t.Fatal(err)
	}

	if val, ok := updated["antigravity_launch_on_switch"].(bool); !ok || val {
		t.Errorf("expected antigravity_launch_on_switch to be false, got %v", updated["antigravity_launch_on_switch"])
	}
	if updated["other_field"] != "keep_me" {
		t.Errorf("expected other_field to be preserved, got %v", updated["other_field"])
	}

	// Verify .bak file created
	if _, err := os.Stat(configFile + ".bak"); err != nil {
		t.Errorf("expected config.json.bak to exist: %v", err)
	}
}

func TestEnsureCockpitAntigravityConfig_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", tmpDir)

	if err := EnsureCockpitAntigravityConfig(); err != nil {
		t.Fatalf("expected nil when config.json is absent, got: %v", err)
	}
}

func TestGetQuotas_ArgonAndSubscriptionTier(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", tmpDir)
	t.Setenv("COCKPIT_TOOLS_DATA_DIR", tmpDir)
	InvalidateQuotaCache()

	accountsJSON := `{
		"current_account_id": "acc-ultra",
		"accounts": [
			{"id": "acc-ultra", "email": "ultra@example.com", "name": "Ultra User"},
			{"id": "acc-pro", "email": "pro@example.com", "name": "Pro User"},
			{"id": "acc-free", "email": "free@example.com", "name": "Free User"}
		]
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, "accounts.json"), []byte(accountsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	accountsDir := filepath.Join(tmpDir, "accounts")
	if err := os.MkdirAll(accountsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(accountsDir, "acc-ultra.json"), []byte(`{"id":"acc-ultra","email":"ultra@example.com","quota":{"subscription_tier":"g1-ultra-tier"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(accountsDir, "acc-pro.json"), []byte(`{"id":"acc-pro","email":"pro@example.com","quota":{"subscription_tier":"g1-pro-tier"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(accountsDir, "acc-free.json"), []byte(`{"id":"acc-free","email":"free@example.com","quota":{"subscription_tier":"free-tier"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(tmpDir, "cache", "quota_api_v1_desktop", "authorized")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeCache := func(email, content string) {
		h := sha256.Sum256([]byte(email))
		hashHex := hex.EncodeToString(h[:])
		if err := os.WriteFile(filepath.Join(cacheDir, hashHex+".json"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Ultra account: has gemini-4-argon-high in models
	writeCache("ultra@example.com", `{
		"updatedAt": 1720000100,
		"payload": {
			"models": {
				"gemini-4-argon-high": {
					"displayName": "Gemini 4 Argon (High)",
					"quotaInfo": {"remainingFraction": 0.92, "resetTime": "2026-09-20T06:00:00Z"}
				}
			},
			"quota_summary": {
				"groups": [{
					"buckets": [
						{"bucketId": "gemini-5h", "remainingFraction": 1.0, "resetTime": "2026-09-14T20:00:00Z"}
					]
				}]
			}
		}
	}`)

	// Pro account: only has gemini-4-argon-disabled with empty quotaInfo (no Argon quota)
	writeCache("pro@example.com", `{
		"updatedAt": 1720000100,
		"payload": {
			"models": {
				"gemini-4-argon-disabled": {
					"displayName": "",
					"quotaInfo": {}
				}
			},
			"quota_summary": {
				"groups": [{
					"buckets": [
						{"bucketId": "gemini-5h", "remainingFraction": 0.8, "resetTime": "2026-09-14T20:00:00Z"}
					]
				}]
			}
		}
	}`)

	// Free account: only has weekly quotas via models, no 5h
	writeCache("free@example.com", `{
		"updatedAt": 1720000100,
		"payload": {
			"models": {
				"claude-opus-4-6-thinking": {
					"displayName": "Claude Opus 4.6 (Thinking)",
					"quotaInfo": {"remainingFraction": 1.0, "resetTime": "2026-09-21T06:00:00Z"}
				},
				"gemini-3.8-flash-high": {
					"displayName": "Gemini 3.8 Flash (High)",
					"quotaInfo": {"remainingFraction": 0.95, "resetTime": "2026-09-21T06:00:00Z"}
				}
			}
		}
	}`)

	resp, err := GetQuotas()
	if err != nil {
		t.Fatalf("GetQuotas failed: %v", err)
	}
	if len(resp.Accounts) != 3 {
		t.Fatalf("expected 3 accounts, got %d", len(resp.Accounts))
	}

	ultra := resp.Accounts[0]
	if ultra.SubscriptionTier != "ULTRA" {
		t.Errorf("expected ultra tier ULTRA, got %q", ultra.SubscriptionTier)
	}
	if ultra.Gemini4Argon == nil || ultra.Gemini4Argon.RemainingPercent != 92.0 {
		t.Errorf("expected ultra Gemini4Argon 92.0%%, got %+v", ultra.Gemini4Argon)
	}
	if ultra.Gemini5h == nil || ultra.Claude5h == nil {
		t.Errorf("expected ultra 5h quotas, got %+v", ultra)
	}

	pro := resp.Accounts[1]
	if pro.SubscriptionTier != "PRO" {
		t.Errorf("expected pro tier PRO, got %q", pro.SubscriptionTier)
	}
	if pro.Gemini4Argon != nil {
		t.Errorf("expected pro Gemini4Argon to be nil (dynamically hidden), got %+v", pro.Gemini4Argon)
	}
	if pro.Gemini5h == nil || pro.Claude5h == nil {
		t.Errorf("expected pro 5h quotas, got %+v", pro)
	}

	freeAcc := resp.Accounts[2]
	if freeAcc.SubscriptionTier != "FREE" {
		t.Errorf("expected free tier FREE, got %q", freeAcc.SubscriptionTier)
	}
	if freeAcc.Gemini4Argon != nil {
		t.Errorf("expected free Gemini4Argon to be nil, got %+v", freeAcc.Gemini4Argon)
	}
	if freeAcc.Claude5h != nil {
		t.Errorf("expected free Claude5h to be nil, got %+v", freeAcc.Claude5h)
	}
	if freeAcc.Gemini5h != nil {
		t.Errorf("expected free Gemini5h to be nil, got %+v", freeAcc.Gemini5h)
	}
	if freeAcc.ClaudeWeekly == nil || freeAcc.ClaudeWeekly.RemainingPercent != 100.0 {
		t.Errorf("expected free ClaudeWeekly to be 100%%, got %+v", freeAcc.ClaudeWeekly)
	}
	if freeAcc.GeminiWeekly == nil || freeAcc.GeminiWeekly.RemainingPercent != 95.0 {
		t.Errorf("expected free GeminiWeekly to be 95%%, got %+v", freeAcc.GeminiWeekly)
	}
}


