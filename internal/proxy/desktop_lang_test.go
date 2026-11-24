package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode"

	"antigravity-mobile/internal/i18n"
)

func containsHan(b []byte) bool {
	for _, r := range string(b) {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func TestResolveDesktopLangPrecedence(t *testing.T) {
	prev := i18n.GetLang()
	i18n.SetLang(i18n.LangZh)
	defer i18n.SetLang(prev)

	// 1. Accept-Language weighted negotiation
	reqJa := httptest.NewRequest(http.MethodGet, "/", nil)
	reqJa.Header.Set("Accept-Language", "en-US;q=0.7, ja-JP;q=0.9, zh-CN;q=0.5")
	if got := ResolveDesktopLang(reqJa); got != i18n.LangJa {
		t.Fatalf("Accept-Language ja highest weight: got %q, want ja", got)
	}

	reqFr := httptest.NewRequest(http.MethodGet, "/", nil)
	reqFr.Header.Set("Accept-Language", "fr-FR,fr;q=0.9")
	if got := ResolveDesktopLang(reqFr); got != i18n.LangEn {
		t.Fatalf("non-zh/ja Accept-Language should map to en, got %q", got)
	}

	// 2. Cookie overrides Accept-Language
	reqCookie := httptest.NewRequest(http.MethodGet, "/", nil)
	reqCookie.Header.Set("Accept-Language", "ja-JP,ja;q=0.9")
	reqCookie.AddCookie(&http.Cookie{Name: DesktopLangCookieName, Value: "en"})
	if got := ResolveDesktopLang(reqCookie); got != i18n.LangEn {
		t.Fatalf("cookie en should override Accept-Language ja, got %q", got)
	}

	// 3. Explicit ?lang= overrides cookie
	reqQuery := httptest.NewRequest(http.MethodGet, "/?lang=zh", nil)
	reqQuery.AddCookie(&http.Cookie{Name: DesktopLangCookieName, Value: "en"})
	if got := ResolveDesktopLang(reqQuery); got != i18n.LangZh {
		t.Fatalf("?lang=zh should override cookie en, got %q", got)
	}

	// 4. Empty Accept-Language falls back to gateway active lang
	reqEmpty := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := ResolveDesktopLang(reqEmpty); got != i18n.LangZh {
		t.Fatalf("empty Accept-Language should fall back to gateway lang zh, got %q", got)
	}
}

func TestLocalizeMainJSForLang_Trilingual(t *testing.T) {
	sample := []byte(`const a = "New Conversation"; const b = { text: "Settings", label: "Automations" }; const c = "This Computer"; const d = "@import url('https://fonts.googleapis.com/css2');"; this._disposeEntry(a)},3E4);`)

	// English: patches applied, zero Chinese characters, native English preserved
	enOut := LocalizeMainJSForLang(sample, i18n.LangEn)
	if containsHan(enOut) {
		t.Fatalf("EN bundle must contain zero Han chars, got: %s", string(enOut))
	}
	if !bytes.Contains(enOut, []byte(`"New Conversation"`)) || !bytes.Contains(enOut, []byte(`text: "Settings"`)) {
		t.Fatalf("EN bundle should preserve native English strings, got: %s", string(enOut))
	}
	if bytes.Contains(enOut, []byte(`fonts.googleapis.com`)) || !bytes.Contains(enOut, []byte(`this._disposeEntry(a)},0)`)) {
		t.Fatalf("EN bundle must still apply runtime patches, got: %s", string(enOut))
	}

	// Japanese: Japanese strings applied, patches applied
	jaOut := LocalizeMainJSForLang(sample, i18n.LangJa)
	if !bytes.Contains(jaOut, []byte(`"新しい会話"`)) || !bytes.Contains(jaOut, []byte(`text: "設定"`)) || !bytes.Contains(jaOut, []byte(`"このPC"`)) {
		t.Fatalf("JA bundle missing expected Japanese translations: %s", string(jaOut))
	}
	if bytes.Contains(jaOut, []byte(`新建会话`)) || bytes.Contains(jaOut, []byte(`系统设置`)) {
		t.Fatalf("JA bundle leaked Chinese strings: %s", string(jaOut))
	}

	// Chinese: existing behavior preserved
	zhOut := LocalizeMainJSForLang(sample, i18n.LangZh)
	if !bytes.Contains(zhOut, []byte(`"新建会话"`)) || !bytes.Contains(zhOut, []byte(`text: "系统设置"`)) {
		t.Fatalf("ZH bundle missing expected Chinese translations: %s", string(zhOut))
	}
}

func TestLocalizeRPCResponseForLang(t *testing.T) {
	body := []byte(`{"groups":[{"displayName":"Gemini Models","description":"You have used some of your weekly limit, it will fully refresh in 2 days, 4 hours."}]}`)
	path := "/exa.language_server_pb.LanguageServerService/RetrieveUserQuotaSummary"

	enOut := LocalizeRPCResponseForLang(path, body, i18n.LangEn)
	if string(enOut) != string(body) {
		t.Fatalf("EN RPC response should remain verbatim English, got: %s", string(enOut))
	}

	jaOut := LocalizeRPCResponseForLang(path, body, i18n.LangJa)
	if !strings.Contains(string(jaOut), "Gemini モデル") || !strings.Contains(string(jaOut), "2日 4時間後に完全リセットされます。") {
		t.Fatalf("JA RPC response mismatch: %s", string(jaOut))
	}

	zhOut := LocalizeRPCResponseForLang(path, body, i18n.LangZh)
	if !strings.Contains(string(zhOut), "Gemini 模型群") {
		t.Fatalf("ZH RPC response mismatch: %s", string(zhOut))
	}
}
