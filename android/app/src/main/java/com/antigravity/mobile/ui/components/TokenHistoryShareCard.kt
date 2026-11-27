package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import android.graphics.Bitmap
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.BarChart
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.MenuBook
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.graphics.drawscope.scale
import androidx.compose.ui.graphics.layer.drawLayer
import androidx.compose.ui.graphics.rememberGraphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.DailyTokenRecord
import com.antigravity.mobile.data.model.TokenHistoryResponse
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.theme.AppColors
import com.antigravity.mobile.ui.theme.DarkAppColors
import com.antigravity.mobile.ui.theme.LightAppColors
import com.antigravity.mobile.ui.theme.LocalAppColors
import com.antigravity.mobile.ui.theme.ThemePreset
import com.antigravity.mobile.ui.theme.ThemePresetCatalog
import com.antigravity.mobile.ui.util.ShareImageUtils
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.*
import kotlin.math.roundToInt

val TOKEN_HISTORY_SHARE_CARD_WIDTH = 390.dp
private const val TOKEN_HISTORY_MAX_PIXEL_HEIGHT = 8192

/**
 * Token 历史长图海报组件（专为图形图层录制与高清长图导出设计）
 */
@Composable
fun TokenHistoryShareCard(
    tokenHistory: TokenHistoryResponse,
    selectedRecord: DailyTokenRecord?,
    modifier: Modifier = Modifier,
    chartType: TokenHistoryChartType = TokenHistoryChartType.HEATMAP,
    skin: ThemePreset? = null
) {
    val baseColors = AntigravityTheme.colors
    val resolvedSkin = skin ?: if (baseColors.isDark) ThemePresetCatalog.defaultDark else ThemePresetCatalog.defaultLight
    val cardColors = remember(skin, baseColors, resolvedSkin) {
        when {
            skin == null -> baseColors
            resolvedSkin.id == "default-dark" -> DarkAppColors
            resolvedSkin.id == "default-light" -> LightAppColors
            else -> resolvedSkin.appColors.copy(surface = resolvedSkin.appColors.surface.copy(alpha = 0.94f))
        }
    }
    val summary = tokenHistory.summary
    val matrix = tokenHistory.dailyMatrix

    val cardBgModifier = if (skin != null && !resolvedSkin.isDefaultGroup) {
        Modifier.background(ShareCardTheme(resolvedSkin).background)
    } else {
        Modifier.background(cardColors.background)
    }

    CompositionLocalProvider(LocalAppColors provides cardColors) {
        Column(
            modifier = modifier
                .width(TOKEN_HISTORY_SHARE_CARD_WIDTH)
                .then(cardBgModifier)
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            // 1. Header
            HeaderSection(colors = cardColors)

            // 2. KPI 4-grid
            KpiGridSection(summary = summary, colors = cardColors)

            // 2b. Global Classic Book Equivalent Easter Egg
            val bookEq = remember(tokenHistory) {
                com.antigravity.mobile.data.model.TokenBookEquivalent.resolve(tokenHistory)
            }
            if (bookEq != null) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(10.dp))
                        .background(cardColors.surface)
                        .border(0.8.dp, cardColors.accentBlue.copy(alpha = 0.28f), RoundedCornerShape(10.dp))
                        .padding(horizontal = 12.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    Box(
                        modifier = Modifier
                            .size(28.dp)
                            .clip(RoundedCornerShape(8.dp))
                            .background(cardColors.accentBlue.copy(alpha = 0.14f)),
                        contentAlignment = Alignment.Center
                    ) {
                        androidx.compose.material3.Icon(
                            imageVector = androidx.compose.material.icons.Icons.Default.MenuBook,
                            contentDescription = null,
                            tint = cardColors.accentBlue,
                            modifier = Modifier.size(14.dp)
                        )
                    }
                    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Text(
                            text = bookEq.sentence,
                            fontSize = 12.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = cardColors.textPrimary,
                            maxLines = 2,
                            overflow = TextOverflow.Ellipsis
                        )
                        Text(
                            text = bookEq.authorOrigin,
                            fontSize = 10.sp,
                            color = cardColors.textSecondary,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }
            }

            // 3. Source ratio bar
            SourceRatioSection(summary = summary, chartType = chartType, colors = cardColors)

            // 4. Chart card (所见即所得，跟随当前选中的图表页)
            ChartCardSection(
                dailyMatrix = matrix,
                selectedDate = selectedRecord?.date,
                chartType = chartType,
                colors = cardColors
            )

            // 5. Inspector card (选定日期)
            if (selectedRecord != null) {
                InspectorCardSection(record = selectedRecord, colors = cardColors)
            }

            // 6. Footer
            FooterSection(tokenHistory = tokenHistory, colors = cardColors)
        }
    }
}

