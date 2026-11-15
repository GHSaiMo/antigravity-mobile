package com.antigravity.mobile.ui.components

import android.util.LruCache
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString

/**
 * Zero-dependency offline syntax highlighter with bounded LRU memoization.
 * Produces adaptive Light/Dark [AnnotatedString] spans without WebViews or remote assets.
 */
object CodeSyntaxHighlighter {
    private const val MAX_HIGHLIGHT_LENGTH = 24_000

    private data class CacheEntry(
        val source: String,
        val annotated: AnnotatedString
    )

    private val cache = object : LruCache<String, CacheEntry>(256) {}

    private data class Palette(
        val keyword: Color,
        val string: Color,
        val comment: Color,
        val number: Color,
        val typeName: Color,
        val function: Color,
        val property: Color,
        val diffAdd: Color,
        val diffDel: Color
    )

    private val darkPalette = Palette(
        keyword = Color(0xFFFF7B72),
        string = Color(0xFFA5D6FF),
        comment = Color(0xFF8B949E),
        number = Color(0xFF79C0FF),
        typeName = Color(0xFFFFA657),
        function = Color(0xFFD2A8FF),
        property = Color(0xFF7EE787),
        diffAdd = Color(0xFF56D364),
        diffDel = Color(0xFFF85149)
    )

    private val lightPalette = Palette(
        keyword = Color(0xFFCF222E),
        string = Color(0xFF0A3069),
        comment = Color(0xFF6E7781),
        number = Color(0xFF0550AE),
        typeName = Color(0xFF953800),
        function = Color(0xFF8250DF),
        property = Color(0xFF116329),
        diffAdd = Color(0xFF1A7F37),
        diffDel = Color(0xFFCF222E)
    )

    private val jsonRegex = Regex(
        """("(?:\\.|[^"\\])*")\s*:|("(?:\\.|[^"\\])*")|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)"""
    )

    private val yamlRegex = Regex(
        """(#[^\n]*)|^(\s*[\w.-]+)\s*:|("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')|\b(true|false|null|yes|no)\b|(\b-?\d+(?:\.\d+)?\b)""",
        RegexOption.MULTILINE
    )

    private val markupRegex = Regex(
        """(<!--[\s\S]*?-->)|(</?[\w:-]+)|(\s[\w:-]+)(?==)|("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')"""
    )

    private val codeRegex = Regex(
        """(//[^\n]*|/\*[\s\S]*?\*/|#(?!!\[)[^\n]*)|("${'"'}${'"'}[\s\S]*?"${'"'}${'"'}|"(?:\\.|[^"\\\n])*"|'(?:\\.|[^'\\\n])*'|`(?:\\.|[^`\\])*`)|(@[A-Za-z_]\w*)|\b(0x[0-9a-fA-F_]+|0b[01_]+|\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)\b|\b(func|fn|fun|def|function|return|if|else|elif|when|for|while|loop|match|switch|case|default|break|continue|struct|class|enum|interface|trait|protocol|extension|impl|type|typealias|import|from|export|package|module|pub|public|private|internal|protected|static|final|const|let|var|val|mut|async|await|try|catch|throw|throws|defer|go|select|chan|range|in|as|is|nil|null|None|True|False|true|false|self|Self|super|this|new|guard|where|actor|nonisolated|some|any|override|open|suspend|data|sealed|companion|object)\b|\b([A-Z][A-Za-z0-9_]*)\b|\b([a-zA-Z_]\w*)(?=\s*\()"""
    )

