package com.antigravity.mobile.ui.components

import android.content.Intent
import android.net.Uri
import android.widget.Toast
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.ClickableText
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForwardIos
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.*
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.ui.theme.AppColors
import com.antigravity.mobile.ui.theme.AntigravityTheme

enum class TableColumnAlignment {
    LEADING, CENTER, TRAILING
}

sealed class MarkdownBlock {
    data class Frontmatter(val id: String, val rawContent: String, val lineCount: Int) : MarkdownBlock()
    data class Heading(val id: String, val level: Int, val text: String) : MarkdownBlock()
    data class Divider(val id: String) : MarkdownBlock()
    data class CodeBlock(val id: String, val lang: String, val code: String) : MarkdownBlock()
    data class Table(
        val id: String,
        val headers: List<String>,
        val rows: List<List<String>>,
        val alignments: List<TableColumnAlignment>
    ) : MarkdownBlock()
    data class BulletList(val id: String, val items: List<String>) : MarkdownBlock()
    data class Paragraph(val id: String, val text: String) : MarkdownBlock()
}

/**
 * 1:1 Markdown parser replicating iOS MarkdownContentView.swift AST generation.
 */
object MarkdownParser {
    fun parse(rawText: String): List<MarkdownBlock> {
        val blocks = mutableListOf<MarkdownBlock>()
        val lines = rawText.replace("\r\n", "\n").replace('\r', '\n').lines()
        var i = 0
        var blockIdx = 0

        while (i < lines.size) {
            val line = lines[i]
            val trimmed = line.trim()

            if (trimmed.isEmpty()) {
                i++
                continue
            }

            // 0. Frontmatter check at document start
            if (blockIdx == 0 && trimmed == "---") {
                var endIdx = i + 1
                var foundEnd = false
                while (endIdx < lines.size) {
                    val t = lines[endIdx].trim()
                    if (t == "---" || t == "...") {
                        foundEnd = true
                        break
                    }
                    endIdx++
                }
                if (foundEnd && endIdx > i + 1) {
                    val raw = lines.subList(i + 1, endIdx).joinToString("\n")
                    blocks.add(MarkdownBlock.Frontmatter("block-${blockIdx++}", raw, endIdx - i + 1))
                    i = endIdx + 1
                    continue
                }
            }

            // 1. Fenced Code Block
            if (trimmed.startsWith("```")) {
                val lang = trimmed.removePrefix("```").trim()
                val codeLines = mutableListOf<String>()
                i++
                while (i < lines.size) {
                    if (lines[i].trim().startsWith("```")) {
                        i++
                        break
                    }
                    codeLines.add(lines[i])
                    i++
                }
                blocks.add(MarkdownBlock.CodeBlock("block-${blockIdx++}", lang, codeLines.joinToString("\n")))
                continue
            }

            // 2. Divider
            if (trimmed == "---" || trimmed == "***" || trimmed == "___") {
                blocks.add(MarkdownBlock.Divider("block-${blockIdx++}"))
                i++
                continue
            }

            // 3. Headings
            if (trimmed.startsWith("#")) {
                var level = 0
                while (level < trimmed.length && trimmed[level] == '#') {
                    level++
                }
                if (level in 1..6 && trimmed.length > level && trimmed[level] == ' ') {
                    val headingText = trimmed.substring(level + 1).trim()
                    blocks.add(MarkdownBlock.Heading("block-${blockIdx++}", level, headingText))
                    i++
                    continue
                }
            }

            // 4. Tables
            if (trimmed.startsWith("|") && trimmed.endsWith("|") && trimmed.contains("|")) {
                val tableLines = mutableListOf<String>()
                while (i < lines.size) {
                    val tLine = lines[i].trim()
                    if (tLine.startsWith("|") && tLine.endsWith("|")) {
                        tableLines.add(tLine)
                        i++
                    } else {
                        break
                    }
                }

                if (tableLines.size >= 2) {
                    val parseRow: (String) -> List<String> = { rowStr ->
                        val placeholder = "\uE000"
                        val sanitized = rowStr.replace("\\|", placeholder)
                        val parts = sanitized.split("|")
                        if (parts.size >= 2) {
                            parts.subList(1, parts.size - 1).map {
                                it.replace(placeholder, "|").trim()
                            }
                        } else emptyList()
                    }

                    val isSeparatorRow: (List<String>) -> Boolean = { cells ->
                        cells.isNotEmpty() && cells.all { cell ->
                            val t = cell.trim()
                            t.isNotEmpty() && t.all { it == '-' || it == ':' }
                        }
                    }

                    val parseAlignments: (List<String>) -> List<TableColumnAlignment> = { cells ->
                        cells.map { cell ->
                            val t = cell.trim()
                            val left = t.startsWith(":")
                            val right = t.endsWith(":")
                            when {
                                left && right -> TableColumnAlignment.CENTER
                                right -> TableColumnAlignment.TRAILING
                                else -> TableColumnAlignment.LEADING
                            }
                        }
                    }

                    val headers = parseRow(tableLines[0])
                    var alignments = emptyList<TableColumnAlignment>()
                    val rows = mutableListOf<List<String>>()

                    for (rowIdx in 1 until tableLines.size) {
                        val r = parseRow(tableLines[rowIdx])
                        if (isSeparatorRow(r)) {
                            if (alignments.isEmpty()) {
                                alignments = parseAlignments(r)
                            }
                        } else {
                            rows.add(r)
                        }
                    }

                    blocks.add(MarkdownBlock.Table("block-${blockIdx++}", headers, rows, alignments))
                    continue
                }
            }

            // 5. Bullet List
            if (trimmed.startsWith("- ") || trimmed.startsWith("* ") || trimmed.startsWith("• ")) {
                val items = mutableListOf<String>()
                while (i < lines.size) {
                    val lLine = lines[i].trim()
                    if (lLine.startsWith("- ") || lLine.startsWith("* ") || lLine.startsWith("• ")) {
                        items.add(lLine.drop(2).trim())
                        i++
                    } else {
                        break
                    }
                }
                blocks.add(MarkdownBlock.BulletList("block-${blockIdx++}", items))
                continue
            }

            // 6. Paragraph
            val paraLines = mutableListOf<String>()
            paraLines.add(line)
            i++
            while (i < lines.size) {
                val nextLine = lines[i]
                val nTrimmed = nextLine.trim()
                if (nTrimmed.isEmpty() ||
                    nTrimmed.startsWith("```") ||
                    nTrimmed.startsWith("#") ||
                    nTrimmed == "---" ||
                    (nTrimmed.startsWith("|") && nTrimmed.endsWith("|")) ||
                    nTrimmed.startsWith("- ") ||
                    nTrimmed.startsWith("* ") ||
                    nTrimmed.startsWith("• ")
                ) {
                    break
                }
                paraLines.add(nextLine)
                i++
            }
            blocks.add(MarkdownBlock.Paragraph("block-${blockIdx++}", paraLines.joinToString("\n")))
        }

        return blocks
    }
}

