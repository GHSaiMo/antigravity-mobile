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
                                val formatted = if (cleaned.endsWith("\n")) cleaned else "$cleaned\n"
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
