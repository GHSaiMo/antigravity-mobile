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