/**
 * Android Jetpack Compose Markdown Content Renderer matching iOS MarkdownContentView 1:1.
 */
@Composable
fun MarkdownContentView(
    content: String,
    modifier: Modifier = Modifier,
    onPlanClick: ((uri: String, title: String) -> Unit)? = null
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
                    CodeBlockView(
                        lang = block.lang,
                        code = block.code,
                        colors = colors
                    )
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

                is MarkdownBlock.Paragraph -> {
                    ParagraphBlockView(
                        text = block.text,
                        colors = colors,
                        onPlanClick = onPlanClick
                    )
                }
            }
        }
    }
}

@Composable
private fun FrontmatterCard(
    rawContent: String,
    lineCount: Int,
    colors: AppColors
) {
    var expanded by remember { mutableStateOf(false) }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .background(colors.surfaceVariant.copy(alpha = 0.4f))
            .border(0.5.dp, colors.border, RoundedCornerShape(8.dp))
            .clickable { expanded = !expanded }
            .padding(horizontal = 12.dp, vertical = 8.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.Description,
                    contentDescription = null,
                    tint = colors.textMuted,
                    modifier = Modifier.size(14.dp)
                )
                Text(
                    text = "YAML 配置 / 元数据 ($lineCount 行)",
                    color = colors.textSecondary,
                    fontSize = 12.sp,
                    fontWeight = FontWeight.Medium
                )
            }
            Icon(
                imageVector = if (expanded) Icons.Default.ExpandLess else Icons.Default.ExpandMore,
                contentDescription = null,
                tint = colors.textMuted,
                modifier = Modifier.size(16.dp)
            )
        }

        AnimatedVisibility(visible = expanded) {
            SelectionContainer {
                Text(
                    text = rawContent,
                    color = colors.textSecondary,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 11.5.sp,
                    lineHeight = 16.sp,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 8.dp)
                )
            }
        }
    }
}

