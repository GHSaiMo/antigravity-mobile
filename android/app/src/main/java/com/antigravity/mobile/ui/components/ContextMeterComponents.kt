package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.L10n
import com.antigravity.mobile.ui.util.loc
import androidx.compose.animation.core.*
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AcUnit
import androidx.compose.material.icons.filled.AddCircleOutline
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.Compress
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ContextMeterItem
import com.antigravity.mobile.ui.theme.AntigravityTheme
import java.util.Locale

@Composable
fun ContextRingCapsule(
    meter: ContextMeterItem?,
    isRunning: Boolean,
    hasPendingInteraction: Boolean = false,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    if (meter == null) return
    val colors = AntigravityTheme.colors
    val pct = meter.percentage.coerceIn(0.0, 100.0)
    val isMissStreak = meter.isCacheMissStreak()
    val staleIdleMins = meter.staleIdleMinutes(
        isRunning = isRunning,
        hasPendingInteraction = hasPendingInteraction
    )
    val hasCacheWarning = isMissStreak || staleIdleMins != null
    val ringColor = when {
        meter.isOverflow || pct >= 90.0 -> colors.accentRed
        pct >= 70.0 || hasCacheWarning -> colors.accentOrange
        else -> colors.accentGreen
    }
    val shouldPulse = isRunning && (meter.isOverflow || pct >= 90.0)
    val pulseTransition = rememberInfiniteTransition(label = "contextPulse")
    val pulseAlpha by pulseTransition.animateFloat(
        initialValue = 1f,
        targetValue = if (shouldPulse) 0.55f else 1f,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = 700, easing = FastOutSlowInEasing),
            repeatMode = RepeatMode.Reverse
        ),
        label = "contextPulseAlpha"
    )

    Row(
        modifier = modifier
            .graphicsLayer { alpha = pulseAlpha }
            .clip(CircleShape)
            .background(ringColor.copy(alpha = 0.12f))
            .border(0.6.dp, ringColor.copy(alpha = 0.35f), CircleShape)
            .clickable(onClick = onClick)
            .padding(horizontal = 7.dp, vertical = 2.5.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(5.dp)
    ) {
        Box(
            modifier = Modifier.size(14.dp),
            contentAlignment = Alignment.Center
        ) {
            Canvas(modifier = Modifier.fillMaxSize()) {
                val strokeWidth = 2.2.dp.toPx()
                drawArc(
                    color = ringColor.copy(alpha = 0.22f),
                    startAngle = -90f,
                    sweepAngle = 360f,
                    useCenter = false,
                    style = Stroke(width = strokeWidth, cap = StrokeCap.Round)
                )
                drawArc(
                    color = ringColor,
                    startAngle = -90f,
                    sweepAngle = (pct.toFloat() / 100f) * 360f,
                    useCenter = false,
                    style = Stroke(width = strokeWidth, cap = StrokeCap.Round)
                )
            }
        }
        Text(
            text = "${pct.toInt()}%",
            fontSize = 10.sp,
            fontWeight = FontWeight.Bold,
            fontFamily = FontFamily.Monospace,
            color = ringColor
        )
        if (isMissStreak) {
            Icon(
                imageVector = Icons.Default.Warning,
                contentDescription = "Cache Miss",
                tint = colors.accentOrange,
                modifier = Modifier.size(10.dp)
            )
        } else if (staleIdleMins != null) {
            Icon(
                imageVector = Icons.Default.AcUnit,
                contentDescription = "Cold Cache",
                tint = colors.accentOrange,
                modifier = Modifier.size(10.dp)
            )
        }
        if (meter.tokensPerSecond > 0.1 && (isRunning || meter.streamingDurationSec > 0.0)) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(1.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.Bolt,
                    contentDescription = "TPS",
                    tint = colors.accentOrange,
                    modifier = Modifier.size(11.dp)
                )
                Text(
                    text = String.format(Locale.US, "%.0f t/s", meter.tokensPerSecond),
                    fontSize = 10.sp,
                    fontWeight = FontWeight.SemiBold,
                    fontFamily = FontFamily.Monospace,
                    color = colors.textSecondary
                )
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContextAuditSheet(
    meter: ContextMeterItem,
    isRunning: Boolean = false,
    hasPendingInteraction: Boolean = false,
    canCompact: Boolean,
    onCompact: () -> Unit,
    onStartNewSession: () -> Unit,
    onJumpToMessage: ((String) -> Unit)? = null,
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val pct = meter.percentage.coerceIn(0.0, 100.0)
    val isMissStreak = meter.isCacheMissStreak()
    val staleIdleMins = meter.staleIdleMinutes(
        isRunning = isRunning,
        hasPendingInteraction = hasPendingInteraction
    )
    val hasCacheWarning = isMissStreak || staleIdleMins != null
    val accent = when {
        meter.isOverflow || pct >= 90.0 -> colors.accentRed
        pct >= 70.0 || hasCacheWarning -> colors.accentOrange
        else -> colors.accentGreen
    }
    val hitRate = meter.effectiveCacheHitRate()
    val cacheHitDisplay = buildString {
        append(formatTokensCompact(meter.cacheReadTokens))
        if (hitRate != null) {
            append(" (${Math.round(hitRate)}%)")
        }
    }
    val sessionHitRate = meter.effectiveSessionCacheHitRate()
    val sessionCacheHitDisplay = buildString {
        append(formatTokensCompact(meter.sessionCacheReadTokens))
        if (sessionHitRate != null) {
            append(" (${Math.round(sessionHitRate)}%)")
        }
    }
    val sessionCostDisplay = if (meter.sessionCostUsd >= 0.01) {
        String.format(Locale.US, "≈ $%.2f", meter.sessionCostUsd)
    } else {
        null
    }
    val cacheHitColor = if (isMissStreak || (hitRate != null && meter.usedTokens >= 24_000L && hitRate < 20.0)) {
        colors.accentOrange
    } else {
        colors.textPrimary
    }

    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = colors.background,
        shape = RoundedCornerShape(topStart = 16.dp, topEnd = 16.dp),
        tonalElevation = 0.dp,
        dragHandle = {
            SheetGrabHandle(color = colors.textMuted.copy(alpha = 0.35f))
        },
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) },
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 20.dp)
                .padding(top = 6.dp, bottom = 26.dp)
                .navigationBarsPadding(),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Text(
                            text = "实时上下文".loc,
                            fontSize = 17.sp,
                            fontWeight = FontWeight.Bold,
                            color = colors.textPrimary
                        )
                        if (meter.compacted) {
                            Box(
                                modifier = Modifier
                                    .clip(CircleShape)
                                    .background(colors.accentGreen.copy(alpha = 0.14f))
                                    .padding(horizontal = 7.dp, vertical = 2.dp)
                            ) {
                                Text(
                                    text = "已压缩".loc,
                                    fontSize = 10.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.accentGreen
                                )
                            }
                        }
                    }
                    Text(
                        text = "${formatTokensCompact(meter.usedTokens)} / ${formatTokensCompact(meter.maxTokens)} Tokens",
                        fontSize = 12.sp,
                        fontFamily = FontFamily.Monospace,
                        color = colors.textSecondary
                    )
                }
                Box(
                    modifier = Modifier
                        .clip(CircleShape)
                        .background(accent.copy(alpha = 0.14f))
                        .padding(horizontal = 10.dp, vertical = 4.dp)
                ) {
                    Text(
                        text = String.format(Locale.US, "%.1f%%", pct),
                        fontSize = 14.sp,
                        fontWeight = FontWeight.Bold,
                        fontFamily = FontFamily.Monospace,
                        color = accent
                    )
                }
            }

            // Linear meter bar
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(8.dp)
                    .clip(CircleShape)
                    .background(colors.textMuted.copy(alpha = 0.16f))
            ) {
                Box(
                    modifier = Modifier
                        .fillMaxWidth((pct.toFloat() / 100f).coerceIn(0.02f, 1f))
                        .fillMaxHeight()
                        .clip(CircleShape)
                        .background(accent)
                )
            }

            // KPI grid: TPS, TTFT, Stream duration
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                AuditMetricTile(
                    label = "输出速度".loc,
                    value = if (meter.tokensPerSecond > 0) String.format(Locale.US, "%.1f t/s", meter.tokensPerSecond) else "--",
                    modifier = Modifier.weight(1f)
                )
                AuditMetricTile(
                    label = "首字延迟 (TTFT)".loc,
                    value = if (meter.timeToFirstTokenSec > 0) String.format(Locale.US, "%.2fs", meter.timeToFirstTokenSec) else "--",
                    modifier = Modifier.weight(1f)
                )
                AuditMetricTile(
                    label = "流式耗时".loc,
                    value = if (meter.streamingDurationSec > 0) String.format(Locale.US, "%.1fs", meter.streamingDurationSec) else "--",
                    modifier = Modifier.weight(1f)
                )
            }

            // Token breakdown card
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Text(
                    text = "最近一步 Token 构成".loc,
                    fontSize = 12.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = colors.textSecondary
                )
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    AuditBreakdownCell("Input", formatTokensCompact(meter.inputTokens))
                    AuditBreakdownCell("Cache Hit", cacheHitDisplay, valueColor = cacheHitColor)
                    AuditBreakdownCell("Output", formatTokensCompact(meter.outputTokens))
                    AuditBreakdownCell("Thinking", formatTokensCompact(meter.thinkingTokens))
                }
                if (meter.hasSessionTotals()) {
                    HorizontalDivider(
                        color = colors.border,
                        thickness = 0.5.dp,
                        modifier = Modifier.padding(vertical = 2.dp)
                    )
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            text = formatSessionTotalsHeader(meter.sessionSteps),
                            fontSize = 12.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = colors.textSecondary
                        )
                        if (sessionCostDisplay != null) {
                            Text(
                                text = sessionCostDisplay,
                                fontSize = 11.sp,
                                fontFamily = FontFamily.Monospace,
                                color = colors.textMuted
                            )
                        }
                    }
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        AuditBreakdownCell("Input", formatTokensCompact(meter.sessionInputTokens))
                        AuditBreakdownCell("Cache Hit", sessionCacheHitDisplay)
                        AuditBreakdownCell("Output", formatTokensCompact(meter.sessionOutputTokens))
                        AuditBreakdownCell("Thinking", formatTokensCompact(meter.sessionThinkingTokens))
                    }
                }
            }

            if (isMissStreak) {
                val streak = maxOf(meter.cacheMissStreak, 2)
                val uncached = formatTokensCompact(if (meter.cacheMissTokens > 0L) meter.cacheMissTokens else meter.inputTokens)
                AuditCacheNoticeBanner(
                    icon = Icons.Default.Warning,
                    text = formatCacheMissNotice(streak, uncached)
                )
            } else if (staleIdleMins != null) {
                AuditCacheNoticeBanner(
                    icon = Icons.Default.AcUnit,
                    text = formatStaleCacheNotice(staleIdleMins, formatTokensCompact(meter.usedTokens))
                )
            } else if (meter.isOverflow || pct >= 95.0) {
                AuditCacheNoticeBanner(
                    icon = Icons.Default.Warning,
                    text = formatCompactionFullNotice(Math.round(pct).toInt())
                )
            } else if (pct >= 75.0) {
                AuditCacheNoticeBanner(
                    icon = Icons.Default.Compress,
                    text = formatCompactionApproachingNotice(Math.round(pct).toInt())
                )
            }

            // Collapsed-by-default Context Breakdown ("What ate the context" Top-3)
            val breakdown = meter.breakdown
            if (breakdown != null && breakdown.categories.isNotEmpty()) {
                var breakdownExpanded by remember { mutableStateOf(false) }
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(12.dp))
                        .background(colors.surface)
                        .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                ) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable { breakdownExpanded = !breakdownExpanded }
                            .padding(horizontal = 14.dp, vertical = 11.dp),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            text = "上下文占用分布".loc,
                            fontSize = 12.5.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = colors.textPrimary
                        )
                        Icon(
                            imageVector = if (breakdownExpanded) Icons.Default.ExpandLess else Icons.Default.ExpandMore,
                            contentDescription = null,
                            tint = colors.textMuted,
                            modifier = Modifier.size(18.dp)
                        )
                    }
                    if (breakdownExpanded) {
                        HorizontalDivider(color = colors.border, thickness = 0.5.dp)
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(14.dp),
                            verticalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            fun categoryColor(key: String): Color = when (key) {
                                "tools" -> colors.accentOrange
                                "conversation" -> colors.accentBlue
                                "thinking" -> Color(0xFFA855F7)
                                else -> Color(0xFF0EA5E9)
                            }
                            fun categoryLabel(key: String): String = when (key) {
                                "tools" -> when (L10n.currentLang) {
                                    L10n.AppLang.EN -> "Tools & Output"
                                    L10n.AppLang.JA -> "ツール出力"
                                    L10n.AppLang.ZH -> "工具与命令"
                                }
                                "conversation" -> when (L10n.currentLang) {
                                    L10n.AppLang.EN -> "Conversation"
                                    L10n.AppLang.JA -> "会話履歴"
                                    L10n.AppLang.ZH -> "对话正文"
                                }
                                "thinking" -> when (L10n.currentLang) {
                                    L10n.AppLang.EN -> "Thinking"
                                    L10n.AppLang.JA -> "思考プロセス"
                                    L10n.AppLang.ZH -> "深度思考"
                                }
                                else -> when (L10n.currentLang) {
                                    L10n.AppLang.EN -> "System & Files"
                                    L10n.AppLang.JA -> "システム・添付"
                                    L10n.AppLang.ZH -> "系统与附件"
                                }
                            }

                            // 4-color proportional bar
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .height(8.dp)
                                    .clip(CircleShape),
                                horizontalArrangement = Arrangement.spacedBy(2.dp)
                            ) {
                                breakdown.categories.filter { it.percent > 0.2 }.forEach { cat ->
                                    Box(
                                        modifier = Modifier
                                            .weight(cat.percent.toFloat().coerceAtLeast(1.5f))
                                            .fillMaxHeight()
                                            .clip(RoundedCornerShape(3.dp))
                                            .background(categoryColor(cat.key))
                                    )
                                }
                            }

                            // Category rows
                            Column(verticalArrangement = Arrangement.spacedBy(5.dp)) {
                                breakdown.categories.forEach { cat ->
                                    Row(
                                        modifier = Modifier.fillMaxWidth(),
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                                    ) {
                                        Box(
                                            modifier = Modifier
                                                .size(7.dp)
                                                .clip(CircleShape)
                                                .background(categoryColor(cat.key))
                                        )
                                        Text(
                                            text = categoryLabel(cat.key),
                                            fontSize = 11.5.sp,
                                            color = colors.textSecondary,
                                            modifier = Modifier.weight(1f)
                                        )
                                        Text(
                                            text = formatTokensCompact(cat.tokens),
                                            fontSize = 11.sp,
                                            fontFamily = FontFamily.Monospace,
                                            fontWeight = FontWeight.SemiBold,
                                            color = colors.textPrimary
                                        )
                                        Text(
                                            text = String.format(Locale.US, "%.0f%%", cat.percent),
                                            fontSize = 10.5.sp,
                                            fontFamily = FontFamily.Monospace,
                                            color = colors.textMuted,
                                            modifier = Modifier.width(34.dp)
                                        )
                                    }
                                }
                            }

                            if (breakdown.topConsumers.isNotEmpty()) {
                                HorizontalDivider(color = colors.border, thickness = 0.5.dp)
                                Text(
                                    text = when (L10n.currentLang) {
                                        L10n.AppLang.EN -> "Top 3 Heaviest Steps"
                                        L10n.AppLang.JA -> "消費量 Top 3 ステップ"
                                        L10n.AppLang.ZH -> "占用最高 Top 3"
                                    },
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.textMuted
                                )
                                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                                    breakdown.topConsumers.take(3).forEachIndexed { idx, consumer ->
                                        val targetId = consumer.messageId?.takeIf { it.isNotBlank() }
                                        val clickable = targetId != null && onJumpToMessage != null
                                        Row(
                                            modifier = Modifier
                                                .fillMaxWidth()
                                                .clip(RoundedCornerShape(8.dp))
                                                .background(colors.surfaceVariant.copy(alpha = 0.45f))
                                                .then(
                                                    if (clickable) {
                                                        Modifier.clickable {
                                                            onDismiss()
                                                            onJumpToMessage?.invoke(targetId!!)
                                                        }
                                                    } else {
                                                        Modifier
                                                    }
                                                )
                                                .padding(horizontal = 10.dp, vertical = 7.dp),
                                            verticalAlignment = Alignment.CenterVertically,
                                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                                        ) {
                                            Text(
                                                text = "#${idx + 1}",
                                                fontSize = 10.5.sp,
                                                fontFamily = FontFamily.Monospace,
                                                fontWeight = FontWeight.Bold,
                                                color = colors.textMuted
                                            )
                                            Text(
                                                text = consumer.label.ifBlank { consumer.kind },
                                                fontSize = 11.5.sp,
                                                color = colors.textPrimary,
                                                maxLines = 1,
                                                overflow = TextOverflow.Ellipsis,
                                                modifier = Modifier.weight(1f)
                                            )
                                            Text(
                                                text = "~${formatTokensCompact(consumer.tokens)}",
                                                fontSize = 11.sp,
                                                fontFamily = FontFamily.Monospace,
                                                fontWeight = FontWeight.SemiBold,
                                                color = colors.accentOrange
                                            )
                                            if (clickable) {
                                                Icon(
                                                    imageVector = Icons.Default.ChevronRight,
                                                    contentDescription = null,
                                                    tint = colors.textMuted,
                                                    modifier = Modifier.size(14.dp)
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

            // Action buttons
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                if (canCompact) {
                    Button(
                        onClick = {
                            onDismiss()
                            onCompact()
                        },
                        colors = ButtonDefaults.buttonColors(containerColor = colors.accentOrange),
                        shape = RoundedCornerShape(12.dp),
                        modifier = Modifier.weight(1f)
                    ) {
                        CompactChevronsIcon(
                            modifier = Modifier.size(14.dp),
                            tint = Color.White
                        )
                        Spacer(modifier = Modifier.width(6.dp))
                        Text(
                            text = "一键压缩".loc,
                            fontSize = 13.sp,
                            fontWeight = FontWeight.SemiBold,
                            maxLines = 1,
                            softWrap = false,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }
                OutlinedButton(
                    onClick = {
                        onDismiss()
                        onStartNewSession()
                    },
                    shape = RoundedCornerShape(12.dp),
                    modifier = Modifier.weight(1f)
                ) {
                    Icon(
                        imageVector = Icons.Default.AddCircleOutline,
                        contentDescription = null,
                        modifier = Modifier.size(16.dp)
                    )
                    Spacer(modifier = Modifier.width(6.dp))
                    Text(
                        text = "开启新会话".loc,
                        fontSize = 13.sp,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        softWrap = false,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            }
        }
    }
}

@Composable
private fun AuditMetricTile(
    label: String,
    value: String,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    Column(
        modifier = modifier
            .clip(RoundedCornerShape(10.dp))
            .background(colors.surface)
            .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
            .padding(horizontal = 10.dp, vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(3.dp)
    ) {
        Text(text = label, fontSize = 10.sp, color = colors.textMuted, maxLines = 1)
        Text(
            text = value,
            fontSize = 14.sp,
            fontWeight = FontWeight.Bold,
            fontFamily = FontFamily.Monospace,
            color = colors.textPrimary,
            maxLines = 1
        )
    }
}

@Composable
private fun AuditBreakdownCell(
    label: String,
    value: String,
    valueColor: Color? = null
) {
    val colors = AntigravityTheme.colors
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(text = label, fontSize = 10.sp, color = colors.textMuted)
        Text(
            text = value,
            fontSize = 12.sp,
            fontWeight = FontWeight.SemiBold,
            fontFamily = FontFamily.Monospace,
            color = valueColor ?: colors.textPrimary,
            maxLines = 1,
            softWrap = false
        )
    }
}

@Composable
private fun AuditCacheNoticeBanner(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    text: String
) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(colors.accentOrange.copy(alpha = if (colors.isDark) 0.14f else 0.10f))
            .border(0.7.dp, colors.accentOrange.copy(alpha = 0.30f), RoundedCornerShape(10.dp))
            .padding(horizontal = 12.dp, vertical = 10.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.Top
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = colors.accentOrange,
            modifier = Modifier
                .padding(top = 1.dp)
                .size(14.dp)
        )
        Text(
            text = text,
            fontSize = 12.sp,
            lineHeight = 17.sp,
            color = colors.textPrimary.copy(alpha = 0.88f)
        )
    }
}

private fun formatIdleDurationText(minutes: Int): String {
    val hours = minutes / 60
    val rem = minutes % 60
    return when (L10n.currentLang) {
        L10n.AppLang.EN -> when {
            minutes < 60 -> "$minutes min"
            rem == 0 -> "${hours}h"
            else -> "${hours}h ${rem}m"
        }
        L10n.AppLang.JA -> when {
            minutes < 60 -> "$minutes 分"
            rem == 0 -> "$hours 時間"
            else -> "$hours 時間 $rem 分"
        }
        L10n.AppLang.ZH -> when {
            minutes < 60 -> "$minutes 分钟"
            rem == 0 -> "$hours 小时"
            else -> "$hours 小时 $rem 分钟"
        }
    }
}

private fun formatCacheMissNotice(streak: Int, uncached: String): String = when (L10n.currentLang) {
    L10n.AppLang.EN -> "$streak consecutive cache misses ($uncached uncached input) may drain weekly quota faster. Consider compacting or starting a new session."
    L10n.AppLang.JA -> "直近 $streak ステップ連続でキャッシュ未命中（再計算 $uncached）。週間クォータを急消費する恐れがあります。圧縮または新規チャットをおすすめします。"
    L10n.AppLang.ZH -> "近 $streak 步均未命中缓存（累计重算 $uncached），可能正在加速消耗周额度，建议压缩或开启新会话。"
}

private fun formatStaleCacheNotice(minutes: Int, usedText: String): String {
    val idleText = formatIdleDurationText(minutes)
    return when (L10n.currentLang) {
        L10n.AppLang.EN -> "Idle for $idleText — prompt cache for ~$usedText context may have expired. Continuing may recompute full input; consider compacting or starting a new session."
        L10n.AppLang.JA -> "$idleText 間アイドル状態です。約 $usedText のキャッシュが失効している可能性があります。このまま続行すると入力を再計算するため、圧縮または新規チャットをおすすめします。"
        L10n.AppLang.ZH -> "已闲置 $idleText，约 $usedText 上下文缓存可能已失效；直接续聊将重算输入额度，建议先压缩或开启新会话。"
    }
}

private fun formatSessionTotalsHeader(steps: Int): String = when (L10n.currentLang) {
    L10n.AppLang.EN -> if (steps > 0) "Session Total · $steps steps" else "Session Total"
    L10n.AppLang.JA -> if (steps > 0) "セッション累計 · $steps ステップ" else "セッション累計"
    L10n.AppLang.ZH -> if (steps > 0) "本会话累计 · $steps 步" else "本会话累计"
}

private fun formatCompactionFullNotice(pct: Int): String = when (L10n.currentLang) {
    L10n.AppLang.EN -> "Context window is nearly full ($pct%) — auto-compaction or truncation may trigger at any time. Consider compacting now or starting a new session."
    L10n.AppLang.JA -> "コンテキスト上限間近（$pct%）です。自動圧縮や切り詰めが発生する恐れがあるため、今すぐ圧縮するか新規チャットをおすすめします。"
    L10n.AppLang.ZH -> "上下文窗口已接近上限（$pct%），随时可能触发自动压缩或截断，建议先一键压缩保留关键上下文，或开启新会话。"
}

private fun formatCompactionApproachingNotice(pct: Int): String = when (L10n.currentLang) {
    L10n.AppLang.EN -> "Context is at $pct%, approaching auto-compaction. Compacting before the next long step helps avoid redundant input cost."
    L10n.AppLang.JA -> "コンテキスト使用率が $pct% に達し、自動圧縮のしきい値に近づいています。長い作業の前に圧縮すると再入力コストを抑えられます。"
    L10n.AppLang.ZH -> "上下文已用 $pct%，临近自动压缩阈值；继续长任务前建议先一键压缩，减少后续重复输入消耗。"
}

@Composable
fun RescueBubbleCard(
    errorText: String,
    onCompactClick: (() -> Unit)?,
    onStartNewSessionClick: (() -> Unit)?,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp)
            .clip(RoundedCornerShape(14.dp))
            .background(colors.accentOrange.copy(alpha = if (colors.isDark) 0.14f else 0.09f))
            .border(1.dp, colors.accentOrange.copy(alpha = 0.35f), RoundedCornerShape(14.dp))
            .padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Box(
                modifier = Modifier
                    .size(28.dp)
                    .clip(CircleShape)
                    .background(colors.accentOrange.copy(alpha = 0.18f)),
                contentAlignment = Alignment.Center
            ) {
                Icon(
                    imageVector = Icons.Default.Warning,
                    contentDescription = "Context Overflow",
                    tint = colors.accentOrange,
                    modifier = Modifier.size(16.dp)
                )
            }
            Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
                Text(
                    text = "上下文窗口已触及上限".loc,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Bold,
                    color = colors.accentOrange
                )
                Text(
                    text = "建议立即压缩历史对话或开启新会话继续工作".loc,
                    fontSize = 12.sp,
                    color = colors.textSecondary
                )
            }
        }

        if (errorText.isNotBlank()) {
            Text(
                text = errorText,
                fontSize = 11.sp,
                fontFamily = FontFamily.Monospace,
                color = colors.textMuted,
                maxLines = 3
            )
        }

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            if (onCompactClick != null) {
                Button(
                    onClick = onCompactClick,
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentOrange),
                    shape = RoundedCornerShape(10.dp),
                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 6.dp),
                    modifier = Modifier.weight(1f)
                ) {
                    CompactChevronsIcon(
                        modifier = Modifier.size(13.dp),
                        tint = Color.White
                    )
                    Spacer(modifier = Modifier.width(5.dp))
                    Text("一键压缩 /compact".loc, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                }
            }
            if (onStartNewSessionClick != null) {
                OutlinedButton(
                    onClick = onStartNewSessionClick,
                    shape = RoundedCornerShape(10.dp),
                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 6.dp),
                    modifier = Modifier.weight(1f)
                ) {
                    Text("开启新会话".loc, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                }
            }
        }
    }
}

/**
 * 客户端风格的一键压缩上下收缩双箭头图标（chevron.down + chevron.up）
 */
@Composable
fun CompactChevronsIcon(
    modifier: Modifier = Modifier,
    tint: Color = LocalContentColor.current
) {
    Canvas(modifier = modifier) {
        val w = size.width
        val h = size.height
        val strokeWidth = 1.6f.dp.toPx()

        // Top chevron pointing down (v)
        val topPath = Path().apply {
            moveTo(w * 0.16f, h * 0.22f)
            lineTo(w * 0.5f, h * 0.44f)
            lineTo(w * 0.84f, h * 0.22f)
        }
        drawPath(
            path = topPath,
            color = tint,
            style = Stroke(width = strokeWidth, cap = StrokeCap.Round, join = StrokeJoin.Round)
        )

        // Bottom chevron pointing up (^)
        val bottomPath = Path().apply {
            moveTo(w * 0.16f, h * 0.78f)
            lineTo(w * 0.5f, h * 0.56f)
            lineTo(w * 0.84f, h * 0.78f)
        }
        drawPath(
            path = bottomPath,
            color = tint,
            style = Stroke(width = strokeWidth, cap = StrokeCap.Round, join = StrokeJoin.Round)
        )
    }
}
