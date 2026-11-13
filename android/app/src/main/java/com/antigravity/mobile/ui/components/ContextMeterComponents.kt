package com.antigravity.mobile.ui.components

import androidx.compose.animation.core.*
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AddCircleOutline
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.Compress
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ContextMeterItem
import com.antigravity.mobile.ui.theme.AntigravityTheme
import java.util.Locale

@Composable
fun ContextRingCapsule(
    meter: ContextMeterItem?,
    isRunning: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    if (meter == null) return
    val colors = AntigravityTheme.colors
    val pct = meter.percentage.coerceIn(0.0, 100.0)
    val ringColor = when {
        meter.isOverflow || pct >= 90.0 -> colors.accentRed
        pct >= 70.0 -> colors.accentOrange
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
    canCompact: Boolean,
    onCompact: () -> Unit,
    onStartNewSession: () -> Unit,
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val pct = meter.percentage.coerceIn(0.0, 100.0)
    val accent = when {
        meter.isOverflow || pct >= 90.0 -> colors.accentRed
        pct >= 70.0 -> colors.accentOrange
        else -> colors.accentGreen
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        containerColor = colors.surface,
        dragHandle = { BottomSheetDefaults.DragHandle(color = colors.textMuted.copy(alpha = 0.35f)) }
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 20.dp)
                .padding(bottom = 26.dp)
                .navigationBarsPadding(),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Text(
                        text = "实时上下文水位",
                        fontSize = 17.sp,
                        fontWeight = FontWeight.Bold,
                        color = colors.textPrimary
                    )
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
                    label = "输出速度",
                    value = if (meter.tokensPerSecond > 0) String.format(Locale.US, "%.1f t/s", meter.tokensPerSecond) else "--",
                    modifier = Modifier.weight(1f)
                )
                AuditMetricTile(
                    label = "首字延迟 (TTFT)",
                    value = if (meter.timeToFirstTokenSec > 0) String.format(Locale.US, "%.2fs", meter.timeToFirstTokenSec) else "--",
                    modifier = Modifier.weight(1f)
                )
                AuditMetricTile(
                    label = "流式耗时",
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
                    text = "最近一步 Token 构成",
                    fontSize = 12.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = colors.textSecondary
                )
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    AuditBreakdownCell("Input", formatTokensCompact(meter.inputTokens))
                    AuditBreakdownCell("Cache Hit", formatTokensCompact(meter.cacheReadTokens))
                    AuditBreakdownCell("Output", formatTokensCompact(meter.outputTokens))
                    AuditBreakdownCell("Thinking", formatTokensCompact(meter.thinkingTokens))
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
                        Icon(
                            imageVector = Icons.Default.Compress,
                            contentDescription = null,
                            modifier = Modifier.size(16.dp)
                        )
                        Spacer(modifier = Modifier.width(6.dp))
                        Text("一键压缩 /compact", fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
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
                    Text("开启新会话", fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
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
private fun AuditBreakdownCell(label: String, value: String) {
    val colors = AntigravityTheme.colors
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(text = label, fontSize = 10.sp, color = colors.textMuted)
        Text(
            text = value,
            fontSize = 13.sp,
            fontWeight = FontWeight.SemiBold,
            fontFamily = FontFamily.Monospace,
            color = colors.textPrimary
        )
    }
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
                    text = "上下文窗口已触及上限",
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Bold,
                    color = colors.accentOrange
                )
                Text(
                    text = "建议立即压缩历史对话或开启新会话继续工作",
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
                    Text("一键压缩 /compact", fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                }
            }
            if (onStartNewSessionClick != null) {
                OutlinedButton(
                    onClick = onStartNewSessionClick,
                    shape = RoundedCornerShape(10.dp),
                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 6.dp),
                    modifier = Modifier.weight(1f)
                ) {
                    Text("开启新会话", fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                }
            }
        }
    }
}
