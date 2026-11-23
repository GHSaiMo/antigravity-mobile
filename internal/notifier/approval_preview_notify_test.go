package notifier

import (
	"testing"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/proxy"
)

func TestNotifyAction_HighRiskApprovalTitle(t *testing.T) {
	prevLang := i18n.GetLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })

	tests := []struct {
		lang      string
		wantTitle string
	}{
		{lang: "zh", wantTitle: "🚨 高危操作审批"},
		{lang: "en", wantTitle: "🚨 High-Risk Approval"},
		{lang: "ja", wantTitle: "🚨 高リスク操作の承認"},
	}

	for idx, tc := range tests {
		t.Run(tc.lang, func(t *testing.T) {
			i18n.SetLang(tc.lang)
			sender := &mockSender{}
			n := NewNotifierWithSender(config.NotificationConfig{Enabled: true}, sender)

			pi := &proxy.PendingInteraction{
				Type:      "run_command",
				Target:    "rm -rf ./dist",
				StepIndex: idx + 1,
				ApprovalPreview: &proxy.ApprovalPreviewDetails{
					RiskLevel:   "high",
					RiskReasons: []string{"destructive_delete"},
				},
			}
			if err := n.NotifyAction("cid-high", "Cleanup Build", pi); err != nil {
				t.Fatalf("NotifyAction failed: %v", err)
			}
			sent := sender.snapshot()
			if len(sent) != 1 {
				t.Fatalf("expected 1 push, got %d", len(sent))
			}
			if sent[0].Title != tc.wantTitle {
				t.Fatalf("Title = %q, want %q", sent[0].Title, tc.wantTitle)
			}
		})
	}
}

func TestNotifyAction_NonHighRiskKeepsStandardTitle(t *testing.T) {
	prevLang := i18n.GetLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })
	i18n.SetLang("zh")

	sender := &mockSender{}
	n := NewNotifierWithSender(config.NotificationConfig{Enabled: true}, sender)

	// Medium risk run_command keeps standard run-command title
	piMed := &proxy.PendingInteraction{
		Type:      "run_command",
		Target:    "git push origin feat/ui",
		StepIndex: 10,
		ApprovalPreview: &proxy.ApprovalPreviewDetails{
			RiskLevel:   "medium",
			RiskReasons: []string{"git_push"},
		},
	}
	if err := n.NotifyAction("cid-med", "Push Branch", piMed); err != nil {
		t.Fatalf("NotifyAction failed: %v", err)
	}

	// Ask question never upgrades to high-risk title
	piQ := &proxy.PendingInteraction{
		Type:      "ask_question",
		Title:     "Which option?",
		StepIndex: 11,
		ApprovalPreview: &proxy.ApprovalPreviewDetails{
			RiskLevel: "high",
		},
	}
	if err := n.NotifyAction("cid-q", "Question", piQ); err != nil {
		t.Fatalf("NotifyAction failed: %v", err)
	}

	sent := sender.snapshot()
	if len(sent) != 2 {
		t.Fatalf("expected 2 pushes, got %d", len(sent))
	}
	if sent[0].Title != MsgTitleRunCommand {
		t.Fatalf("medium risk title = %q, want %q", sent[0].Title, MsgTitleRunCommand)
	}
	if sent[1].Title != MsgTitleQuestion {
		t.Fatalf("ask_question title = %q, want %q", sent[1].Title, MsgTitleQuestion)
	}
}
