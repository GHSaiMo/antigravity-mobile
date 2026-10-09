package notifier

import (
	"context"
	"sync"
	"testing"

	"antigravity-mobile/internal/config"
)

type captureSender struct {
	mu   sync.Mutex
	sent []BarkPayload
}

func (c *captureSender) Send(_ context.Context, p BarkPayload) error {
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
	if err := n.NotifyQuotaAlert("acc|2026-10-09T10:00:00Z", "⚠️ Gemini 5h 额度仅剩 18%", "账号 t***n@gmail.com", false); err != nil {
		t.Fatal(err)
	}
	if len(s.sent) != 1 {
		t.Fatalf("sent %d", len(s.sent))
	}
	p := s.sent[0]
	if p.Level != "active" || p.Sound != "" || p.Category != "quota_alert" || p.Group != "Antigravity" {
		t.Errorf("warn payload should be a normal-priority, normal-priority push with the default tone: %+v", p)
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
