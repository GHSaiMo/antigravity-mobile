package cockpit

import (
	"testing"
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
	}
}
