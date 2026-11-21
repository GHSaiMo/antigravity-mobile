package com.antigravity.mobile.ui.components

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.DailyTokenRecord
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.theme.AppColors
import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.*
import kotlin.math.floor

/**
 * Token 历史页三张可滑动切换的图表类型（顺序固定：每日趋势柱状图 → 模型消耗占比 → 每日活跃热力图）
 */
enum class TokenHistoryChartType(val storageKey: String, val pageIndex: Int) {
    DAILY_BARS("daily_bars", 0),
    MODEL_BREAKDOWN("model_breakdown", 1),
    HEATMAP("heatmap", 2);

    fun statusBarLabel(): String = when (this) {
        DAILY_BARS -> "近 14 日趋势".loc
        MODEL_BREAKDOWN -> "累计模型占比".loc
        HEATMAP -> "近 20 周热力图".loc
    }

    companion object {
        fun fromStorageKey(raw: String?): TokenHistoryChartType? {
            if (raw.isNullOrBlank()) return null
            return entries.firstOrNull { it.storageKey == raw }
        }

        fun fromPageIndex(index: Int): TokenHistoryChartType {
            return entries.firstOrNull { it.pageIndex == index } ?: DAILY_BARS
        }

        /**
         * 首次打开智能默认：用户手动切换过则沿用记忆；否则活跃 < 28 天默认柱状图，≥ 28 天默认热力图。
         */
        fun resolveDefault(savedRaw: String?, activeDays: Int): TokenHistoryChartType {
            fromStorageKey(savedRaw)?.let { return it }
            return if (activeDays < 28) DAILY_BARS else HEATMAP
        }
    }
}

// MARK: - Data Builders

data class DailyBarSlot(
    val date: String,
    val shortLabel: String,
    val total: Long,
    val record: DailyTokenRecord?
)

data class DailyBarsSeries(
    val slots: List<DailyBarSlot>,
    val maxTotal: Long,
    val activeAverage: Long
)

data class ModelBreakdownSlice(
    val id: String,
    val name: String,
    val tokens: Long,
    val percent: Double,
    val colorIndex: Int,
    val isOther: Boolean
)

data class ModelBreakdownSeries(
    val slices: List<ModelBreakdownSlice>,
    val uniqueModelCount: Int,
    val grandTotal: Long
)

data class HeatmapCell(
    val date: String,
    val record: DailyTokenRecord?,
    val level: Int,
    val isFuture: Boolean
)

object TokenHistoryChartData {
    private fun dayFormatter(): SimpleDateFormat =
        SimpleDateFormat("yyyy-MM-dd", Locale.US).apply {
            isLenient = false
        }

    private fun shortDayFormatter(): SimpleDateFormat =
        SimpleDateFormat("MM/dd", Locale.US)

    private fun startOfDay(cal: Calendar) {
        cal.set(Calendar.HOUR_OF_DAY, 0)
        cal.set(Calendar.MINUTE, 0)
        cal.set(Calendar.SECOND, 0)
        cal.set(Calendar.MILLISECOND, 0)
    }

    private fun resolveAnchorCalendar(matrix: List<DailyTokenRecord>): Calendar {
        val today = Calendar.getInstance().also { startOfDay(it) }
        val lastKey = matrix.lastOrNull()?.date ?: return today
        val parsed = runCatching { dayFormatter().parse(lastKey) }.getOrNull() ?: return today
        val lastCal = Calendar.getInstance().apply {
            time = parsed
            startOfDay(this)
        }
        return if (lastCal.timeInMillis > today.timeInMillis) lastCal else today
    }

    fun buildDailyBars(matrix: List<DailyTokenRecord>, days: Int = 14): DailyBarsSeries {
        val anchor = resolveAnchorCalendar(matrix)
        val byDate = matrix.associateBy { it.date }
        val dayFmt = dayFormatter()
        val shortFmt = shortDayFormatter()

        val slots = ArrayList<DailyBarSlot>(days)
        var maxTotal = 0L
        var activeSum = 0L
        var activeCount = 0L

        for (offset in -(days - 1)..0) {
            val cal = (anchor.clone() as Calendar).apply { add(Calendar.DAY_OF_YEAR, offset) }
            val key = dayFmt.format(cal.time)
            val shortLabel = shortFmt.format(cal.time)
            val rec = byDate[key]
            val total = rec?.total ?: 0L
            if (total > maxTotal) maxTotal = total
            if (total > 0L) {
                activeSum += total
                activeCount += 1L
            }
            slots.add(DailyBarSlot(date = key, shortLabel = shortLabel, total = total, record = rec))
        }

        val avg = if (activeCount > 0L) activeSum / activeCount else 0L
        return DailyBarsSeries(slots = slots, maxTotal = maxTotal, activeAverage = avg)
    }