@Composable
private fun HeadingBlockView(
    level: Int,
    text: String,
    colors: AppColors,
    onPlanClick: ((String, String) -> Unit)?
) {
    val fontSize = when (level) {
        1 -> 20.sp
        2 -> 18.sp
        3 -> 16.sp
        4 -> 15.sp
        else -> 14.sp
    }

    val topPadding = if (level <= 2) 6.dp else 2.dp

    Box(modifier = Modifier.padding(top = topPadding, bottom = 2.dp)) {
        RichTextRenderer(
            text = text,
            colors = colors,
            baseFontSize = fontSize,
            baseFontWeight = FontWeight.Bold,
            onPlanClick = onPlanClick
        )
    }
}

@Composable
private fun CodeBlockView(
    lang: String,
    code: String,
    colors: AppColors
) {
    val clipboardManager = LocalClipboardManager.current
    val context = LocalContext.current
    var copied by remember { mutableStateOf(false) }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(colors.codeBlockBg)
            .border(0.8.dp, colors.border, RoundedCornerShape(10.dp))
    ) {
        // Code Block Header
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .background(colors.surfaceVariant.copy(alpha = 0.6f))
                .padding(horizontal = 12.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Text(
                text = if (lang.isNotBlank()) lang.uppercase() else "CODE",
                color = colors.textMuted,
                fontSize = 10.5.sp,
                fontWeight = FontWeight.Bold,
                fontFamily = FontFamily.Monospace
            )

            Row(
                modifier = Modifier
                    .clip(RoundedCornerShape(6.dp))
                    .clickable {
                        clipboardManager.setText(AnnotatedString(code))
                        copied = true
                        Toast.makeText(context, "已复制到剪贴板", Toast.LENGTH_SHORT).show()
                    }
                    .padding(horizontal = 6.dp, vertical = 3.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                Icon(
                    imageVector = if (copied) Icons.Default.Check else Icons.Default.ContentCopy,
                    contentDescription = "Copy",
                    tint = if (copied) colors.accentIndigo else colors.textSecondary,
                    modifier = Modifier.size(12.dp)
                )
                Text(
                    text = if (copied) "已复制" else "复制",
                    color = if (copied) colors.accentIndigo else colors.textSecondary,
                    fontSize = 11.sp
                )
            }
        }

        HorizontalDivider(color = colors.border.copy(alpha = 0.4f), thickness = 0.5.dp)

        // Horizontally Scrollable Monospaced Code
        SelectionContainer {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .horizontalScroll(rememberScrollState())
                    .padding(12.dp)
            ) {
                Text(
                    text = code.trimEnd(),
                    color = colors.textPrimary,
                    fontSize = 12.5.sp,
                    fontFamily = FontFamily.Monospace,
                    lineHeight = 18.sp
                )
            }
        }
    }
}

