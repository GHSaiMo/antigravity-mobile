package com.antigravity.mobile.ui.components

import android.content.Intent
import android.net.Uri
import android.widget.Toast
import java.net.URLDecoder
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.InlineTextContent
import androidx.compose.foundation.text.appendInlineContent
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForwardIos
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.OpenInNew
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.*
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.Placeholder
import androidx.compose.ui.text.PlaceholderVerticalAlign
import androidx.compose.ui.text.TextLayoutResult
import com.antigravity.mobile.data.service.FileIconResolver
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImagePainter
import coil.compose.SubcomposeAsyncImage
import coil.compose.SubcomposeAsyncImageContent
import coil.request.ImageRequest
import com.antigravity.mobile.data.service.MathSymbolProcessor
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
    data class OrderedList(val id: String, val startIndex: Int, val items: List<String>) : MarkdownBlock()
    data class Paragraph(val id: String, val text: String) : MarkdownBlock()
    data class Image(val id: String, val alt: String, val url: String) : MarkdownBlock()
}

private val ORDERED_LIST_REGEX = Regex("""^(\d{1,9})[.)]\s+(.*)$""")

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

            // 5b. Ordered List
            val orderedMatch = ORDERED_LIST_REGEX.find(trimmed)
            if (orderedMatch != null) {
                val startNum = orderedMatch.groups[1]?.value?.toIntOrNull() ?: 1
                val items = mutableListOf<String>()
                while (i < lines.size) {
                    val lLine = lines[i].trim()
                    val m = ORDERED_LIST_REGEX.find(lLine)
                    if (m != null) {
                        items.add(m.groups[2]?.value?.trim() ?: "")
                        i++
                    } else {
                        break
                    }
                }
                blocks.add(MarkdownBlock.OrderedList("block-${blockIdx++}", startNum, items))
                continue
            }

            // 6. Standalone image line: ![alt](url), [![alt](thumb)](url), MEDIA:url
            val standaloneImg = parseStandaloneImage(trimmed)
            if (standaloneImg != null) {
                blocks.add(MarkdownBlock.Image("block-${blockIdx++}", standaloneImg.first, standaloneImg.second))
                i++
                continue
            }

            // 7. Paragraph
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
                    nTrimmed.startsWith("• ") ||
                    ORDERED_LIST_REGEX.containsMatchIn(nTrimmed) ||
                    parseStandaloneImage(nTrimmed) != null
                ) {
                    break
                }
                paraLines.add(nextLine)
                i++
            }
            val paraText = paraLines.joinToString("\n")
            val imagesInPara = findImages(paraText)
            if (imagesInPara.isEmpty()) {
                blocks.add(MarkdownBlock.Paragraph("block-${blockIdx++}", paraText))
            } else {
                var curIdx = 0
                for (img in imagesInPara) {
                    if (img.range.first > curIdx) {
                        val textBefore = paraText.substring(curIdx, img.range.first).trim()
                        if (textBefore.isNotEmpty()) {
                            blocks.add(MarkdownBlock.Paragraph("block-${blockIdx++}", textBefore))
                        }
                    }
                    blocks.add(MarkdownBlock.Image("block-${blockIdx++}", img.alt, img.url))
                    curIdx = img.range.last + 1
                }
                if (curIdx < paraText.length) {
                    val textAfter = paraText.substring(curIdx).trim()
                    if (textAfter.isNotEmpty()) {
                        blocks.add(MarkdownBlock.Paragraph("block-${blockIdx++}", textAfter))
                    }
                }
            }
        }

        return blocks
    }

    private val linkedImageRegex = Regex("""\[!\[(.*?)\]\(([^\s\)]+)(?:\s+"[^"]*")?\)\]\(([^\s\)]+)(?:\s+"[^"]*")?\)""")
    private val markdownImageRegex = Regex("""!\[(.*?)\]\(([^\s\)]+)(?:\s+"[^"]*")?\)""")
    private val mediaPrefixRegex = Regex("""(?:^|\s|<br\s*/?>)MEDIA:\s*([^\s)<>"'`]+)""", RegexOption.IGNORE_CASE)

    private fun cleanImageURL(raw: String): String {
        return raw.trim().trim('`', '"', '\'', '(', ')', '[', ']', '<', '>')
    }

    private data class FoundImage(val alt: String, val url: String, val range: IntRange)

    private fun findImages(text: String): List<FoundImage> {
        if (text.isEmpty()) return emptyList()
        val results = mutableListOf<FoundImage>()

        linkedImageRegex.findAll(text).forEach { match ->
            val alt = match.groups[1]?.value.orEmpty()
            val orig = match.groups[3]?.value.orEmpty()
            val cleaned = cleanImageURL(orig)
            if (cleaned.isNotBlank()) {
                results.add(FoundImage(alt.trim(), cleaned, match.range))
            }
        }

        markdownImageRegex.findAll(text).forEach { match ->
            val alt = match.groups[1]?.value.orEmpty()
            val url = match.groups[2]?.value.orEmpty()
            val cleaned = cleanImageURL(url)
            if (cleaned.isNotBlank() && results.none { it.range.contains(match.range.first) }) {
                results.add(FoundImage(alt.trim(), cleaned, match.range))
            }
        }

        mediaPrefixRegex.findAll(text).forEach { match ->
            val url = match.groups[1]?.value.orEmpty()
            val cleaned = cleanImageURL(url)
            if (cleaned.isNotBlank() && results.none { it.range.contains(match.range.first) }) {
                val fileName = cleaned.substringAfterLast('/')
                results.add(FoundImage(fileName, cleaned, match.range))
            }
        }

        return results.sortedBy { it.range.first }
    }

    private fun parseStandaloneImage(trimmed: String): Pair<String, String>? {
        val images = findImages(trimmed)
        if (images.size != 1) return null
        val img = images[0]
        val before = trimmed.substring(0, img.range.first).trim()
        val after = trimmed.substring(img.range.last + 1).trim()
        if (before.isEmpty() && after.isEmpty()) {
            return Pair(img.alt, img.url)
        }
        return null
    }
}

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