    fun buildModelBreakdown(matrix: List<DailyTokenRecord>): ModelBreakdownSeries {
        val totals = HashMap<String, Long>()
        for (rec in matrix) {
            for ((model, count) in rec.models) {
                if (count > 0L) {
                    totals[model] = (totals[model] ?: 0L) + count
                }
            }
        }
        val sorted = totals.entries.sortedWith(
            compareByDescending<Map.Entry<String, Long>> { it.value }.thenBy { it.key }
        )
        val grandTotal = sorted.sumOf { it.value }
        if (grandTotal <= 0L) {
            return ModelBreakdownSeries(emptyList(), 0, 0L)
        }

        val slices = ArrayList<ModelBreakdownSlice>(4)
        if (sorted.size <= 4) {
            sorted.forEachIndexed { idx, entry ->
                slices.add(
                    ModelBreakdownSlice(
                        id = entry.key,
                        name = entry.key,
                        tokens = entry.value,
                        percent = entry.value * 100.0 / grandTotal.toDouble(),
                        colorIndex = idx,
                        isOther = false
                    )
                )
            }
        } else {
            for (idx in 0 until 3) {
                val entry = sorted[idx]
                slices.add(
                    ModelBreakdownSlice(
                        id = entry.key,
                        name = entry.key,
                        tokens = entry.value,
                        percent = entry.value * 100.0 / grandTotal.toDouble(),
                        colorIndex = idx,
                        isOther = false
                    )
                )
            }
            val otherTokens = sorted.drop(3).sumOf { it.value }
            slices.add(
                ModelBreakdownSlice(
                    id = "__other__",
                    name = "其他".loc,
                    tokens = otherTokens,
                    percent = otherTokens * 100.0 / grandTotal.toDouble(),
                    colorIndex = 4,
                    isOther = true
                )
            )
        }

        return ModelBreakdownSeries(
            slices = slices,
            uniqueModelCount = sorted.size,
            grandTotal = grandTotal
        )
    }

    fun buildFixedHeatmapWeeks(
        matrix: List<DailyTokenRecord>,
        weekCount: Int = 20
    ): List<List<HeatmapCell>> {
        val anchor = resolveAnchorCalendar(matrix)
        val anchorMs = anchor.timeInMillis
        val weekdayOffset = (anchor.get(Calendar.DAY_OF_WEEK) - Calendar.SUNDAY).coerceIn(0, 6)
        val startSunday = (anchor.clone() as Calendar).apply {
            add(Calendar.DAY_OF_YEAR, -weekdayOffset - (weekCount - 1) * 7)
        }

        val byDate = matrix.associateBy { it.date }
        val maxTotal = matrix.maxOfOrNull { it.total }?.coerceAtLeast(1L) ?: 1L
        val dayFmt = dayFormatter()

        val weeks = ArrayList<List<HeatmapCell>>(weekCount)
        for (w in 0 until weekCount) {
            val col = ArrayList<HeatmapCell>(7)
            for (d in 0 until 7) {
                val cal = (startSunday.clone() as Calendar).apply { add(Calendar.DAY_OF_YEAR, w * 7 + d) }
                val key = dayFmt.format(cal.time)
                if (cal.timeInMillis > anchorMs) {
                    col.add(HeatmapCell(date = key, record = null, level = 0, isFuture = true))
                } else {
                    val rec = byDate[key]
                    val total = rec?.total ?: 0L
                    val ratio = total.toDouble() / maxTotal.toDouble()
                    val level = when {
                        total <= 0L -> 0
                        ratio < 0.2 -> 1
                        ratio < 0.45 -> 2
                        ratio < 0.75 -> 3
                        else -> 4
                    }
                    col.add(HeatmapCell(date = key, record = rec, level = level, isFuture = false))
                }
            }
            weeks.add(col)
        }
        return weeks
    }
}

