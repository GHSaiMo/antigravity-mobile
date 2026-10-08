package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.draw.clip
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.selection.DisableSelection
import androidx.compose.foundation.text.selection.SelectionContainer
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
    val isExport = LocalShareExport.current

    WithCleanCopyToolbar(rawMarkdown = content) {
    SelectionContainer {
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
                    if (isExport && (block.lang.equals("mermaid", ignoreCase = true) || block.lang.equals("diagram", ignoreCase = true))) {
                        ShareExportPlaceholder("Mermaid 图表，请在 App 中查看")
                    } else if (block.lang.equals("mermaid", ignoreCase = true) || block.lang.equals("diagram", ignoreCase = true)) {
                        MermaidDiagramView(
                            code = block.code,
                            colors = colors
                        )
                    } else if (isExport) {
                        ShareExportPlaceholder("代码块（${block.code.trimEnd().lines().size} 行），请在 App 中查看")
                    } else {
                        CodeBlockView(
                            lang = block.lang,
                            code = block.code,
                            colors = colors
                        )
                    }
                }

                is MarkdownBlock.Table -> {
                    val columns = maxOf(block.headers.size, block.rows.maxOfOrNull { it.size } ?: 0)
                    if (isExport && (columns >= 4 || block.rows.size > 12)) {
                        ShareExportPlaceholder("表格（${block.rows.size} 行 × $columns 列），请在 App 中查看")
                    } else
                    DisableSelection {
                    TableBlockView(
                        headers = block.headers,
                        rows = block.rows,
                        alignments = block.alignments,
                        colors = colors,
                        onPlanClick = onPlanClick
                    )
                    }
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

                is MarkdownBlock.AgentEmbed -> {
                    if (isExport) {
                        ShareExportPlaceholder("交互内容，请在 App 中查看")
                    } else {
                        AgentEmbedView(
                            src = block.src,
                            colors = colors,
                            urlResolver = urlResolver
                        )
                    }
                }

                is MarkdownBlock.Carousel -> {
                    if (isExport) {
                        ShareExportPlaceholder("图片轮播（${block.slides.size} 张），请在 App 中查看")
                    } else {
                        CarouselBlockView(
                            slides = block.slides,
                            colors = colors,
                            onPlanClick = onPlanClick,
                            urlResolver = urlResolver,
                            onImageClick = onImageClick
                        )
                    }
                }
            }
        }
    }
    }
    }
}

/** 长图导出时，WebView / 交互类块无法离屏绘制，用占位提示代替。 */
@Composable
private fun ShareExportPlaceholder(text: String) {
    val colors = AntigravityTheme.colors
    androidx.compose.material3.Text(
        text = text,
        color = colors.textSecondary,
        fontSize = androidx.compose.ui.unit.TextUnit(12f, androidx.compose.ui.unit.TextUnitType.Sp),
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(colors.surfaceVariant.copy(alpha = 0.5f))
            .padding(10.dp)
    )
}
