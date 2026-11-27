package i18n

import (
	"testing"
)

func TestI18nBasic(t *testing.T) {
	SetLang("zh")
	if !IsZh() || IsEn() {
		t.Fatalf("expected zh, got %s", GetLang())
	}
	if got := T("你好", "Hello"); got != "你好" {
		t.Fatalf("expected '你好', got '%s'", got)
	}
	if got := Sprintf("端口: %d", "Port: %d", 58900); got != "端口: 58900" {
		t.Fatalf("unexpected formatted string: %s", got)
	}

	SetLang("en")
	if !IsEn() || IsZh() || IsJa() {
		t.Fatalf("expected en, got %s", GetLang())
	}
	if got := T("你好", "Hello"); got != "Hello" {
		t.Fatalf("expected 'Hello', got '%s'", got)
	}
	if got := Sprintf("端口: %d", "Port: %d", 58900); got != "Port: 58900" {
		t.Fatalf("unexpected formatted string: %s", got)
	}

	SetLang("ja")
	t.Cleanup(func() { SetLang("zh") })
	if !IsJa() || IsZh() || IsEn() {
		t.Fatalf("expected ja, got %s", GetLang())
	}
	if got := T3("你好", "Hello", "こんにちは"); got != "こんにちは" {
		t.Fatalf("expected 'こんにちは', got '%s'", got)
	}
	if got := Sprintf3("端口: %d", "Port: %d", "ポート: %d", 58900); got != "ポート: 58900" {
		t.Fatalf("unexpected ja formatted string: %s", got)
	}
	// T(zh, en) keeps English fallback when Japanese is active
	if got := T("你好", "Hello"); got != "Hello" {
		t.Fatalf("expected English fallback for T() under ja, got '%s'", got)
	}
}

func TestI18nInitOverride(t *testing.T) {
	t.Cleanup(func() { SetLang("zh") })
	Init("en")
	if !IsEn() {
		t.Fatalf("expected en from override, got %s", GetLang())
	}

	Init("ja-JP")
	if !IsJa() {
		t.Fatalf("expected ja from override, got %s", GetLang())
	}

	Init("zh-CN")
	if !IsZh() {
		t.Fatalf("expected zh from override, got %s", GetLang())
	}
}

func TestI18nEnvOverride(t *testing.T) {
	t.Cleanup(func() { SetLang("zh") })
	t.Setenv("MGY_LANG", "en")
	Init("")
	if !IsEn() {
		t.Fatalf("expected en from MGY_LANG, got %s", GetLang())
	}

	t.Setenv("MGY_LANG", "ja")
	Init("")
	if !IsJa() {
		t.Fatalf("expected ja from MGY_LANG, got %s", GetLang())
	}

	t.Setenv("MGY_LANG", "zh")
	Init("")
	if !IsZh() {
		t.Fatalf("expected zh from MGY_LANG, got %s", GetLang())
	}
}

func TestDetectSystemLanguagePosixLocales(t *testing.T) {
	t.Cleanup(func() { SetLang("zh") })
	t.Setenv("MGY_LANG", "")
	t.Setenv("MULTIGRAVITY_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANGUAGE", "")

	t.Setenv("LANG", "ja_JP.UTF-8")
	Init("")
	if !IsJa() {
		t.Fatalf("expected ja from LANG=ja_JP.UTF-8, got %s", GetLang())
	}

	t.Setenv("LANG", "en_US.UTF-8")
	Init("")
	if !IsEn() {
		t.Fatalf("expected en from LANG=en_US.UTF-8, got %s", GetLang())
	}

	t.Setenv("LANG", "zh_CN.UTF-8")
	Init("")
	if !IsZh() {
		t.Fatalf("expected zh from LANG=zh_CN.UTF-8, got %s", GetLang())
	}
}

