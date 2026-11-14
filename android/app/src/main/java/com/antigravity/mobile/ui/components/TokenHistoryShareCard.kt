package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.BarChart
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
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

val TOKEN_HISTORY_SHARE_CARD_WIDTH = 390.dp

/**
 * Token 历史长图海报组件（专为图形图层录制与高清长图导出设计）
 */
@Composable
fun TokenHistoryShareCard(
    tokenHistory: TokenHistoryResponse,
    selectedRecord: DailyTokenRecord?,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val summary = tokenHistory.summary
    val matrix = tokenHistory.dailyMatrix

    Column(
        modifier = modifier
            .width(TOKEN_HISTORY_SHARE_CARD_WIDTH)
            .background(colors.background)
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        // 1. Header
        HeaderSection(colors = colors)

        // 2. KPI 4-grid
        KpiGridSection(summary = summary, colors = colors)

        // 3. Source ratio bar
        SourceRatioSection(summary = summary, colors = colors)

        // 4. Heatmap (近 18 周活跃区)
        HeatmapCardSection(
            dailyMatrix = matrix,
            selectedDate = selectedRecord?.date,
            colors = colors
        )

        // 5. Inspector card (选定日期)
        if (selectedRecord != null) {
            InspectorCardSection(record = selectedRecord, colors = colors)
        }

        // 6. Footer
        FooterSection(tokenHistory = tokenHistory, colors = colors)
    }
}

@Composable
private fun HeaderSection(colors: AppColors) {
    val dateStr = remember {
        val sdf = SimpleDateFormat("yyyy-MM-dd HH:mm", Locale.getDefault())
        sdf.format(Date())
    }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 4.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Row(
            horizontalArrangement = Arrangement.spacedBy(10.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            androidx.compose.foundation.Image(
                painter = androidx.compose.ui.res.painterResource(com.antigravity.mobile.R.drawable.share_app_logo),
                contentDescription = null,
                modifier = Modifier
                    .size(36.dp)
                    .clip(RoundedCornerShape(8.dp))
                    .border(0.6.dp, colors.border, RoundedCornerShape(8.dp))
            )

            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(
                    text = "Multigravity",
                    fontSize = 16.sp,
                    fontWeight = FontWeight.Bold,
                    color = colors.textPrimary
                )
                Text(
                    text = "Token 消耗分析报告",
                    fontSize = 11.sp,
                    fontWeight = FontWeight.Medium,
                    color = colors.textSecondary
                )
            }
        }

        Row(
            modifier = Modifier
                .clip(CircleShape)
                .background(colors.surface)
                .border(0.8.dp, colors.border, CircleShape)
                .padding(horizontal = 8.dp, vertical = 4.dp),
            horizontalArrangement = Arrangement.spacedBy(4.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Box(
                modifier = Modifier
                    .size(5.dp)
                    .clip(CircleShape)
                    .background(Color(0xFF22C55E))
            )
            Text(
                text = dateStr,
                fontSize = 10.sp,
                fontFamily = FontFamily.Monospace,
                fontWeight = FontWeight.Medium,
                color = colors.textSecondary
            )
        }
    }
}

@Composable
private fun KpiGridSection(
    summary: com.antigravity.mobile.data.model.TokenHistorySummary,
    colors: AppColors
) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            KpiCell(
                title = "今日消耗",
                value = formatTokensCompact(summary.todayTokens),
                sub = "${summary.activeDays} 活跃日",
                colors = colors,
                modifier = Modifier.weight(1f)
            )
            KpiCell(
                title = "近 7 日",
                value = formatTokensCompact(summary.last7dTokens),
                sub = "${summary.totalSteps} 轮采样",
                colors = colors,
                modifier = Modifier.weight(1f)
            )
        }
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            KpiCell(
                title = "累计总 Token",
                value = formatTokensCompact(summary.totalTokens),
                sub = "IDE ${formatTokensCompact(summary.desktopTokens)} · CLI ${formatTokensCompact(summary.cliTokens)}",
                colors = colors,
                modifier = Modifier.weight(1f)
            )
            KpiCell(
                title = "API 折算",
                value = String.format(Locale.US, "$%.2f", summary.estimatedCostUsd),
                sub = String.format(Locale.US, "≈ ¥%.1f", summary.estimatedCostCny),
                colors = colors,
                modifier = Modifier.weight(1f)
            )
        }
    }
}

@Composable
private fun KpiCell(
    title: String,
    value: String,
    sub: String,
    colors: AppColors,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface)
            .border(0.8.dp, colors.border, RoundedCornerShape(12.dp))
            .padding(horizontal = 12.dp, vertical = 10.dp),
        verticalArrangement = Arrangement.spacedBy(3.dp)
    ) {
        Text(
            text = title,
            fontSize = 11.sp,
            color = colors.textSecondary,
            maxLines = 1
        )
        Text(
            text = value,
            fontSize = 17.sp,
            fontFamily = FontFamily.Monospace,
            fontWeight = FontWeight.Bold,
            color = colors.textPrimary,
            maxLines = 1
        )
        Text(
            text = sub,
            fontSize = 10.5.sp,
            color = colors.textMuted,
            maxLines = 1
        )
    }
}