@Composable
private fun TableBlockView(
    headers: List<String>,
    rows: List<List<String>>,
    alignments: List<TableColumnAlignment>,
    colors: AppColors,
    onPlanClick: ((String, String) -> Unit)?
) {
    val columnCount = maxOf(headers.size, rows.maxOfOrNull { it.size } ?: 0)
    if (columnCount == 0) return

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .border(0.8.dp, colors.border, RoundedCornerShape(10.dp))
            .horizontalScroll(rememberScrollState())
    ) {
        Column {
            // Header Row
            if (headers.isNotEmpty()) {
                Row(
                    modifier = Modifier.background(colors.surfaceVariant.copy(alpha = 0.8f))
                ) {
                    for (colIdx in 0 until columnCount) {
                        val headerText = headers.getOrNull(colIdx) ?: ""
                        val alignment = alignments.getOrNull(colIdx) ?: TableColumnAlignment.LEADING

                        Box(
                            modifier = Modifier
                                .widthIn(min = 96.dp)
                                .padding(horizontal = 12.dp, vertical = 8.dp),
                            contentAlignment = when (alignment) {
                                TableColumnAlignment.LEADING -> Alignment.CenterStart
                                TableColumnAlignment.CENTER -> Alignment.Center
                                TableColumnAlignment.TRAILING -> Alignment.CenterEnd
                            }
                        ) {
                            RichTextRenderer(
                                text = headerText,
                                colors = colors,
                                baseFontSize = 13.sp,
                                baseFontWeight = FontWeight.Bold,
                                onPlanClick = onPlanClick
                            )
                        }

                        if (colIdx < columnCount - 1) {
                            Box(
                                modifier = Modifier
                                    .width(0.5.dp)
                                    .height(34.dp)
                                    .background(colors.border.copy(alpha = 0.3f))
                            )
                        }
                    }
                }
                HorizontalDivider(color = colors.border.copy(alpha = 0.4f), thickness = 0.8.dp)
            }

            // Data Rows
            rows.forEachIndexed { rowIdx, row ->
                val isEven = rowIdx % 2 == 0
                val rowBg = if (isEven) Color.Transparent else colors.surfaceVariant.copy(alpha = 0.35f)

                Row(modifier = Modifier.background(rowBg)) {
                    for (colIdx in 0 until columnCount) {
                        val cellText = row.getOrNull(colIdx) ?: ""
                        val alignment = alignments.getOrNull(colIdx) ?: TableColumnAlignment.LEADING

                        Box(
                            modifier = Modifier
                                .widthIn(min = 96.dp)
                                .padding(horizontal = 12.dp, vertical = 8.dp),
                            contentAlignment = when (alignment) {
                                TableColumnAlignment.LEADING -> Alignment.CenterStart
                                TableColumnAlignment.CENTER -> Alignment.Center
                                TableColumnAlignment.TRAILING -> Alignment.CenterEnd
                            }
                        ) {
                            RichTextRenderer(
                                text = cellText,
                                colors = colors,
                                baseFontSize = 13.sp,
                                onPlanClick = onPlanClick
                            )
                        }

                        if (colIdx < columnCount - 1) {
                            Box(
                                modifier = Modifier
                                    .width(0.5.dp)
                                    .height(32.dp)
                                    .background(colors.border.copy(alpha = 0.2f))
                            )
                        }
                    }
                }

                if (rowIdx < rows.size - 1) {
                    HorizontalDivider(color = colors.border.copy(alpha = 0.2f), thickness = 0.5.dp)
                }
            }
        }
    }
}

@Composable
private fun BulletListBlockView(
    items: List<String>,
    colors: AppColors,
    onPlanClick: ((String, String) -> Unit)?
) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        items.forEach { item ->
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.Top,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Text(
                    text = "•",
                    color = colors.accentIndigo,
                    fontSize = 15.sp,
                    fontWeight = FontWeight.Bold,
                    modifier = Modifier.padding(top = 1.dp)
                )
                Box(modifier = Modifier.weight(1f)) {
                    ParagraphBlockView(
                        text = item,
                        colors = colors,
                        onPlanClick = onPlanClick
                    )
                }
            }
        }
    }
}

@Composable
private fun ParagraphBlockView(
    text: String,
    colors: AppColors,
    onPlanClick: ((String, String) -> Unit)?
) {
    // If text references implementation_plan.md or walkthrough.md, provide standalone plan pill button
    val hasPlanRef = text.contains("implementation_plan.md", ignoreCase = true) ||
            text.contains("walkthrough.md", ignoreCase = true)

    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        RichTextRenderer(
            text = text,
            colors = colors,
            baseFontSize = 15.sp,
            onPlanClick = onPlanClick
        )

        if (hasPlanRef && onPlanClick != null) {
            val planFile = if (text.contains("walkthrough.md", ignoreCase = true)) {
                "walkthrough.md"
            } else {
                "implementation_plan.md"
            }
            val title = if (planFile == "walkthrough.md") "实施结果走查 (Walkthrough)" else "实施方案 (Implementation Plan)"

            PlanButtonCard(
                title = title,
                filename = planFile,
                colors = colors,
                onClick = { onPlanClick(planFile, title) }
            )
        }
    }
}