private val CHART_CONTENT_HEIGHT = 112.dp
private val BAR_PLOT_HEIGHT = 92.dp

internal fun tokenChartHeatColor(level: Int, colors: AppColors): Color {
    return when (level) {
        0 -> colors.textMuted.copy(alpha = if (colors.isDark) 0.16f else 0.12f)
        1 -> Color(0xFF9BE9A8).copy(alpha = 0.68f)
        2 -> Color(0xFF40C463).copy(alpha = 0.85f)
        3 -> Color(0xFF30A14E)
        else -> Color(0xFF216E39)
    }
}

private fun modelSliceColor(index: Int, isOther: Boolean, colors: AppColors): Color {
    if (isOther) return colors.textMuted.copy(alpha = 0.48f)
    return when (index) {
        0 -> colors.accentBlue
        1 -> Color(0xFF22C55E)
        2 -> Color(0xFFF59E0B)
        3 -> Color(0xFFA855F7)
        else -> colors.textMuted.copy(alpha = 0.48f)
    }
}

// MARK: - Chart 1: 14-Day Daily Bar Chart

@Composable
fun TokenDailyBarsChartContent(
    dailyMatrix: List<DailyTokenRecord>,
    selectedDate: String?,
    colors: AppColors,
    onSelectDate: ((String) -> Unit)? = null
) {
    val series = remember(dailyMatrix) { TokenHistoryChartData.buildDailyBars(dailyMatrix, 14) }
    val haptic = LocalHapticFeedback.current
    val gridLineColor = colors.textPrimary.copy(alpha = if (colors.isDark) 0.08f else 0.07f)
    val emptyBarColor = colors.textMuted.copy(alpha = if (colors.isDark) 0.16f else 0.12f)
    val activeBarBrush = remember {
        Brush.verticalGradient(listOf(Color(0xFF40C463), Color(0xFF30A14E)))
    }

    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "近 14 日趋势".loc,
                fontSize = 12.sp,
                fontWeight = FontWeight.SemiBold,
                color = colors.textPrimary
            )
            Text(
                text = if (series.maxTotal > 0L) {
                    "峰值 ${formatTokensCompact(series.maxTotal)}".loc
                } else {
                    "暂无用量".loc
                },
                fontSize = 10.5.sp,
                fontFamily = FontFamily.Monospace,
                fontWeight = FontWeight.Medium,
                color = colors.textSecondary
            )
        }

        Column(
            modifier = Modifier
                .fillMaxWidth()
                .height(CHART_CONTENT_HEIGHT),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(BAR_PLOT_HEIGHT)
            ) {
                // Subtle horizontal guide lines
                Column(
                    modifier = Modifier.fillMaxSize(),
                    verticalArrangement = Arrangement.SpaceBetween
                ) {
                    Box(Modifier.fillMaxWidth().height(0.5.dp).background(gridLineColor))
                    Box(Modifier.fillMaxWidth().height(0.5.dp).background(gridLineColor))
                    Box(Modifier.fillMaxWidth().height(0.5.dp).background(gridLineColor))
                }

                // Active-day average reference dashed line
                if (series.maxTotal > 0L && series.activeAverage > 0L && series.activeAverage < series.maxTotal) {
                    val avgRatio = (series.activeAverage.toFloat() / series.maxTotal.toFloat()).coerceIn(0.05f, 0.95f)
                    val bottomOffset = BAR_PLOT_HEIGHT * avgRatio
                    Row(
                        modifier = Modifier
                            .align(Alignment.BottomCenter)
                            .fillMaxWidth()
                            .padding(bottom = bottomOffset),
                        horizontalArrangement = Arrangement.spacedBy(3.dp)
                    ) {
                        repeat(24) {
                            Box(
                                modifier = Modifier
                                    .weight(1f)
                                    .height(1.dp)
                                    .clip(CircleShape)
                                    .background(colors.textSecondary.copy(alpha = 0.24f))
                            )
                        }
                    }
                }

                // 14 vertical bars
                Row(
                    modifier = Modifier.fillMaxSize(),
                    horizontalArrangement = Arrangement.spacedBy(5.dp),
                    verticalAlignment = Alignment.Bottom
                ) {
                    series.slots.forEach { slot ->
                        val isSelected = (slot.date == selectedDate) && (slot.total > 0L)
                        val barHeight: Dp = if (series.maxTotal > 0L && slot.total > 0L) {
                            val ratio = (slot.total.toFloat() / series.maxTotal.toFloat()).coerceIn(0f, 1f)
                            (BAR_PLOT_HEIGHT * ratio).coerceAtLeast(6.dp)
                        } else {
                            3.dp
                        }
                        val barShape = RoundedCornerShape(
                            topStart = 3.5.dp,
                            topEnd = 3.5.dp,
                            bottomStart = 1.5.dp,
                            bottomEnd = 1.5.dp
                        )

                        Box(
                            modifier = Modifier
                                .weight(1f)
                                .fillMaxHeight()
                                .then(
                                    if (slot.record != null && slot.total > 0L && onSelectDate != null) {
                                        Modifier.clickable(
                                            interactionSource = remember { MutableInteractionSource() },
                                            indication = null
                                        ) {
                                            haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                                            onSelectDate(slot.record.date)
                                        }
                                    } else {
                                        Modifier
                                    }
                                ),
                            contentAlignment = Alignment.BottomCenter
                        ) {
                            Box(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .height(barHeight)
                                    .clip(barShape)
                                    .then(
                                        when {
                                            slot.total <= 0L -> Modifier.background(emptyBarColor)
                                            isSelected -> Modifier.background(colors.accentBlue)
                                            else -> Modifier.background(activeBarBrush)
                                        }
                                    )
                            )
                        }
                    }
                }
            }

            // Crisp 3-anchor X-axis labels
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(14.dp),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = series.slots.firstOrNull()?.shortLabel ?: "",
                    fontSize = 9.5.sp,
                    fontFamily = FontFamily.Monospace,
                    color = colors.textMuted
                )
                if (series.slots.size > 7) {
                    Text(
                        text = series.slots[series.slots.size / 2].shortLabel,
                        fontSize = 9.5.sp,
                        fontFamily = FontFamily.Monospace,
                        color = colors.textMuted
                    )
                }
                Text(
                    text = series.slots.lastOrNull()?.shortLabel ?: "",
                    fontSize = 9.5.sp,
                    fontFamily = FontFamily.Monospace,
                    color = colors.textMuted
                )
            }
        }
    }
}

