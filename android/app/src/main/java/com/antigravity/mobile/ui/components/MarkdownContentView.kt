package com.antigravity.mobile.ui.components

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.*
import androidx.compose.ui.unit.dp
import com.antigravity.mobile.ui.theme.AntigravityTheme

/**
 * Android Jetpack Compose Markdown Content Renderer matching iOS MarkdownContentView 1:1.
 */
@Composable
fun MarkdownContentView(
    content: String,
    modifier: Modifier = Modifier,
    onPlanClick: ((uri: String, title: String) -> Unit)? = null,
    urlResolver: ((String) -> String)? = null,
    onImageClick: ((url: String) -> Unit)? = null
) {
    val blocks = remember(content) { MarkdownParser.parse(content) }
    val colors = AntigravityTheme.colors

    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        blocks.forEach { block ->
            when (block) {
                is MarkdownBlock.Frontmatter -> {
                    FrontmatterCard(
                        rawContent = block.rawContent,
                        lineCount = block.lineCount,
                        colors = colors
                    )
                }

                is MarkdownBlock.Heading -> {
                    HeadingBlockView(
                        level = block.level,
                        text = block.text,
                        colors = colors,
                        onPlanClick = onPlanClick
                    )
                }

                is MarkdownBlock.Divider -> {
                    HorizontalDivider(
                        color = colors.border.copy(alpha = 0.5f),
                        thickness = 0.8.dp,
                        modifier = Modifier.padding(vertical = 4.dp)
                    )
                }

                is MarkdownBlock.CodeBlock -> {
                    if (block.lang.equals("mermaid", ignoreCase = true) || block.lang.equals("diagram", ignoreCase = true)) {
                        MermaidDiagramView(
                            code = block.code,
                            colors = colors
                        )
                    } else {
                        CodeBlockView(
                            lang = block.lang,
                            code = block.code,
                            colors = colors
                        )
                    }
                }

                is MarkdownBlock.Table -> {
                    TableBlockView(
                        headers = block.headers,
                        rows = block.rows,
                        alignments = block.alignments,
                        colors = colors,
                        onPlanClick = onPlanClick
                    )
                }

                is MarkdownBlock.BulletList -> {
                    BulletListBlockView(
                        items = block.items,
                        colors = colors,
                        onPlanClick = onPlanClick
                    )
                }

                is MarkdownBlock.OrderedList -> {
                    OrderedListBlockView(
                        startIndex = block.startIndex,
                        items = block.items,
                        colors = colors,
                        onPlanClick = onPlanClick
                    )
                }

                is MarkdownBlock.Paragraph -> {
                    ParagraphBlockView(
                        text = block.text,
                        colors = colors,
                        onPlanClick = onPlanClick
                    )
                }

                is MarkdownBlock.Image -> {
                    MarkdownImageView(
                        alt = block.alt,
                        url = block.url,
                        colors = colors,
                        urlResolver = urlResolver,
                        onImageClick = onImageClick
                    )
                }
            }
        }
    }
}
