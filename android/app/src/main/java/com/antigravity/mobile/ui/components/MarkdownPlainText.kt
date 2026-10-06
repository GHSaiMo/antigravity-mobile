package com.antigravity.mobile.ui.components

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.layout.onGloballyPositioned
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
}

/** Tracks the on-screen bounds of selectable text blocks so a bubble can tell "blank area" from "text". */
class TextRegionRegistry {
    val regions = HashMap<Any, Rect>()
    fun containsRoot(p: androidx.compose.ui.geometry.Offset) = regions.values.any { it.contains(p) }
}

val LocalTextRegions = compositionLocalOf<TextRegionRegistry?> { null }

/** Each text block gets its own SelectionContainer so a selection can never span several blocks. */
@Composable
internal fun SelectableTextRegion(content: @Composable () -> Unit) {
    val registry = LocalTextRegions.current
    val key = remember { Any() }
    DisposableEffect(registry) { onDispose { registry?.regions?.remove(key) } }
    SelectionContainer(
        modifier = Modifier.onGloballyPositioned { registry?.regions?.put(key, it.boundsInRoot()) }
    ) { content() }
}

/**
 * Wraps the system selection toolbar so that copying a selection drops the inline file-icon
 * placeholders (a space + thin space) that the rich text renderer inserts.
 */
@Composable
internal fun WithCleanCopyToolbar(content: @Composable () -> Unit) {
    val base = LocalTextToolbar.current
    val clipboard = LocalClipboardManager.current
    val toolbar = remember(base, clipboard) {
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
                                if (cleaned != raw) clipboard.setText(AnnotatedString(cleaned))
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
