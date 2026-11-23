package notifier

import (
	"context"
	"strings"
	"sync"
	"testing"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/proxy"
)

type captureSender struct {
	mu   sync.Mutex
	sent []PushPayload
}

func (c *captureSender) Send(_ context.Context, p PushPayload) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, p)
	return nil
}

func newQuotaTestNotifier() (*Notifier, *captureSender) {
	s := &captureSender{}
	n := NewNotifierWithSender(config.NotificationConfig{Enabled: true, Group: "Antigravity", SoundAction: "alarm"}, s)
	return n, s
}

func TestNotifyQuotaAlertWarnIsQuiet(t *testing.T) {
	n, s := newQuotaTestNotifier()
	if err := n.NotifyQuotaAlert("acc|2026-10-09T10:00:00Z", "⚠️ Gemini 5h 额度仅剩 18%", "账号 taojiuzhen@gmail.com", false); err != nil {
		t.Fatal(err)
	}
	if len(s.sent) != 1 {
		t.Fatalf("sent %d", len(s.sent))
	}
	p := s.sent[0]
	if p.Level != "active" || p.Sound != "" || p.Category != "quota_alert" || p.Group != "Antigravity" || p.URL != "antigravity://quota" {
		t.Errorf("warn payload should be a normal-priority push with quota deeplink and default tone: %+v", p)
	}
}

func TestNotifyQuotaAlertCriticalBreaksThroughFocusButNotMute(t *testing.T) {
	n, s := newQuotaTestNotifier()
	if err := n.NotifyQuotaAlert("acc|2026-10-09T10:00:00Z", "🚨 Gemini 5h 额度仅剩 4%", "body", true); err != nil {
		t.Fatal(err)
	}
	p := s.sent[0]
	if p.Level != "timeSensitive" || p.Sound != "alarm" {
		t.Errorf("critical must be time-sensitive with the alert sound (never 'critical', which ignores the mute switch): %+v", p)
	}
}

func TestNotifyQuotaAlertDedupesPerLevelAndKey(t *testing.T) {
	n, s := newQuotaTestNotifier()
	_ = n.NotifyQuotaAlert("k1", "t", "b", false)
	_ = n.NotifyQuotaAlert("k1", "t", "b", false) // 同周期同档重复
	_ = n.NotifyQuotaAlert("k1", "t", "b", true)  // 同周期但升级到紧急：要发
	_ = n.NotifyQuotaAlert("k2", "t", "b", false) // 新周期：要发
	if len(s.sent) != 3 {
		t.Errorf("want 3 pushes (warn, critical, next cycle warn), got %d", len(s.sent))
	}
}

func TestNotifyQuotaAlertDisabledNotifierIsNoop(t *testing.T) {
	n := NewNotifier(config.NotificationConfig{Enabled: false})
	if err := n.NotifyQuotaAlert("k", "t", "b", true); err != nil {
		t.Errorf("disabled notifier must be a silent no-op, got %v", err)
	}
}

func TestNotifierTrilingualPushTemplates(t *testing.T) {
	prevLang := i18n.GetLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })

	for _, tc := range []struct {
		lang          string
		wantCmdTitle  string
		wantDoneTitle string
		wantDoneSub   string
	}{
		{"zh", "⚠️ 确认执行终端命令", "🎉 Antigravity 任务已完成", "共执行 7 个步骤"},
		{"en", "⚠️ Confirm Terminal Command", "🎉 Antigravity Task Completed", "(7 steps)"},
		{"ja", "⚠️ コマンド実行の確認", "🎉 Antigravity タスク完了", "全 7 ステップ"},
	} {
		i18n.SetLang(tc.lang)
		n, s := newQuotaTestNotifier()
		_ = n.NotifyAction("cas-1", "Demo", &proxy.PendingInteraction{Type: "run_command", Target: "go test", StepIndex: 2})
		_ = n.NotifyCompleted("cas-1", "Demo", 7)
		if len(s.sent) != 2 {
			t.Fatalf("[%s] expected 2 notifications, got %d", tc.lang, len(s.sent))
		}
		if s.sent[0].Title != tc.wantCmdTitle {
			t.Errorf("[%s] action title = %q, want %q", tc.lang, s.sent[0].Title, tc.wantCmdTitle)
		}
		if s.sent[1].Title != tc.wantDoneTitle || !strings.Contains(s.sent[1].Body, tc.wantDoneSub) {
			t.Errorf("[%s] completed = (%q, %q), want title %q and body containing %q", tc.lang, s.sent[1].Title, s.sent[1].Body, tc.wantDoneTitle, tc.wantDoneSub)
		}
	}
}
