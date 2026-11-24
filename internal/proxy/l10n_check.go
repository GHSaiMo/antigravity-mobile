package proxy

import (
	"bytes"
	"fmt"
	"log/slog"

	"antigravity-mobile/internal/i18n"
)

// localizeMainJSChecked runs LocalizeMainJS (Chinese) and warns when it changed nothing.
func localizeMainJSChecked(data []byte) []byte {
	return localizeMainJSCheckedForLang(data, i18n.LangZh)
}

// localizeMainJSCheckedForLang runs LocalizeMainJSForLang and warns on unexpected no-op localization.
func localizeMainJSCheckedForLang(data []byte, lang string) []byte {
	norm := i18n.NormalizeLang(lang)
	out := LocalizeMainJSForLang(data, norm)
	if norm == i18n.LangZh && len(data) > 0 && bytes.Equal(out, data) {
		slog.Warn(fmt.Sprintf("[L10n] WARNING: no localization rule matched main.js (%d bytes); upstream IDE bundle may have changed and l10n rules need regeneration", len(data)))
	}
	return out
}
