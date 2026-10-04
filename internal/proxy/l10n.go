package proxy

import (
	"bytes"
	"regexp"
)

// Gateway-level localization of the desktop workbench bundle (main.js).
//
// The bundle is translated once on the server (and cached by HandleDesktopStatic), so the
// browser never walks or mutates the DOM. Translation is a single linear pass over the
// bundle: every double-quoted string literal is looked up in hash tables, so cost is
// O(bundle size) regardless of how many phrases the dictionaries hold. (The former
// implementation ran one bytes.ReplaceAll per phrase and took ~10s for ~3800 rules.)
//
// Dictionaries:
//   - l10nAnywhere / l10nUIProp / l10nHeaderProp   (l10n_dict_legacy.go) migrated rules
//   - l10nSafe / l10nTern / l10nStrict              (l10n_dict_extra.go)  broader UI coverage
//   - l10nPatches                                   (l10n_patches.go)     dynamic, concatenated text
//
// Keys are the raw JS source between the quotes of a literal, so literals containing
// escapes (\n, \", …) must be written with the same escapes.

var reDisposeGC = regexp.MustCompile(`(this\._disposeEntry\([a-zA-Z0-9_$]+\)\s*\}\s*,\s*)(?:3E4|30000)(\s*\))`)

// l10nMaxLiteral bounds how far we look for a literal's closing quote.
const l10nMaxLiteral = 600

// uiPropKeys are the legacy property names whose string values l10nUIProp may translate.
var uiPropKeys = map[string]bool{
	"text": true, "label": true, "tooltip": true, "tooltipText": true,
	"placeholder": true, "title": true, `"aria-label"`: true,
}

// uiKeyExact / uiKeySuffixes decide whether `key:"literal"` carries user-visible text.
// The dictionaries are curated, so a generous key list is safe: a literal is only ever
// replaced when its exact text is in a dictionary.
var uiKeyExact = map[string]bool{
	"text": true, "label": true, "title": true, "tooltip": true, "tooltipText": true,
	"placeholder": true, "description": true, "subtitle": true, "detail": true,
	"alt": true, "header": true, "heading": true, "caption": true, "hint": true,
	"message": true, "emptyMessage": true, `"aria-label"`: true, "ariaLabel": true,
	"linkLabel": true, "deleteTitle": true, "buttonText": true,
	"content": true, "error": true, "reason": true, "prefix": true, "displayName": true,
}

var uiKeySuffixes = []string{"Label", "Title", "Placeholder", "Tooltip", "Description", "Text", "Subtitle", "Message", "Caption", "Hint"}

func isUIKey(key string) bool {
	if uiKeyExact[key] {
		return true
	}
	for _, s := range uiKeySuffixes {
		if len(key) > len(s) && key[len(key)-len(s):] == s {
			return true
		}
	}
	return false
}

type literalCtx struct {
	call    bool   // literal is the argument of includes( / indexOf( / startsWith( (matches server text)
	compare bool   // literal is the operand of ===, !==, ==, !=, case
	key     string // object-literal property key for `key:"literal"` ("" when not a property value)
	tern    bool   // literal is a ternary branch (`c?"literal":x` or `c?x:"literal"`)
	child   bool   // positional createElement child: `...},"literal")` / `null,"literal",`
}

func tailHas(w []byte, suffix string) bool { return bytes.HasSuffix(w, []byte(suffix)) }

