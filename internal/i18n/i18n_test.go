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
	if !IsEn() || IsZh() {
		t.Fatalf("expected en, got %s", GetLang())
	}
	if got := T("你好", "Hello"); got != "Hello" {
		t.Fatalf("expected 'Hello', got '%s'", got)
	}
	if got := Sprintf("端口: %d", "Port: %d", 58900); got != "Port: 58900" {
		t.Fatalf("unexpected formatted string: %s", got)
	}
}

func TestI18nInitOverride(t *testing.T) {
	Init("en")
	if !IsEn() {
		t.Fatalf("expected en from override, got %s", GetLang())
	}

	Init("zh-CN")
	if !IsZh() {
		t.Fatalf("expected zh from override, got %s", GetLang())
	}
}

func TestI18nEnvOverride(t *testing.T) {
	t.Setenv("MGY_LANG", "en")
	Init("")
	if !IsEn() {
		t.Fatalf("expected en from MGY_LANG, got %s", GetLang())
	}

	t.Setenv("MGY_LANG", "zh")
	Init("")
	if !IsZh() {
		t.Fatalf("expected zh from MGY_LANG, got %s", GetLang())
	}
}