@Composable
private fun MarkdownImageView(
    alt: String,
    url: String,
    colors: AppColors,
    urlResolver: ((String) -> String)? = null,
    onImageClick: ((String) -> Unit)? = null
) {
    val context = LocalContext.current
    val resolvedUrl = remember(url) { urlResolver?.invoke(url) ?: url }

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surfaceVariant.copy(alpha = 0.5f))
            .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            .clickable { onImageClick?.invoke(resolvedUrl) },
        contentAlignment = Alignment.Center
    ) {
        SubcomposeAsyncImage(
            model = ImageRequest.Builder(context)
                .data(resolvedUrl)
                .crossfade(true)
                .build(),
            contentDescription = alt.ifBlank { "Markdown image" },
            contentScale = ContentScale.Fit,
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(max = 320.dp)
        ) {
            val state = painter.state
            when (state) {
                is AsyncImagePainter.State.Loading -> {
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(140.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        CircularProgressIndicator(
                            color = colors.accentIndigo,
                            strokeWidth = 2.dp,
                            modifier = Modifier.size(24.dp)
                        )
                    }
                }
                is AsyncImagePainter.State.Error -> {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(16.dp),
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Description,
                            contentDescription = "Failed",
                            tint = colors.textMuted,
                            modifier = Modifier.size(28.dp)
                        )
                        Text(
                            text = if (alt.isNotBlank()) alt else "图片加载失败",
                            color = colors.textSecondary,
                            fontSize = 12.sp
                        )
                    }
                }
                else -> {
                    SubcomposeAsyncImageContent()
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

        AnimatedVisibility(
            visible = expanded,
            enter = expandVertically(
                animationSpec = spring(
                    dampingRatio = 0.82f,
                    stiffness = Spring.StiffnessMediumLow
                )
            ) + fadeIn(
                animationSpec = spring(dampingRatio = 0.82f)
            ),
            exit = shrinkVertically(
                animationSpec = spring(
                    dampingRatio = 0.82f,
                    stiffness = Spring.StiffnessMediumLow
                )
            ) + fadeOut(
                animationSpec = spring(dampingRatio = 0.82f)
            )
        ) {
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
    val haptic = com.antigravity.mobile.ui.util.rememberHaptic()
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
            val fileType = remember(lang) { com.antigravity.mobile.data.service.FileIconResolver.resolve(lang) }
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(7.dp)
                        .clip(CircleShape)
                        .background(fileType.color)
                )
                if (fileType.glyph.isNotEmpty()) {
                    Text(
                        text = fileType.glyph,
                        fontSize = 11.sp
                    )
                }
                Text(
                    text = fileType.displayName,
                    color = fileType.color,
                    fontSize = 11.sp,
                    fontWeight = FontWeight.Bold,
                    fontFamily = FontFamily.Monospace
                )
            }

            Row(
                modifier = Modifier
                    .clip(RoundedCornerShape(6.dp))
                    .clickable {
                        haptic.medium()
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

    // Calculate synchronized column widths across header and all rows
    val columnWidths = remember(headers, rows, columnCount) {
        (0 until columnCount).map { colIdx ->
            val headerText = headers.getOrNull(colIdx) ?: ""
            val rowTexts = rows.map { it.getOrNull(colIdx) ?: "" }
            val allTexts = listOf(headerText) + rowTexts

            // Effective display length: non-ASCII characters count as 2, ASCII as 1
            val maxLen = allTexts.maxOfOrNull { text ->
                text.fold(0) { acc, ch -> acc + if (ch.code > 127) 2 else 1 }
            } ?: 0

            when {
                columnCount == 2 && colIdx == 0 -> {
                    // Two-column table first column (Key/Property): compact but fits 4-8 Chinese chars nicely
                    (maxLen * 8.5f + 32f).coerceIn(104f, 150f).dp
                }
                columnCount == 2 && colIdx == 1 -> {
                    // Two-column table second column (Value/Detail): spacious with auto-wrapping
                    maxOf(220f, (maxLen * 7.5f + 32f).coerceAtMost(360f)).dp
                }
                else -> {
                    // Multi-column table: balanced column width
                    (maxLen * 8f + 28f).coerceIn(96f, 260f).dp
                }
            }
        }
    }

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
                    modifier = Modifier
                        .background(colors.surfaceVariant.copy(alpha = 0.85f))
                        .height(IntrinsicSize.Min)
                ) {
                    for (colIdx in 0 until columnCount) {
                        val headerText = headers.getOrNull(colIdx) ?: ""
                        val alignment = alignments.getOrNull(colIdx) ?: TableColumnAlignment.LEADING
                        val width = columnWidths.getOrElse(colIdx) { 110.dp }

                        Box(
                            modifier = Modifier
                                .width(width)
                                .padding(horizontal = 12.dp, vertical = 9.dp),
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
                                    .fillMaxHeight()
                                    .width(0.8.dp)
                                    .background(colors.border.copy(alpha = 0.4f))
                            )
                        }
                    }
                }
                HorizontalDivider(color = colors.border.copy(alpha = 0.5f), thickness = 0.8.dp)
            }

            // Data Rows
            rows.forEachIndexed { rowIdx, row ->
                val isEven = rowIdx % 2 == 0
                val rowBg = if (isEven) Color.Transparent else colors.surfaceVariant.copy(alpha = 0.35f)

                Row(
                    modifier = Modifier
                        .background(rowBg)
                        .height(IntrinsicSize.Min)
                ) {
                    for (colIdx in 0 until columnCount) {
                        val cellText = row.getOrNull(colIdx) ?: ""
                        val alignment = alignments.getOrNull(colIdx) ?: TableColumnAlignment.LEADING
                        val width = columnWidths.getOrElse(colIdx) { 110.dp }

                        Box(
                            modifier = Modifier
                                .width(width)
                                .padding(horizontal = 12.dp, vertical = 9.dp),
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
                                    .fillMaxHeight()
                                    .width(0.8.dp)
                                    .background(colors.border.copy(alpha = 0.3f))
                            )
                        }
                    }
                }

                if (rowIdx < rows.size - 1) {
                    HorizontalDivider(color = colors.border.copy(alpha = 0.25f), thickness = 0.5.dp)
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
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Text(
                    text = "•",
                    color = colors.textSecondary,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Bold,
                    modifier = Modifier.padding(top = 2.dp)
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
private fun OrderedListBlockView(
    startIndex: Int,
    items: List<String>,
    colors: AppColors,
    onPlanClick: ((String, String) -> Unit)?
) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        items.forEachIndexed { idx, item ->
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.Top,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Text(
                    text = "${startIndex + idx}.",
                    color = colors.textSecondary,
                    fontSize = 13.5.sp,
                    fontWeight = FontWeight.SemiBold,
                    fontFamily = FontFamily.Monospace,
                    modifier = Modifier.padding(top = 2.dp)
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

sealed class PlanSegment {
    abstract val id: String
    data class Text(val content: String, override val id: String) : PlanSegment()
    data class PlanButton(val title: String, val uri: String, override val id: String) : PlanSegment()
}

fun splitCodeSpans(text: String): List<Pair<String, Boolean>> {
    if (!text.contains('`')) {
        return listOf(text to false)
    }
    val segments = mutableListOf<Pair<String, Boolean>>()
    val chars = text.toCharArray()
    var i = 0
    var lastIdx = 0
    while (i < chars.size) {
        if (chars[i] == '`') {
            val tickStart = i
            while (i < chars.size && chars[i] == '`') {
                i++
            }
            val tickLen = i - tickStart
            if (tickStart > lastIdx) {
                segments.add(text.substring(lastIdx, tickStart) to false)
            }
            var closeFound = false
            var j = i
            while (j < chars.size) {
                if (chars[j] == '`') {
                    val cStart = j
                    while (j < chars.size && chars[j] == '`') {
                        j++
                    }
                    if (j - cStart == tickLen) {
                        closeFound = true
                        segments.add(text.substring(tickStart, j) to true)
                        i = j
                        lastIdx = j
                        break
                    }
                } else {
                    j++
                }
            }
            if (!closeFound) {
                i = tickStart + 1
            }
        } else {
            i++
        }
    }
    if (lastIdx < chars.size) {
        segments.add(text.substring(lastIdx) to false)
    }
    return segments
}

fun findCodeSpanRanges(text: String): List<IntRange> {
    if (!text.contains('`')) return emptyList()
    val ranges = mutableListOf<IntRange>()
    var loc = 0
    for ((content, isCode) in splitCodeSpans(text)) {
        val len = content.length
        if (isCode && len > 0) {
            ranges.add(loc until (loc + len))
        }
        loc += len
    }
    return ranges
}

private val PLAN_REGEX = Regex(
    """(?:(?<!\!)\[([^\]]+)\]\(([^)]+)\)|(?<![a-zA-Z0-9_\-./])((?:implementation_plan|walkthrough)\.md)(?![a-zA-Z0-9_\-./]))""",
    RegexOption.IGNORE_CASE
)

fun parsePlanSegments(rawText: String): List<PlanSegment> {
    if (!rawText.contains("implementation_plan", ignoreCase = true) &&
        !rawText.contains("walkthrough", ignoreCase = true)
    ) {
        return listOf(PlanSegment.Text(rawText, "text-0"))
    }

    val allMatches = PLAN_REGEX.findAll(rawText).toList()
    val codeSpanRanges = if (rawText.contains('`')) findCodeSpanRanges(rawText) else emptyList()
    val matches = allMatches.filter { match ->
        if (codeSpanRanges.isNotEmpty()) {
            val r = match.range
            if (codeSpanRanges.any { cs -> r.first < cs.last && r.last >= cs.first }) {
                return@filter false
            }
        }
        val g1 = match.groups[1]?.value?.lowercase()
        val g2 = match.groups[2]?.value?.lowercase()
        val g3 = match.groups[3]?.value
        if (g1 != null && g2 != null) {
            g1.contains("implementation_plan") || g1.contains("walkthrough") ||
            g2.contains("implementation_plan") || g2.contains("walkthrough")
        } else {
            g3 != null
        }
    }

    if (matches.isEmpty()) {
        return listOf(PlanSegment.Text(rawText, "text-0"))
    }

    val segments = mutableListOf<PlanSegment>()
    var lastEnd = 0
    var segIdx = 0

    for ((idx, match) in matches.withIndex()) {
        val range = match.range
        var prefix = ""
        if (range.first > lastEnd) {
            prefix = rawText.substring(lastEnd, range.first)
        }

        val nextIndex = range.last + 1
        val suffixLength = if (idx + 1 < matches.size) {
            matches[idx + 1].range.first - nextIndex
        } else {
            rawText.length - nextIndex
        }
        val suffixPreview = if (suffixLength > 0 && nextIndex + suffixLength <= rawText.length) {
            rawText.substring(nextIndex, nextIndex + suffixLength)
        } else {
            ""
        }

        var strippedLength = 0
        if (prefix.endsWith("**") && suffixPreview.startsWith("**")) {
            prefix = prefix.dropLast(2)
            strippedLength = 2
        } else if (prefix.endsWith("*") && suffixPreview.startsWith("*")) {
            prefix = prefix.dropLast(1)
            strippedLength = 1
        } else if (prefix.endsWith("__") && suffixPreview.startsWith("__")) {
            prefix = prefix.dropLast(2)
            strippedLength = 2
        } else if (prefix.endsWith("`") && suffixPreview.startsWith("`")) {
            prefix = prefix.dropLast(1)
            strippedLength = 1
        }

        val trimmedPrefix = prefix.trim()
        if (trimmedPrefix.isNotEmpty()) {
            segments.add(PlanSegment.Text(prefix, "seg-${segIdx++}"))
        }

        val g1 = match.groups[1]?.value
        val g2 = match.groups[2]?.value
        val g3 = match.groups[3]?.value

        val (title, uri) = when {
            g1 != null && g2 != null -> g1.trim() to g2.trim()
            g3 != null -> g3.trim() to g3.trim()
            else -> "implementation_plan.md" to "implementation_plan.md"
        }

        segments.add(PlanSegment.PlanButton(title, uri, "seg-${segIdx++}"))
        lastEnd = range.last + 1 + strippedLength
    }

    if (lastEnd < rawText.length) {
        val suffix = rawText.substring(lastEnd)
        val punctChars = setOf('。', '.', '，', ',', '！', '!', '？', '?', '；', ';', '：', ':')
        val trimmedSuffix = suffix.trim()
        val hasMeaningfulContent = trimmedSuffix.any { it !in punctChars }
        if (hasMeaningfulContent) {
            segments.add(PlanSegment.Text(suffix, "seg-${segIdx++}"))
        }
    }

    return segments
}

@Composable
fun PlanButtonView(
    title: String,
    uri: String,
    colors: AppColors,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val haptic = com.antigravity.mobile.ui.util.rememberHaptic()
    val iconName = remember(uri, title) {
        FileIconResolver.resolveIcon(uri) ?: FileIconResolver.resolveIcon(title)
    }

    Row(
        modifier = modifier
            .clip(RoundedCornerShape(8.dp))
            .background(colors.accentBlue.copy(alpha = 0.12f))
            .border(1.dp, colors.accentBlue.copy(alpha = 0.35f), RoundedCornerShape(8.dp))
            .clickable {
                haptic.light()
                onClick()
            }
            .padding(horizontal = 9.dp, vertical = 4.5.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(5.dp)
    ) {
        if (iconName != null) {
            FileIconSvgView(
                iconName = iconName,
                modifier = Modifier.size(14.dp)
            )
        } else {
            Icon(
                imageVector = Icons.Default.Description,
                contentDescription = null,
                tint = colors.accentBlue,
                modifier = Modifier.size(14.dp)
            )
        }

        Text(
            text = title,
            color = colors.accentBlue,
            fontSize = 13.sp,
            fontWeight = FontWeight.SemiBold,
            fontFamily = FontFamily.Monospace,
            maxLines = 1
        )

        Icon(
            imageVector = Icons.Default.OpenInNew,
            contentDescription = null,
            tint = colors.accentBlue.copy(alpha = 0.65f),
            modifier = Modifier.size(11.dp)
        )
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ParagraphBlockView(
    text: String,
    colors: AppColors,
    onPlanClick: ((String, String) -> Unit)?
) {
    val cleanText = replaceHtmlBreaks(text).trim()
    val segments = remember(cleanText) { parsePlanSegments(cleanText) }

    if (segments.size <= 1 && segments.firstOrNull() !is PlanSegment.PlanButton) {
        RichTextRenderer(
            text = cleanText,
            colors = colors,
            baseFontSize = 15.sp,
            onPlanClick = onPlanClick
        )
    } else {
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(4.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            segments.forEach { segment ->
                when (segment) {
                    is PlanSegment.Text -> {
                        val trimmed = segment.content.trim()
                        if (trimmed.isNotEmpty()) {
                            RichTextRenderer(
                                text = trimmed,
                                colors = colors,
                                baseFontSize = 15.sp,
                                onPlanClick = onPlanClick
                            )
                        }
                    }
                    is PlanSegment.PlanButton -> {
                        PlanButtonView(
                            title = segment.title,
                            uri = segment.uri,
                            colors = colors,
                            onClick = {
                                onPlanClick?.invoke(segment.uri, segment.title)
                            }
                        )
                    }
                }
            }
        }
    }
}

@Composable
fun FileIconSvgView(
    iconName: String,
    modifier: Modifier = Modifier
) {
    val context = LocalContext.current
    SubcomposeAsyncImage(
        model = ImageRequest.Builder(context)
            .data("file:///android_asset/file_icons/$iconName.svg")
            .crossfade(false)
            .build(),
        contentDescription = null,
        modifier = modifier,
        contentScale = ContentScale.Fit
    )
}

private data class RichTextRenderData(
    val annotatedString: AnnotatedString,
    val inlineContent: Map<String, InlineTextContent>
)

/**
 * Parses and renders inline markdown formatting:
 * - Bold: `**text**` or `__text__`
 * - Italic: `*text*` or `_text_`
 * - Inline code: `` `code` `` in Desktop Amber (#E5C07B)
 * - Links: `[text](url)` with inline file icon SVG, compact monospace font, and script filtering
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
    val renderData = remember(text, colors, baseFontSize, baseFontWeight) {
        buildRichTextRenderData(
            rawText = text,
            colors = colors,
            baseFontSize = baseFontSize,
            baseFontWeight = baseFontWeight
        )
    }

    var layoutResult by remember { mutableStateOf<TextLayoutResult?>(null) }

    Text(
        text = renderData.annotatedString,
        modifier = Modifier.pointerInput(renderData.annotatedString) {
            detectTapGestures { pos ->
                layoutResult?.let { layout ->
                    val offset = layout.getOffsetForPosition(pos)
                    renderData.annotatedString.getStringAnnotations(tag = "URL", start = offset, end = offset)
                        .firstOrNull()?.let { annotation ->
                            val url = annotation.item
                            val titleAnnotation = renderData.annotatedString.getStringAnnotations(tag = "URL_TITLE", start = offset, end = offset).firstOrNull()
                            val rawTitle = titleAnnotation?.item?.ifBlank { url.substringAfterLast('/') } ?: url.substringAfterLast('/')
                            val decodedTitle = try {
                                URLDecoder.decode(rawTitle, "UTF-8")
                            } catch (_: Exception) {
                                rawTitle
                            }

                            // Script and code files do not need to be accessible (safety filter)
                            if (FileIconResolver.isScriptFile(url) || FileIconResolver.isScriptFile(decodedTitle)) {
                                return@let
                            }

                            if (onPlanClick != null && FileIconResolver.isAccessibleDocument(url)) {
                                onPlanClick(url, decodedTitle)
                            } else if (url.startsWith("http://") || url.startsWith("https://")) {
                                try {
                                    val intent = Intent(Intent.ACTION_VIEW, Uri.parse(url)).apply {
                                        flags = Intent.FLAG_ACTIVITY_NEW_TASK
                                    }
                                    context.startActivity(intent)
                                } catch (_: Exception) {
                                    onPlanClick?.invoke(url, decodedTitle)
                                }
                            } else if (onPlanClick != null && !FileIconResolver.isScriptFile(url)) {
                                onPlanClick(url, decodedTitle)
                            }
                        }
                }
            }
        },
        style = TextStyle(
            color = colors.textPrimary,
            fontSize = baseFontSize,
            fontWeight = baseFontWeight,
            lineHeight = (baseFontSize.value * 1.45f).sp
        ),
        inlineContent = renderData.inlineContent,
        onTextLayout = { layoutResult = it }
    )
}

private val BACKTICK_BOLD_REGEX = Regex("""`+(\*{2,3})\s*([^\*`\n]+?)\s*\1`+""")

private fun unwrapBacktickBold(text: String): String {
    if (!text.contains('`') || !text.contains("**")) return text
    return BACKTICK_BOLD_REGEX.replace(text) { match ->
        val stars = match.groups[1]?.value.orEmpty()
        val inner = match.groups[2]?.value.orEmpty()
        "$stars$inner$stars"
    }
}

private val BOLD_PAIR_REGEX = Regex("""(?<!\*)(\*{2,3})((?:[^\*]|\*(?!\*))+?)\1(?!\*)""")

private fun normalizeBoldSpaces(text: String): String {
    if (!text.contains("**")) return text
    return BOLD_PAIR_REGEX.replace(text) { match ->
        val stars = match.groups[1]?.value.orEmpty()
        val inner = match.groups[2]?.value.orEmpty()
        val trimmed = inner.trim(' ', '\t')
        if (trimmed.isEmpty()) {
            match.value
        } else {
            val leading = inner.takeWhile { it == ' ' || it == '\t' }
            val trailing = inner.takeLastWhile { it == ' ' || it == '\t' }
            "$leading$stars$trimmed$stars$trailing"
        }
    }
}

private val INLINE_TOKEN_REGEX = Regex(
    """(?<!\!)\[([^\]]+)\]\(([^)]+)\)|`([^`]+)`|\*\*\*([^\n]+?)\*\*\*|___([^\n]+?)___|\*\*([^\n]+?)\*\*|__([^\n]+?)__|(?<!\*)\*([^*\n]+?)\*(?!\*)|(?<!_)_([^_\n]+?)_(?!_)|~~([^\n]+?)~~"""
)

private fun buildRichTextRenderData(
    rawText: String,
    colors: AppColors,
    baseFontSize: TextUnit,
    baseFontWeight: FontWeight
): RichTextRenderData {
    var processed = MathSymbolProcessor.process(rawText)
    processed = replaceHtmlBreaks(processed)
    processed = unwrapBacktickBold(processed)
    processed = normalizeBoldSpaces(processed)

    // Auto-link bare implementation_plan.md, walkthrough.md, task.md if not already in markdown link and outside code spans
    if (processed.contains("implementation_plan.md") || processed.contains("walkthrough.md") || processed.contains("task.md")) {
        val segments = splitCodeSpans(processed)
        val sb = StringBuilder()
        for ((content, isCode) in segments) {
            if (isCode) {
                sb.append(content)
            } else {
                var seg = content
                if (seg.contains("implementation_plan.md") && !seg.contains("[implementation_plan.md]") && !seg.contains("](implementation_plan.md)")) {
                    seg = seg.replace("implementation_plan.md", "[implementation_plan.md](implementation_plan.md)")
                }
                if (seg.contains("walkthrough.md") && !seg.contains("[walkthrough.md]") && !seg.contains("](walkthrough.md)")) {
                    seg = seg.replace("walkthrough.md", "[walkthrough.md](walkthrough.md)")
                }
                if (seg.contains("task.md") && !seg.contains("[task.md]") && !seg.contains("](task.md)")) {
                    seg = seg.replace("task.md", "[task.md](task.md)")
                }
                sb.append(seg)
            }
        }
        processed = sb.toString()
    }

    // Auto-link MEDIA: paths into clickable image links
    if (processed.contains("MEDIA:")) {
        val mediaPattern = Regex("""(?:^|\s|<br\s*/?>)MEDIA:\s*([^\s\)\<\>\"\'\`]+)""", RegexOption.IGNORE_CASE)
        processed = mediaPattern.replace(processed) { match ->
            val rawPath = match.groups[1]?.value.orEmpty()
            val clean = rawPath.trim('`', '"', '\'', '(', ')', '[', ']', '<', '>')
            val fn = clean.substringAfterLast('/')
            val linkTarget = if (clean.startsWith("file://") || clean.startsWith("http://") || clean.startsWith("https://")) clean else "file://$clean"
            "\n[点击放大查看图片 ($fn)]($linkTarget)"
        }
    }

    val inlineContentMap = mutableMapOf<String, InlineTextContent>()
    val codeColor = Color(0xFFE5C07B) // Desktop Amber
    var iconIndex = 0

    val annotatedString = buildAnnotatedString {
        fun appendStyledText(
            plain: String,
            isBold: Boolean,
            isItalic: Boolean,
            isStrike: Boolean
        ) {
            if (plain.isEmpty()) return
            val start = length
            append(plain)
            if (isBold || isItalic || isStrike) {
                addStyle(
                    SpanStyle(
                        fontWeight = if (isBold) FontWeight.Bold else baseFontWeight,
                        fontStyle = if (isItalic) FontStyle.Italic else FontStyle.Normal,
                        textDecoration = if (isStrike) TextDecoration.LineThrough else TextDecoration.None
                    ),
                    start,
                    length
                )
            }
        }

        fun appendInline(
            text: String,
            isBold: Boolean = false,
            isItalic: Boolean = false,
            isStrike: Boolean = false,
            depth: Int = 0
        ) {
            if (text.isEmpty()) return
            if (depth > 5) {
                appendStyledText(text, isBold, isItalic, isStrike)
                return
            }

            var lastIndex = 0
            val matches = INLINE_TOKEN_REGEX.findAll(text)

            for (match in matches) {
                val range = match.range
                if (range.first > lastIndex) {
                    appendStyledText(
                        plain = text.substring(lastIndex, range.first),
                        isBold = isBold,
                        isItalic = isItalic,
                        isStrike = isStrike
                    )
                }

                val linkText = match.groups[1]?.value
                val linkUrl = match.groups[2]?.value
                val inlineCode = match.groups[3]?.value
                val boldItalic1 = match.groups[4]?.value
                val boldItalic2 = match.groups[5]?.value
                val boldText1 = match.groups[6]?.value
                val boldText2 = match.groups[7]?.value
                val italicText1 = match.groups[8]?.value
                val italicText2 = match.groups[9]?.value
                val strikeText = match.groups[10]?.value

                when {
                    linkText != null && linkUrl != null -> {
                        val isInnerBold = (linkText.startsWith("**") && linkText.endsWith("**") && linkText.length >= 4) ||
                                (linkText.startsWith("__") && linkText.endsWith("__") && linkText.length >= 4)
                        val cleanTitle = linkText.trim('`', '\'', '"', '*', '_', ' ').ifEmpty {
                            linkUrl.substringAfterLast('/').ifEmpty { linkText }
                        }
                        val effectiveBold = isBold || isInnerBold
                        val isPlan = linkUrl.contains("implementation_plan") || linkUrl.contains("walkthrough")
                        val isScript = FileIconResolver.isScriptFile(linkUrl) || FileIconResolver.isScriptFile(cleanTitle)

                        // 1. Resolve SVG Icon (1:1 with iOS resolveIcon)
                        val iconName = FileIconResolver.resolveIcon(cleanTitle) ?: FileIconResolver.resolveIcon(linkUrl)
                        if (iconName != null) {
                            val inlineId = "icon_${iconName}_${iconIndex++}"
                            val iconSp = (baseFontSize.value * 0.9f).sp
                            appendInlineContent(id = inlineId, alternateText = " ")
                            append("\u2009") // Thin space (Unicode U+2009) identical to iOS
                            inlineContentMap[inlineId] = InlineTextContent(
                                placeholder = Placeholder(
                                    width = iconSp,
                                    height = iconSp,
                                    placeholderVerticalAlign = PlaceholderVerticalAlign.Center
                                )
                            ) {
                                FileIconSvgView(
                                    iconName = iconName,
                                    modifier = Modifier.fillMaxSize()
                                )
                            }
                        }

                        // 2. Append link text with scaled down font, monospaced, Apple Blue, no underline
                        val start = length
                        append(cleanTitle)
                        addStyle(
                            style = SpanStyle(
                                color = colors.accentBlue,
                                fontFamily = FontFamily.Monospace,
                                fontSize = (baseFontSize.value * 0.88f).sp,
                                fontWeight = if (effectiveBold || baseFontWeight == FontWeight.Bold) FontWeight.Bold else FontWeight.Medium,
                                fontStyle = if (isItalic) FontStyle.Italic else FontStyle.Normal,
                                textDecoration = if (isStrike) TextDecoration.LineThrough else TextDecoration.None,
                                background = if (isPlan) colors.accentBlue.copy(alpha = 0.12f) else Color.Transparent
                            ),
                            start = start,
                            end = length
                        )

                        // 3. Script files do NOT need to be accessible, so do not add URL annotation
                        if (!isScript) {
                            addStringAnnotation(
                                tag = "URL",
                                annotation = linkUrl,
                                start = start,
                                end = length
                            )
                            addStringAnnotation(
                                tag = "URL_TITLE",
                                annotation = cleanTitle,
                                start = start,
                                end = length
                            )
                        }
                    }

                    inlineCode != null -> {
                        // Check if bare inline code is a file reference that has a known icon
                        val isFileCode = (inlineCode.contains('.') || inlineCode.contains('/')) &&
                                FileIconResolver.resolveIcon(inlineCode) != null
                        val codeIcon = if (isFileCode) FileIconResolver.resolveIcon(inlineCode) else null

                        if (codeIcon != null) {
                            val inlineId = "icon_${codeIcon}_${iconIndex++}"
                            val iconSp = (baseFontSize.value * 0.9f).sp
                            appendInlineContent(id = inlineId, alternateText = " ")
                            append("\u2009")
                            inlineContentMap[inlineId] = InlineTextContent(
                                placeholder = Placeholder(
                                    width = iconSp,
                                    height = iconSp,
                                    placeholderVerticalAlign = PlaceholderVerticalAlign.Center
                                )
                            ) {
                                FileIconSvgView(
                                    iconName = codeIcon,
                                    modifier = Modifier.fillMaxSize()
                                )
                            }
                            val start = length
                            append(inlineCode)
                            addStyle(
                                style = SpanStyle(
                                    color = colors.accentBlue,
                                    fontFamily = FontFamily.Monospace,
                                    fontSize = (baseFontSize.value * 0.88f).sp,
                                    fontWeight = if (isBold || baseFontWeight == FontWeight.Bold) FontWeight.Bold else FontWeight.Medium,
                                    fontStyle = if (isItalic) FontStyle.Italic else FontStyle.Normal,
                                    textDecoration = if (isStrike) TextDecoration.LineThrough else TextDecoration.None
                                ),
                                start = start,
                                end = length
                            )
                        } else {
                            // Standard inline code symbol (e.g. rawTrajectorySignature, readBodyToPool) in Amber
                            val start = length
                            append(inlineCode)
                            addStyle(
                                style = SpanStyle(
                                    color = codeColor,
                                    fontFamily = FontFamily.Monospace,
                                    fontWeight = if (isBold || baseFontWeight == FontWeight.Bold) FontWeight.Bold else FontWeight.Medium,
                                    fontSize = (baseFontSize.value * 0.9f).sp,
                                    fontStyle = if (isItalic) FontStyle.Italic else FontStyle.Normal,
                                    textDecoration = if (isStrike) TextDecoration.LineThrough else TextDecoration.None,
                                    background = colors.surfaceVariant.copy(alpha = 0.45f)
                                ),
                                start = start,
                                end = length
                            )
                        }
                    }

                    (boldItalic1 != null || boldItalic2 != null) -> {
                        val content = boldItalic1 ?: boldItalic2 ?: ""
                        appendInline(
                            text = content,
                            isBold = true,
                            isItalic = true,
                            isStrike = isStrike,
                            depth = depth + 1
                        )
                    }

                    (boldText1 != null || boldText2 != null) -> {
                        val content = boldText1 ?: boldText2 ?: ""
                        appendInline(
                            text = content,
                            isBold = true,
                            isItalic = isItalic,
                            isStrike = isStrike,
                            depth = depth + 1
                        )
                    }

                    (italicText1 != null || italicText2 != null) -> {
                        val content = italicText1 ?: italicText2 ?: ""
                        appendInline(
                            text = content,
                            isBold = isBold,
                            isItalic = true,
                            isStrike = isStrike,
                            depth = depth + 1
                        )
                    }

                    strikeText != null -> {
                        appendInline(
                            text = strikeText,
                            isBold = isBold,
                            isItalic = isItalic,
                            isStrike = true,
                            depth = depth + 1
                        )
                    }

                    else -> {
                        appendStyledText(
                            plain = match.value,
                            isBold = isBold,
                            isItalic = isItalic,
                            isStrike = isStrike
                        )
                    }
                }

                lastIndex = range.last + 1
            }

            if (lastIndex < text.length) {
                appendStyledText(
                    plain = text.substring(lastIndex),
                    isBold = isBold,
                    isItalic = isItalic,
                    isStrike = isStrike
                )
            }
        }

        appendInline(processed)
    }

    return RichTextRenderData(annotatedString, inlineContentMap)
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