// endsWithWord reports whether w ends with the keyword word (not as part of a longer identifier).
func endsWithWord(w []byte, word string) bool {
	if !tailHas(w, word) {
		return false
	}
	n := len(w) - len(word)
	return n == 0 || !isIdentByte(w[n-1])
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// literalContext inspects the bytes around the literal opening at data[start] and data[end]
// (closing quote).
func literalContext(data []byte, start, end int) literalCtx {
	var c literalCtx
	lo := start - 64
	if lo < 0 {
		lo = 0
	}
	w := bytes.TrimRight(data[lo:start], " \n\t")
	if tailHas(w, ".includes(") || tailHas(w, "indexOf(") || tailHas(w, ".startsWith(") {
		c.call = true
		return c
	}
	if tailHas(w, "===") || tailHas(w, "!==") || tailHas(w, "==") || tailHas(w, "!=") || tailHas(w, "case") {
		c.compare = true
		return c
	}
	n := len(w)
	if n > 0 && w[n-1] == '?' || tailHas(w, "??") || tailHas(w, "||") || tailHas(w, "=>") || endsWithWord(w, "return") {
		c.tern = true
		return c
	}
	if n > 0 && w[n-1] == '=' && !tailHas(w, "==") {
		// destructuring default `{cancelLabel:l="Cancel"}`: report the outer property key
		k := n - 2
		for k >= 0 && isIdentByte(w[k]) {
			k--
		}
		if k >= 0 && w[k] == ':' {
			j := k - 1
			e := j
			for j >= 0 && isIdentByte(w[j]) {
				j--
			}
			if j < e {
				c.key = string(w[j+1 : e+1])
			}
		} else if !tailHas(w, "]=") {
			c.tern = true // `x="label"` assignment (but not a TS enum reverse mapping `E[E.A=1]="A"`)
		}
		return c
	}
	if n > 0 && w[n-1] == ':' {
		// `{key:"x"}` property value, or the else-branch of a ternary.
		j := n - 2
		k := j
		key := ""
		if j >= 0 && w[j] == '"' { // quoted key such as "aria-label"
			k = j - 1
			for k >= 0 && (isIdentByte(w[k]) || w[k] == '-') {
				k--
			}
			if k >= 0 && w[k] == '"' {
				key = string(w[k : j+1])
				k--
			}
		} else {
			for k >= 0 && isIdentByte(w[k]) {
				k--
			}
			if k < j {
				key = string(w[k+1 : j+1])
			}
		}
		p := k
		for p >= 0 && (w[p] == ' ' || w[p] == '\n' || w[p] == '\t') {
			p--
		}
		if key != "" && (p < 0 || w[p] == '{' || w[p] == ',') {
			c.key = key
		} else {
			c.tern = true
		}
		return c
	}
	if end+1 < len(data) && (data[end+1] == ',' || data[end+1] == ')') &&
		(tailHas(w, "},") || tailHas(w, "),") || tailHas(w, "null,")) {
		c.child = true
	}
	return c
}

// translateLiteral returns the translation for the literal content at data[start:end+1].
func translateLiteral(data []byte, start, end int) (string, bool) {
	content := data[start+1 : end]
	if len(content) < 2 {
		return "", false
	}
	zhAny, inAny := l10nAnywhere[string(content)]
	zhProp, inProp := l10nUIProp[string(content)]
	zhHdr, inHdr := l10nHeaderProp[string(content)]
	zhSafe, inSafe := l10nSafe[string(content)]
	zhTern, inTern := l10nTern[string(content)]
	zhStrict, inStrict := l10nStrict[string(content)]
	if !inAny && !inProp && !inHdr && !inSafe && !inTern && !inStrict {
		return "", false
	}
	ctx := literalContext(data, start, end)
	if ctx.call {
		return "", false
	}
	// Legacy anywhere-rules also translate ===/case operands: the settings navigation uses
	// its titles as ids, so both sides must be translated consistently.
	if inAny {
		return zhAny, true
	}
	if ctx.compare {
		return "", false
	}
	if inProp && uiPropKeys[ctx.key] {
		return zhProp, true
	}
	if inHdr && ctx.key == "header" {
		return zhHdr, true
	}
	uiKey := ctx.key != "" && isUIKey(ctx.key)
	if inStrict && (uiKey || ctx.child) {
		return zhStrict, true
	}
	if inTern && (uiKey || ctx.child || ctx.tern) {
		return zhTern, true
	}
	if inSafe && (ctx.key == "" || uiKey) {
		return zhSafe, true
	}
	return "", false
}

// translateStringLiterals does the single-pass dictionary translation.
// Every `"` is tried as a literal opening so a stray quote inside a regex, template or
// single-quoted string can never desynchronise the scan.
func translateStringLiterals(data []byte) []byte {
	var out []byte
	last := 0
	i := 0
	for i < len(data) {
		q := bytes.IndexByte(data[i:], '"')
		if q < 0 {
			break
		}
		start := i + q
		end := -1
		limit := start + l10nMaxLiteral
		if limit > len(data) {
			limit = len(data)
		}
		for j := start + 1; j < limit; j++ {
			c := data[j]
			if c == '\\' {
				j++
				continue
			}
			if c == '\n' {
				break
			}
			if c == '"' {
				end = j
				break
			}
		}
		if end > 0 {
			if zh, ok := translateLiteral(data, start, end); ok {
				if out == nil {
					out = make([]byte, 0, len(data)+len(data)/32)
				}
				out = append(out, data[last:start+1]...)
				out = append(out, zh...)
				last = end
				i = end + 1
				continue
			}
		}
		i = start + 1
	}
	if out == nil {
		return data
	}
	return append(out, data[last:]...)
}

// LocalizeMainJS performs zero-runtime-overhead static localization on main.js.
func LocalizeMainJS(data []byte) []byte {
	// Strip blocking Google Fonts stylesheet imports to eliminate 15-30s browser render lock
	data = bytes.ReplaceAll(data, []byte("@import url('https://fonts.googleapis.com/"), []byte("/* @import disabled */ /* "))

	// Immediately abort old conversation StreamAgentStateUpdates streams upon session switch.
	// Upstream TrajectoriesContextProvider sets a 30-second delay (3E4 ms) in _scheduleGc
	// before calling _disposeEntry(session) and aborting the stream's AbortController.
	// In HTTP/1.1 (plain HTTP to localhost/LAN), Chrome caps concurrent connections at 6 per origin.
	// When switching sessions, these lingering long-lived streams saturate Chrome's connection pool,
	// causing subsequent requests (and new session streams) to stall in browser queue for exactly 30 seconds.
	// Replacing 3E4 with 0 ensures the previous conversation's stream AbortController aborts on the next event loop tick.
	if bytes.Contains(data, []byte("this._disposeEntry(a)},3E4)")) {
		data = bytes.ReplaceAll(data, []byte("this._disposeEntry(a)},3E4)"), []byte("this._disposeEntry(a)},0)"))
	} else if reDisposeGC.Match(data) {
		data = reDisposeGC.ReplaceAll(data, []byte("${1}0${2}"))
	}

	// Patches match raw upstream text, so they run before the literal pass.
	data = applyL10nPatches(data)
	return translateStringLiterals(data)
}
