package com.antigravity.mobile.ui.components

import org.json.JSONObject

/**
 * Shared Antigravity Generative UI HTML theme injector for both inline `<agent-embed>` widgets
 * and standalone `.html` artifact previews (`DocumentPreviewSheet`).
 */
internal object GenerativeUIHtmlHelper {
    const val STYLE_ELEMENT_ID = "antigravity-theme-vars"
    const val DARK_BG_HEX = "#0f1117"
    const val LIGHT_BG_HEX = "#ffffff"

    enum class RenderMode {
        INLINE_EMBED,
        STANDALONE
    }

    data class PreparedHtml(
        val html: String,
        val isGenerativeUi: Boolean
    )

    private val selfDefinesBackgroundRegex = Regex("""--background\s*:""", RegexOption.IGNORE_CASE)
    private val selfDefinesForegroundRegex = Regex("""--foreground\s*:""", RegexOption.IGNORE_CASE)
    private val viewportMetaRegex = Regex("""<meta\s+[^>]*?name\s*=\s*["']viewport["'][^>]*>""", RegexOption.IGNORE_CASE)
    private val htmlTagRegex = Regex("""<html\b([^>]*)>""", RegexOption.IGNORE_CASE)
    private val classAttrRegex = Regex("""(\bclass\s*=\s*)(["'])([^"']*)\2""", RegexOption.IGNORE_CASE)
    private val headWithOptionalCharsetRegex = Regex("""<head\b[^>]*>(?:\s*<meta\s+[^>]*charset[^>]*>)?""", RegexOption.IGNORE_CASE)

    /**
     * Detects whether an HTML document relies on Antigravity Generative UI host-injected theme variables
     * (excluding self-styled Marp decks or documents that already define their own :root palette).
     */
    fun usesAntigravityTheme(html: String): Boolean {
        val lower = html.lowercase()
        if (lower.contains("data-marpit-svg") || lower.contains("bespoke-marp") || lower.contains("marpit")) {
            return false
        }
        if (lower.contains("gstatic.com/antigravity")) {
            return true
        }
        if (lower.contains("themechange") && lower.contains("addeventlistener")) {
            return true
        }
        if (lower.contains("classlist.contains('light')") || lower.contains("classlist.contains(\"light\")")) {
            return true
        }
        val semanticRefs = listOf(
            "var(--background)",
            "var(--foreground)",
            "var(--card)",
            "var(--card-border)",
            "var(--content)",
            "var(--muted-foreground)",
            "var(--chart-1)",
            "var(--chart-2)",
            "var(--chart-3)",
            "var(--code-foreground)"
        )
        if (semanticRefs.none { lower.contains(it) }) {
            return false
        }
        val selfDefinesBg = selfDefinesBackgroundRegex.containsMatchIn(html)
        val selfDefinesFg = selfDefinesForegroundRegex.containsMatchIn(html)
        return !(selfDefinesBg && selfDefinesFg)
    }

