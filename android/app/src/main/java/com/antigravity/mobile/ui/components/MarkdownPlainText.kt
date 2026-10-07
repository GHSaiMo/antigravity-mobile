package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntRect
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupPositionProvider
import kotlinx.coroutines.delay
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalTextToolbar
import androidx.compose.ui.platform.TextToolbar
import androidx.compose.ui.text.AnnotatedString
import com.antigravity.mobile.data.service.MathSymbolProcessor

/**
 * Converts Markdown into the text the user actually sees on screen: inline markers
 * (bold / italic / strike / inline code / link syntax / heading '#') are removed while
 * line breaks, list bullets and numbers, table rows and code block bodies are kept.
 */
object MarkdownPlainText {
    fun convert(content: String): String {
        val parts = MarkdownParser.parse(content).mapNotNull { block ->
            when (block) {
                is MarkdownBlock.Frontmatter -> block.rawContent.trim()
                is MarkdownBlock.Heading -> inline(block.text)
                is MarkdownBlock.Divider -> "────────"
                is MarkdownBlock.CodeBlock -> block.code.trim('\n')
                is MarkdownBlock.Table -> (listOf(block.headers) + block.rows)
                    .joinToString("\n") { row -> row.joinToString("\t") { inline(it) } }
                is MarkdownBlock.BulletList -> block.items.joinToString("\n") { "• " + inline(it) }
                is MarkdownBlock.OrderedList -> block.items.withIndex()
                    .joinToString("\n") { (i, s) -> "${block.startIndex + i}. " + inline(s) }
                is MarkdownBlock.Paragraph -> inline(block.text)
                is MarkdownBlock.Image -> block.alt.takeIf { it.isNotBlank() }
                is MarkdownBlock.AgentEmbed -> "[交互组件: ${block.src.substringAfterLast('/')}]"
                is MarkdownBlock.Carousel -> block.slides.joinToString("\n\n") { convert(it.content) }
            }
        }
        return parts.filter { it.isNotEmpty() }.joinToString("\n\n")
    }

    internal fun inline(text: String): String {
        var s = MathSymbolProcessor.process(text)
        s = replaceHtmlBreaks(s)
        s = unwrapBacktickBold(s)
        s = normalizeBoldSpaces(s)
        return stripInline(s).trim()
    }

    private fun stripInline(text: String): String =
        INLINE_TOKEN_REGEX.replace(text) { m ->
            val g = m.groups
            when {
                g[1] != null -> stripInline(g[1]!!.value)
                g[3] != null -> g[3]!!.value
                else -> {
                    val inner = (4..10).firstNotNullOfOrNull { g[it]?.value } ?: m.value
                    stripInline(inner)
                }
            }
        }