@Composable
private fun SourceRatioSection(
    summary: com.antigravity.mobile.data.model.TokenHistorySummary,
    colors: AppColors
) {
    val totalSources = (summary.desktopTokens + summary.cliTokens).coerceAtLeast(1L)
    val desktopPct = ((summary.desktopTokens * 100.0) / totalSources).toInt()
    val cliPct = if (summary.cliTokens > 0) 100 - desktopPct else 0

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(colors.surface)
            .border(0.8.dp, colors.border, RoundedCornerShape(10.dp))
            .padding(horizontal = 12.dp, vertical = 9.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "桌面端 Antigravity $desktopPct% · CLI $cliPct%",
                fontSize = 11.5.sp,
                fontWeight = FontWeight.Medium,
                color = colors.textSecondary
            )
            Text(
                text = "近 18 周活跃热力",
                fontSize = 10.5.sp,
                color = colors.textMuted
            )
        }

        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(6.dp)
                .clip(RoundedCornerShape(3.dp))
                .background(Color(0xFF8B5CF6).copy(alpha = 0.5f))
        ) {
            Box(
                modifier = Modifier
                    .fillMaxHeight()
                    .fillMaxWidth(fraction = (desktopPct / 100f).coerceIn(0f, 1f))
                    .clip(RoundedCornerShape(3.dp))
                    .background(colors.accentBlue)
            )
        }
    }
}

@Composable
private fun HeatmapCardSection(
    dailyMatrix: List<DailyTokenRecord>,
    selectedDate: String?,
    colors: AppColors
) {
    val maxTotal = remember(dailyMatrix) {
        dailyMatrix.maxOfOrNull { it.total }?.coerceAtLeast(1L) ?: 1L
    }
    val allWeeks = remember(dailyMatrix) {
        buildWeeks(dailyMatrix)
    }
    val displayWeeks = remember(allWeeks) {
        allWeeks.takeLast(18)
    }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface)
            .border(0.8.dp, colors.border, RoundedCornerShape(12.dp))
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
                            .size(8.dp)
                            .clip(RoundedCornerShape(2.dp))
                            .background(heatColor(lvl, colors))
                    )
                }
                Text("多", fontSize = 10.sp, color = colors.textMuted)
            }
        }

        // 平铺 18 周（左对齐，与 App 内部显示保持一致）
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(vertical = 2.dp),
            horizontalArrangement = Arrangement.Start
        ) {
            Row(horizontalArrangement = Arrangement.spacedBy(3.dp)) {
                displayWeeks.forEach { week ->
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
                                        .background(heatColor(level, colors))
                                        .then(
                                            if (isSelected) {
                                                Modifier.border(
                                                    1.5.dp,
                                                    colors.accentBlue,
                                                    RoundedCornerShape(2.5.dp)
                                                )
                                            } else Modifier
                                        )
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun InspectorCardSection(
    record: DailyTokenRecord,
    colors: AppColors
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface)
            .border(0.8.dp, colors.border, RoundedCornerShape(12.dp))
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

        HorizontalDivider(thickness = 0.8.dp, color = colors.border)

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            InspectorCell("Input", formatTokensCompact(record.input), colors)
            InspectorCell("Cache Hit", formatTokensCompact(record.cacheRead), colors)
            InspectorCell("Output", formatTokensCompact(record.output), colors)
            InspectorCell("Thinking", formatTokensCompact(record.thinking), colors)
        }

        if (record.models.isNotEmpty()) {
            val topModels = remember(record.models) {
                record.models.entries.sortedByDescending { it.value }.take(4)
            }
            HorizontalDivider(thickness = 0.8.dp, color = colors.border)
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
private fun InspectorCell(label: String, value: String, colors: AppColors) {
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

@Composable
private fun FooterSection(
    tokenHistory: TokenHistoryResponse,
    colors: AppColors
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 4.dp, vertical = 2.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        HorizontalDivider(thickness = 0.8.dp, color = colors.border)
        Text(
            text = "由 Multigravity 自动统计生成 · 跨端协同 AI 控制台",
            fontSize = 10.sp,
            fontWeight = FontWeight.Medium,
            color = colors.textSecondary
        )
        if (tokenHistory.updatedAt > 0) {
            val timeFmt = remember { SimpleDateFormat("HH:mm:ss", Locale.getDefault()) }
            val timeStr = timeFmt.format(Date(tokenHistory.updatedAt * 1000))
            Text(
                text = "本地数据扫描于 $timeStr",
                fontSize = 9.5.sp,
                color = colors.textMuted
            )
        }
    }
}

private fun heatColor(level: Int, colors: AppColors): Color {
    return when (level) {
        0 -> colors.textMuted.copy(alpha = 0.12f)
        1 -> Color(0xFF9BE9A8).copy(alpha = 0.65f)
        2 -> Color(0xFF40C463).copy(alpha = 0.85f)
        3 -> Color(0xFF30A14E)
        else -> Color(0xFF216E39)
    }
}

private fun buildWeeks(dailyMatrix: List<DailyTokenRecord>): List<List<DailyTokenRecord?>> {
    if (dailyMatrix.isEmpty()) return emptyList()
    val sdf = SimpleDateFormat("yyyy-MM-dd", Locale.US)
    val cal = Calendar.getInstance()
    var padStart = 0
    try {
        val firstDate = sdf.parse(dailyMatrix[0].date)
        if (firstDate != null) {
            cal.time = firstDate
            padStart = (cal.get(Calendar.DAY_OF_WEEK) - Calendar.SUNDAY).coerceAtLeast(0)
        }
    } catch (_: Exception) {}

    val padded = ArrayList<DailyTokenRecord?>()
    repeat(padStart) { padded.add(null) }
    padded.addAll(dailyMatrix)

    val weeks = ArrayList<List<DailyTokenRecord?>>()
    var idx = 0
    while (idx < padded.size) {
        val end = (idx + 7).coerceAtMost(padded.size)
        weeks.add(padded.subList(idx, end))
        idx = end
    }
    return weeks
}
