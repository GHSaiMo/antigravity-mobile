package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.service.MathSymbolProcessor
import com.antigravity.mobile.ui.theme.AntigravityTheme

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

/** Dialog showing markdown-stripped text in a SelectionContainer for free-range selection and copy. */
@Composable
fun SelectableTextDialog(text: String, onDismiss: () -> Unit) {
    val colors = AntigravityTheme.colors
    val clipboard = LocalClipboardManager.current
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("选择文字") },
        text = {
            SelectionContainer {
                Text(
                    text = text,
                    color = colors.textPrimary,
                    fontSize = 15.sp,
                    lineHeight = 22.sp,
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(max = 420.dp)
                        .verticalScroll(rememberScrollState())
                )
            }
        },
        confirmButton = {
            TextButton(onClick = {
                clipboard.setText(AnnotatedString(text))
                onDismiss()
            }) { Text("复制全部") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("完成") } }
    )
}