    fun themeCssBody(isDark: Boolean, mode: RenderMode, includeContainerTheme: Boolean): String {
        val paletteDark = if (includeContainerTheme) isDark else false
        val colorSchemeLine = if (includeContainerTheme) {
            "\n  color-scheme: ${if (isDark) "dark" else "light"};"
        } else {
            ""
        }
        val vars = """
            :root {
              --background: ${if (paletteDark) DARK_BG_HEX else LIGHT_BG_HEX};
              --content: ${if (paletteDark) "#13161f" else "#f8fafc"};
              --card: ${if (paletteDark) "#181c26" else "#f4f4f5"};
              --sidebar: ${if (paletteDark) "#141720" else "#fafafa"};
              --card-border: ${if (paletteDark) "rgba(255, 255, 255, 0.10)" else "rgba(15, 23, 42, 0.10)"};
              --border: ${if (paletteDark) "rgba(255, 255, 255, 0.14)" else "rgba(15, 23, 42, 0.13)"};
              --foreground: ${if (paletteDark) "#f4f4f5" else "#18181b"};
              --muted-foreground: ${if (paletteDark) "#a1a1aa" else "#64748b"};
              --placeholder: ${if (paletteDark) "#71717a" else "#94a3b8"};
              --code-foreground: ${if (paletteDark) "#e4e4e7" else "#1e293b"};
              --primary: ${if (paletteDark) "#6e7bf2" else "#4f46e5"};
              --primary-foreground: #ffffff;
              --secondary: ${if (paletteDark) "#27272a" else "#e4e4e7"};
              --secondary-foreground: ${if (paletteDark) "#f4f4f5" else "#18181b"};
              --accent: ${if (paletteDark) "rgba(255, 255, 255, 0.08)" else "rgba(15, 23, 42, 0.06)"};
              --accent-foreground: ${if (paletteDark) "#f4f4f5" else "#18181b"};
              --muted: ${if (paletteDark) "#27272a" else "#f4f4f5"};
              --link: ${if (paletteDark) "#60a5fa" else "#2563eb"};
              --success: ${if (paletteDark) "#34d399" else "#16a34a"};
              --warning: ${if (paletteDark) "#fbbf24" else "#d97706"};
              --error: ${if (paletteDark) "#f87171" else "#dc2626"};
              --destructive: ${if (paletteDark) "#f87171" else "#dc2626"};
              --destructive-foreground: #ffffff;
              --chart-1: ${if (paletteDark) "#60a5fa" else "#2563eb"};
              --chart-2: ${if (paletteDark) "#a78bfa" else "#7c3aed"};
              --chart-3: ${if (paletteDark) "#2dd4bf" else "#0d9488"};
              --chart-4: ${if (paletteDark) "#fb923c" else "#ea580c"};
              --chart-5: ${if (paletteDark) "#f472b6" else "#db2777"};
              --chart-6: ${if (paletteDark) "#facc15" else "#ca8a04"};$colorSchemeLine
            }
        """.trimIndent()

        return when (mode) {
            RenderMode.INLINE_EMBED -> """
                $vars
                :where(*) {
                  box-sizing: border-box;
                }
                :where(html, body) {
                  margin: 0;
                  padding: 8px;
                  background: transparent;
                  color: var(--foreground);
                  font-size: 14px;
                  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
                  -webkit-text-size-adjust: 100%;
                  -webkit-tap-highlight-color: transparent;
                  touch-action: pan-x pan-y;
                  overscroll-behavior: none;
                }
                :where(img, svg, video) {
                  max-width: 100%;
                }
            """.trimIndent()

            RenderMode.STANDALONE -> {
                if (!includeContainerTheme) {
                    vars
                } else {
                    """
                        $vars
                        :where(*) {
                          box-sizing: border-box;
                        }
                        :where(html, body) {
                          margin: 0;
                          background-color: var(--background);
                          color: var(--foreground);
                          font-size: 14px;
                          font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
                          -webkit-text-size-adjust: 100%;
                          -webkit-tap-highlight-color: transparent;
                        }
                    """.trimIndent()
                }
            }
        }
    }

    fun liveThemeUpdateJs(isDark: Boolean, mode: RenderMode): String {
        val cssBody = themeCssBody(isDark = isDark, mode = mode, includeContainerTheme = true)
        val encodedCss = JSONObject.quote(cssBody)
        val addClass = if (isDark) "dark" else "light"
        val removeClass = if (isDark) "light" else "dark"
        return """
            (function() {
              try {
                var styleEl = document.getElementById('$STYLE_ELEMENT_ID');
                if (!styleEl) {
                  styleEl = document.createElement('style');
                  styleEl.id = '$STYLE_ELEMENT_ID';
                  var head = document.head || document.documentElement;
                  head.insertBefore(styleEl, head.firstChild);
                }
                styleEl.textContent = $encodedCss;
                var root = document.documentElement;
                if (root) {
                  root.classList.remove('$removeClass');
                  root.classList.add('$addClass');
                }
                window.dispatchEvent(new Event('themechange'));
              } catch (e) {}
            })();
        """.trimIndent()
    }

