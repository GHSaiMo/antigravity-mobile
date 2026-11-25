//go:build !windows

package i18n

import "testing"

func TestDetectSystemLanguagePosixNeutralFallback(t *testing.T) {
	clearLocaleEnv := func(t *testing.T) {
		t.Helper()
		for _, k := range []string{"MGY_LANG", "MULTIGRAVITY_LANG", "LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
			t.Setenv(k, "")
		}
	}

	tests := []struct {
		name      string
		env       map[string]string
		isDarwin  bool
		macLocale string
		want      string
	}{
		{
			name:      "macOS LANG=C.UTF-8 defers to zh-Hans AppleLanguages",
			env:       map[string]string{"LANG": "C.UTF-8"},
			isDarwin:  true,
			macLocale: "zh-Hans",
			want:      LangZh,
		},
		{
			name:      "macOS LANG=C.UTF-8 defers to ja-JP AppleLanguages",
			env:       map[string]string{"LANG": "C.UTF-8"},
			isDarwin:  true,
			macLocale: "ja-JP",
			want:      LangJa,
		},
		{
			name:      "macOS LANG=C.UTF-8 defers to en-US AppleLanguages",
			env:       map[string]string{"LANG": "C.UTF-8"},
			isDarwin:  true,
			macLocale: "en-US",
			want:      LangEn,
		},
		{
			name:      "macOS LANG=C.UTF-8 defers to non-zh/ja AppleLanguages as en",
			env:       map[string]string{"LANG": "C.UTF-8"},
			isDarwin:  true,
			macLocale: "fr-FR",
			want:      LangEn,
		},
		{
			name:      "macOS LANG=POSIX defers to zh-Hans AppleLanguages",
			env:       map[string]string{"LANG": "POSIX"},
			isDarwin:  true,
			macLocale: "zh-Hans-CN",
			want:      LangZh,
		},
		{
			name:      "macOS LANG=C.UTF-8 falls back to en when AppleLanguages unavailable",
			env:       map[string]string{"LANG": "C.UTF-8"},
			isDarwin:  true,
			macLocale: "",
			want:      LangEn,
		},
		{
			name:      "macOS explicit LANG=en_US.UTF-8 overrides zh-Hans AppleLanguages",
			env:       map[string]string{"LANG": "en_US.UTF-8"},
			isDarwin:  true,
			macLocale: "zh-Hans",
			want:      LangEn,
		},
		{
			name:      "macOS empty env with fr-FR AppleLanguages returns en",
			env:       map[string]string{},
			isDarwin:  true,
			macLocale: "fr-FR",
			want:      LangEn,
		},
		{
			name:     "Linux LANG=C.UTF-8 falls back to en",
			env:      map[string]string{"LANG": "C.UTF-8"},
			isDarwin: false,
			want:     LangEn,
		},
		{
			name:     "Linux LANG=C falls back to en",
			env:      map[string]string{"LANG": "C"},
			isDarwin: false,
			want:     LangEn,
		},
		{
			name:     "Linux LANG=POSIX.UTF-8 falls back to en",
			env:      map[string]string{"LANG": "POSIX.UTF-8"},
			isDarwin: false,
			want:     LangEn,
		},
		{
			name:     "Linux LANG=C.UTF-8 defers to LANGUAGE=zh_CN:zh",
			env:      map[string]string{"LANG": "C.UTF-8", "LANGUAGE": "zh_CN:zh"},
			isDarwin: false,
			want:     LangZh,
		},
		{
			name:     "Linux LANGUAGE skips neutral C prefix",
			env:      map[string]string{"LANGUAGE": "C.UTF-8:ja_JP:en"},
			isDarwin: false,
			want:     LangJa,
		},
		{
			name:     "Linux empty locale env defaults to en",
			env:      map[string]string{},
			isDarwin: false,
			want:     LangEn,
		},
		{
			name:     "Linux LANG=UTF-8 only defaults to en",
			env:      map[string]string{"LANG": "UTF-8"},
			isDarwin: false,
			want:     LangEn,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearLocaleEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got := detectSystemLanguagePosix(tc.isDarwin, func() string { return tc.macLocale })
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseMacOSDefaultsList(t *testing.T) {
	raw := "(\n    \"zh-Hans-CN\",\n    \"en-US\"\n)\n"
	if got := parseMacOSDefaultsList(raw); got != "zh-Hans-CN" {
		t.Fatalf("got %q, want zh-Hans-CN", got)
	}
	rawUnquoted := "(\n    ja,\n    en\n)\n"
	if got := parseMacOSDefaultsList(rawUnquoted); got != "ja" {
		t.Fatalf("got %q, want ja", got)
	}
	if got := parseMacOSDefaultsList("(\n)\n"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
