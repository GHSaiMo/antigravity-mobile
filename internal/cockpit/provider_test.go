package cockpit

import (
	"crypto/sha256"
	"encoding/hex"
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
		t.Logf("[%d] %s (current: %v) | Gemini 5h: %.1f%% (%s) | Claude 5h: %.1f%% (%s)",
			i, a.Email, a.IsCurrent,
			a.Gemini5h.RemainingPercent, a.Gemini5h.ResetFriendly,
			a.Claude5h.RemainingPercent, a.Claude5h.ResetFriendly)
		t.Logf("    Weekly -> Gemini: %.1f%% (%s) | Claude: %.1f%% (%s)",
			a.GeminiWeekly.RemainingPercent, a.GeminiWeekly.ResetFriendly,
			a.ClaudeWeekly.RemainingPercent, a.ClaudeWeekly.ResetFriendly)
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
