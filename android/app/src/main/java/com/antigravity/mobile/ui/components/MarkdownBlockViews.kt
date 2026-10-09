package com.antigravity.mobile.ui.components

import androidx.compose.ui.layout.Layout
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.draw.drawBehind
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
    val hasHeader = headers.isNotEmpty()
    val lastRow = rows.size - 1

    @Composable
    fun Cell(text: String, colIdx: Int, isHeader: Boolean, rowIdx: Int) {
        val alignment = alignments.getOrNull(colIdx) ?: TableColumnAlignment.LEADING
        val bg = when {
            isHeader -> colors.surfaceVariant.copy(alpha = 0.85f)
            rowIdx % 2 == 0 -> Color.Transparent
            else -> colors.surfaceVariant.copy(alpha = 0.35f)
        }
        val vLine = colors.border.copy(alpha = if (isHeader) 0.4f else 0.3f)
        val hLine = if (isHeader) colors.border.copy(alpha = 0.5f) else colors.border.copy(alpha = 0.25f)
        val drawBottom = isHeader || rowIdx < lastRow
        val isLastCol = colIdx == columnCount - 1
        Box(
            modifier = Modifier
                .background(bg)
                .drawBehind {
                    if (!isLastCol) {
                        drawLine(vLine, Offset(size.width, 0f), Offset(size.width, size.height), strokeWidth = 0.8.dp.toPx())
                    }
                    if (drawBottom) {
                        drawLine(hLine, Offset(0f, size.height), Offset(size.width, size.height), strokeWidth = 0.6.dp.toPx())
                    }
                }
                .padding(horizontal = 12.dp, vertical = 9.dp),
            contentAlignment = when (alignment) {
                TableColumnAlignment.LEADING -> Alignment.CenterStart
                TableColumnAlignment.CENTER -> Alignment.Center
                TableColumnAlignment.TRAILING -> Alignment.CenterEnd
            }
        ) {
            RichTextRenderer(
                text = text,
                colors = colors,
                baseFontSize = 13.sp,
                baseFontWeight = if (isHeader) FontWeight.Bold else FontWeight.Normal,
                onPlanClick = onPlanClick
            )
        }
    }

    // 容器宽度来自外层约束（横向滚动内部约束是无限的），列宽按内容自适应，占用面积尽量小
    BoxWithConstraints(modifier = Modifier.fillMaxWidth()) {
        val limitPx = with(LocalDensity.current) { maxWidth.roundToPx() }
        Box(
            modifier = Modifier
                .clip(RoundedCornerShape(10.dp))
                .border(0.8.dp, colors.border, RoundedCornerShape(10.dp))
                .then(if (isExport) Modifier else Modifier.horizontalScroll(rememberScrollState()))
        ) {
            AutoTableLayout(columnCount = columnCount, limitPx = limitPx) {
                if (hasHeader) {
                    for (c in 0 until columnCount) Cell(headers.getOrNull(c) ?: "", c, true, -1)
                }
                rows.forEachIndexed { r, row ->
                    for (c in 0 until columnCount) Cell(row.getOrNull(c) ?: "", c, false, r)
                }
            }
        }
    }
}

/**
 * 表格自适应列宽：容器装得下时每列取内容的单行宽度；装不下时在「最窄宽度」与「单行宽度」之间按比例分配；
 * 即使全取最窄宽度仍超出，则保持最窄宽度并交给外层横向滚动。对齐 iOS 的 AutoTableLayout。
 */
@Composable
private fun AutoTableLayout(columnCount: Int, limitPx: Int, content: @Composable () -> Unit) {
    Layout(content = content) { measurables, constraints ->
        val minW = IntArray(columnCount)
        val maxW = IntArray(columnCount)
        measurables.forEachIndexed { i, m ->
            val c = i % columnCount
            minW[c] = maxOf(minW[c], m.minIntrinsicWidth(0))
            maxW[c] = maxOf(maxW[c], m.maxIntrinsicWidth(Constraints.Infinity))
        }
        for (c in 0 until columnCount) maxW[c] = maxOf(maxW[c], minW[c])
        val sumMin = minW.sum()
        val sumMax = maxW.sum()
        val limit = if (constraints.hasBoundedWidth) minOf(limitPx, constraints.maxWidth) else limitPx
        val widths = when {
            limit <= 0 || sumMax <= limit -> maxW
            sumMin >= limit -> minW
            else -> {
                val t = (limit - sumMin).toFloat() / (sumMax - sumMin)
                IntArray(columnCount) { minW[it] + ((maxW[it] - minW[it]) * t).toInt() }
            }
        }
        val rowCount = (measurables.size + columnCount - 1) / columnCount
        val rowHeights = IntArray(rowCount)
        for (r in 0 until rowCount) {
            for (c in 0 until columnCount) {
                val i = r * columnCount + c
                if (i < measurables.size) {
                    rowHeights[r] = maxOf(rowHeights[r], measurables[i].minIntrinsicHeight(widths[c]))
                }
            }
        }
        val placeables = measurables.mapIndexed { i, m ->
            val c = i % columnCount
            m.measure(Constraints.fixed(widths[c], rowHeights[i / columnCount]))
        }
        layout(widths.sum(), rowHeights.sum()) {
            var y = 0
            for (r in 0 until rowCount) {
                var x = 0
                for (c in 0 until columnCount) {
                    val i = r * columnCount + c
                    if (i < placeables.size) placeables[i].placeRelative(x, y)
                    x += widths[c]
                }
                y += rowHeights[r]
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