func TestEnglishFirstFallbackPolicy(t *testing.T) {
	t.Cleanup(func() { SetLang(LangEn) })

	// 1. Explicit non-zh/ja override or env resolves to en even if system locale is Chinese.
	t.Setenv("LANG", "zh_CN.UTF-8")
	t.Setenv("MGY_LANG", "")
	t.Setenv("MULTIGRAVITY_LANG", "")
	Init("fr-FR")
	if !IsEn() {
		t.Fatalf("expected en for Init(fr-FR), got %s", GetLang())
	}

	t.Setenv("MGY_LANG", "ko-KR")
	Init("")
	if !IsEn() {
		t.Fatalf("expected en for MGY_LANG=ko-KR, got %s", GetLang())
	}

	// 2. ResolveLang maps any explicit unsupported locale to en even when gateway activeLang is zh.
	SetLang(LangZh)
	for _, raw := range []string{"fr-FR", "ko-KR", "de", "es-ES", "en-GB"} {
		if got := ResolveLang(raw); got != LangEn {
			t.Fatalf("ResolveLang(%q) under zh host = %q, want en", raw, got)
		}
	}
	if got := ResolveLang(""); got != LangZh {
		t.Fatalf("ResolveLang(\"\") under zh host = %q, want zh", got)
	}
	SetLang(LangEn)
	if got := ResolveLang(""); got != LangEn {
		t.Fatalf("ResolveLang(\"\") under en host = %q, want en", got)
	}

	// 3. Missing Japanese copy in T3/T3For falls back to English, never Chinese.
	SetLang(LangJa)
	if got := T3("中文", "English", ""); got != "English" {
		t.Fatalf("T3 missing ja = %q, want English", got)
	}
	if got := T3For("ja", "中文", "English", ""); got != "English" {
		t.Fatalf("T3For(ja) missing ja = %q, want English", got)
	}
	if got := T3For("ko", "中文", "English", "日本語"); got != "English" {
		t.Fatalf("T3For(ko) = %q, want English", got)
	}
}

func TestGetNativeLocale_PreservesNonZhJaEnWhileUIFallsBackToEn(t *testing.T) {
	t.Cleanup(func() { SetLang(LangEn) })
	t.Setenv("MGY_LANG", "")
	t.Setenv("MULTIGRAVITY_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANGUAGE", "")

	Init("fr-FR")
	if gotUI, gotNative := GetLang(), GetNativeLocale(); gotUI != LangEn || gotNative != "fr-FR" {
		t.Fatalf("Init(fr-FR): ui=%q native=%q, want en / fr-FR", gotUI, gotNative)
	}

	t.Setenv("MGY_LANG", "ko_KR.UTF-8")
	Init("")
	if gotUI, gotNative := GetLang(), GetNativeLocale(); gotUI != LangEn || gotNative != "ko-KR" {
		t.Fatalf("MGY_LANG=ko_KR.UTF-8: ui=%q native=%q, want en / ko-KR", gotUI, gotNative)
	}

	t.Setenv("MGY_LANG", "")
	t.Setenv("LANG", "zh_TW.UTF-8")
	Init("")
	if gotUI, gotNative := GetLang(), GetNativeLocale(); gotUI != LangZh || gotNative != "zh-TW" {
		t.Fatalf("LANG=zh_TW.UTF-8: ui=%q native=%q, want zh / zh-TW", gotUI, gotNative)
	}

	t.Setenv("LANG", "ja_JP.UTF-8")
	Init("")
	if gotUI, gotNative := GetLang(), GetNativeLocale(); gotUI != LangJa || gotNative != "ja-JP" {
		t.Fatalf("LANG=ja_JP.UTF-8: ui=%q native=%q, want ja / ja-JP", gotUI, gotNative)
	}

	t.Setenv("LANG", "de_DE.UTF-8")
	Init("")
	if gotUI, gotNative := GetLang(), GetNativeLocale(); gotUI != LangEn || gotNative != "de-DE" {
		t.Fatalf("LANG=de_DE.UTF-8: ui=%q native=%q, want en / de-DE", gotUI, gotNative)
	}

	for _, tc := range []struct {
		in   string
		want string
	}{
		{"fr_fr.UTF-8", "fr-FR"},
		{"zh_hans_cn", "zh-Hans-CN"},
		{"zh_HANT_tw.UTF-8", "zh-Hant-TW"},
		{"C.UTF-8", ""},
		{"POSIX", ""},
		{"auto", ""},
		{"system", ""},
	} {
		if got := CleanLocaleTag(tc.in); got != tc.want {
			t.Errorf("CleanLocaleTag(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
