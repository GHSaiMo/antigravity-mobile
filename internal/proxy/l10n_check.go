package proxy

import (
	"bytes"
	"log"
)

// localizeMainJSChecked runs LocalizeMainJS and warns when it changed nothing.
// The l10n rules match minified upstream IDE text, so an IDE upgrade can silently
// invalidate all of them (and the session-switch GC patch); surface that in the log.
func localizeMainJSChecked(data []byte) []byte {
	out := LocalizeMainJS(data)
	if len(data) > 0 && bytes.Equal(out, data) {
		log.Printf("[L10n] WARNING: no localization rule matched main.js (%d bytes); upstream IDE bundle may have changed and l10n rules need regeneration", len(data))
	}
	return out
}