@Composable
private fun HeaderSection(colors: AppColors) {
    val dateStr = remember(TimeZone.getDefault().id) {
        val sdf = SimpleDateFormat("yyyy-MM-dd HH:mm", Locale.US).apply {
            timeZone = TimeZone.getDefault()
        }
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
                    text = "Token 消耗分析报告".loc,
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
                title = "今日消耗".loc,
                value = formatTokensCompact(summary.todayTokens),
                sub = "${summary.activeDays} 活跃日".loc,
                colors = colors,
                modifier = Modifier.weight(1f)
            )
            KpiCell(
                title = "近 7 日".loc,
                value = formatTokensCompact(summary.last7dTokens),
                sub = "${summary.totalSteps} 轮采样".loc,
                colors = colors,
                modifier = Modifier.weight(1f)
            )
        }
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            KpiCell(
                title = "累计总 Token".loc,
                value = formatTokensCompact(summary.totalTokens),
                sub = "IDE ${formatTokensCompact(summary.desktopTokens)} · CLI ${formatTokensCompact(summary.cliTokens)}",
                colors = colors,
                modifier = Modifier.weight(1f)
            )
            KpiCell(
                title = "API 折算".loc,
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
    chartType: TokenHistoryChartType,
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
                text = "桌面端 Antigravity %1\$d%% · CLI %2\$d%%".loc(desktopPct, cliPct),
                fontSize = 11.5.sp,
                fontWeight = FontWeight.Medium,
                color = colors.textSecondary
            )
            Text(
                text = chartType.statusBarLabel(),
                fontSize = 10.5.sp,
                color = colors.textMuted
            )
        }

        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(6.dp)
                .clip(RoundedCornerShape(3.dp))
                .background((if (colors.themeId.startsWith("default")) Color(0xFF8B5CF6) else colors.accentPurple).copy(alpha = 0.55f))
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
private fun ChartCardSection(
    dailyMatrix: List<DailyTokenRecord>,
    selectedDate: String?,
    chartType: TokenHistoryChartType,
    colors: AppColors
) {
    val innerWidthDp = TOKEN_HISTORY_SHARE_CARD_WIDTH - 32.dp - 24.dp
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface)
            .border(0.8.dp, colors.border, RoundedCornerShape(12.dp))
            .padding(12.dp)
    ) {
        when (chartType) {
            TokenHistoryChartType.DAILY_BARS -> TokenDailyBarsChartContent(
                dailyMatrix = dailyMatrix,
                selectedDate = selectedDate,
                colors = colors
            )
            TokenHistoryChartType.MODEL_BREAKDOWN -> TokenModelBreakdownChartContent(
                dailyMatrix = dailyMatrix,
                colors = colors
            )
            TokenHistoryChartType.HEATMAP -> TokenHeatmapGridChartContent(
                dailyMatrix = dailyMatrix,
                selectedDate = selectedDate,
                colors = colors,
                fixedWidthDp = innerWidthDp
            )
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
            val allDayModels = remember(record.models) {
                record.models.entries.sortedWith(
                    compareByDescending<Map.Entry<String, Long>> { it.value }.thenBy { it.key }
                )
            }
            HorizontalDivider(thickness = 0.8.dp, color = colors.border)
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                allDayModels.forEach { (model, tokens) ->
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        verticalAlignment = Alignment.CenterVertically
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
            text = "由 Multigravity 自动统计生成 · 跨端协同 AI 控制台".loc,
            fontSize = 10.sp,
            fontWeight = FontWeight.Medium,
            color = colors.textSecondary
        )
        if (tokenHistory.updatedAt > 0) {
            val timeFmt = remember(TimeZone.getDefault().id) {
                SimpleDateFormat("HH:mm:ss", Locale.US).apply {
                    timeZone = TimeZone.getDefault()
                }
            }
            val timeStr = timeFmt.format(Date(tokenHistory.updatedAtEpochMs))
            Text(
                text = "本地数据扫描于 %s".loc(timeStr),
                fontSize = 9.5.sp,
                color = colors.textMuted
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TokenHistoryShareCardSheet(
    tokenHistory: TokenHistoryResponse,
    selectedRecord: DailyTokenRecord?,
    chartType: TokenHistoryChartType = TokenHistoryChartType.HEATMAP,
    onDismiss: () -> Unit
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val prefs = remember(context) { PreferencesManager(context) }
    val isSystemDark = isSystemInDarkTheme()
    var selectedSkin by remember { mutableStateOf(prefs.resolvedShareCardSkin(isSystemDark)) }
    var showSkinPicker by remember { mutableStateOf(false) }
    var busy by remember { mutableStateOf(false) }
    val layer = rememberGraphicsLayer()
    val scaledLayer = rememberGraphicsLayer()
    val density = LocalDensity.current

    suspend fun capture(): Bitmap? {
        return try {
            val size = layer.size
            if (size.width <= 0 || size.height <= 0) return null
            if (size.height > TOKEN_HISTORY_MAX_PIXEL_HEIGHT) {
                val ratio = TOKEN_HISTORY_MAX_PIXEL_HEIGHT.toFloat() / size.height
                val target = androidx.compose.ui.unit.IntSize(
                    (size.width * ratio).roundToInt().coerceAtLeast(1),
                    TOKEN_HISTORY_MAX_PIXEL_HEIGHT
                )
                scaledLayer.record(density, androidx.compose.ui.unit.LayoutDirection.Ltr, target) {
                    scale(ratio, pivot = Offset.Zero) {
                        drawLayer(layer)
                    }
                }
                scaledLayer.toImageBitmap().asAndroidBitmap()
            } else {
                layer.toImageBitmap().asAndroidBitmap()
            }
        } catch (t: Throwable) {
            android.util.Log.e("TokenHistoryShareCard", "capture bitmap failed", t)
            null
        }
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        sheetMaxWidth = androidx.compose.ui.unit.Dp.Unspecified,
        containerColor = AntigravityTheme.colors.surface,
        shape = RoundedCornerShape(topStart = 20.dp, topEnd = 20.dp)
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .fillMaxHeight(0.92f)
                .navigationBarsPadding()
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 8.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                IconButton(
                    onClick = {
                        scope.launch {
                            busy = true
                            ShareImageUtils.saveBitmapToGallery(context, capture())
                            busy = false
                        }
                    },
                    enabled = !busy
                ) {
                    Icon(Icons.Default.FileDownload, contentDescription = "保存到相册".loc)
                }
                Text(
                    text = "分享 Token 报告".loc,
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f),
                    textAlign = androidx.compose.ui.text.style.TextAlign.Center
                )
                IconButton(
                    onClick = {
                        scope.launch {
                            busy = true
                            ShareImageUtils.shareBitmap(context, capture())
                            busy = false
                        }
                    },
                    enabled = !busy
                ) {
                    Icon(Icons.Default.Share, contentDescription = "分享".loc)
                }
            }

            Box(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 4.dp)
            ) {
                ScaledToFitWidth(designWidth = TOKEN_HISTORY_SHARE_CARD_WIDTH) {
                    Box(
                        modifier = Modifier.drawWithContent {
                            layer.record { this@drawWithContent.drawContent() }
                            drawLayer(layer)
                        }
                    ) {
                        TokenHistoryShareCard(
                            tokenHistory = tokenHistory,
                            selectedRecord = selectedRecord,
                            chartType = chartType,
                            skin = selectedSkin
                        )
                    }
                }
            }

            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                ShareSkinPickerPill(
                    preset = selectedSkin,
                    onClick = { showSkinPicker = true }
                )
                Spacer(Modifier.weight(1f))
            }
        }
    }

    if (showSkinPicker) {
        ShareCardSkinPickerSheet(
            selectedPreset = selectedSkin,
            onSelect = { preset ->
                prefs.shareCardSkinId = preset.id
                selectedSkin = preset
                showSkinPicker = false
            },
            onDismiss = { showSkinPicker = false }
        )
    }
}