    fun prepareStandaloneHtml(html: String, isDark: Boolean): PreparedHtml {
        val isGenUi = usesAntigravityTheme(html)
        val styleTag = "<style id=\"$STYLE_ELEMENT_ID\">\n${themeCssBody(isDark, RenderMode.STANDALONE, includeContainerTheme = isGenUi)}\n</style>"
        val viewportTag = if (viewportMetaRegex.containsMatchIn(html)) {
            ""
        } else {
            """<meta name="viewport" content="width=device-width, initial-scale=1.0, viewport-fit=cover">"""
        }
        var modified = if (isGenUi) applyHtmlRootThemeClass(html, isDark) else html
        val injection = listOf(viewportTag, styleTag).filter { it.isNotEmpty() }.joinToString("\n")
        val rootClass = if (isGenUi) (if (isDark) "dark" else "light") else null
        modified = injectIntoHead(modified, injection, rootClass)
        return PreparedHtml(html = modified, isGenerativeUi = isGenUi)
    }

    fun prepareInlineEmbedHtml(
        html: String,
        isDark: Boolean,
        viewportMeta: String,
        reporterScript: String
    ): String {
        val darkClass = if (isDark) "dark" else "light"
        val themeStyle = "<style id=\"$STYLE_ELEMENT_ID\">\n${themeCssBody(isDark, RenderMode.INLINE_EMBED, includeContainerTheme = true)}\n</style>"
        var modified = viewportMetaRegex.replace(html, "")
        modified = applyHtmlRootThemeClass(modified, isDark)
        val injection = listOf(viewportMeta, themeStyle, reporterScript).joinToString("\n")
        return injectIntoHead(modified, injection, darkClass)
    }

    /**
     * Safely sets `dark` or `light` ONLY on the opening `<html ...>` tag without altering descendant `class="..."` attributes.
     */
    fun applyHtmlRootThemeClass(html: String, isDark: Boolean): String {
        val darkClass = if (isDark) "dark" else "light"
        val oppositeClass = if (isDark) "light" else "dark"
        val match = htmlTagRegex.find(html) ?: return html
        val attrs = match.groupValues[1]
        val classMatch = classAttrRegex.find(attrs)
        val updatedTag = if (classMatch != null) {
            val prefix = classMatch.groupValues[1]
            val quote = classMatch.groupValues[2]
            val existing = classMatch.groupValues[3]
            val tokens = existing.split(Regex("\\s+")).filter { it.isNotEmpty() && it != oppositeClass && it != darkClass }
            val merged = (listOf(darkClass) + tokens).joinToString(" ")
            val newAttrs = attrs.replaceRange(classMatch.range, "$prefix$quote$merged$quote")
            "<html$newAttrs>"
        } else {
            "<html class=\"$darkClass\"$attrs>"
        }
        return html.replaceRange(match.range, updatedTag)
    }

    /**
     * Inserts [injection] at the beginning of `<head>` (after an immediate `<meta charset=...>` tag if present,
     * preserving HTML5 1024-byte charset prescan while remaining ahead of page `<style>`/`<script>` tags).
     */
    fun injectIntoHead(html: String, injection: String, rootThemeClass: String?): String {
        val block = "\n$injection\n"
        val headMatch = headWithOptionalCharsetRegex.find(html)
        if (headMatch != null) {
            val insertPos = headMatch.range.last + 1
            return html.substring(0, insertPos) + block + html.substring(insertPos)
        }
        val htmlMatch = htmlTagRegex.find(html)
        if (htmlMatch != null) {
            val insertPos = htmlMatch.range.last + 1
            return html.substring(0, insertPos) + "<head><meta charset=\"UTF-8\">$block</head>" + html.substring(insertPos)
        }
        val classAttr = if (!rootThemeClass.isNullOrBlank()) " class=\"$rootThemeClass\"" else ""
        return "<!DOCTYPE html><html$classAttr><head><meta charset=\"UTF-8\">$block</head><body>$html</body></html>"
    }
}
