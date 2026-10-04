package com.antigravity.mobile.ui.components

import android.widget.Toast
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.ContentCopy
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
import androidx.compose.ui.text.font.FontWeight
import com.antigravity.mobile.data.service.FileIconResolver
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.ui.theme.AppColors

@Composable
internal fun HeadingBlockView(
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
internal fun CodeBlockView(
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
internal fun TableBlockView(
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
internal fun BulletListBlockView(
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
internal fun OrderedListBlockView(
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