// MARK: - Chart 2: Model Breakdown Chart

@Composable
fun TokenModelBreakdownChartContent(
    dailyMatrix: List<DailyTokenRecord>,
    colors: AppColors
) {
    val series = remember(dailyMatrix) { TokenHistoryChartData.buildModelBreakdown(dailyMatrix) }

    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "模型消耗占比".loc,
                fontSize = 12.sp,
                fontWeight = FontWeight.SemiBold,
                color = colors.textPrimary
            )
            Text(
                text = if (series.uniqueModelCount > 0) {
                    "${series.uniqueModelCount} 款模型".loc
                } else {
                    "暂无数据".loc
                },
                fontSize = 10.5.sp,
                fontFamily = FontFamily.Monospace,
                fontWeight = FontWeight.Medium,
                color = colors.textSecondary
            )
        }

        if (series.slices.isEmpty()) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(CHART_CONTENT_HEIGHT),
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = "暂无模型明细数据".loc,
                    fontSize = 12.sp,
                    color = colors.textMuted
                )
            }
        } else {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(CHART_CONTENT_HEIGHT),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                // Top segmented proportional bar
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(10.dp)
                        .clip(CircleShape),
                    horizontalArrangement = Arrangement.spacedBy(2.dp)
                ) {
                    series.slices.forEach { slice ->
                        val weight = slice.percent.toFloat().coerceAtLeast(1.5f)
                        Box(
                            modifier = Modifier
                                .weight(weight)
                                .fillMaxHeight()
                                .clip(RoundedCornerShape(3.dp))
                                .background(modelSliceColor(slice.colorIndex, slice.isOther, colors))
                        )
                    }
                }

                // Top 4 model rows
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    series.slices.forEach { slice ->
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Box(
                                modifier = Modifier
                                    .size(7.dp)
                                    .clip(CircleShape)
                                    .background(modelSliceColor(slice.colorIndex, slice.isOther, colors))
                            )
                            Text(
                                text = slice.name,
                                fontSize = 11.5.sp,
                                fontWeight = FontWeight.Medium,
                                color = colors.textPrimary,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                modifier = Modifier.weight(1f)
                            )
                            Text(
                                text = formatTokensCompact(slice.tokens),
                                fontSize = 11.sp,
                                fontFamily = FontFamily.Monospace,
                                fontWeight = FontWeight.SemiBold,
                                color = colors.textPrimary
                            )
                            Text(
                                text = String.format(Locale.US, "%.1f%%", slice.percent),
                                fontSize = 10.5.sp,
                                fontFamily = FontFamily.Monospace,
                                color = colors.textSecondary,
                                textAlign = TextAlign.End,
                                modifier = Modifier.width(42.dp)
                            )
                        }
                    }
                }
            }
        }
    }
}