@Composable
private fun PlanButtonCard(
    title: String,
    filename: String,
    colors: AppColors,
    onClick: () -> Unit
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(colors.accentIndigo.copy(alpha = 0.10f))
            .border(0.8.dp, colors.accentIndigo.copy(alpha = 0.35f), RoundedCornerShape(10.dp))
            .clickable(onClick = onClick)
            .padding(horizontal = 12.dp, vertical = 9.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        Box(
            modifier = Modifier
                .size(32.dp)
                .clip(CircleShape)
                .background(colors.accentIndigo.copy(alpha = 0.15f)),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                imageVector = Icons.Default.Description,
                contentDescription = null,
                tint = colors.accentIndigo,
                modifier = Modifier.size(16.dp)
            )
        }

        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = title,
                color = colors.textPrimary,
                fontSize = 13.5.sp,
                fontWeight = FontWeight.SemiBold
            )
            Text(
                text = filename,
                color = colors.accentIndigo,
                fontSize = 11.sp,
                fontFamily = FontFamily.Monospace
            )
        }

        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            Text(
                text = "点击查看",
                color = colors.accentIndigo,
                fontSize = 12.sp,
                fontWeight = FontWeight.Medium
            )
            Icon(
                imageVector = Icons.AutoMirrored.Filled.ArrowForwardIos,
                contentDescription = null,
                tint = colors.accentIndigo,
                modifier = Modifier.size(11.dp)
            )
        }
    }
}

/**
 * Parses and renders inline markdown formatting:
 * - Bold: `**text**` or `__text__`
 * - Italic: `*text*` or `_text_`
 * - Inline code: `` `code` `` in Desktop Amber (#E5C07B)
 * - Links: `[text](url)`
 * - LaTeX Arrows: `\to` -> `→`, etc.
 */
@Composable
private fun RichTextRenderer(
    text: String,
    colors: AppColors,
    baseFontSize: TextUnit = 15.sp,
    baseFontWeight: FontWeight = FontWeight.Normal,
    onPlanClick: ((String, String) -> Unit)? = null
) {
    val context = LocalContext.current
    val annotatedString = remember(text, colors, baseFontSize, baseFontWeight) {
        buildRichTextAnnotatedString(
            rawText = text,
            colors = colors,
            baseFontSize = baseFontSize,
            baseFontWeight = baseFontWeight
        )
    }

    ClickableText(
        text = annotatedString,
        style = TextStyle(
            color = colors.textPrimary,
            fontSize = baseFontSize,
            fontWeight = baseFontWeight,
            lineHeight = (baseFontSize.value * 1.45f).sp
        ),
        onClick = { offset ->
            annotatedString.getStringAnnotations(tag = "URL", start = offset, end = offset)
                .firstOrNull()?.let { annotation ->
                    val url = annotation.item
                    if (url.contains("implementation_plan") || url.contains("walkthrough") || url.contains(".md")) {
                        onPlanClick?.invoke(url, url.substringAfterLast('/'))
                    } else {
                        try {
                            val intent = Intent(Intent.ACTION_VIEW, Uri.parse(url)).apply {
                                flags = Intent.FLAG_ACTIVITY_NEW_TASK
                            }
                            context.startActivity(intent)
                        } catch (_: Exception) {}
                    }
                }
        }
    )
}

private val INLINE_TOKEN_REGEX = Regex(
    """(?<!\!)\[([^\]]+)\]\(([^)]+)\)|`([^`]+)`|\*\*([^*]+)\*\*|__([^_]+)__|(?<!\*)\*([^*\n]+)\*(?!\*)|(?<!_)_([^_\n]+)_(?!_)|~~([^~]+)~~"""
)