    fun highlight(code: String, language: String, isDark: Boolean): AnnotatedString {
        if (code.isEmpty() || code.length > MAX_HIGHLIGHT_LENGTH) {
            return AnnotatedString(code)
        }
        val normLang = normalizeLanguage(language)
        if (normLang == "text") {
            return AnnotatedString(code)
        }

        val key = "${if (isDark) 'd' else 'l'}:$normLang:${code.length}:${code.hashCode()}"
        synchronized(cache) {
            cache.get(key)?.let { if (it.source == code) return it.annotated }
        }

        val palette = if (isDark) darkPalette else lightPalette
        val result = buildAnnotatedString {
            append(code)
            when (normLang) {
                "diff" -> {
                    var idx = 0
                    while (idx < code.length) {
                        val nl = code.indexOf('\n', idx)
                        val end = if (nl == -1) code.length else nl + 1
                        val first = code[idx]
                        when (first) {
                            '+' -> addStyle(SpanStyle(color = palette.diffAdd), idx, end)
                            '-' -> addStyle(SpanStyle(color = palette.diffDel), idx, end)
                            '@' -> addStyle(SpanStyle(color = palette.function), idx, end)
                        }
                        idx = end
                    }
                }
                "json" -> {
                    for (m in jsonRegex.findAll(code)) {
                        val g = m.groups
                        when {
                            g[1] != null -> addStyle(SpanStyle(color = palette.property), g[1]!!.range.first, g[1]!!.range.last + 1)
                            g[2] != null -> addStyle(SpanStyle(color = palette.string), g[2]!!.range.first, g[2]!!.range.last + 1)
                            g[3] != null -> addStyle(SpanStyle(color = palette.keyword), g[3]!!.range.first, g[3]!!.range.last + 1)
                            g[4] != null -> addStyle(SpanStyle(color = palette.number), g[4]!!.range.first, g[4]!!.range.last + 1)
                        }
                    }
                }
                "yaml" -> {
                    for (m in yamlRegex.findAll(code)) {
                        val g = m.groups
                        when {
                            g[1] != null -> addStyle(SpanStyle(color = palette.comment), g[1]!!.range.first, g[1]!!.range.last + 1)
                            g[2] != null -> addStyle(SpanStyle(color = palette.property), g[2]!!.range.first, g[2]!!.range.last + 1)
                            g[3] != null -> addStyle(SpanStyle(color = palette.string), g[3]!!.range.first, g[3]!!.range.last + 1)
                            g[4] != null -> addStyle(SpanStyle(color = palette.keyword), g[4]!!.range.first, g[4]!!.range.last + 1)
                            g[5] != null -> addStyle(SpanStyle(color = palette.number), g[5]!!.range.first, g[5]!!.range.last + 1)
                        }
                    }
                }
                "markup" -> {
                    for (m in markupRegex.findAll(code)) {
                        val g = m.groups
                        when {
                            g[1] != null -> addStyle(SpanStyle(color = palette.comment), g[1]!!.range.first, g[1]!!.range.last + 1)
                            g[2] != null -> addStyle(SpanStyle(color = palette.keyword), g[2]!!.range.first, g[2]!!.range.last + 1)
                            g[3] != null -> addStyle(SpanStyle(color = palette.property), g[3]!!.range.first, g[3]!!.range.last + 1)
                            g[4] != null -> addStyle(SpanStyle(color = palette.string), g[4]!!.range.first, g[4]!!.range.last + 1)
                        }
                    }
                }
                else -> {
                    for (m in codeRegex.findAll(code)) {
                        val g = m.groups
                        when {
                            g[1] != null -> addStyle(SpanStyle(color = palette.comment), g[1]!!.range.first, g[1]!!.range.last + 1)
                            g[2] != null -> addStyle(SpanStyle(color = palette.string), g[2]!!.range.first, g[2]!!.range.last + 1)
                            g[3] != null -> addStyle(SpanStyle(color = palette.function), g[3]!!.range.first, g[3]!!.range.last + 1)
                            g[4] != null -> addStyle(SpanStyle(color = palette.number), g[4]!!.range.first, g[4]!!.range.last + 1)
                            g[5] != null -> addStyle(SpanStyle(color = palette.keyword), g[5]!!.range.first, g[5]!!.range.last + 1)
                            g[6] != null -> addStyle(SpanStyle(color = palette.typeName), g[6]!!.range.first, g[6]!!.range.last + 1)
                            g[7] != null -> addStyle(SpanStyle(color = palette.function), g[7]!!.range.first, g[7]!!.range.last + 1)
                        }
                    }
                }
            }
        }

        synchronized(cache) {
            cache.put(key, CacheEntry(code, result))
        }
        return result
    }

    private fun normalizeLanguage(raw: String): String = when (raw.trim().lowercase()) {
        "txt", "plain", "plaintext", "output", "log" -> "text"
        "json", "jsonc", "jsonl" -> "json"
        "yaml", "yml", "toml" -> "yaml"
        "diff", "patch" -> "diff"
        "html", "xml", "svg" -> "markup"
        else -> "code"
    }
}
