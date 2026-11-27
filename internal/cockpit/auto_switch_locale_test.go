package cockpit

import (
	"strings"
	"testing"
	"unicode"

	"antigravity-mobile/internal/i18n"
)

func containsHanRunes(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func TestBuildAutoSwitchAlert_FourLocales(t *testing.T) {
	prev := i18n.GetNativeLocale()
	t.Cleanup(func() { i18n.SetNativeLocale(prev) })
	i18n.SetNativeLocale("zh-CN")

	alert := buildAutoSwitchAlert("fr-FR", AutoSwitchBucketGemini5h, "a@example.com", "b@example.com", 85, "n1")
	if containsHanRunes(alert.Title) || containsHanRunes(alert.Body) {
		t.Fatalf("fr-FR auto-switch alert leaked Chinese: title=%q body=%q", alert.Title, alert.Body)
	}
	if !strings.Contains(alert.Title, "Switched Account") {
		t.Fatalf("expected English fallback title for fr-FR, got %q", alert.Title)
	}

	zhTitle, zhBody := alert.Localize("zh-CN")
	if !strings.Contains(zhTitle, "已自动切换账号") || !strings.Contains(zhBody, "b@example.com") {
		t.Fatalf("unexpected zh localize: %q / %q", zhTitle, zhBody)
	}
	jaTitle, jaBody := alert.Localize("ja-JP")
	if !strings.Contains(jaTitle, "アカウント自動切替") || !strings.Contains(jaBody, "自動切替") {
		t.Fatalf("unexpected ja localize: %q / %q", jaTitle, jaBody)
	}
	for _, other := range []string{"en", "en-US", "fr-FR", "ko-KR", "de-DE"} {
		t2, b2 := alert.Localize(other)
		if containsHanRunes(t2) || containsHanRunes(b2) || !strings.Contains(t2, "Switched Account") {
			t.Errorf("Localize(%q) leaked non-English: %q / %q", other, t2, b2)
		}
	}
}