// MARK: - Chart 3: Fixed 20-Week Calendar Heatmap

@Composable
fun TokenHeatmapGridChartContent(
    dailyMatrix: List<DailyTokenRecord>,
    selectedDate: String?,
    colors: AppColors,
    fixedWidthDp: Dp? = null,
    onSelectDate: ((String) -> Unit)? = null
) {
    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "每日活跃密度".loc,
                fontSize = 12.sp,
                fontWeight = FontWeight.SemiBold,
                color = colors.textPrimary
            )
            Row(
                horizontalArrangement = Arrangement.spacedBy(3.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text("少".loc, fontSize = 10.sp, color = colors.textMuted)
                (0..4).forEach { lvl ->
                    Box(
                        modifier = Modifier
                            .size(8.5.dp)
                            .clip(RoundedCornerShape(2.dp))
                            .background(tokenChartHeatColor(lvl, colors))
                    )
                }
                Text("多".loc, fontSize = 10.sp, color = colors.textMuted)
            }
        }

        if (fixedWidthDp != null) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(CHART_CONTENT_HEIGHT),
                contentAlignment = Alignment.Center
            ) {
                HeatmapGridMatrix(
                    dailyMatrix = dailyMatrix,
                    selectedDate = selectedDate,
                    availableWidthDp = fixedWidthDp,
                    colors = colors,
                    onSelectDate = onSelectDate
                )
            }
        } else {
            BoxWithConstraints(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(CHART_CONTENT_HEIGHT),
                contentAlignment = Alignment.Center
            ) {
                HeatmapGridMatrix(
                    dailyMatrix = dailyMatrix,
                    selectedDate = selectedDate,
                    availableWidthDp = maxWidth,
                    colors = colors,
                    onSelectDate = onSelectDate
                )
            }
        }
    }
}

@Composable
private fun HeatmapGridMatrix(
    dailyMatrix: List<DailyTokenRecord>,
    selectedDate: String?,
    availableWidthDp: Dp,
    colors: AppColors,
    onSelectDate: ((String) -> Unit)?
) {
    val haptic = LocalHapticFeedback.current
    val spacingDp = 3f
    val weekCount = if (availableWidthDp.value < 290f) 18 else 20
    val rawCell = floor((availableWidthDp.value - spacingDp * (weekCount - 1)) / weekCount.toFloat())
    val cellSize = rawCell.coerceIn(9f, 13.5f).dp

    val weeks = remember(dailyMatrix, weekCount) {
        TokenHistoryChartData.buildFixedHeatmapWeeks(dailyMatrix, weekCount)
    }

    Row(horizontalArrangement = Arrangement.spacedBy(spacingDp.dp)) {
        weeks.forEach { week ->
            Column(verticalArrangement = Arrangement.spacedBy(spacingDp.dp)) {
                week.forEach { cell ->
                    if (cell.isFuture) {
                        Spacer(modifier = Modifier.size(cellSize))
                    } else {
                        val isSelected = cell.date == selectedDate
                        Box(
                            modifier = Modifier
                                .size(cellSize)
                                .clip(RoundedCornerShape(2.5.dp))
                                .background(tokenChartHeatColor(cell.level, colors))
                                .then(
                                    if (isSelected) {
                                        Modifier.border(
                                            1.5.dp,
                                            colors.accentBlue,
                                            RoundedCornerShape(2.5.dp)
                                        )
                                    } else {
                                        Modifier
                                    }
                                )
                                .then(
                                    if (cell.record != null && onSelectDate != null) {
                                        Modifier.clickable {
                                            haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                                            onSelectDate(cell.record.date)
                                        }
                                    } else {
                                        Modifier
                                    }
                                )
                        )
                    }
                }
            }
        }
    }
}