    /**
     * Reconstructs natural newlines and bullet formatting for selected plain text by mapping it
     * against the authoritative full plain-text structure of the message.
     */
    fun restoreFormattedSelection(raw: String, fullPlainText: String): String {
        val cleanedRaw = raw.replace(" \u2009", "").replace("\u2009", "").trim()
        if (cleanedRaw.isEmpty()) return ""
        if (fullPlainText.isBlank()) return cleanedRaw

        // Identify structural list marker prefixes at the beginning of each line (e.g. "• ", "1. ", "12. ")
        val isMarker = BooleanArray(fullPlainText.length)
        val listPrefixRegex = Regex("""^(\s*(?:•|\d+[.)])\s+)""")
        var lineStart = 0
        while (lineStart < fullPlainText.length) {
            val lineEnd = fullPlainText.indexOf('\n', lineStart).let { if (it == -1) fullPlainText.length else it }
            val line = fullPlainText.substring(lineStart, lineEnd)
            val match = listPrefixRegex.find(line)
            if (match != null) {
                val markerLen = match.value.length
                for (i in 0 until markerLen) {
                    isMarker[lineStart + i] = true
                }
            }
            lineStart = lineEnd + 1
        }

        // Canonical content string of authoritative text (excluding markers and whitespace)
        val canonToFullIdx = ArrayList<Int>()
        val fullCanonBuilder = StringBuilder()
        for (i in fullPlainText.indices) {
            if (!isMarker[i] && !fullPlainText[i].isWhitespace()) {
                canonToFullIdx.add(i)
                fullCanonBuilder.append(fullPlainText[i])
            }
        }
        val fullCanon = fullCanonBuilder.toString()

        // Canonical content string of raw selection (excluding bullets and whitespace)
        val rawCanonBuilder = StringBuilder()
        for (ch in cleanedRaw) {
            if (!ch.isWhitespace() && ch != '•') {
                rawCanonBuilder.append(ch)
            }
        }
        val rawCanon = rawCanonBuilder.toString()

        if (rawCanon.isEmpty()) return cleanedRaw

        // Full selection / Select All / high coverage (> 90%): return full authoritative plain text directly
        if (rawCanon == fullCanon ||
            (rawCanon.length > 20 && fullCanon.contains(rawCanon) && rawCanon.length >= (fullCanon.length * 0.90f))
        ) {
            return fullPlainText.trim()
        }

        // Short sub-selection within a single line (e.g. commit hash, single word): keep exact raw text
        if (rawCanon.length < 8 && fullPlainText.contains(cleanedRaw)) {
            return cleanedRaw
        }

        // Find canonical substring match in authoritative text
        val matchStart = fullCanon.indexOf(rawCanon)
        if (matchStart != -1 && matchStart + rawCanon.length <= canonToFullIdx.size) {
            var realStart = canonToFullIdx[matchStart]
            val realEnd = canonToFullIdx[matchStart + rawCanon.length - 1] + 1

            // If the selection starts at the beginning of a list item content, include the marker prefix
            val prevNewline = fullPlainText.lastIndexOf('\n', realStart - 1)
            val lineStartOfRealStart = if (prevNewline == -1) 0 else prevNewline + 1
            val linePrefix = fullPlainText.substring(lineStartOfRealStart, realStart)
            if (linePrefix.isNotBlank() && (linePrefix.startsWith("•") || listPrefixRegex.matches(linePrefix))) {
                realStart = lineStartOfRealStart
            }

            return fullPlainText.substring(realStart, realEnd).trim()
        }

        return cleanedRaw
    }
}

/**
 * Wraps the system selection toolbar so that copying a selection drops the inline file-icon
 * placeholders, reconstructs block newlines and list prefixes, and avoids trailing extra newlines.
 */
@Composable
internal fun WithCleanCopyToolbar(
    rawMarkdown: String? = null,
    content: @Composable () -> Unit
) {
    val base = LocalTextToolbar.current
    val clipboard = LocalClipboardManager.current
    val toolbar = remember(base, clipboard, rawMarkdown) {
        object : TextToolbar by base {
            override fun showMenu(
                rect: Rect,
                onCopyRequested: (() -> Unit)?,
                onPasteRequested: (() -> Unit)?,
                onCutRequested: (() -> Unit)?,
                onSelectAllRequested: (() -> Unit)?
            ) {
                base.showMenu(
                    rect,
                    onCopyRequested?.let { copy ->
                        {
                            copy()
                            clipboard.getText()?.text?.let { raw ->
                                val cleaned = raw.replace(" \u2009", "").replace("\u2009", "")
                                val formatted = if (!rawMarkdown.isNullOrBlank()) {
                                    val fullPlainText = MarkdownPlainText.convert(rawMarkdown)
                                    MarkdownPlainText.restoreFormattedSelection(cleaned, fullPlainText)
                                } else {
                                    cleaned.trimEnd('\r', '\n')
                                }
                                clipboard.setText(AnnotatedString(formatted))
                            }
                        }
                    },
                    onPasteRequested, onCutRequested, onSelectAllRequested
                )
            }
        }
    }
    CompositionLocalProvider(LocalTextToolbar provides toolbar) { content() }
}

/** Lightweight "copied" hint centered on the screen (not on the bubble); auto-dismisses. */
@Composable
internal fun CopiedHint(visible: Boolean, onHidden: () -> Unit) {
    if (!visible) return
    LaunchedEffect(Unit) {
        delay(1200)
        onHidden()
    }
    val provider = remember {
        object : PopupPositionProvider {
            override fun calculatePosition(
                anchorBounds: IntRect,
                windowSize: IntSize,
                layoutDirection: LayoutDirection,
                popupContentSize: IntSize
            ) = IntOffset(
                (windowSize.width - popupContentSize.width) / 2,
                (windowSize.height - popupContentSize.height) / 2
            )
        }
    }
    Popup(popupPositionProvider = provider) {
        Text(
            text = "已复制全部文字",
            color = Color.White,
            fontSize = 15.sp,
            modifier = Modifier
                .background(Color.Black.copy(alpha = 0.78f), RoundedCornerShape(14.dp))
                .padding(horizontal = 20.dp, vertical = 12.dp)
        )
    }
}