private fun buildRichTextAnnotatedString(
    rawText: String,
    colors: AppColors,
    baseFontSize: TextUnit,
    baseFontWeight: FontWeight
): AnnotatedString {
    var processed = preprocessArrows(rawText)
    processed = replaceHtmlBreaks(processed)

    // Antigravity Desktop Code Amber/Yellow color: #E5C07B (RGB: 229, 192, 123)
    val codeColor = Color(0xFFE5C07B)

    return buildAnnotatedString {
        var lastIndex = 0
        val matches = INLINE_TOKEN_REGEX.findAll(processed)

        for (match in matches) {
            val range = match.range
            if (range.first > lastIndex) {
                append(processed.substring(lastIndex, range.first))
            }

            val linkText = match.groups[1]?.value
            val linkUrl = match.groups[2]?.value
            val inlineCode = match.groups[3]?.value
            val boldText1 = match.groups[4]?.value
            val boldText2 = match.groups[5]?.value
            val italicText1 = match.groups[6]?.value
            val italicText2 = match.groups[7]?.value
            val strikeText = match.groups[8]?.value

            when {
                linkText != null && linkUrl != null -> {
                    val start = length
                    append(linkText)
                    val isPlan = linkUrl.contains("implementation_plan") || linkUrl.contains("walkthrough")
                    addStyle(
                        style = SpanStyle(
                            color = colors.accentIndigo,
                            fontWeight = FontWeight.SemiBold,
                            textDecoration = TextDecoration.Underline,
                            background = if (isPlan) colors.accentIndigo.copy(alpha = 0.12f) else Color.Transparent
                        ),
                        start = start,
                        end = length
                    )
                    addStringAnnotation(
                        tag = "URL",
                        annotation = linkUrl,
                        start = start,
                        end = length
                    )
                }

                inlineCode != null -> {
                    val start = length
                    append(inlineCode)
                    addStyle(
                        style = SpanStyle(
                            color = codeColor,
                            fontFamily = FontFamily.Monospace,
                            fontWeight = FontWeight.Medium,
                            fontSize = (baseFontSize.value * 0.9f).sp,
                            background = colors.surfaceVariant.copy(alpha = 0.45f)
                        ),
                        start = start,
                        end = length
                    )
                }

                (boldText1 != null || boldText2 != null) -> {
                    val content = boldText1 ?: boldText2 ?: ""
                    val start = length
                    append(content)
                    addStyle(
                        style = SpanStyle(fontWeight = FontWeight.Bold),
                        start = start,
                        end = length
                    )
                }

                (italicText1 != null || italicText2 != null) -> {
                    val content = italicText1 ?: italicText2 ?: ""
                    val start = length
                    append(content)
                    addStyle(
                        style = SpanStyle(fontStyle = FontStyle.Italic),
                        start = start,
                        end = length
                    )
                }

                strikeText != null -> {
                    val start = length
                    append(strikeText)
                    addStyle(
                        style = SpanStyle(textDecoration = TextDecoration.LineThrough),
                        start = start,
                        end = length
                    )
                }

                else -> {
                    append(match.value)
                }
            }

            lastIndex = range.last + 1
        }

        if (lastIndex < processed.length) {
            append(processed.substring(lastIndex))
        }
    }
}

/**
 * Preprocesses bare LaTeX arrow commands into native Unicode arrows.
 */
private fun preprocessArrows(text: String): String {
    if (!text.contains('\\')) return text
    return text
        .replace(Regex("""\\(?:to|rightarrow)\b"""), "→")
        .replace(Regex("""\\(?:gets|leftarrow)\b"""), "←")
        .replace(Regex("""\\(?:implies|Rightarrow)\b"""), "⇒")
        .replace(Regex("""\\(?:iff|Leftrightarrow)\b"""), "⇔")
}

/**
 * Replaces HTML line breaks (<br>, <br/>, <br />, </br>) with newlines.
 */
private fun replaceHtmlBreaks(text: String): String {
    if (!text.contains("<br", ignoreCase = true)) return text
    return text
        .replace(Regex("""<br\s*/?>""", RegexOption.IGNORE_CASE), "\n")
        .replace("</br>", "\n", ignoreCase = true)
}