// MARK: - Interactive Swipeable Carousel Card

@OptIn(ExperimentalFoundationApi::class)
@Composable
fun TokenHistoryChartCarouselCard(
    dailyMatrix: List<DailyTokenRecord>,
    selectedDate: String?,
    selectedChart: TokenHistoryChartType,
    onSelectDate: (String) -> Unit,
    onUserSelectChart: (TokenHistoryChartType) -> Unit
) {
    val colors = AntigravityTheme.colors
    val scope = rememberCoroutineScope()
    val haptic = LocalHapticFeedback.current
    val pagerState = rememberPagerState(
        initialPage = selectedChart.pageIndex,
        pageCount = { TokenHistoryChartType.entries.size }
    )

    var animatingToPage by remember { mutableStateOf<Int?>(null) }

    // External state change (e.g., async load resolving default chart) -> align pager
    LaunchedEffect(selectedChart) {
        if (animatingToPage == selectedChart.pageIndex) return@LaunchedEffect
        if (pagerState.currentPage != selectedChart.pageIndex && !pagerState.isScrollInProgress) {
            pagerState.scrollToPage(selectedChart.pageIndex)
        }
    }

    // User swipe settled -> update & persist preference
    LaunchedEffect(pagerState.settledPage) {
        if (animatingToPage == pagerState.settledPage) {
            animatingToPage = null
        }
        val settledChart = TokenHistoryChartType.fromPageIndex(pagerState.settledPage)
        if (settledChart != selectedChart) {
            onUserSelectChart(settledChart)
        }
    }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface)
            .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            .padding(vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        HorizontalPager(
            state = pagerState,
            modifier = Modifier.fillMaxWidth()
        ) { page ->
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp)
            ) {
                when (TokenHistoryChartType.fromPageIndex(page)) {
                    TokenHistoryChartType.DAILY_BARS -> TokenDailyBarsChartContent(
                        dailyMatrix = dailyMatrix,
                        selectedDate = selectedDate,
                        colors = colors,
                        onSelectDate = onSelectDate
                    )
                    TokenHistoryChartType.MODEL_BREAKDOWN -> TokenModelBreakdownChartContent(
                        dailyMatrix = dailyMatrix,
                        colors = colors
                    )
                    TokenHistoryChartType.HEATMAP -> TokenHeatmapGridChartContent(
                        dailyMatrix = dailyMatrix,
                        selectedDate = selectedDate,
                        colors = colors,
                        fixedWidthDp = null,
                        onSelectDate = onSelectDate
                    )
                }
            }
        }

        // Minimal 3-dot page indicator (tappable)
        Row(
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            TokenHistoryChartType.entries.forEach { chart ->
                val isActive = pagerState.currentPage == chart.pageIndex
                val dotWidth by animateDpAsState(
                    targetValue = if (isActive) 16.dp else 5.dp,
                    label = "chartDotWidth"
                )
                val dotColor by animateColorAsState(
                    targetValue = if (isActive) {
                        colors.textPrimary.copy(alpha = 0.65f)
                    } else {
                        colors.textMuted.copy(alpha = 0.30f)
                    },
                    label = "chartDotColor"
                )
                Box(
                    modifier = Modifier
                        .clickable(
                            interactionSource = remember { MutableInteractionSource() },
                            indication = null
                        ) {
                            if (pagerState.currentPage != chart.pageIndex) {
                                haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                                animatingToPage = chart.pageIndex
                                onUserSelectChart(chart)
                                scope.launch {
                                    try {
                                        pagerState.animateScrollToPage(chart.pageIndex)
                                    } finally {
                                        if (animatingToPage == chart.pageIndex) {
                                            animatingToPage = null
                                        }
                                    }
                                }
                            }
                        }
                        .padding(vertical = 4.dp)
                ) {
                    Box(
                        modifier = Modifier
                            .width(dotWidth)
                            .height(5.dp)
                            .clip(CircleShape)
                            .background(dotColor)
                    )
                }
            }
        }
    }
}
