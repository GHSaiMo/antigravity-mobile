package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.DailyTokenRecord
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
    modifier: Modifier = Modifier
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
                    text = "正在统计本地 Transcript Token 历史...",
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
                    text = "暂无 Token 历史数据",
                    fontSize = 14.sp,
                    color = colors.textSecondary,
                    fontWeight = FontWeight.Medium
                )
                Text(
                    text = "点击加载",
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
    var selectedDate by remember(matrix) {
        mutableStateOf(
            matrix.lastOrNull { it.total > 0 }?.date ?: matrix.lastOrNull()?.date
        )
    }
    val selectedRecord = remember(matrix, selectedDate) {
        matrix.firstOrNull { it.date == selectedDate }
    }

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
                    label = "今日消耗",
                    value = formatTokensCompact(summary.todayTokens),
                    sub = "${summary.activeDays} 活跃日",
                    modifier = Modifier.weight(1f)
                )
                TokenKpiCard(
                    label = "近 7 日",
                    value = formatTokensCompact(summary.last7dTokens),
                    sub = "${summary.totalSteps} 轮采样",
                    modifier = Modifier.weight(1f)
                )
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                TokenKpiCard(
                    label = "累计总 Token",
                    value = formatTokensCompact(summary.totalTokens),
                    sub = "IDE ${formatTokensCompact(summary.desktopTokens)} · CLI ${formatTokensCompact(summary.cliTokens)}",
                    modifier = Modifier.weight(1f)
                )
                TokenKpiCard(
                    label = "API 折算",
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
                text = "桌面端 Antigravity $desktopPct% · CLI $cliPct%",
                fontSize = 12.sp,
                color = colors.textSecondary,
                fontWeight = FontWeight.Medium
            )
            Text(
                text = "近 52 周热力图",
                fontSize = 11.sp,
                color = colors.textMuted
            )
        }

        // 52-week horizontally scrollable heatmap
        TokenHeatmapSection(
            dailyMatrix = matrix,
            selectedDate = selectedDate,
            onSelectDate = { selectedDate = it }
        )

        // Inline inspector card
        selectedRecord?.let { rec ->
            TokenInspectorCard(record = rec)
        }

        if (tokenHistory.updatedAt > 0) {
            val timeFmt = remember { SimpleDateFormat("HH:mm:ss", Locale.getDefault()) }
            Text(
                text = "本地历史扫描于 ${timeFmt.format(Date(tokenHistory.updatedAt * 1000))}",
                fontSize = 11.sp,
                color = colors.textMuted,
                modifier = Modifier
                    .fillMaxWidth()
                    .wrapContentWidth(Alignment.CenterHorizontally)
                    .padding(bottom = 12.dp)
            )
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
private fun TokenHeatmapSection(
    dailyMatrix: List<DailyTokenRecord>,
    selectedDate: String?,
    onSelectDate: (String) -> Unit
) {
    val colors = AntigravityTheme.colors
    val maxTotal = remember(dailyMatrix) {
        dailyMatrix.maxOfOrNull { it.total }?.coerceAtLeast(1L) ?: 1L
    }

    // Group into columns of 7 days (Sunday..Saturday alignment)
    val weeks = remember(dailyMatrix) {
        if (dailyMatrix.isEmpty()) emptyList()
        else {
            val sdf = SimpleDateFormat("yyyy-MM-dd", Locale.US)
            val cal = Calendar.getInstance()
            val first = runCatching { sdf.parse(dailyMatrix.first().date) }.getOrNull()
            val padStart = if (first != null) {
                cal.time = first
                (cal.get(Calendar.DAY_OF_WEEK) - Calendar.SUNDAY).coerceIn(0, 6)
            } else 0
            val padded = MutableList<DailyTokenRecord?>(padStart) { null }
            padded.addAll(dailyMatrix)
            padded.chunked(7)
        }
    }

    val scrollState = rememberScrollState()
    LaunchedEffect(weeks.size) {
        if (weeks.isNotEmpty()) {
            scrollState.scrollTo(scrollState.maxValue)
        }
    }

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
                text = "每日活跃密度",
                fontSize = 12.sp,
                fontWeight = FontWeight.SemiBold,
                color = colors.textPrimary
            )
            Row(
                horizontalArrangement = Arrangement.spacedBy(3.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text("少", fontSize = 10.sp, color = colors.textMuted)
                (0..4).forEach { lvl ->
                    Box(
                        modifier = Modifier
                            .size(9.dp)
                            .clip(RoundedCornerShape(2.dp))
                            .background(heatLevelColor(lvl, colors))
                    )
                }
                Text("多", fontSize = 10.sp, color = colors.textMuted)
            }
        }

        Row(
            modifier = Modifier
                .fillMaxWidth()
                .horizontalScroll(scrollState),
            horizontalArrangement = Arrangement.spacedBy(3.dp)
        ) {
            weeks.forEach { week ->
                Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
                    for (dayIdx in 0 until 7) {
                        val rec = week.getOrNull(dayIdx)
                        if (rec == null) {
                            Spacer(modifier = Modifier.size(12.dp))
                        } else {
                            val ratio = rec.total.toDouble() / maxTotal.toDouble()
                            val level = when {
                                rec.total <= 0L -> 0
                                ratio < 0.2 -> 1
                                ratio < 0.45 -> 2
                                ratio < 0.75 -> 3
                                else -> 4
                            }
                            val isSelected = rec.date == selectedDate
                            Box(
                                modifier = Modifier
                                    .size(12.dp)
                                    .clip(RoundedCornerShape(2.5.dp))
                                    .background(heatLevelColor(level, colors))
                                    .then(
                                        if (isSelected) {
                                            Modifier.border(
                                                1.5.dp,
                                                colors.accentBlue,
                                                RoundedCornerShape(2.5.dp)
                                            )
                                        } else Modifier
                                    )
                                    .clickable { onSelectDate(rec.date) }
                            )
                        }
                    }
                }
            }
        }
    }
}

private fun heatLevelColor(
    level: Int,
    colors: AppColors
): Color {
    return when (level) {
        0 -> colors.textMuted.copy(alpha = 0.12f)
        1 -> Color(0xFF9BE9A8).copy(alpha = 0.65f)
        2 -> Color(0xFF40C463).copy(alpha = 0.85f)
        3 -> Color(0xFF30A14E)
        else -> Color(0xFF216E39)
    }
}

@Composable
private fun TokenInspectorCard(record: DailyTokenRecord) {
    val colors = AntigravityTheme.colors
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
                text = "${record.date} 用量明细",
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
            val topModels = remember(record.models) {
                record.models.entries.sortedByDescending { it.value }.take(4)
            }
            HorizontalDivider(thickness = 0.5.dp, color = colors.border)
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                topModels.forEach { (model, tokens) ->
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Text(
                            text = model,
                            fontSize = 11.sp,
                            color = colors.textSecondary,
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
