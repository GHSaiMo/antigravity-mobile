package cockpit

import (
	"testing"
	"time"
)

func TestGetQuotas(t *testing.T) {
	resp, err := GetQuotas()
	if err != nil {
		t.Fatalf("GetQuotas failed: %v", err)
	}
	if resp.CurrentAccount == nil {
		t.Fatalf("Expected CurrentAccount to be non-nil")
	}
	if len(resp.Accounts) == 0 {
		t.Fatalf("Expected Accounts to have items")
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
