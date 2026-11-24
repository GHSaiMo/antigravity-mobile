package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.DailyTokenRecord
import com.antigravity.mobile.data.model.ModelTokenBreakdown
import com.antigravity.mobile.data.model.TokenHistoryResponse
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.theme.AppColors
import java.text.SimpleDateFormat
import java.util.*

@Composable
fun TokenHistoryTabContent(
    tokenHistory: TokenHistoryResponse?,
    isLoading: Boolean,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
    selectedDate: String? = null,
    onSelectDate: ((String) -> Unit)? = null,
    selectedChart: TokenHistoryChartType = TokenHistoryChartType.DAILY_BARS,
    onSelectChart: ((TokenHistoryChartType) -> Unit)? = null
) {
    val colors = AntigravityTheme.colors
    if (tokenHistory == null && isLoading) {
        Box(
            modifier = modifier
                .fillMaxWidth()
                .padding(vertical = 48.dp),
            contentAlignment = Alignment.Center
        ) {
            Column(
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                CircularProgressIndicator(
                    modifier = Modifier.size(24.dp),
                    strokeWidth = 2.5.dp,
                    color = colors.accentBlue
                )
                Text(
                    text = "正在统计本地 Transcript Token 历史...".loc,
                    fontSize = 13.sp,
                    color = colors.textSecondary
                )
            }
        }
        return
    }

    if (tokenHistory == null) {
        Box(
            modifier = modifier
                .fillMaxWidth()
                .padding(vertical = 40.dp),
            contentAlignment = Alignment.Center
        ) {
            Column(
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Text(
                    text = "暂无 Token 历史数据".loc,
                    fontSize = 14.sp,
                    color = colors.textSecondary,
                    fontWeight = FontWeight.Medium
                )
                Text(
                    text = "点击加载".loc,
                    fontSize = 13.sp,
                    color = colors.accentBlue,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier
                        .clip(RoundedCornerShape(8.dp))
                        .background(colors.accentBlue.copy(alpha = 0.12f))
                        .clickable(onClick = onRetry)
                        .padding(horizontal = 14.dp, vertical = 6.dp)
                )
            }
        }
        return
    }

    val summary = tokenHistory.summary
    val matrix = tokenHistory.dailyMatrix
    var internalSelectedDate by remember(matrix) {
        mutableStateOf(
            matrix.lastOrNull { it.total > 0 }?.date ?: matrix.lastOrNull()?.date
        )
    }
    val effectiveSelectedDate = selectedDate ?: internalSelectedDate
    val handleSelectDate: (String) -> Unit = { date ->
        if (onSelectDate != null) {
            onSelectDate(date)
        } else {
            internalSelectedDate = date
        }
    }
    val selectedRecord = remember(matrix, effectiveSelectedDate) {
        matrix.firstOrNull { it.date == effectiveSelectedDate }
    }
    var showAllModelsSheet by remember { mutableStateOf(false) }
    val allModels = remember(tokenHistory) { tokenHistory.resolvedModelSummary() }

    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        // KPI 4-grid
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                TokenKpiCard(
                    label = "今日消耗".loc,
                    value = formatTokensCompact(summary.todayTokens),
                    sub = "${summary.activeDays} 活跃日".loc,
                    modifier = Modifier.weight(1f)
                )
                TokenKpiCard(
                    label = "近 7 日".loc,
                    value = formatTokensCompact(summary.last7dTokens),
                    sub = "${summary.totalSteps} 轮采样".loc,
                    modifier = Modifier.weight(1f)
                )
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                TokenKpiCard(
                    label = "累计总 Token".loc,
                    value = formatTokensCompact(summary.totalTokens),
                    sub = "IDE ${formatTokensCompact(summary.desktopTokens)} · CLI ${formatTokensCompact(summary.cliTokens)}",
                    modifier = Modifier.weight(1f)
                )
                TokenKpiCard(
                    label = "API 折算".loc,
                    value = String.format(Locale.US, "$%.2f", summary.estimatedCostUsd),
                    sub = String.format(Locale.US, "≈ ¥%.1f", summary.estimatedCostCny),
                    modifier = Modifier.weight(1f)
                )
            }
        }

        // Source ratio pill
        val totalSources = (summary.desktopTokens + summary.cliTokens).coerceAtLeast(1L)
        val desktopPct = ((summary.desktopTokens * 100.0) / totalSources).toInt()
        val cliPct = if (summary.cliTokens > 0) 100 - desktopPct else 0
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(10.dp))
                .background(colors.surface)
                .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
                .padding(horizontal = 12.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "桌面端 Antigravity %1\$d%% · CLI %2\$d%%".loc(desktopPct, cliPct),
                fontSize = 12.sp,
                color = colors.textSecondary,
                fontWeight = FontWeight.Medium
            )
            Text(
                text = selectedChart.statusBarLabel(),
                fontSize = 11.sp,
                color = colors.textMuted
            )
        }

        // Swipeable 3-chart carousel (14d bars / model breakdown / 20w heatmap)
        TokenHistoryChartCarouselCard(
            dailyMatrix = matrix,
            selectedDate = effectiveSelectedDate,
            selectedChart = selectedChart,
            onSelectDate = handleSelectDate,
            onUserSelectChart = { chart -> onSelectChart?.invoke(chart) },
            onOpenAllModels = if (allModels.isNotEmpty()) { { showAllModelsSheet = true } } else null
        )

        // Inline inspector card
        selectedRecord?.let { rec ->
            TokenInspectorCard(record = rec)
        }

        if (tokenHistory.updatedAt > 0) {
            val timeFmt = remember(TimeZone.getDefault().id) {
                SimpleDateFormat("HH:mm:ss", Locale.US).apply {
                    timeZone = TimeZone.getDefault()
                }
            }
            Text(
                text = "本地历史扫描于 %s".loc(timeFmt.format(Date(tokenHistory.updatedAtEpochMs))),
                fontSize = 11.sp,
                color = colors.textMuted,
                modifier = Modifier
                    .fillMaxWidth()
                    .wrapContentWidth(Alignment.CenterHorizontally)
                    .padding(bottom = 12.dp)
            )
        }
    }

    if (showAllModelsSheet && allModels.isNotEmpty()) {
        ModelUsageBreakdownSheet(
            models = allModels,
            grandTotal = allModels.sumOf { it.total }.coerceAtLeast(1L),
            onDismiss = { showAllModelsSheet = false }
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ModelUsageBreakdownSheet(
    models: List<ModelTokenBreakdown>,
    grandTotal: Long,
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = Color.Transparent,
        shape = RectangleShape,
        tonalElevation = 0.dp,
        dragHandle = null,
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) },
        modifier = Modifier
            .fillMaxWidth()
            .fillMaxHeight()
    ) {
        TallSheetBody(containerColor = colors.background, onDismiss = onDismiss) {
            Column(
                modifier = Modifier.fillMaxSize()
            ) {
                // Floating grab handle hinting pull-down dismissal (matching AccountQuotaSheet / other tall sheets)
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 8.dp, bottom = 10.dp),
                    contentAlignment = Alignment.Center
                ) {
                    Box(
                        modifier = Modifier
                            .size(width = 38.dp, height = 5.dp)
                            .clip(CircleShape)
                            .background(colors.textMuted.copy(alpha = 0.35f))
                    )
                }

                Text(
                    text = "模型用量明细".loc,
                    fontSize = 17.sp,
                    fontWeight = FontWeight.Bold,
                    color = colors.textPrimary,
                    textAlign = TextAlign.Center,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp)
                        .padding(bottom = 12.dp)
                )

                HorizontalDivider(thickness = 0.5.dp, color = colors.border)

                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                        .verticalScroll(rememberScrollState())
                        .padding(horizontal = 16.dp)
                        .padding(top = 12.dp, bottom = 24.dp)
                        .navigationBarsPadding(),
                    verticalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    models.forEachIndexed { idx, item ->
                        val pct = item.total * 100.0 / grandTotal.toDouble()
                        val dotColor = when (idx % 4) {
                            0 -> colors.accentBlue
                            1 -> Color(0xFF22C55E)
                            2 -> Color(0xFFF59E0B)
                            else -> Color(0xFFA855F7)
                        }
                        val hasDetail = item.input > 0L || item.output > 0L || item.cacheRead > 0L || item.thinking > 0L
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(12.dp))
                                .background(colors.surface)
                                .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                                .padding(12.dp),
                            verticalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                Box(
                                    modifier = Modifier
                                        .size(7.dp)
                                        .clip(CircleShape)
                                        .background(dotColor)
                                )
                                Text(
                                    text = item.model,
                                    fontSize = 12.5.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.textPrimary,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                    modifier = Modifier.weight(1f)
                                )
                                Text(
                                    text = formatTokensCompact(item.total),
                                    fontSize = 12.sp,
                                    fontFamily = FontFamily.Monospace,
                                    fontWeight = FontWeight.Bold,
                                    color = colors.accentBlue
                                )
                                Text(
                                    text = String.format(Locale.US, "%.1f%%", pct),
                                    fontSize = 11.sp,
                                    fontFamily = FontFamily.Monospace,
                                    color = colors.textSecondary
                                )
                            }

                            if (hasDetail) {
                                HorizontalDivider(thickness = 0.5.dp, color = colors.border)
                                Row(
                                    modifier = Modifier.fillMaxWidth(),
                                    horizontalArrangement = Arrangement.SpaceBetween
                                ) {
                                    InspectorMetric("Input", formatTokensCompact(item.input))
                                    InspectorMetric("Cache Hit", formatTokensCompact(item.cacheRead))
                                    InspectorMetric("Output", formatTokensCompact(item.output))
                                    InspectorMetric("Thinking", formatTokensCompact(item.thinking))
                                }
                                if (item.steps > 0 || item.costUsd > 0.0) {
                                    Row(
                                        modifier = Modifier.fillMaxWidth(),
                                        horizontalArrangement = Arrangement.SpaceBetween
                                    ) {
                                        Text(
                                            text = "${item.steps} 轮采样".loc,
                                            fontSize = 10.5.sp,
                                            color = colors.textMuted
                                        )
                                        Text(
                                            text = String.format(Locale.US, "$%.3f", item.costUsd),
                                            fontSize = 10.5.sp,
                                            fontFamily = FontFamily.Monospace,
                                            color = colors.textSecondary
                                        )
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun TokenKpiCard(
    label: String,
    value: String,
    sub: String,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    Column(
        modifier = modifier
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface)
            .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            .padding(horizontal = 12.dp, vertical = 10.dp),
        verticalArrangement = Arrangement.spacedBy(3.dp)
    ) {
        Text(
            text = label,
            fontSize = 11.sp,
            color = colors.textSecondary,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
        Text(
            text = value,
            fontSize = 18.sp,
            fontWeight = FontWeight.Bold,
            fontFamily = FontFamily.Monospace,
            color = colors.textPrimary,
            maxLines = 1
        )
        Text(
            text = sub,
            fontSize = 11.sp,
            color = colors.textMuted,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

@Composable
private fun TokenInspectorCard(record: DailyTokenRecord) {
    val colors = AntigravityTheme.colors
    var expandedModel by remember(record.date) { mutableStateOf<String?>(null) }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface)
            .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "${record.date} 用量明细".loc,
                fontSize = 13.sp,
                fontWeight = FontWeight.SemiBold,
                color = colors.textPrimary
            )
            Text(
                text = "${formatTokensCompact(record.total)} · ${String.format(Locale.US, "$%.3f", record.costUsd)}",
                fontSize = 12.sp,
                fontFamily = FontFamily.Monospace,
                fontWeight = FontWeight.SemiBold,
                color = colors.accentBlue
            )
        }

        HorizontalDivider(thickness = 0.5.dp, color = colors.border)

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            InspectorMetric("Input", formatTokensCompact(record.input))
            InspectorMetric("Cache Hit", formatTokensCompact(record.cacheRead))
            InspectorMetric("Output", formatTokensCompact(record.output))
            InspectorMetric("Thinking", formatTokensCompact(record.thinking))
        }

        if (record.models.isNotEmpty()) {
            val allDayModels = remember(record.models) {
                record.models.entries.sortedWith(
                    compareByDescending<Map.Entry<String, Long>> { it.value }.thenBy { it.key }
                )
            }
            HorizontalDivider(thickness = 0.5.dp, color = colors.border)
            Column(verticalArrangement = Arrangement.spacedBy(5.dp)) {
                allDayModels.forEach { (model, tokens) ->
                    val detail = record.modelDetails[model]
                    val canExpand = detail != null && (detail.input > 0L || detail.output > 0L || detail.cacheRead > 0L || detail.thinking > 0L)
                    val isExpanded = canExpand && expandedModel == model
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .then(
                                if (canExpand) {
                                    Modifier.clickable {
                                        expandedModel = if (isExpanded) null else model
                                    }
                                } else {
                                    Modifier
                                }
                            ),
                        verticalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text(
                                text = model,
                                fontSize = 11.sp,
                                color = if (isExpanded) colors.textPrimary else colors.textSecondary,
                                fontWeight = if (isExpanded) FontWeight.Medium else FontWeight.Normal,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                modifier = Modifier.weight(1f)
                            )
                            Text(
                                text = formatTokensCompact(tokens),
                                fontSize = 11.sp,
                                fontFamily = FontFamily.Monospace,
                                color = colors.textPrimary
                            )
                        }
                        if (isExpanded) {
                            val d = detail!!
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clip(RoundedCornerShape(6.dp))
                                    .background(colors.background.copy(alpha = 0.65f))
                                    .padding(horizontal = 8.dp, vertical = 5.dp),
                                horizontalArrangement = Arrangement.SpaceBetween
                            ) {
                                Text(
                                    text = "In ${formatTokensCompact(d.input)}",
                                    fontSize = 10.sp,
                                    fontFamily = FontFamily.Monospace,
                                    color = colors.textSecondary
                                )
                                Text(
                                    text = "Cache ${formatTokensCompact(d.cacheRead)}",
                                    fontSize = 10.sp,
                                    fontFamily = FontFamily.Monospace,
                                    color = colors.textSecondary
                                )
                                Text(
                                    text = "Out ${formatTokensCompact(d.output)}",
                                    fontSize = 10.sp,
                                    fontFamily = FontFamily.Monospace,
                                    color = colors.textSecondary
                                )
                                if (d.thinking > 0L) {
                                    Text(
                                        text = "Think ${formatTokensCompact(d.thinking)}",
                                        fontSize = 10.sp,
                                        fontFamily = FontFamily.Monospace,
                                        color = colors.textSecondary
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun InspectorMetric(label: String, value: String) {
    val colors = AntigravityTheme.colors
    Column(horizontalAlignment = Alignment.Start) {
        Text(text = label, fontSize = 10.sp, color = colors.textMuted)
        Text(
            text = value,
            fontSize = 12.sp,
            fontFamily = FontFamily.Monospace,
            fontWeight = FontWeight.SemiBold,
            color = colors.textPrimary
        )
    }
}

fun formatTokensCompact(tokens: Long): String {
    return when {
        tokens >= 1_000_000_000L -> String.format(Locale.US, "%.2fB", tokens / 1_000_000_000.0)
        tokens >= 1_000_000L -> String.format(Locale.US, "%.2fM", tokens / 1_000_000.0)
        tokens >= 1_000L -> String.format(Locale.US, "%.1fK", tokens / 1_000.0)
        else -> tokens.toString()
    }
}
