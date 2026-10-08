package com.antigravity.mobile.ui.components

import android.widget.Toast
import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.DisableSelection
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Collections
import androidx.compose.material.icons.filled.ContentCopy
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
import androidx.compose.ui.text.font.FontWeight
import com.antigravity.mobile.data.service.FileIconResolver
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.ui.theme.AppColors

/** 为 true 时表示正在为分享长图离屏渲染：代码块 / 表格不再横向滚动，而是铺满宽度自动换行。 */
internal val LocalShareExport = compositionLocalOf { false }

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

            if (!LocalShareExport.current) {
            Row(
                modifier = Modifier
                    .clip(RoundedCornerShape(6.dp))
                    .clickable {
                        haptic.medium()
                        clipboardManager.setText(AnnotatedString(code.trimEnd('\r', '\n')))
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
        }

        HorizontalDivider(color = colors.border.copy(alpha = 0.4f), thickness = 0.5.dp)

        // Horizontally Scrollable Monospaced Code
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .then(if (LocalShareExport.current) Modifier else Modifier.horizontalScroll(rememberScrollState()))
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
    val isExport = LocalShareExport.current

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
            .then(if (isExport) Modifier else Modifier.horizontalScroll(rememberScrollState()))
    ) {
        Column(modifier = if (isExport) Modifier.fillMaxWidth() else Modifier) {
            // Header Row
            if (headers.isNotEmpty()) {
                Row(
                    modifier = Modifier
                        .then(if (isExport) Modifier.fillMaxWidth() else Modifier)
                        .background(colors.surfaceVariant.copy(alpha = 0.85f))
                        .height(IntrinsicSize.Min)
                ) {
                    for (colIdx in 0 until columnCount) {
                        val headerText = headers.getOrNull(colIdx) ?: ""
                        val alignment = alignments.getOrNull(colIdx) ?: TableColumnAlignment.LEADING
                        val width = columnWidths.getOrElse(colIdx) { 110.dp }

                        Box(
                            modifier = (if (isExport) Modifier.weight(width.value) else Modifier.width(width))
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
                        .then(if (isExport) Modifier.fillMaxWidth() else Modifier)
                        .background(rowBg)
                        .height(IntrinsicSize.Min)
                ) {
                    for (colIdx in 0 until columnCount) {
                        val cellText = row.getOrNull(colIdx) ?: ""
                        val alignment = alignments.getOrNull(colIdx) ?: TableColumnAlignment.LEADING
                        val width = columnWidths.getOrElse(colIdx) { 110.dp }

                        Box(
                            modifier = (if (isExport) Modifier.weight(width.value) else Modifier.width(width))
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
                DisableSelection {
                    Text(
                        text = "•",
                        color = colors.textSecondary,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.Bold,
                        modifier = Modifier.padding(top = 2.dp)
                    )
                }
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
                DisableSelection {
                    Text(
                        text = "${startIndex + idx}.",
                        color = colors.textSecondary,
                        fontSize = 13.5.sp,
                        fontWeight = FontWeight.SemiBold,
                        fontFamily = FontFamily.Monospace,
                        modifier = Modifier.padding(top = 2.dp)
                    )
                }
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
internal fun CarouselBlockView(
    slides: List<MarkdownCarouselSlide>,
    colors: AppColors,
    modifier: Modifier = Modifier,
    onPlanClick: ((String, String) -> Unit)? = null,
    urlResolver: ((String) -> String)? = null,
    onImageClick: ((String) -> Unit)? = null
) {
    if (slides.isEmpty()) return

    var currentIndex by remember { mutableIntStateOf(0) }
    val safeIndex = currentIndex.coerceIn(0, slides.size - 1)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface)
            .border(1.dp, colors.border.copy(alpha = 0.5f), RoundedCornerShape(12.dp))
    ) {
        // Header Bar
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .background(colors.surfaceVariant.copy(alpha = 0.6f))
                .padding(horizontal = 12.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.Collections,
                    contentDescription = null,
                    tint = colors.accentBlue,
                    modifier = Modifier.size(16.dp)
                )
                Text(
                    text = "照片轮播",
                    color = colors.textPrimary,
                    fontSize = 12.5.sp,
                    fontWeight = FontWeight.SemiBold
                )
                Box(
                    modifier = Modifier
                        .clip(RoundedCornerShape(10.dp))
                        .background(colors.surfaceVariant)
                        .padding(horizontal = 7.dp, vertical = 2.dp)
                ) {
                    Text(
                        text = "${safeIndex + 1} / ${slides.size}",
                        color = colors.textSecondary,
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Bold,
                        fontFamily = FontFamily.Monospace
                    )
                }
            }

            Spacer(modifier = Modifier.weight(1f))

            // Navigation Buttons (上一张 / 下一张)
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                // 上一张
                val prevEnabled = safeIndex > 0
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(8.dp))
                        .border(
                            0.8.dp,
                            colors.border.copy(alpha = if (prevEnabled) 0.5f else 0.2f),
                            RoundedCornerShape(8.dp)
                        )
                        .background(if (prevEnabled) colors.surfaceVariant.copy(alpha = 0.8f) else Color.Transparent)
                        .clickable(enabled = prevEnabled) {
                            if (safeIndex > 0) currentIndex--
                        }
                        .padding(horizontal = 8.dp, vertical = 5.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(3.dp)
                ) {
                    Icon(
                        imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                        contentDescription = "上一张",
                        tint = if (prevEnabled) colors.textPrimary else colors.textSecondary.copy(alpha = 0.35f),
                        modifier = Modifier.size(12.dp)
                    )
                    Text(
                        text = "上一张",
                        fontSize = 11.5.sp,
                        fontWeight = FontWeight.Medium,
                        color = if (prevEnabled) colors.textPrimary else colors.textSecondary.copy(alpha = 0.35f)
                    )
                }

                // 下一张
                val nextEnabled = safeIndex < slides.size - 1
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(8.dp))
                        .border(
                            0.8.dp,
                            colors.border.copy(alpha = if (nextEnabled) 0.5f else 0.2f),
                            RoundedCornerShape(8.dp)
                        )
                        .background(if (nextEnabled) colors.surfaceVariant.copy(alpha = 0.8f) else Color.Transparent)
                        .clickable(enabled = nextEnabled) {
                            if (safeIndex < slides.size - 1) currentIndex++
                        }
                        .padding(horizontal = 8.dp, vertical = 5.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(3.dp)
                ) {
                    Text(
                        text = "下一张",
                        fontSize = 11.5.sp,
                        fontWeight = FontWeight.Medium,
                        color = if (nextEnabled) colors.textPrimary else colors.textSecondary.copy(alpha = 0.35f)
                    )
                    Icon(
                        imageVector = Icons.AutoMirrored.Filled.ArrowForward,
                        contentDescription = "下一张",
                        tint = if (nextEnabled) colors.textPrimary else colors.textSecondary.copy(alpha = 0.35f),
                        modifier = Modifier.size(12.dp)
                    )
                }
            }
        }

        HorizontalDivider(
            color = colors.border.copy(alpha = 0.4f),
            thickness = 0.8.dp
        )

        // Slide Content Area with swipe support
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp, vertical = 10.dp)
                .pointerInput(safeIndex, slides.size) {
                    var totalDrag = 0f
                    detectHorizontalDragGestures(
                        onDragStart = { totalDrag = 0f },
                        onDragEnd = {
                            if (totalDrag < -60f && safeIndex < slides.size - 1) {
                                currentIndex++
                            } else if (totalDrag > 60f && safeIndex > 0) {
                                currentIndex--
                            }
                        },
                        onHorizontalDrag = { _, dragAmount ->
                            totalDrag += dragAmount
                        }
                    )
                }
        ) {
            AnimatedContent(
                targetState = safeIndex,
                transitionSpec = {
                    if (targetState > initialState) {
                        (slideInHorizontally { width -> width / 3 } + fadeIn()).togetherWith(
                            slideOutHorizontally { width -> -width / 3 } + fadeOut()
                        )
                    } else {
                        (slideInHorizontally { width -> -width / 3 } + fadeIn()).togetherWith(
                            slideOutHorizontally { width -> width / 3 } + fadeOut()
                        )
                    }
                },
                label = "carousel_slide"
            ) { targetIdx ->
                val slide = slides[targetIdx]
                MarkdownContentView(
                    content = slide.content,
                    onPlanClick = onPlanClick,
                    urlResolver = urlResolver,
                    onImageClick = onImageClick
                )
            }
        }

        // Bottom pagination dots
        if (slides.size > 1) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 2.dp, bottom = 10.dp),
                horizontalArrangement = Arrangement.Center,
                verticalAlignment = Alignment.CenterVertically
            ) {
                slides.forEachIndexed { idx, _ ->
                    Box(
                        modifier = Modifier
                            .padding(horizontal = 3.dp)
                            .size(width = if (idx == safeIndex) 18.dp else 6.dp, height = 6.dp)
                            .clip(CircleShape)
                            .background(if (idx == safeIndex) colors.accentBlue else colors.textSecondary.copy(alpha = 0.25f))
                            .clickable { currentIndex = idx }
                    )
                }
            }
        }
    }
}

